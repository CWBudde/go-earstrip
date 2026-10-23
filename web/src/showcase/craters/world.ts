// The ground of the crater sandbox as 8 × 4 tiles. Each tile is one polygon:
// its square as the outer ring, and as holes the sky above the surface, the
// caves and every crater that touches it. The holes overlap each other and
// cross the square, so the triangulator's constrained fallback computes "the
// square minus all holes" by non-zero winding: a boolean difference without a
// clipping library. Only tiles whose holes changed are triangulated again.
//
// The triangles of each tile also serve the balls as their collision shape,
// through a grid of buckets per tile.

import type { FrameStats } from "@/showcase/stats";
import type { Earstrip, Result, Status } from "@/wasm/earstrip";

import {
  COLS,
  ROWS,
  surfaceRange,
  surfaceY,
  TILE_H,
  TILE_W,
  WORLD_W,
  type Terrain,
} from "./terrain";

/**
 * The most craters one tile holds. Past it the tile's oldest crater is merged
 * with its nearest neighbor into the smallest circle around both (see
 * World.#enforceCap), so ground only ever disappears and never grows back.
 */
export const HOLE_CAP = 100;

/** The vertices of a crater's ring. */
const CRATER_VERTICES = 24;
/** How far the rim of a crater wobbles off its radius, relatively. */
const WOBBLE = 0.05;
/**
 * A crater's ring lies within r * OUTER of its centre and holds the disc of
 * r * INNER: the wobble, and for INNER also the chords between its vertices.
 */
const OUTER = 1 + WOBBLE;
const INNER = (1 - WOBBLE) * Math.cos(Math.PI / 24);
/** The spacing of the sky ring's samples along the surface. */
const SKY_STEP = 4;
/** How far the sky ring reaches past its tile, so that it crosses the square. */
const SKY_MARGIN = 6;
/** The buckets per tile of the collision grid. */
const GX = 8;
const GY = 9;
const CELL_W = TILE_W / GX;
const CELL_H = TILE_H / GY;

export interface Crater {
  x: number;
  y: number;
  r: number;
  /** Increases with every crater, so that a smaller age is older. */
  age: number;
  ring: Float64Array;
}

/** What World.contact found. */
export const Contact = {
  None: 0,
  /** The circle touches the ground; the closest point is in the out array. */
  Touch: 1,
  /** The centre is inside the ground. */
  Inside: 2,
} as const;
export type Contact = (typeof Contact)[keyof typeof Contact];

export class Tile {
  readonly index: number;
  readonly x0: number;
  readonly y0: number;
  readonly x1: number;
  readonly y1: number;
  dirty = true;
  /** 1 right after a triangulation, fading to 0. */
  flash = 0;
  /** The craters in the polygon of the latest run. */
  holes = 0;
  status: Status | undefined;
  result: Result | undefined;
  /** The polygon's vertices followed by the run's extra points. */
  points: Float64Array = new Float64Array(0);
  /** The run's triangles, three indices into points each. */
  triangles: Uint32Array = new Uint32Array(0);
  /** The collision grid: the triangles of bucket b are items[start[b]..start[b+1]). */
  readonly start = new Int32Array(GX * GY + 1);
  items = new Int32Array(0);

  constructor(index: number) {
    this.index = index;
    const col = index % COLS;
    const row = Math.floor(index / COLS);
    this.x0 = col * TILE_W;
    this.y0 = row * TILE_H;
    this.x1 = this.x0 + TILE_W;
    this.y1 = this.y0 + TILE_H;
  }

  /** Whether the circle reaches into the tile. */
  touches(x: number, y: number, r: number): boolean {
    const dx = x - Math.min(Math.max(x, this.x0), this.x1);
    const dy = y - Math.min(Math.max(y, this.y0), this.y1);
    return dx * dx + dy * dy < r * r;
  }
}

export class World {
  readonly tiles: Tile[] = [];
  craters: Crater[] = [];
  terrain: Terrain;
  /** Increases whenever any tile's triangles change, for the renderer. */
  version = 0;
  /** How many tiles the latest update triangulated. */
  lastDirty = 0;
  #age = 0;
  #poly = new Float64Array(4096);
  #holeStarts: number[] = [];

  constructor(terrain: Terrain) {
    this.terrain = terrain;
    for (let i = 0; i < COLS * ROWS; i++) this.tiles.push(new Tile(i));
  }

  /** Starts over on a new landscape. */
  reset(terrain: Terrain): void {
    this.terrain = terrain;
    this.craters = [];
    for (const t of this.tiles) t.dirty = true;
  }

