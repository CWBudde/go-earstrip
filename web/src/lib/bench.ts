// The triangulators of the Performance tab, how each is timed, and what one
// run of each gives, for the quality columns.
//
// go-earstrip is timed inside the WebAssembly module (internal/demo.Runner's
// Time), the JavaScript libraries by the same doubling loop here: a batch of
// runs doubles until it takes at least the budget. Every measurement is the
// best of a few batches after one to warm up (the JIT, the buffers).

import earcut, { refine } from "earcut";

import { Tessellator } from "@/lib/libtess";
import type { Earstrip, Flat, Options, Status, Timing } from "@/wasm/earstrip";

export type AlgorithmId =
  | "earstrip"
  | "earcut"
  | "libtess"
  | "earstrip-list"
  | "earstrip-js"
  | "earstrip-ears"
  | "earcut-refine";

export interface Algorithm {
  id: AlgorithmId;
  name: string;
  /** What it is and what the time includes. */
  about: string;
  /** Its series color, a custom property of index.css. */
  color: string;
}

/** In the order of their colors, which stay with them. */
export const ALGORITHMS: Algorithm[] = [
  {
    id: "earstrip",
    name: "go-earstrip",
    about:
      "Triangle strips, the default settings. Timed inside the WebAssembly module: the polygon is already copied in.",
    color: "var(--series-1)",
  },
  {
    id: "earcut",
    name: "earcut",
    about:
      "mapbox/earcut 3.2.4 in JavaScript: ear clipping with a z-order index, writes a triangle list. Rings that cross are not handled.",
    color: "var(--series-2)",
  },
  {
    id: "libtess",
    name: "libtess",
    about:
      "libtess.js 1.2.2, the GLU tessellator in JavaScript: a sweep line into monotone regions, adds points where edges cross. Positive winding, the outer ring turned counter-clockwise and the holes clockwise beforehand.",
    color: "var(--series-3)",
  },
  {
    id: "earstrip-list",
    name: "go-earstrip, triangle list",
    about: "Three indices per triangle instead of strips.",
    color: "var(--series-4)",
  },
  {
    id: "earstrip-js",
    name: "go-earstrip, called from JS",
    about:
      "The default settings, timed in JavaScript: every run copies the polygon into the module and the strips, triangles and extra points out.",
    color: "var(--series-5)",
  },
  {
    id: "earstrip-ears",
    name: "go-earstrip, ears only",
    about:
      "The constrained triangulation off: polygons the ears cannot finish are untangled and cut by forced cuts.",
    color: "var(--series-6)",
  },
  {
    id: "earcut-refine",
    name: "earcut + refine",
    about:
      "earcut, then its refine pass: Lawson flips toward the constrained Delaunay triangulation, which removes most slivers.",
    color: "var(--series-7)",
  },
];

export const DEFAULT_OPTIONS: Options = {
  forcedCuts: true,
  constrained: true,
  extraPoints: true,
  triangleList: false,
};

const EARSTRIP_OPTIONS: Partial<Record<AlgorithmId, Options>> = {
  earstrip: DEFAULT_OPTIONS,
  "earstrip-js": DEFAULT_OPTIONS,
  "earstrip-list": { ...DEFAULT_OPTIONS, triangleList: true },
  "earstrip-ears": { ...DEFAULT_OPTIONS, constrained: false },
};

/** What one run gives. */
export interface Outcome {
  triangles: number;
  /** The length of the index buffer: strips or triangle list. */
  indices: number;
  /** The sum of the triangle areas. */
  area: number;
  extraPoints: number;
  /** go-earstrip's status. */
  status?: Status;
  /** What went wrong, if the triangulator reported it. */
  error?: string;
}

export interface Measurement extends Outcome {
  nsPerRun: number;
  /** The runs of the best batch. */
  runs: number;
}

/** The sum of the areas of the triangles; index n + k is extra point k. */
export function triangleArea(
  coords: ArrayLike<number>,
  extra: ArrayLike<number>,
  triangles: ArrayLike<number>,
): number {
  const n = coords.length / 2;
  const x = (i: number) => (i < n ? coords[2 * i] : extra[2 * (i - n)]);
  const y = (i: number) => (i < n ? coords[2 * i + 1] : extra[2 * (i - n) + 1]);
  let sum = 0;
  for (let t = 0; t + 2 < triangles.length; t += 3) {
    const [a, b, c] = [triangles[t], triangles[t + 1], triangles[t + 2]];
    sum += Math.abs(
      (x(b) - x(a)) * (y(c) - y(a)) - (x(c) - x(a)) * (y(b) - y(a)),
    );
  }
  return sum / 2;
}

