// From the field to polygons: marching squares give the closed contours of an
// iso level, an even/odd containment tree over the contours of two neighboring
// levels gives the band between them as polygons with holes, and each polygon
// becomes one triangulate call.

import type { Field } from "./field";

/**
 * The segments of a marching squares cell, by its case and whether its center
 * is inside: pairs of cell edges, from where the contour enters the cell to
 * where it leaves. Corner k is bottom left, bottom right, top right, top left
 * for k = 0..3, and edge k runs from corner k to corner k + 1, so that walking
 * the edges in order goes counter-clockwise around the cell.
 *
 * A segment starts on an edge that goes from an inside to an outside corner
 * and ends on one that goes from outside to inside; that keeps the inside on
 * the left of every contour, so that the boundary of a blob runs
 * counter-clockwise and the shore of a lake in it clockwise. A cell with one
 * in-out edge has one choice. The saddles (cases 5 and 10) have two: pairing
 * each in-out edge with the next out-in edge cuts the outside corners off and
 * joins the inside ones across the center; pairing it with the previous one
 * cuts the inside corners off. The center decides, the mean of the corners.
 */
const SEGMENTS: number[][] = (() => {
  const table: number[][] = [];
  for (let c = 0; c < 16; c++) {
    for (const centerIn of [false, true]) {
      const inside = (k: number) => ((c >> (k & 3)) & 1) === 1;
      const out: number[] = [];
      for (let k = 0; k < 4; k++) {
        if (!inside(k) || inside(k + 1)) continue;
        // Edge k goes in to out. Its partner is the next out-in edge
        // counter-clockwise, or, for a saddle with its center outside,
        // the previous one.
        const saddle = c === 5 || c === 10;
        const step = saddle && !centerIn ? 3 : 1;
        let e = (k + step) & 3;
        while (!(!inside(e) && inside(e + 1))) e = (e + step) & 3;
        out.push(k, e);
      }
      table.push(out);
    }
  }
  return table;
})();

/**
 * The contours of the latest frame, all levels together: their vertices in
 * cell units from the stage's bottom left, and per contour its level, signed
 * area, bounds and place in the containment tree. The arrays are reused from
 * frame to frame.
 */
export class Contours {
  /** x, y of the contours' vertices, one contour after the other. */
  coords = new Float64Array(1 << 14);
  /** Vertex index where contour k starts; contour k ends where k + 1 starts. */
  readonly start: number[] = [0];
  readonly level: number[] = [];
  /** Positive for counter-clockwise: the boundary of a blob, else a lake's shore. */
  readonly area: number[] = [];
  readonly bounds: number[] = [];

  // The marching squares: the next edge of each edge crossed by the contour,
  // the edges it was set for, and which ones the walk has passed, marked with
  // a stamp so that they need no clearing.
  #next = new Int32Array(0);
  #seen = new Int32Array(0);
  #stamp = 0;
  readonly #starts: number[] = [];

  // The containment tree of assemble.
  readonly #members: number[] = [];
  readonly #depth: number[] = [];
  readonly #parent: number[] = [];
  /** The polygon of assemble's callback: outer contour, then its holes. */
  #polygon = new Float64Array(1 << 12);
  readonly #holeStarts: number[] = [];

  get count(): number {
    return this.level.length;
  }

  reset(): void {
    this.start.length = 1;
    this.level.length = 0;
    this.area.length = 0;
    this.bounds.length = 0;
  }

  /** Appends the closed contours of field = iso, tagged with level. */
  march(field: Field, iso: number, level: number): void {
    const { nx, ny, pad, values } = field;
    // Horizontal edges first: (i, j)-(i + 1, j) is j (nx - 1) + i; then the
    // vertical ones: (i, j)-(i, j + 1) is h + j nx + i.
    const h = ny * (nx - 1);
    const edges = h + (ny - 1) * nx;
    if (this.#next.length < edges) {
      this.#next = new Int32Array(edges);
      this.#seen = new Int32Array(edges);
      this.#stamp = 0;
    }
    const next = this.#next;
    const starts = this.#starts;
    starts.length = 0;
    const cell = [0, 0, 0, 0];

    for (let j = 0; j < ny - 1; j++) {
      for (let i = 0; i < nx - 1; i++) {
        const s = j * nx + i;
        const v0 = values[s];
        const v1 = values[s + 1];
        const v2 = values[s + nx + 1];
        const v3 = values[s + nx];
        const c =
          (v0 > iso ? 1 : 0) |
          (v1 > iso ? 2 : 0) |
          (v2 > iso ? 4 : 0) |
          (v3 > iso ? 8 : 0);
        if (c === 0 || c === 15) continue;
        const centerIn = (v0 + v1 + v2 + v3) / 4 > iso ? 1 : 0;
        const seg = SEGMENTS[2 * c + centerIn];
        cell[0] = j * (nx - 1) + i;
        cell[1] = h + j * nx + i + 1;
        cell[2] = (j + 1) * (nx - 1) + i;
        cell[3] = h + j * nx + i;
        for (let k = 0; k < seg.length; k += 2) {
          const from = cell[seg[k]];
          next[from] = cell[seg[k + 1]];
          starts.push(from);
        }
      }
    }

    // Walk each contour once, from the first of its edges the scan found.
    const seen = this.#seen;
    const stamp = ++this.#stamp;
    for (const first of starts) {
      if (seen[first] === stamp) continue;
      let n = this.start[this.start.length - 1];
      let e = first;
      let area = 0;
      let minX = Infinity;
      let minY = Infinity;
      let maxX = -Infinity;
      let maxY = -Infinity;
      let px = 0;
      let py = 0;
      let fx = 0;
      let fy = 0;
      const begin = n;
      do {
        seen[e] = stamp;
        // Where the field crosses iso on edge e, by linear interpolation from
        // its lower sample, so that both cells of an edge agree. The clamp
        // keeps the crossings of two edges that share a sample apart when that
        // sample is exactly iso.
        let a: number;
        let dx = 0;
        let dy = 0;
        if (e < h) {
          a = Math.floor(e / (nx - 1)) * nx + (e % (nx - 1));
          dx = 1;
        } else {
          a = e - h;
          dy = 1;
        }
        const va = values[a];
        const vb = values[a + (dx === 1 ? 1 : nx)];
        let t = (iso - va) / (vb - va);
        t = t < 1e-3 ? 1e-3 : t > 1 - 1e-3 ? 1 - 1e-3 : t;
        const x = (a % nx) - pad + t * dx;
        const y = Math.floor(a / nx) - pad + t * dy;
        if (2 * n + 2 > this.coords.length) {
          const grown = new Float64Array(2 * this.coords.length);
          grown.set(this.coords);
          this.coords = grown;
        }
        this.coords[2 * n] = x;
        this.coords[2 * n + 1] = y;
        if (n === begin) {
          fx = x;
          fy = y;
        } else {
          area += px * y - x * py;
        }
        px = x;
        py = y;
        if (x < minX) minX = x;
        if (x > maxX) maxX = x;
        if (y < minY) minY = y;
        if (y > maxY) maxY = y;
        n++;
        e = next[e];
      } while (e !== first);
      area += px * fy - fx * py;
      this.start.push(n);
      this.level.push(level);
      this.area.push(area / 2);
      this.bounds.push(minX, minY, maxX, maxY);
    }
  }

