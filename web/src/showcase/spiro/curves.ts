// The curves of the Spirograph showcase and the winding numbers of their
// triangles.
//
// Every family draws a closed curve from two whole numbers p and q and a pen in
// [0, 1]. A pen near 0 gives the family's simplest shape (a circle, a convex
// polygon, a wavy ring), and a pen near 1 crosses itself tens to thousands of
// times, so an animated pen moves the triangulator between Exact and
// Constrained. The curves are scaled into the unit disk.

/** A whole-number parameter of a family, as its slider shows it. */
export interface Knob {
  label: string;
  min: number;
  max: number;
  value: number;
}

export interface Family {
  id: string;
  name: string;
  /** One sentence for the controls: what p, q and the pen do. */
  about: string;
  p: Knob;
  q: Knob;
  /** The largest q that makes sense with p, if it is less than q.max. */
  qLimit?: (p: number) => number;
  /** The pen the animation starts from, where the curve is simplest. */
  penFloor: number;
  /** The pen the animation reaches, before the pen slider scales it. */
  penCeil: number;
  /**
   * The number of vertices for count, which a family may round to fit its
   * shape (a star needs a multiple of its corners).
   */
  vertices(count: number, p: number, q: number): number;
  /**
   * Writes the curve's vertices into out, x then y each, scaled into the unit
   * disk. phase turns a part of the curve against the rest, so that the shape
   * moves while p, q and the pen stay.
   */
  sample(
    out: Float64Array,
    count: number,
    p: number,
    q: number,
    pen: number,
    phase: number,
  ): void;
}

function gcd(a: number, b: number): number {
  while (b !== 0) [a, b] = [b, a % b];
  return a;
}

const TAU = 2 * Math.PI;

const asIs = (count: number) => count;

/**
 * The trochoids: a wheel with q teeth rolls inside (sign -1) or outside
 * (sign +1) a ring with p teeth, and the pen sits pen × 1.5 wheel radii from
 * the wheel's center. The curve closes after q / gcd(p, q) turns.
 */
function trochoid(
  out: Float64Array,
  count: number,
  ring: number,
  wheel: number,
  pen: number,
  phase: number,
  sign: number,
): void {
  const g = gcd(ring, wheel);
  const period = (TAU * wheel) / g;
  const center = ring + sign * wheel;
  const d = pen * wheel * 1.5;
  const k = center / wheel;
  const norm = 1 / (Math.abs(center) + d);
  for (let i = 0; i < count; i++) {
    const t = (period * i) / count;
    const a = k * t + phase;
    out[2 * i] = (center * Math.cos(t) - sign * d * Math.cos(a)) * norm;
    out[2 * i + 1] = (center * Math.sin(t) - d * Math.sin(a)) * norm;
  }
}