  /**
   * Blasts a crater. A crater that the others and the sky already cover is
   * not added, and the craters that the new one leaves covered are dropped
   * (see #covered). Returns whether the ground changed.
   */
  blast(x: number, y: number, r: number): boolean {
    const crater = this.#crater(x, y, r);
    if (this.#covered(crater, this.craters)) {
      this.#age--;
      return false;
    }
    this.#add(crater);
    this.#enforceCap(crater);
    return true;
  }

  #crater(x: number, y: number, r: number): Crater {
    const ring = new Float64Array(2 * CRATER_VERTICES);
    const phase = (x * 12.9898 + y * 78.233) % 6.283;
    for (let k = 0; k < CRATER_VERTICES; k++) {
      // Clockwise in y-up terms, as a hole; the triangulator takes either.
      const a = (k / CRATER_VERTICES) * 2 * Math.PI;
      const rr = r * (1 + WOBBLE * Math.sin(3 * a + phase) * Math.cos(2 * a));
      ring[2 * k] = x + Math.cos(a) * rr;
      ring[2 * k + 1] = y + Math.sin(a) * rr;
    }
    return { x, y, r, age: this.#age++, ring };
  }

  /**
   * Adds the crater and marks its tiles. Older craters that it overlaps and
   * that are now covered by the others are dropped: they no longer change the
   * ground, but they would cost the triangulation their crossings.
   */
  #add(crater: Crater): void {
    this.craters.push(crater);
    this.#mark(crater);
    const reach = crater.r * OUTER;
    const near = this.craters.filter(
      (c) =>
        c !== crater &&
        Math.hypot(c.x - crater.x, c.y - crater.y) < c.r * OUTER + reach,
    );
    for (const c of near) {
      if (this.#covered(c, this.craters)) {
        this.craters = this.craters.filter((o) => o !== c);
        this.#mark(c);
      }
    }
  }

  /**
   * Whether the crater's ring lies wholly in the other craters' and the sky's
   * union, so that leaving it out gives the same ground. The test is
   * conservative: it splits the ring's bounds into square cells and wants each
   * cell that may meet the ring inside one other crater's inner disc, or above
   * the surface everywhere along it.
   */
  #covered(a: Crater, others: Crater[]): boolean {
    const R = a.r * OUTER;
    const near = others.filter(
      (c) => c !== a && Math.hypot(c.x - a.x, c.y - a.y) < c.r * INNER + R,
    );
    const heights = this.terrain.heights;
    const cells = 10;
    const step = (2 * R) / cells;
    const half = step / 2;
    // The distance from a cell's centre to its corners.
    const corner = half * Math.SQRT2;
    for (let j = 0; j < cells; j++) {
      const y = a.y - R + half + j * step;
      for (let i = 0; i < cells; i++) {
        const x = a.x - R + half + i * step;
        if (Math.hypot(x - a.x, y - a.y) > R + corner) continue;
        let inside = false;
        for (const c of near) {
          if (Math.hypot(x - c.x, y - c.y) + corner <= c.r * INNER) {
            inside = true;
            break;
          }
        }
        if (!inside) {
          const [top] = surfaceRange(heights, x - half, x + half);
          inside = y + half < top;
        }
        if (!inside) return false;
      }
    }
    return true;
  }

  #mark(c: Crater): void {
    const r = c.r * OUTER;
    for (const t of this.tiles) if (t.touches(c.x, c.y, r)) t.dirty = true;
  }

  #cratersOf(t: Tile): Crater[] {
    return this.craters.filter((c) => t.touches(c.x, c.y, c.r * OUTER));
  }

  /**
   * Keeps every tile at HOLE_CAP craters or fewer. While a tile has more, its
   * oldest crater and the crater nearest to it become the smallest circle that
   * holds both. The new circle covers both old ones, so the ground only loses
   * area; it can reach into another tile, which is then checked as well.
   */
  #enforceCap(start: Crater): void {
    const queue = this.tiles.filter((t) =>
      t.touches(start.x, start.y, start.r * OUTER),
    );
    while (queue.length > 0) {
      const t = queue.pop()!;
      const mine = this.#cratersOf(t);
      if (mine.length <= HOLE_CAP) continue;
      let a = mine[0];
      for (const c of mine) if (c.age < a.age) a = c;
      let b: Crater | undefined;
      let best = Infinity;
      for (const c of mine) {
        if (c === a) continue;
        const d = Math.hypot(c.x - a.x, c.y - a.y) - c.r;
        if (d < best) {
          best = d;
          b = c;
        }
      }
      if (b === undefined) break;
      const d = Math.hypot(b.x - a.x, b.y - a.y);
      let m: Crater;
      if (d + b.r <= a.r) {
        m = this.#crater(a.x, a.y, a.r);
      } else if (d + a.r <= b.r) {
        m = this.#crater(b.x, b.y, b.r);
      } else {
        const r = (d + a.r + b.r) / 2;
        const k = (r - a.r) / d;
        m = this.#crater(a.x + (b.x - a.x) * k, a.y + (b.y - a.y) * k, r);
      }
      m.age = b.age;
      this.craters = this.craters.filter((c) => c !== a && c !== b);
      this.#mark(a);
      this.#mark(b);
      this.#add(m);
      queue.push(t);
      for (const o of this.tiles) {
        if (o !== t && o.touches(m.x, m.y, m.r * OUTER)) queue.push(o);
      }
    }
  }

  /** Triangulates the dirty tiles; returns how many. */
  update(api: Earstrip, stats: FrameStats): number {
    let n = 0;
    for (const t of this.tiles) {
      if (!t.dirty) continue;
      t.dirty = false;
      this.#triangulate(t, api, stats);
      t.flash = 1;
      n++;
    }
    if (n > 0) {
      this.version++;
      this.lastDirty = n;
    }
    return n;
  }

  #grow(n: number): void {
    if (this.#poly.length >= n) return;
    const p = new Float64Array(Math.max(n, this.#poly.length * 2));
    p.set(this.#poly);
    this.#poly = p;
  }

  #triangulate(t: Tile, api: Earstrip, stats: FrameStats): void {
    const { heights, caves } = this.terrain;
    const [top, bottom] = surfaceRange(
      heights,
      t.x0 - SKY_MARGIN,
      t.x1 + SKY_MARGIN,
    );
    t.result = undefined;
    t.status = undefined;
    t.holes = 0;
    t.points = new Float64Array(0);
    t.triangles = new Uint32Array(0);
    t.start.fill(0);
    if (top >= t.y1) return; // All sky.

    let p = this.#poly;
    let k = 0;
    const holeStarts = this.#holeStarts;
    holeStarts.length = 0;
    const push = (x: number, y: number) => {
      p[k++] = x;
      p[k++] = y;
    };
    push(t.x0, t.y0);
    push(t.x1, t.y0);
    push(t.x1, t.y1);
    push(t.x0, t.y1);

    if (bottom > t.y0) {
      // The sky: the surface across the tile and a little past it, closed
      // above whatever is highest.
      holeStarts.push(k / 2);
      const x0 = Math.max(t.x0 - SKY_MARGIN, -SKY_MARGIN);
      const x1 = Math.min(t.x1 + SKY_MARGIN, WORLD_W + SKY_MARGIN);
      const steps = Math.ceil((x1 - x0) / SKY_STEP);
      for (let i = 0; i <= steps; i++) {
        const x = x0 + ((x1 - x0) * i) / steps;
        push(x, surfaceY(heights, x));
      }
      const lid = Math.min(t.y0, top) - SKY_MARGIN;
      push(x1, lid);
      push(x0, lid);
    }

    const inside = (minX: number, minY: number, maxX: number, maxY: number) =>
      maxX > t.x0 && minX < t.x1 && maxY > t.y0 && minY < t.y1;
    const addRing = (ring: Float64Array) => {
      this.#grow(k + ring.length);
      p = this.#poly;
      holeStarts.push(k / 2);
      p.set(ring, k);
      k += ring.length;
    };
    for (const c of caves) {
      if (inside(c.minX, c.minY, c.maxX, c.maxY)) addRing(c.coords);
    }
    for (const c of this.craters) {
      if (t.touches(c.x, c.y, c.r * OUTER)) {
        addRing(c.ring);
        t.holes++;
      }
    }

    const coords = p.subarray(0, k);
    const r = stats.run(api, { coords, holeStarts });
    t.result = r;
    t.status = r.status;
    const points = new Float64Array(k + r.extra.length);
    points.set(coords);
    points.set(r.extra, k);
    t.points = points;
    t.triangles = r.triangles;
    this.#bucket(t);
  }

  /** Fills the tile's collision grid from its triangles' bounds. */
  #bucket(t: Tile): void {
    const { points, triangles, start } = t;
    const span = (tri: number) => {
      const a = 2 * triangles[tri];
      const b = 2 * triangles[tri + 1];
      const c = 2 * triangles[tri + 2];
      const minX = Math.min(points[a], points[b], points[c]) - t.x0;
      const maxX = Math.max(points[a], points[b], points[c]) - t.x0;
      const minY = Math.min(points[a + 1], points[b + 1], points[c + 1]) - t.y0;
      const maxY = Math.max(points[a + 1], points[b + 1], points[c + 1]) - t.y0;
      return [
        Math.max(0, Math.floor(minX / CELL_W)),
        Math.min(GX - 1, Math.floor(maxX / CELL_W)),
        Math.max(0, Math.floor(minY / CELL_H)),
        Math.min(GY - 1, Math.floor(maxY / CELL_H)),
      ];
    };
    start.fill(0);
    for (let tri = 0; tri < triangles.length; tri += 3) {
      const [cx0, cx1, cy0, cy1] = span(tri);
      for (let cy = cy0; cy <= cy1; cy++)
        for (let cx = cx0; cx <= cx1; cx++) start[cy * GX + cx + 1]++;
    }
    for (let b = 0; b < GX * GY; b++) start[b + 1] += start[b];
    const items = new Int32Array(start[GX * GY]);
    const fill = start.slice(0, GX * GY);
    for (let tri = 0; tri < triangles.length; tri += 3) {
      const [cx0, cx1, cy0, cy1] = span(tri);
      for (let cy = cy0; cy <= cy1; cy++)
        for (let cx = cx0; cx <= cx1; cx++) items[fill[cy * GX + cx]++] = tri;
    }
    t.items = items;
  }

  /**
   * Finds the ground within r of (x, y). For a centre outside the ground the
   * closest point over all triangles is the closest point on the boundary,
   * which the triangles' shared inner edges do not affect. It goes to out as
   * x, y, distance.
   */
  contact(x: number, y: number, r: number, out: Float64Array): Contact {
    let best = r * r;
    let found = false;
    const c0 = Math.max(0, Math.floor((x - r) / TILE_W));
    const c1 = Math.min(COLS - 1, Math.floor((x + r) / TILE_W));
    const r0 = Math.max(0, Math.floor((y - r) / TILE_H));
    const r1 = Math.min(ROWS - 1, Math.floor((y + r) / TILE_H));
    for (let row = r0; row <= r1; row++) {
      for (let col = c0; col <= c1; col++) {
        const t = this.tiles[row * COLS + col];
        if (t.triangles.length === 0) continue;
        const { points, triangles, start, items } = t;
        const gx0 = Math.max(0, Math.floor((x - r - t.x0) / CELL_W));
        const gx1 = Math.min(GX - 1, Math.floor((x + r - t.x0) / CELL_W));
        const gy0 = Math.max(0, Math.floor((y - r - t.y0) / CELL_H));
        const gy1 = Math.min(GY - 1, Math.floor((y + r - t.y0) / CELL_H));
        for (let gy = gy0; gy <= gy1; gy++) {
          for (let gx = gx0; gx <= gx1; gx++) {
            const b = gy * GX + gx;
            for (let i = start[b]; i < start[b + 1]; i++) {
              const tri = items[i];
              const a = 2 * triangles[tri];
              const bb = 2 * triangles[tri + 1];
              const c = 2 * triangles[tri + 2];
              const ax = points[a];
              const ay = points[a + 1];
              const bx = points[bb];
              const by = points[bb + 1];
              const cx = points[c];
              const cy = points[c + 1];
              const s1 = (bx - ax) * (y - ay) - (by - ay) * (x - ax);
              const s2 = (cx - bx) * (y - by) - (cy - by) * (x - bx);
              const s3 = (ax - cx) * (y - cy) - (ay - cy) * (x - cx);
              if ((s1 > 0 && s2 > 0 && s3 > 0) || (s1 < 0 && s2 < 0 && s3 < 0))
                return Contact.Inside;
              for (let e = 0; e < 3; e++) {
                const px = e === 0 ? ax : e === 1 ? bx : cx;
                const py = e === 0 ? ay : e === 1 ? by : cy;
                const qx = e === 0 ? bx : e === 1 ? cx : ax;
                const qy = e === 0 ? by : e === 1 ? cy : ay;
                const ex = qx - px;
                const ey = qy - py;
                const len = ex * ex + ey * ey;
                let u = len > 0 ? ((x - px) * ex + (y - py) * ey) / len : 0;
                u = u < 0 ? 0 : u > 1 ? 1 : u;
                const hx = px + ex * u;
                const hy = py + ey * u;
                const d = (x - hx) * (x - hx) + (y - hy) * (y - hy);
                if (d < best) {
                  best = d;
                  found = true;
                  out[0] = hx;
                  out[1] = hy;
                }
              }
            }
          }
        }
      }
    }
    if (!found) return Contact.None;
    out[2] = Math.sqrt(best);
    return Contact.Touch;
  }

  /** The tiles by status, for the HUD. */
  statuses(): Partial<Record<Status, number>> {
    const s: Partial<Record<Status, number>> = {};
    for (const t of this.tiles) {
      if (t.status !== undefined) s[t.status] = (s[t.status] ?? 0) + 1;
    }
    return s;
  }
}