/** Holds the result of the last timed run, so that no run can be skipped. */
export let lastResult: unknown;

/** Runs fn in batches that double until one takes at least budgetMs. */
function timeJS(fn: () => unknown, budgetMs: number): Timing {
  for (let n = 1; ; n *= 2) {
    const start = performance.now();
    for (let i = 0; i < n; i++) lastResult = fn();
    const ms = performance.now() - start;
    if (ms >= budgetMs || n >= 1 << 24) {
      return { runs: n, nsPerRun: (ms * 1e6) / n };
    }
  }
}

/** One triangulator, ready for one polygon. */
interface Prepared {
  time(budgetMs: number): Timing;
  outcome(): Outcome;
}

const GLU_ERRORS: Record<number, string> = {
  100151: "missing begin polygon",
  100152: "missing begin contour",
  100153: "missing end polygon",
  100154: "missing end contour",
  100155: "coordinate too large",
  100156: "needs a combine callback",
};

export class Bench {
  readonly #api: Earstrip;
  readonly #tess = new Tessellator();

  constructor(api: Earstrip) {
    this.#api = api;
  }

  #prepare(id: AlgorithmId, polygon: Flat): Prepared {
    const api = this.#api;
    const { coords, holeStarts } = polygon;
    const options = EARSTRIP_OPTIONS[id];
    if (options !== undefined) {
      const outcome = (): Outcome => {
        const r = api.triangulate(polygon, options);
        return {
          triangles: r.triangles.length / 3,
          indices: options.triangleList ? r.triangles.length : r.strips.length,
          area: triangleArea(coords, r.extra, r.triangles),
          extraPoints: r.extra.length / 2,
          status: r.status,
        };
      };
      if (id === "earstrip-js") {
        return {
          time: (budget) =>
            timeJS(() => api.triangulate(polygon, options), budget),
          outcome,
        };
      }
      return { time: (budget) => api.time(polygon, options, budget), outcome };
    }
    const list = (triangles: number[], extra: number[] = []): Outcome => ({
      triangles: triangles.length / 3,
      indices: triangles.length,
      area: triangleArea(coords, extra, triangles),
      extraPoints: extra.length / 2,
    });
    switch (id) {
      case "earcut":
        return {
          time: (budget) => timeJS(() => earcut(coords, holeStarts), budget),
          outcome: () => list(earcut(coords, holeStarts)),
        };
      case "earcut-refine": {
        const run = () => {
          const triangles = earcut(coords, holeStarts);
          refine(triangles, coords);
          return triangles;
        };
        return {
          time: (budget) => timeJS(run, budget),
          outcome: () => list(run()),
        };
      }
      case "libtess": {
        const run = this.#tess.prepare(polygon);
        return {
          time: (budget) => timeJS(run, budget),
          outcome: () => {
            const r = run();
            const out = list(r.triangles, r.extra);
            if (r.error !== undefined) {
              out.error = `GLU error: ${GLU_ERRORS[r.error] ?? r.error}`;
            }
            return out;
          },
        };
      }
    }
    throw new Error(`unknown triangulator ${id}`);
  }

  /**
   * Times the triangulator on the polygon: the best of rounds batches after
   * one to warm up, each at least budgetMs long. It blocks for about
   * 2 (rounds + 1) budgetMs. When a single run takes more than twice the
   * budget, one batch after the warm-up counts, and when it takes more than a
   * second, the warm-up does.
   */
  measure(
    id: AlgorithmId,
    polygon: Flat,
    budgetMs: number,
    rounds: number,
  ): Measurement {
    const p = this.#prepare(id, polygon);
    const outcome = p.outcome();
    let best = p.time(budgetMs);
    const n =
      best.nsPerRun > 1e9 ? 0 : best.nsPerRun > 2e6 * budgetMs ? 1 : rounds;
    for (let r = 0; r < n; r++) {
      const t = p.time(budgetMs);
      // The first batch only warms up.
      if (r === 0 || t.nsPerRun < best.nsPerRun) best = t;
    }
    return { ...outcome, nsPerRun: best.nsPerRun, runs: best.runs };
  }

  /** go-earstrip's result with the default settings, the reference. */
  reference(polygon: Flat): Outcome {
    return this.#prepare("earstrip", polygon).outcome();
  }
}