export const FAMILIES: Family[] = [
  {
    id: "hypo",
    name: "Hypotrochoid",
    about:
      "The classic Spirograph: a wheel rolls inside a ring. Unless the wheel's teeth divide the ring's, the pen circles several times and the loops cross each other everywhere.",
    p: { label: "Ring teeth", min: 3, max: 30, value: 17 },
    q: { label: "Wheel teeth", min: 1, max: 29, value: 7 },
    qLimit: (p) => p - 1,
    penFloor: 0.2,
    penCeil: 1,
    vertices: asIs,
    sample(out, count, p, q, pen, phase) {
      trochoid(out, count, p, Math.min(q, p - 1), pen, phase, -1);
    },
  },
  {
    id: "epi",
    name: "Epitrochoid",
    about:
      "A wheel rolls around the outside of a ring. With one wheel tooth a short pen draws a simple wavy ring, and a long one throws a loop per tooth.",
    p: { label: "Ring teeth", min: 2, max: 24, value: 7 },
    q: { label: "Wheel teeth", min: 1, max: 12, value: 1 },
    penFloor: 0.05,
    penCeil: 1,
    vertices: asIs,
    sample(out, count, p, q, pen, phase) {
      trochoid(out, count, p, q, pen, phase, 1);
    },
  },
  {
    id: "rose",
    name: "Rose",
    about:
      "The rose r = a + cos(p/q · θ). The pen shrinks a from 1.3 toward 0: the petals grow inner loops and finally all but meet in the middle.",
    p: { label: "Petals p", min: 1, max: 24, value: 7 },
    q: { label: "Turns q", min: 1, max: 12, value: 3 },
    penFloor: 0,
    penCeil: 0.95,
    vertices: asIs,
    sample(out, count, p, q, pen, phase) {
      const g = gcd(p, q);
      const k = p / q;
      const period = (TAU * q) / g;
      const a = 1.3 * (1 - pen);
      const norm = 1 / (a + 1);
      for (let i = 0; i < count; i++) {
        const t = (period * i) / count;
        const r = (a + Math.cos(k * t + phase)) * norm;
        out[2 * i] = r * Math.cos(t);
        out[2 * i + 1] = r * Math.sin(t);
      }
    },
  },
  {
    id: "star",
    name: "Star polygon",
    about:
      "The star {p/q}: p corners, every q-th joined. The pen morphs it out of the convex p-gon, and the edges bow in and out as it turns.",
    p: { label: "Corners p", min: 5, max: 72, value: 31 },
    q: { label: "Step q", min: 2, max: 35, value: 12 },
    qLimit: (p) => Math.floor((p - 1) / 2),
    penFloor: 0,
    penCeil: 1,
    vertices(count, p, q) {
      const m = p / gcd(p, Math.min(q, Math.floor((p - 1) / 2)));
      return m * Math.max(1, Math.floor(count / m));
    },
    sample(out, count, p, q, pen, phase) {
      const step = Math.min(q, Math.floor((p - 1) / 2));
      const g = gcd(p, step);
      const m = p / g;
      const k = step / g;
      const sub = count / m;
      const bulge = 0.22 * Math.sin(phase);
      // The corners: the convex m-gon blended into the star, which turns.
      const corner = (i: number, xy: 0 | 1) => {
        const t = (TAU * i) / m;
        const s = k * t + phase / k;
        return xy === 0
          ? (1 - pen) * Math.cos(t) + pen * Math.cos(s)
          : (1 - pen) * Math.sin(t) + pen * Math.sin(s);
      };
      let v = 0;
      for (let i = 0; i < m; i++) {
        const x0 = corner(i, 0);
        const y0 = corner(i, 1);
        const x1 = corner(i + 1, 0);
        const y1 = corner(i + 1, 1);
        // Each edge bows along its normal by a sine arc.
        for (let j = 0; j < sub; j++) {
          const u = j / sub;
          const b = bulge * Math.sin(Math.PI * u);
          out[2 * v] = x0 + u * (x1 - x0) - b * (y1 - y0);
          out[2 * v + 1] = y0 + u * (y1 - y0) + b * (x1 - x0);
          v++;
        }
      }
    },
  },
  {
    id: "lissajous",
    name: "Lissajous",
    about:
      "A circle blended into the Lissajous figure x = sin(p·t + φ), y = sin(q·t): the pen sets the blend, and the phase φ keeps the crossings moving.",
    p: { label: "Frequency p", min: 1, max: 15, value: 5 },
    q: { label: "Frequency q", min: 1, max: 15, value: 4 },
    penFloor: 0,
    penCeil: 1,
    vertices: asIs,
    sample(out, count, p, q, pen, phase) {
      const g = gcd(p, q);
      const a = p / g;
      const b = q / g;
      for (let i = 0; i < count; i++) {
        const t = (TAU * i) / count;
        out[2 * i] = (1 - pen) * Math.cos(t) + pen * Math.sin(a * t + phase);
        out[2 * i + 1] = (1 - pen) * Math.sin(t) + pen * Math.sin(b * t);
      }
    },
  },
  {
    id: "farris",
    name: "Farris wheel",
    about:
      "Three circles turning at 1, 1 + p and 1 − q·p times the speed (Frank Farris's mystery curves): always p-fold symmetric, and the pen adds the two small wheels.",
    p: { label: "Symmetry p", min: 2, max: 13, value: 7 },
    q: { label: "Harmonic q", min: 1, max: 8, value: 4 },
    penFloor: 0,
    penCeil: 1,
    vertices: asIs,
    sample(out, count, p, q, pen, phase) {
      const f2 = 1 + p;
      const f3 = 1 - q * p;
      const a = 0.55 * pen;
      const b = 0.42 * pen;
      const norm = 1 / (1 + a + b);
      for (let i = 0; i < count; i++) {
        const t = (TAU * i) / count;
        const s = f3 * t + phase;
        out[2 * i] =
          (Math.cos(t) + a * Math.cos(f2 * t) + b * Math.cos(s)) * norm;
        out[2 * i + 1] =
          (Math.sin(t) + a * Math.sin(f2 * t) + b * Math.sin(s)) * norm;
      }
    },
  },
];

