// The procedural landscape of the crater sandbox: a seeded height field (hills
// and one tall mountain, higher at the edges so that balls roll inwards) and
// the caves under it, as closed rings. The world is y down, in world units.

export const WORLD_W = 1280;
export const WORLD_H = 720;
export const COLS = 8;
export const ROWS = 4;
export const TILE_W = WORLD_W / COLS;
export const TILE_H = WORLD_H / ROWS;

/** The distance between two samples of the height field. */
export const HEIGHT_STEP = 2;
export const HEIGHT_SAMPLES = WORLD_W / HEIGHT_STEP + 1;

/** A closed ring with its bounds, as x, y pairs. */
export interface Ring {
  coords: Float64Array;
  minX: number;
  minY: number;
  maxX: number;
  maxY: number;
}

export interface Terrain {
  seed: number;
  /** The y of the surface every HEIGHT_STEP from x = 0. */
  heights: Float32Array;
  /** The caves: holes in the ground, which may break through the surface. */
  caves: Ring[];
}

/** mulberry32: a small seeded generator of numbers in [0, 1). */
export function random(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** The y of the surface at x, interpolated linearly between the samples. */
export function surfaceY(heights: Float32Array, x: number): number {
  const f = Math.min(Math.max(x / HEIGHT_STEP, 0), HEIGHT_SAMPLES - 1);
  const i = Math.min(Math.floor(f), HEIGHT_SAMPLES - 2);
  const t = f - i;
  return heights[i] * (1 - t) + heights[i + 1] * t;
}

/** The lowest and highest y of the surface over [x0, x1]. */
export function surfaceRange(
  heights: Float32Array,
  x0: number,
  x1: number,
): [number, number] {
  let lo = Math.min(surfaceY(heights, x0), surfaceY(heights, x1));
  let hi = Math.max(surfaceY(heights, x0), surfaceY(heights, x1));
  const i0 = Math.max(0, Math.ceil(x0 / HEIGHT_STEP));
  const i1 = Math.min(HEIGHT_SAMPLES - 1, Math.floor(x1 / HEIGHT_STEP));
  for (let i = i0; i <= i1; i++) {
    lo = Math.min(lo, heights[i]);
    hi = Math.max(hi, heights[i]);
  }
  return [lo, hi];
}

function ring(points: number[]): Ring {
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  for (let i = 0; i < points.length; i += 2) {
    minX = Math.min(minX, points[i]);
    maxX = Math.max(maxX, points[i]);
    minY = Math.min(minY, points[i + 1]);
    maxY = Math.max(maxY, points[i + 1]);
  }
  return { coords: new Float64Array(points), minX, minY, maxX, maxY };
}

/**
 * A worm-like tunnel from one point to another: a path that steers towards
 * the target with some wobble, widened to a closed outline with round caps.
 * The turn per step is small against the width, so the outline does not
 * cross itself.
 */
function tunnel(
  rand: () => number,
  x: number,
  y: number,
  tx: number,
  ty: number,
  width: number,
): Ring {
  const step = 12;
  const path: number[] = [];
  let heading = Math.atan2(ty - y, tx - x) + (rand() - 0.5) * 0.6;
  for (let i = 0; i < 80; i++) {
    path.push(x, y);
    if (Math.hypot(tx - x, ty - y) < step) break;
    let turn = Math.atan2(ty - y, tx - x) - heading;
    turn = Math.atan2(Math.sin(turn), Math.cos(turn));
    heading += Math.max(-0.1, Math.min(0.1, turn)) + (rand() - 0.5) * 0.1;
    x += Math.cos(heading) * step;
    y += Math.sin(heading) * step;
  }
  const n = path.length / 2;
  const phase = rand() * 6;
  const half = (i: number) => width * (1 + 0.22 * Math.sin(i * 0.45 + phase));
  const left: number[] = [];
  const right: number[] = [];
  const angle = (i: number) => {
    const a = Math.max(0, i - 1);
    const b = Math.min(n - 1, i + 1);
    return Math.atan2(path[2 * b + 1] - path[2 * a + 1], path[2 * b] - path[2 * a]);
  };
  for (let i = 0; i < n; i++) {
    const a = angle(i);
    const w = half(i);
    const nx = -Math.sin(a) * w;
    const ny = Math.cos(a) * w;
    left.push(path[2 * i] + nx, path[2 * i + 1] + ny);
    right.push(path[2 * i] - nx, path[2 * i + 1] - ny);
  }
  const out = left.slice();
  // The cap at the end, from the left side round the front to the right.
  const end = angle(n - 1);
  for (let k = 1; k < 6; k++) {
    const a = end + Math.PI / 2 - (k * Math.PI) / 6;
    const w = half(n - 1);
    out.push(path[2 * n - 2] + Math.cos(a) * w, path[2 * n - 1] + Math.sin(a) * w);
  }
  for (let i = n - 1; i >= 0; i--) out.push(right[2 * i], right[2 * i + 1]);
  // The cap at the start, from the right side round the back to the left.
  const start = angle(0);
  for (let k = 1; k < 6; k++) {
    const a = start - Math.PI / 2 - (k * Math.PI) / 6;
    const w = half(0);
    out.push(path[0] + Math.cos(a) * w, path[1] + Math.sin(a) * w);
  }
  return ring(out);
}

/** A round cave room with a wobbly wall. */
function room(rand: () => number, cx: number, cy: number, r: number): Ring {
  const a = rand() * 6;
  const b = rand() * 6;
  const out: number[] = [];
  for (let k = 0; k < 36; k++) {
    const t = (k / 36) * 2 * Math.PI;
    const rr = r * (1 + 0.16 * Math.sin(3 * t + a) + 0.08 * Math.sin(5 * t + b));
    out.push(cx + Math.cos(t) * rr, cy + Math.sin(t) * rr * 0.72);
  }
  return ring(out);
}

export function makeTerrain(seed: number): Terrain {
  const rand = random(seed);
  const waves = [1, 2, 3.7, 7.3, 15.1].map((f, i) => ({
    f,
    amp: [62, 40, 20, 8, 3][i] * (0.6 + 0.7 * rand()),
    phase: rand() * 2 * Math.PI,
  }));
  const hillX = WORLD_W * (0.28 + 0.44 * rand());
  const hillW = 95 + 40 * rand();
  const hillH = 150 + 50 * rand();

  const heights = new Float32Array(HEIGHT_SAMPLES);
  for (let i = 0; i < HEIGHT_SAMPLES; i++) {
    const x = i * HEIGHT_STEP;
    const u = x / WORLD_W;
    let y = 440;
    for (const w of waves) y += w.amp * Math.sin(2 * Math.PI * w.f * u + w.phase);
    y -= hillH * Math.exp(-(((x - hillX) / hillW) ** 2));
    // Raised edges, so that balls roll towards the middle.
    const edge = Math.min(u, 1 - u);
    y -= 90 * Math.max(0, 1 - edge / 0.12) ** 2;
    heights[i] = Math.min(Math.max(y, 120), 620);
  }

  const caves: Ring[] = [];
  // A tunnel that opens on the mountain's flank and runs down and away from
  // it into a long gallery: balls that roll in come out in the deep.
  const side = rand() < 0.5 ? -1 : 1;
  const sx = hillX + side * hillW * 0.95;
  const sy = surfaceY(heights, sx) - 4;
  const tx = Math.min(Math.max(sx + side * 200, 80), WORLD_W - 80);
  const ty = Math.min(surfaceY(heights, tx) + 140, WORLD_H - 80);
  caves.push(tunnel(rand, sx, sy, tx, ty, 17));
  const ux = Math.min(Math.max(tx + side * 300, 70), WORLD_W - 70);
  const uy = Math.min(
    Math.max(ty + 30, surfaceY(heights, ux) + 70),
    WORLD_H - 50,
  );
  caves.push(tunnel(rand, tx, ty, ux, uy, 20));
  // An overhang: a short cut into the other flank, just under the surface.
  const ox = hillX - side * hillW * 1.25;
  const oy = surfaceY(heights, ox) + 10;
  caves.push(tunnel(rand, ox, oy, hillX - side * hillW * 0.45, oy + 6, 15));
  // A cave room deep under the mountain.
  caves.push(
    room(rand, hillX + side * 30, Math.min(surfaceY(heights, hillX) + 220, WORLD_H - 90), 52),
  );
  return { seed, heights, caves };
}
