// What the world map draws, built from one triangulateMany result:
//
// - the shared mesh, the buffers as an engine would upload them: every vertex
//   once, and the restart index buffer or the triangle list into it;
// - the strip mesh, a vertex per index of the restart buffer, so that a
//   vertex can tell its strip and its place in the strip (strips share
//   vertices, so a shared vertex could not): for the colors by strip, the
//   wireframe and the intro. Its indices are the restart buffer's positions,
//   so both meshes draw the same triangles in the same order;
// - the borders, every ring as a closed line strip into the shared vertices.

import { RESTART_INDEX, type BatchResult, type Status } from "@/wasm/earstrip";

import type { World } from "./world";

export interface CountryStats {
  polygons: number;
  triangles: number;
  strips: number;
  /** The indices of its strips, with the restart indices between them. */
  indices: number;
  /** The statuses of its polygons, how many of each. */
  statuses: Partial<Record<Status, number>>;
}

export interface Geometry {
  /** x, y per shared vertex: the result's vertices. */
  sharedPositions: Float32Array;
  /** Country, MAPCOLOR7 per shared vertex. */
  sharedInfo: Float32Array;
  /** x, y per position of the restart buffer (0, 0 at a restart index). */
  stripPositions: Float32Array;
  stripInfo: Float32Array;
  /** Strip number, place in the strip per position of the restart buffer. */
  stripPlace: Float32Array;
  /** 0, 1, 2, … with RESTART_INDEX where the restart buffer has one. */
  stripIndices: Uint32Array;
  /** The triangle list into the strip mesh, the same triangles as list. */
  stripList: Uint32Array;
  /** Every ring closed, RESTART_INDEX between rings, into the shared vertices. */
  borders: Uint32Array;
  /** What each country's polygons gave. */
  countries: CountryStats[];
  strips: number;
}

/** Where polygon p's triangles are in result.list, three indices each. */
export function listRange(r: BatchResult, p: number): [number, number] {
  const s0 = r.polygonStrips[p];
  const s1 = r.polygonStrips[p + 1];
  const from = s0 > 0 ? r.stripListEnds[s0 - 1] : 0;
  return [from, s1 > s0 ? r.stripListEnds[s1 - 1] : from];
}

/** Where polygon p's strips are in result.restart. */
export function restartRange(r: BatchResult, p: number): [number, number] {
  const s0 = r.polygonStrips[p];
  const s1 = r.polygonStrips[p + 1];
  if (s1 === s0) return [0, 0];
  const from = s0 > 0 ? r.stripRestartEnds[s0 - 1] + 1 : 0;
  return [from, r.stripRestartEnds[s1 - 1]];
}

