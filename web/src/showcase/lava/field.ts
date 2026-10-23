// The wax of the lava lamp: metaballs that rise and sink, and a pool of wax at
// the bottom and a thinner one at the top that they leave and join. The field
// is sampled on a grid in cell units, which is what the marching squares read.

/** The most blobs the count slider offers. */
export const MAX_BLOBS = 12;

/** How a blob moves; all lengths are in units of the stage's height. */
interface Blob {
  /** Its column, as a fraction of the width. */
  x: number;
  sway: number;
  swaySpeed: number;
  swayPhase: number;
  riseSpeed: number;
  risePhase: number;
  radius: number;
  pulsePhase: number;
}

/** mulberry32: a small seeded generator, so that every load moves the same. */
function random(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/**
 * The blobs, fixed once: the count slider takes the first n, so that a blob
 * keeps its path when others come or go. The columns are spread over the
 * width by the golden ratio, so that any prefix covers it evenly.
 */
const BLOBS: Blob[] = (() => {
  const r = random(7);
  const blobs: Blob[] = [];
  for (let i = 0; i < MAX_BLOBS; i++) {
    blobs.push({
      x: 0.1 + 0.8 * ((0.37 + i * 0.618034) % 1),
      sway: 0.02 + 0.05 * r(),
      swaySpeed: 0.15 + 0.25 * r(),
      swayPhase: 2 * Math.PI * r(),
      riseSpeed: 0.12 + 0.16 * r(),
      risePhase: 2 * Math.PI * r(),
      radius: 0.065 + 0.05 * r(),
      pulsePhase: 2 * Math.PI * r(),
    });
  }
  return blobs;
})();

/** The pool at the bottom: (radius / (height + offset))³, like a blob's. */
const POOL_RADIUS = 0.13;
const POOL_OFFSET = 0.05;
/** The thinner pool under the cap at the top. */
const CAP_RADIUS = 0.035;
const CAP_OFFSET = 0.04;

/**
 * The field of the blobs and the pools on a grid. The stage is cols × rows
 * square cells; the grid reaches pad cells past it on every side, and its
 * outermost samples are 0, below every iso level, so that every contour
 * closes, and closes out of sight (the pool runs into the sides).
 *
 * A blob adds (r / d)³ at distance d, not the classic (r / d)²: the tails
 * fall off faster, so that blobs merge when they come close, not into one
 * mass when there are many.
 */
export class Field {
  readonly pad = 3;
  cols = 0;
  rows = 0;
  /** Samples per row of the grid: cols + 2 pad + 1. */
  nx = 0;
  /** Rows of samples: rows + 2 pad + 1. */
  ny = 0;
  /** The field at sample (i, j), at index j nx + i; x = i - pad, y = j - pad. */
  values = new Float32Array(0);
  /** The blobs of the latest sample: u, v and radius² each, in units of the height. */
  readonly #blobs = new Float64Array(3 * (MAX_BLOBS + 1));

  /** Sets the stage's size in cells; the samples are kept when it is the same. */
  resize(cols: number, rows: number): void {
    if (cols === this.cols && rows === this.rows) return;
    this.cols = cols;
    this.rows = rows;
    this.nx = cols + 2 * this.pad + 1;
    this.ny = rows + 2 * this.pad + 1;
    this.values = new Float32Array(this.nx * this.ny);
  }

  /**
   * Samples the field of the first count blobs at time t (seconds of lamp
   * time). pointer, in cells from the stage's bottom left, adds a blob of
   * heat where the user holds it.
   */
  sample(t: number, count: number, pointer?: { x: number; y: number }): void {
    const { nx, ny, pad, rows, values } = this;
    const aspect = this.cols / rows;
    const b = this.#blobs;
    let n = 0;
    for (let k = 0; k < count; k++) {
      const blob = BLOBS[k];
      // Half a triangle wave, half a sine: a blob crosses the middle at an
      // even pace, like wax that has let go of the pool, and slows down
      // where it turns, near the pools.
      const s = Math.sin(blob.riseSpeed * t + blob.risePhase);
      const rise = 0.5 * s + (1 / Math.PI) * Math.asin(s);
      const radius =
        blob.radius * (1 + 0.08 * Math.sin(0.7 * t + blob.pulsePhase));
      b[3 * n] =
        aspect *
        (blob.x + blob.sway * Math.sin(blob.swaySpeed * t + blob.swayPhase));
      b[3 * n + 1] = 0.5 + 0.37 * rise;
      b[3 * n + 2] = radius * radius;
      n++;
    }
    if (pointer !== undefined) {
      b[3 * n] = pointer.x / rows;
      b[3 * n + 1] = pointer.y / rows;
      b[3 * n + 2] = 0.1 * 0.1;
      n++;
    }

    const inv = 1 / rows;
    for (let j = 0; j < ny; j++) {
      const row = j * nx;
      if (j === 0 || j === ny - 1) {
        values.fill(0, row, row + nx);
        continue;
      }
      const v = (j - pad) * inv;
      const below = Math.max(v + POOL_OFFSET, 0.005);
      const above = Math.max(1 - v + CAP_OFFSET, 0.005);
      const pool = (POOL_RADIUS * POOL_RADIUS) / (below * below);
      const cap = (CAP_RADIUS * CAP_RADIUS) / (above * above);
      const pools = pool * Math.sqrt(pool) + cap * Math.sqrt(cap);
      values[row] = 0;
      values[row + nx - 1] = 0;
      for (let i = 1; i < nx - 1; i++) {
        const u = (i - pad) * inv;
        let f = pools;
        for (let k = 0; k < n; k++) {
          const du = u - b[3 * k];
          const dv = v - b[3 * k + 1];
          const q = b[3 * k + 2] / (du * du + dv * dv + 1e-6);
          f += q * Math.sqrt(q);
        }
        values[row + i] = f;
      }
    }
  }
}
