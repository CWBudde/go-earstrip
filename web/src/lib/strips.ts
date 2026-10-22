// Which strip each triangle of Result.triangles comes from. Triangles walks
// the strips in order, a strip of k indices giving k - 2 triangles, so the
// strip ends tell it.

import type { Result } from "@/wasm/earstrip";

export interface TriangleStrips {
  /** Per triangle, its strip, or -1 in triangle list mode. */
  strip: Int32Array;
  /** Per triangle, its position in its strip (in the list in list mode). */
  pos: Int32Array;
  /** Triangles per strip. */
  counts: number[];
}

export function triangleStrips(r: Result | undefined): TriangleStrips {
  const n = r === undefined ? 0 : r.triangles.length / 3;
  const strip = new Int32Array(n).fill(-1);
  const pos = new Int32Array(n);
  const counts: number[] = [];
  if (r === undefined || r.stripEnds.length === 0) {
    for (let t = 0; t < n; t++) pos[t] = t;
    return { strip, pos, counts };
  }
  let t = 0;
  let begin = 0;
  r.stripEnds.forEach((end, s) => {
    const count = Math.max(end - begin - 2, 0);
    for (let k = 0; k < count; k++, t++) {
      strip[t] = s;
      pos[t] = k;
    }
    counts.push(count);
    begin = end;
  });
  return { strip, pos, counts };
}

/**
 * The length of Triangulator.Restart's index buffer: every strip, a strip
 * that starts clockwise with its first index twice, a restart index between
 * strips.
 */
export function restartLength(r: Result): number {
  const strips = r.stripEnds.length;
  if (strips === 0) return 0;
  let cw = 0;
  for (const c of r.stripClockwise) cw += c === 0 ? 0 : 1;
  return r.strips.length + cw + strips - 1;
}

/** A color per strip, golden-angle hues apart. */
export function stripColor(s: number, dark: boolean): string {
  const hue = (s * 137.508) % 360;
  return dark ? `oklch(0.55 0.13 ${hue})` : `oklch(0.8 0.12 ${hue})`;
}

/** A color along the list order, for triangle list mode. */
export function orderColor(t: number, n: number, dark: boolean): string {
  const hue = 250 - (200 * t) / Math.max(n - 1, 1);
  return dark ? `oklch(0.55 0.13 ${hue})` : `oklch(0.8 0.12 ${hue})`;
}