  /** Whether (x, y) lies inside contour k, by the even/odd rule. */
  #contains(k: number, x: number, y: number): boolean {
    const b = this.bounds;
    if (
      x < b[4 * k] ||
      x > b[4 * k + 2] ||
      y < b[4 * k + 1] ||
      y > b[4 * k + 3]
    )
      return false;
    const c = this.coords;
    const begin = this.start[k];
    const end = this.start[k + 1];
    let inside = false;
    let xj = c[2 * end - 2];
    let yj = c[2 * end - 1];
    for (let i = begin; i < end; i++) {
      const xi = c[2 * i];
      const yi = c[2 * i + 1];
      if (yi > y !== yj > y && x < ((xj - xi) * (y - yi)) / (yj - yi) + xi)
        inside = !inside;
      xj = xi;
      yj = yi;
    }
    return inside;
  }

  /**
   * Calls emit with the polygons of the region between the contours of the
   * levels lo and hi: inside lo and outside hi (hi < 0: inside lo). That
   * region is the even/odd fill of both levels' contours together, since hi's
   * region lies within lo's. So the containment tree over them decides it: a
   * contour inside an even number of others is an outer ring, and the
   * contours right inside it are its holes. An island in a lake is an outer
   * ring again, its own polygon.
   *
   * emit gets the polygon's coords and hole starts, both reused for the next
   * call, and returns nothing it keeps.
   */
  assemble(
    lo: number,
    hi: number,
    emit: (coords: Float64Array, holeStarts: number[]) => void,
  ): void {
    const members = this.#members;
    const depth = this.#depth;
    const parent = this.#parent;
    members.length = 0;
    for (let k = 0; k < this.count; k++) {
      if (this.level[k] === lo || this.level[k] === hi) members.push(k);
    }
    const m = members.length;
    depth.length = m;
    parent.length = m;
    for (let a = 0; a < m; a++) {
      const k = members[a];
      const x = this.coords[2 * this.start[k]];
      const y = this.coords[2 * this.start[k] + 1];
      const area = Math.abs(this.area[k]);
      let d = 0;
      let p = -1;
      for (let b = 0; b < m; b++) {
        const q = members[b];
        if (b === a || Math.abs(this.area[q]) <= area) continue;
        if (!this.#contains(q, x, y)) continue;
        d++;
        // The innermost container is the parent: the smallest one.
        if (p < 0 || Math.abs(this.area[q]) < Math.abs(this.area[members[p]]))
          p = b;
      }
      depth[a] = d;
      parent[a] = p;
    }

    const holeStarts = this.#holeStarts;
    for (let a = 0; a < m; a++) {
      if (depth[a] % 2 !== 0) continue;
      holeStarts.length = 0;
      let n = this.#copy(members[a], 0);
      for (let b = 0; b < m; b++) {
        if (parent[b] !== a || depth[b] % 2 === 0) continue;
        holeStarts.push(n);
        n = this.#copy(members[b], n);
      }
      emit(this.#polygon.subarray(0, 2 * n), holeStarts);
    }
  }

  /** Copies contour k into the polygon at vertex n; returns the end. */
  #copy(k: number, n: number): number {
    const begin = this.start[k];
    const end = this.start[k + 1];
    const need = 2 * (n + end - begin);
    if (need > this.#polygon.length) {
      const grown = new Float64Array(Math.max(need, 2 * this.#polygon.length));
      grown.set(this.#polygon.subarray(0, 2 * n));
      this.#polygon = grown;
    }
    this.#polygon.set(this.coords.subarray(2 * begin, 2 * end), 2 * n);
    return n + end - begin;
  }
}