/**
 * The winding number of points against a closed curve, by Dan Sunday's
 * crossing rule over the edges of one horizontal band: the edges are filed by
 * the bands their y range touches, so a query looks at a few dozen edges
 * instead of all of them. The arrays are reused from one build to the next.
 */
export class Winding {
  #coords: Float64Array<ArrayBufferLike> = new Float64Array(0);
  #n = 0;
  #starts = new Int32Array(0);
  #edges = new Int32Array(0);
  #bands = 1;
  #y0 = 0;
  #perBand = 1;

  /** Files the n edges of the closed curve through coords' first n points. */
  build(coords: Float64Array, n: number): void {
    this.#coords = coords;
    this.#n = n;
    let lo = Infinity;
    let hi = -Infinity;
    for (let i = 0; i < n; i++) {
      const y = coords[2 * i + 1];
      if (y < lo) lo = y;
      if (y > hi) hi = y;
    }
    const bands = Math.max(8, Math.min(512, Math.round(Math.sqrt(n) * 4)));
    this.#bands = bands;
    this.#y0 = lo;
    this.#perBand = hi > lo ? bands / (hi - lo) : 0;
    if (this.#starts.length < bands + 1)
      this.#starts = new Int32Array(bands + 1);
    const starts = this.#starts;
    starts.fill(0, 0, bands + 1);
    // Count, then sum up, then file: the edges of band b end up at
    // edges[starts[b] .. starts[b + 1]).
    for (let i = 0; i < n; i++) {
      const j = i + 1 === n ? 0 : i + 1;
      const ya = coords[2 * i + 1];
      const yb = coords[2 * j + 1];
      const b1 = this.#band(ya < yb ? yb : ya);
      for (let b = this.#band(ya < yb ? ya : yb); b <= b1; b++) starts[b + 1]++;
    }
    for (let b = 0; b < bands; b++) starts[b + 1] += starts[b];
    const total = starts[bands];
    if (this.#edges.length < total) {
      this.#edges = new Int32Array(Math.ceil(total * 1.5));
    }
    const edges = this.#edges;
    for (let i = 0; i < n; i++) {
      const j = i + 1 === n ? 0 : i + 1;
      const ya = coords[2 * i + 1];
      const yb = coords[2 * j + 1];
      const b1 = this.#band(ya < yb ? yb : ya);
      for (let b = this.#band(ya < yb ? ya : yb); b <= b1; b++) {
        edges[starts[b]++] = i;
      }
    }
    // Filing moved every start to the next band's; shift them back.
    for (let b = bands; b > 0; b--) starts[b] = starts[b - 1];
    starts[0] = 0;
  }

  #band(y: number): number {
    const b = Math.floor((y - this.#y0) * this.#perBand);
    return b < 0 ? 0 : b >= this.#bands ? this.#bands - 1 : b;
  }

  /** The winding number of (x, y): counter-clockwise turns count +1. */
  at(x: number, y: number): number {
    const c = this.#coords;
    const n = this.#n;
    const edges = this.#edges;
    const b = this.#band(y);
    const end = this.#starts[b + 1];
    let w = 0;
    for (let k = this.#starts[b]; k < end; k++) {
      const i = edges[k];
      const j = i + 1 === n ? 0 : i + 1;
      const x0 = c[2 * i];
      const y0 = c[2 * i + 1];
      const x1 = c[2 * j];
      const y1 = c[2 * j + 1];
      const left = (x1 - x0) * (y - y0) - (x - x0) * (y1 - y0);
      if (y0 <= y) {
        if (y1 > y && left > 0) w++;
      } else if (y1 <= y && left < 0) {
        w--;
      }
    }
    return w;
  }
}
