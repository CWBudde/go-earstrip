// Loads the Go wasm build of cmd/demo once and wraps its globalThis.earstrip
// in typed, synchronous methods.
//
// It fetches wasm_exec.js and runs it as a classic script, which defines
// globalThis.Go. Both files come from import.meta.env.BASE_URL (web/public,
// filled by `just build-demo`).

/** A polygon as RunFlat takes it: x, y per vertex, and where each hole starts. */
export interface Flat {
  coords: Float64Array;
  holeStarts: number[];
}

/** The settings of the Triangulator; see the README. */
export interface Options {
  forcedCuts: boolean;
  constrained: boolean;
  extraPoints: boolean;
  triangleList: boolean;
}

/** earstrip.Status as its String method writes it. */
export type Status =
  | "Exact"
  | "HolesLeftOut"
  | "Approximate"
  | "Refused"
  | "Constrained";

/** What a run gives; see internal/demo.Result. */
export interface Result {
  status: Status;
  holesLeftOut: boolean;
  strips: Uint32Array;
  stripEnds: Uint32Array;
  stripClockwise: Uint8Array;
  /** Three indices per counter-clockwise triangle, in strip order. */
  triangles: Uint32Array;
  /**
   * All strips as one index buffer for a TRIANGLE_STRIP draw call, with
   * RESTART_INDEX between strips (Triangulator.Restart). Every triangle is
   * counter-clockwise: a strip that starts clockwise has its first index twice.
   * Empty in triangle list mode.
   */
  restart: Uint32Array;
  /** Points the constrained triangulation added, x, y each: index n + k. */
  extra: Float64Array;
}

/** The primitive restart index of Result.restart, WebGL 2's for 32-bit indices. */
export const RESTART_INDEX = 0xffffffff;

export interface Timing {
  runs: number;
  nsPerRun: number;
}

/** A polygon family of the benchmark; see internal/demo.Workloads. */
export interface WorkloadInfo {
  name: string;
  about: string;
}

export interface PresetInfo {
  group: string;
  name: string;
}

/**
 * What a Go function returns instead of its result when it fails; see
 * cmd/demo/bridge_js.go.
 */
interface WasmError {
  readonly error: string;
}

type WasmResult<T> = T | WasmError;

/** globalThis.earstrip, published by cmd/demo/main_js.go. */
interface EarstripGlobal {
  readonly goVersion: string;
  presets(): WasmResult<PresetInfo[]>;
  preset(index: number): WasmResult<Flat>;
  kinds(): WasmResult<string[]>;
  generate(kind: string, seed: number): WasmResult<Flat>;
  workloads(): WasmResult<WorkloadInfo[]>;
  workload(name: string, vertices: number): WasmResult<Flat>;
  triangulate(
    coords: Float64Array,
    holeStarts: number[],
    options: Options,
  ): WasmResult<Result>;
  time(
    coords: Float64Array,
    holeStarts: number[],
    options: Options,
    budgetMs: number,
  ): WasmResult<Timing>;
}

/** Returns the result of a Go function or throws its error as an Error. */
function unwrap<T>(result: WasmResult<T>): T {
  if (
    typeof result === "object" &&
    result !== null &&
    "error" in result &&
    typeof result.error === "string"
  ) {
    throw new Error(result.error);
  }
  return result as T;
}

/** The loaded module. Every method runs synchronously in Go. */
export class Earstrip {
  readonly #api: EarstripGlobal;

  constructor(api: EarstripGlobal) {
    this.#api = api;
  }

  /** runtime.Version() of the Go toolchain that built demo.wasm. */
  get goVersion(): string {
    return this.#api.goVersion;
  }

  presets(): PresetInfo[] {
    return unwrap(this.#api.presets());
  }

  preset(index: number): Flat {
    return unwrap(this.#api.preset(index));
  }

  /** The families of generated polygons of the differential tests. */
  kinds(): string[] {
    return unwrap(this.#api.kinds());
  }

  generate(kind: string, seed: number): Flat {
    return unwrap(this.#api.generate(kind, seed));
  }

  /** The polygon families of the benchmark, which come in any size. */
  workloads(): WorkloadInfo[] {
    return unwrap(this.#api.workloads());
  }

  /** The polygon of the family with about this many vertices. */
  workload(name: string, vertices: number): Flat {
    return unwrap(this.#api.workload(name, vertices));
  }

  triangulate(polygon: Flat, options: Options): Result {
    return unwrap(
      this.#api.triangulate(polygon.coords, polygon.holeStarts, options),
    );
  }

  /** Repeats the run until a batch takes at least budgetMs. */
  time(polygon: Flat, options: Options, budgetMs: number): Timing {
    return unwrap(
      this.#api.time(polygon.coords, polygon.holeStarts, options, budgetMs),
    );
  }
}

/** The part of wasm_exec.js's Go class the loader uses. */
interface GoRuntime {
  readonly importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

/** The globals wasm_exec.js and cmd/demo read and write. */
type WasmGlobals = typeof globalThis & {
  Go?: new () => GoRuntime;
  earstrip?: EarstripGlobal;
  onEarstripReady?: () => void;
};

const globals = globalThis as WasmGlobals;

let loading: Promise<Earstrip> | undefined;

/**
 * Loads demo.wasm on the first call and returns the same promise afterwards.
 * After a failure, or if the Go program ends later, the next call loads it
 * again.
 */
export function loadEarstrip(): Promise<Earstrip> {
  loading ??= load().catch((err: unknown) => {
    loading = undefined;
    throw err;
  });
  return loading;
}

async function fetchOk(file: string): Promise<Response> {
  const url = `${import.meta.env.BASE_URL}${file}`;
  // demo.wasm and wasm_exec.js have no content hash in their names, and GitHub
  // Pages caches them for 10 minutes; revalidate so a fresh deploy never runs
  // its new JS against an old demo.wasm.
  const response = await fetch(url, { cache: "no-cache" });
  if (!response.ok) {
    throw new Error(`${url}: HTTP ${response.status} ${response.statusText}`);
  }
  // Vite's dev server answers a missing file with index.html.
  if (response.headers.get("content-type")?.startsWith("text/html")) {
    throw new Error(`${url} is missing; run just build-demo or just dev-demo`);
  }
  return response;
}

async function load(): Promise<Earstrip> {
  if (globals.Go === undefined) {
    const source = await (await fetchOk("wasm_exec.js")).text();
    // A Function body runs in the global scope, like a classic script. (A
    // Content-Security-Policy would have to allow 'unsafe-eval' for this.)
    new Function(source)();
  }
  const Go = globals.Go;
  if (Go === undefined) {
    throw new Error("wasm_exec.js did not define globalThis.Go");
  }

  const go = new Go();
  const bytes = await (await fetchOk("demo.wasm")).arrayBuffer();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);

  return new Promise<Earstrip>((resolve, reject) => {
    // cmd/demo calls this once globalThis.earstrip is complete.
    globals.onEarstripReady = () => {
      globals.onEarstripReady = undefined;
      if (globals.earstrip === undefined) {
        reject(new Error("demo.wasm did not define globalThis.earstrip"));
      } else {
        resolve(new Earstrip(globals.earstrip));
      }
    };
    // run settles only when the Go program ends, which it never should.
    const ended = (err: unknown) => {
      globals.onEarstripReady = undefined;
      globals.earstrip = undefined;
      loading = undefined;
      reject(err);
    };
    go.run(instance).then(() => ended(new Error("demo.wasm exited")), ended);
  });
}
