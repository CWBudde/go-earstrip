// The polygon the page edits: rings of points, ring 0 the outer ring. Vertex
// indices count the points of all rings in order, as in the strips.

import type { Flat } from "@/wasm/earstrip";

export type Point = [number, number];
export type Ring = Point[];
export type Rings = Ring[];

export function fromFlat({ coords, holeStarts }: Flat): Rings {
  const n = coords.length / 2;
  const starts = [0, ...holeStarts, n];
  const rings: Rings = [];
  for (let r = 0; r + 1 < starts.length; r++) {
    const ring: Ring = [];
    for (let i = starts[r]; i < starts[r + 1]; i++) {
      ring.push([coords[2 * i], coords[2 * i + 1]]);
    }
    rings.push(ring);
  }
  return rings;
}

export function toFlat(rings: Rings): Flat {
  const coords = new Float64Array(2 * vertexCount(rings));
  const holeStarts: number[] = [];
  let k = 0;
  rings.forEach((ring, r) => {
    if (r > 0) holeStarts.push(k / 2);
    for (const [x, y] of ring) {
      coords[k++] = x;
      coords[k++] = y;
    }
  });
  return { coords, holeStarts };
}

export function vertexCount(rings: Rings): number {
  return rings.reduce((n, ring) => n + ring.length, 0);
}

/**
 * The input vertices followed by the extra points, x, y each: point i of the
 * result has index i.
 */
export function allPoints(rings: Rings, extra?: Float64Array): Float64Array {
  const { coords } = toFlat(rings);
  if (extra === undefined || extra.length === 0) return coords;
  const out = new Float64Array(coords.length + extra.length);
  out.set(coords);
  out.set(extra, coords.length);
  return out;
}

export interface Bounds {
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
}

/** The bounding box of x, y pairs, or undefined for none. */
export function bounds(points: Float64Array): Bounds | undefined {
  if (points.length < 2) return undefined;
  const b = {
    minX: Infinity,
    minY: Infinity,
    maxX: -Infinity,
    maxY: -Infinity,
  };
  for (let i = 0; i + 1 < points.length; i += 2) {
    b.minX = Math.min(b.minX, points[i]);
    b.maxX = Math.max(b.maxX, points[i]);
    b.minY = Math.min(b.minY, points[i + 1]);
    b.maxY = Math.max(b.maxY, points[i + 1]);
  }
  return b;
}

/** JSON with one ring per line. */
export function formatJSON(rings: Rings): string {
  const ring = (r: Ring) => `[${r.map(([x, y]) => `[${x},${y}]`).join(",")}]`;
  return `[\n  ${rings.map(ring).join(",\n  ")}\n]\n`;
}

function isPoint(v: unknown): v is Point {
  return (
    Array.isArray(v) &&
    v.length >= 2 &&
    typeof v[0] === "number" &&
    typeof v[1] === "number" &&
    Number.isFinite(v[0]) &&
    Number.isFinite(v[1])
  );
}

/**
 * Reads rings of [x, y] points: an array of rings, a single ring, or a
 * GeoJSON Polygon (also its coordinates). Throws on anything else.
 */
export function parseJSON(text: string): Rings {
  let v: unknown = JSON.parse(text);
  if (
    typeof v === "object" &&
    v !== null &&
    !Array.isArray(v) &&
    "coordinates" in v
  ) {
    v = v.coordinates;
  }
  if (!Array.isArray(v)) throw new Error("want an array of rings");
  if (v.length > 0 && isPoint(v[0])) v = [v];
  const rings: Rings = [];
  for (const [r, ring] of (v as unknown[]).entries()) {
    if (!Array.isArray(ring)) throw new Error(`ring ${r}: want an array`);
    rings.push(
      ring.map((p: unknown, i) => {
        if (!isPoint(p)) {
          throw new Error(`ring ${r}, point ${i}: want [x, y], finite`);
        }
        return [p[0], p[1]];
      }),
    );
  }
  return rings;
}