export function buildGeometry(world: World, r: BatchResult): Geometry {
  const { batch, polygonCountry, countries } = world;
  const polygons = polygonCountry.length;
  const restart = r.restart;
  const shared = r.vertices.length / 2;

  // The country of every shared vertex: of the input vertices by their
  // rings, of the extra points by the strips that use them.
  const sharedInfo = new Float32Array(2 * shared);
  const color = (c: number) => countries[c].color;
  for (let p = 0; p < polygons; p++) {
    const c = polygonCountry[p];
    const ring0 = batch.polygonStarts[p];
    const ringEnd =
      p + 1 < polygons ? batch.polygonStarts[p + 1] : batch.ringStarts.length;
    const first = batch.ringStarts[ring0];
    const end =
      ringEnd < batch.ringStarts.length
        ? batch.ringStarts[ringEnd]
        : batch.coords.length / 2;
    for (let i = first; i < end; i++) {
      sharedInfo[2 * i] = c;
      sharedInfo[2 * i + 1] = color(c);
    }
    const [from, to] = restartRange(r, p);
    for (let k = from; k < to; k++) {
      const v = restart[k];
      if (v === RESTART_INDEX) continue;
      sharedInfo[2 * v] = c;
      sharedInfo[2 * v + 1] = color(c);
    }
  }

  // The strip mesh: position k of the restart buffer is vertex k.
  const n = restart.length;
  const stripPositions = new Float32Array(2 * n);
  const stripInfo = new Float32Array(2 * n);
  const stripPlace = new Float32Array(2 * n);
  const stripIndices = new Uint32Array(n);
  const stripList = new Uint32Array(r.list.length);
  const vertices = r.vertices;
  let strip = 0;
  let place = 0;
  let begin = 0;
  let listAt = 0;
  // A strip from begin to end (exclusive) in the restart buffer: its
  // triangles as a list, skipping the degenerate first one of a strip that
  // starts clockwise (its first index twice), as Triangles does.
  const listStrip = (end: number) => {
    let k = begin;
    if (end - begin >= 3 && restart[begin] === restart[begin + 1]) k++;
    for (; k + 2 < end; k++) {
      stripList[listAt++] = k;
      stripList[listAt++] = k + 1;
      stripList[listAt++] = k + 2;
    }
  };
  for (let k = 0; k < n; k++) {
    const v = restart[k];
    if (v === RESTART_INDEX) {
      stripIndices[k] = RESTART_INDEX;
      listStrip(k);
      strip++;
      place = 0;
      begin = k + 1;
      continue;
    }
    stripIndices[k] = k;
    stripPositions[2 * k] = vertices[2 * v];
    stripPositions[2 * k + 1] = vertices[2 * v + 1];
    stripInfo[2 * k] = sharedInfo[2 * v];
    stripInfo[2 * k + 1] = sharedInfo[2 * v + 1];
    stripPlace[2 * k] = strip;
    stripPlace[2 * k + 1] = place++;
  }
  if (n > 0) listStrip(n);
  // Every restart index ends a strip, between polygons as well, so strip
  // counted them all.
  const strips = r.stripListEnds.length;
  if (listAt !== stripList.length) {
    throw new Error(
      `strip mesh: ${listAt} list indices, want ${stripList.length}`,
    );
  }

  // The borders: every ring of the input as a closed line strip.
  const rings = batch.ringStarts.length;
  const vertexCount = batch.coords.length / 2;
  const borders = new Uint32Array(vertexCount + 2 * rings);
  let b = 0;
  for (let ring = 0; ring < rings; ring++) {
    const first = batch.ringStarts[ring];
    const end = ring + 1 < rings ? batch.ringStarts[ring + 1] : vertexCount;
    if (b > 0) borders[b++] = RESTART_INDEX;
    for (let i = first; i < end; i++) borders[b++] = i;
    borders[b++] = first;
  }

  const stats: CountryStats[] = countries.map(() => ({
    polygons: 0,
    triangles: 0,
    strips: 0,
    indices: 0,
    statuses: {},
  }));
  for (let p = 0; p < polygons; p++) {
    const s = stats[polygonCountry[p]];
    const [l0, l1] = listRange(r, p);
    const [r0, r1] = restartRange(r, p);
    const status = r.polygonStatus[p];
    s.polygons++;
    s.triangles += (l1 - l0) / 3;
    s.strips += r.polygonStrips[p + 1] - r.polygonStrips[p];
    s.indices += r1 - r0;
    s.statuses[status] = (s.statuses[status] ?? 0) + 1;
  }

  return {
    sharedPositions: vertices,
    sharedInfo,
    stripPositions,
    stripInfo,
    stripPlace,
    stripIndices,
    stripList,
    borders: borders.subarray(0, b),
    countries: stats,
    strips,
  };
}

/**
 * The country under the point (x, y on the grid), by the triangles drawn
 * there, or -1: a test of the polygons' bounds, then of their triangles.
 */
export function countryAt(
  world: World,
  r: BatchResult,
  x: number,
  y: number,
): number {
  const { bounds, polygonCountry } = world;
  const v = r.vertices;
  const list = r.list;
  for (let p = 0; p < polygonCountry.length; p++) {
    if (
      x < bounds[4 * p] ||
      y < bounds[4 * p + 1] ||
      x > bounds[4 * p + 2] ||
      y > bounds[4 * p + 3]
    ) {
      continue;
    }
    const [from, to] = listRange(r, p);
    for (let t = from; t < to; t += 3) {
      const ax = v[2 * list[t]];
      const ay = v[2 * list[t] + 1];
      const bx = v[2 * list[t + 1]];
      const by = v[2 * list[t + 1] + 1];
      const cx = v[2 * list[t + 2]];
      const cy = v[2 * list[t + 2] + 1];
      // Counter-clockwise triangles: inside is left of all three edges.
      const d1 = (bx - ax) * (y - ay) - (by - ay) * (x - ax);
      const d2 = (cx - bx) * (y - by) - (cy - by) * (x - bx);
      const d3 = (ax - cx) * (y - cy) - (ay - cy) * (x - cx);
      if (d1 >= 0 && d2 >= 0 && d3 >= 0) return polygonCountry[p];
    }
  }
  return -1;
}
