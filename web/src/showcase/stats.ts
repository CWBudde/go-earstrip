// What the showcase's HUD shows: the triangulations of the latest frame that
// ran any, and moving averages of their time and of the frame rate. A demo
// triangulates through FrameStats.run, which times the wasm call.

import type {
  Batch,
  BatchResult,
  Earstrip,
  Flat,
  Options,
  Result,
  Status,
} from "@/wasm/earstrip";

/** The settings of the Playground's defaults. */
export const DEFAULT_OPTIONS: Options = {
  forcedCuts: true,
  constrained: true,
  extraPoints: true,
  triangleList: false,
};

export interface Snapshot {
  /** Triangulate calls of the frame. */
  calls: number;
  statuses: Partial<Record<Status, number>>;
  vertices: number;
  rings: number;
  triangles: number;
  strips: number;
  /** The length of the restart index buffers. */
  stripIndices: number;
  extraPoints: number;
  /** Milliseconds in triangulate per frame that ran it, moving average. */
  triangulateMs: number;
  /** Frames per second, moving average; 0 while paused. */
  fps: number;
}

/** The weight of the newest sample in the moving averages. */
const SMOOTHING = 0.1;

function empty(): Omit<Snapshot, "triangulateMs" | "fps"> {
  return {
    calls: 0,
    statuses: {},
    vertices: 0,
    rings: 0,
    triangles: 0,
    strips: 0,
    stripIndices: 0,
    extraPoints: 0,
  };
}

export class FrameStats {
  #frame = empty();
  #frameMs = 0;
  #shown = empty();
  #triangulateMs = 0;
  #frameDt = 0;
  #last = 0;

  /** Starts a frame at now (requestAnimationFrame's time). */
  begin(now: number): void {
    if (this.#last > 0) {
      const dt = now - this.#last;
      this.#frameDt =
        this.#frameDt === 0
          ? dt
          : this.#frameDt + SMOOTHING * (dt - this.#frameDt);
    }
    this.#last = now;
    this.#frame = empty();
    this.#frameMs = 0;
  }

  /** Ends the frame; a frame without triangulations keeps the last numbers. */
  end(): void {
    if (this.#frame.calls === 0) return;
    this.#shown = this.#frame;
    this.#triangulateMs =
      this.#triangulateMs === 0
        ? this.#frameMs
        : this.#triangulateMs +
          SMOOTHING * (this.#frameMs - this.#triangulateMs);
  }

  /** Forgets the frame rate, for a pause: the next frame starts it anew. */
  pause(): void {
    this.#last = 0;
    this.#frameDt = 0;
  }

  /** Triangulates the polygon, timed, and counts it into the frame. */
  run(
    api: Earstrip,
    polygon: Flat,
    options: Options = DEFAULT_OPTIONS,
  ): Result {
    const start = performance.now();
    const r = api.triangulate(polygon, options);
    this.#frameMs += performance.now() - start;
    const f = this.#frame;
    f.calls++;
    f.statuses[r.status] = (f.statuses[r.status] ?? 0) + 1;
    f.vertices += polygon.coords.length / 2;
    f.rings += polygon.holeStarts.length + 1;
    f.triangles += r.triangles.length / 3;
    f.strips += r.stripEnds.length;
    f.stripIndices += r.restart.length;
    f.extraPoints += r.extra.length / 2;
    return r;
  }

  /**
   * Triangulates the batch in one call, timed, and counts each of its
   * polygons into the frame as run would.
   */
  runMany(
    api: Earstrip,
    batch: Batch,
    options: Options = DEFAULT_OPTIONS,
  ): BatchResult {
    const start = performance.now();
    const r = api.triangulateMany(batch, options);
    this.#frameMs += performance.now() - start;
    const f = this.#frame;
    f.calls += batch.polygonStarts.length;
    for (const [k, n] of Object.entries(r.statuses) as [Status, number][]) {
      f.statuses[k] = (f.statuses[k] ?? 0) + n;
    }
    f.vertices += batch.coords.length / 2;
    f.rings += batch.ringStarts.length;
    f.triangles += r.list.length / 3;
    f.strips += r.stripListEnds.length;
    // The whole buffer, with the restart indices between polygons.
    f.stripIndices += r.restart.length;
    f.extraPoints += r.extraPoints;
    return r;
  }

  snapshot(): Snapshot {
    return {
      ...this.#shown,
      statuses: { ...this.#shown.statuses },
      triangulateMs: this.#triangulateMs,
      fps: this.#frameDt > 0 ? 1000 / this.#frameDt : 0,
    };
  }
}
