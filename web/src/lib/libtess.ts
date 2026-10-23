// libtess.js, the JavaScript port of the GLU tessellator, set up to give a
// triangle list like the other triangulators of the benchmark.
//
// It fills by the positive winding rule with the outer ring turned
// counter-clockwise and the holes clockwise: what lies inside the outer ring
// and outside every hole, close to earstrip's constrained triangulation (which
// fills a self-crossing outer ring by the non-zero rule instead). Where edges
// cross it adds points, as earstrip's extra points.

import * as libtess from "libtess";

import type { Flat } from "@/wasm/earstrip";

export interface TessResult {
  /** Three indices per triangle; index n + k is extra point k. */
  triangles: number[];
  /** The points added where edges cross, x, y each. */
  extra: number[];
  /** The GLU error number, if the tessellator reported one. */
  error?: number;
}

/** Signed area of the ring of vertices begin to end, counter-clockwise positive. */
function ringArea(coords: Float64Array, begin: number, end: number): number {
  let a = 0;
  for (let i = begin, j = end - 1; i < end; j = i++) {
    a += coords[2 * j] * coords[2 * i + 1] - coords[2 * i] * coords[2 * j + 1];
  }
  return a / 2;
}

/** One reused GluTesselator, as an application would keep it. */
export class Tessellator {
  readonly #tess = new libtess.GluTesselator();
  #out: TessResult = { triangles: [], extra: [] };
  #n = 0;

  constructor() {
    const t = this.#tess;
    const e = libtess.gluEnum;
    t.gluTessProperty(
      e.GLU_TESS_WINDING_RULE,
      libtess.windingRule.GLU_TESS_WINDING_POSITIVE,
    );
    t.gluTessNormal(0, 0, 1);
    t.gluTessCallback(e.GLU_TESS_VERTEX_DATA, (index: number) => {
      this.#out.triangles.push(index);
    });
    // An edge flag callback makes it write separate triangles, no fans or
    // strips.
    t.gluTessCallback(e.GLU_TESS_EDGE_FLAG, () => {});
    t.gluTessCallback(e.GLU_TESS_COMBINE, (coords: number[]) => {
      this.#out.extra.push(coords[0], coords[1]);
      return this.#n + this.#out.extra.length / 2 - 1;
    });
    t.gluTessCallback(e.GLU_TESS_ERROR, (errno: number) => {
      this.#out.error ??= errno;
    });
  }

  /**
   * Prepares the polygon once: the coordinates as libtess takes them and the
   * order of each ring. The returned function triangulates it.
   */
  prepare(polygon: Flat): () => TessResult {
    const { coords, holeStarts } = polygon;
    const n = coords.length / 2;
    const points: number[][] = [];
    for (let i = 0; i < n; i++) {
      points.push([coords[2 * i], coords[2 * i + 1], 0]);
    }
    const starts = [0, ...holeStarts, n];
    const rings: number[][] = [];
    for (let r = 0; r + 1 < starts.length; r++) {
      const ring: number[] = [];
      for (let i = starts[r]; i < starts[r + 1]; i++) ring.push(i);
      const area = ringArea(coords, starts[r], starts[r + 1]);
      if (r === 0 ? area < 0 : area > 0) ring.reverse();
      if (ring.length > 0) rings.push(ring);
    }
    return () => {
      const t = this.#tess;
      this.#n = n;
      this.#out = { triangles: [], extra: [] };
      t.gluTessBeginPolygon(null);
      for (const ring of rings) {
        t.gluTessBeginContour();
        for (const i of ring) t.gluTessVertex(points[i], i);
        t.gluTessEndContour();
      }
      t.gluTessEndPolygon();
      return this.#out;
    };
  }
}
