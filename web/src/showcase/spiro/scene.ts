// The WebGL side of the Spirograph showcase: one triangulation per frame, drawn
// as many times as the kaleidoscope has mirrors. Each copy is the same mesh
// under another rotation (and reflection) uniform, so the triangulator runs
// once however many copies there are.
//
// Every triangle is colored by the winding number of the curve around it. The
// strips have one vertex per index of Result.restart, and a strip's triangle
// takes its flat attributes from its last vertex (WebGL's provoking vertex), so
// writing triangle k's winding to the strip's vertex k + 2 colors each triangle
// on its own.

import { createProgram, Mesh, pixelRatio, StripBuilder } from "@/lib/gl";
import { RESTART_INDEX, type Result } from "@/wasm/earstrip";

import { Winding } from "./curves";

/**
 * Where a vertex lands: the curve turned and scaled and moved out, which puts
 * it in the first wedge (vWedge), then mirrored and turned into its copy.
 */
const PLACE = `
uniform vec2 view;
uniform vec2 spin;
uniform float zoom;
uniform vec2 offset;
uniform vec3 copy;
out vec2 vWedge;
vec2 turn(vec2 p, vec2 cs) {
  return vec2(cs.x * p.x - cs.y * p.y, cs.y * p.x + cs.x * p.y);
}
vec4 place(vec2 p) {
  vec2 q = turn(p, spin) * zoom + offset;
  vWedge = q;
  q.y *= copy.z;
  return vec4(turn(q, copy.xy) * view, 0.0, 1.0);
}`;

/**
 * Whether a fragment lies outside its copy's wedge, between the angles 0 and
 * wedge, or outside the unit disk. A wedge of a full turn or more clips
 * nothing: the copies then lie over each other.
 */
const CLIP = `
uniform float wedge;
bool clipped(vec2 q) {
  if (wedge >= 6.28) return false;
  float a = atan(q.y, q.x);
  if (a < 0.0) a += 6.2831853;
  return a > wedge || dot(q, q) > 1.0;
}`;

const FILL_VERTEX = `#version 300 es
layout(location = 0) in vec2 position;
layout(location = 1) in float strip;
layout(location = 2) in float wind;
${PLACE}
flat out float vStrip;
flat out float vWind;
out float vRadius;
void main() {
  vStrip = strip;
  vWind = wind;
  vRadius = length(position);
  gl_Position = place(position);
}`;

const FILL_FRAGMENT = `#version 300 es
precision highp float;
flat in float vStrip;
flat in float vWind;
in float vRadius;
in vec2 vWedge;
${CLIP}
uniform int mode;
uniform float dark;
uniform float depth;
uniform float alpha;
uniform float hue;
uniform vec4 wire;
out vec4 color;

vec3 palette(float t) {
  return 0.5 + 0.5 * cos(6.28318 * (t + vec3(0.0, 0.15, 0.3)));
}

/** The four colors a, b, c, d spread over t in [0, 1]. */
vec3 ramp(vec3 a, vec3 b, vec3 c, vec3 d, float t) {
  float u = 3.0 * t;
  return u < 1.0 ? mix(a, b, u) : u < 2.0 ? mix(b, c, u - 1.0) : mix(c, d, u - 2.0);
}

/** c turned by the angle turns × 2π around the gray axis. */
vec3 turnHue(vec3 c, float turns) {
  vec3 k = vec3(0.57735);
  float a = 6.28318 * turns;
  return c * cos(a) + cross(k, c) * sin(a) + k * dot(k, c) * (1.0 - cos(a));
}

void main() {
  if (clipped(vWedge)) discard;
  vec3 c;
  float a = alpha;
  if (mode == 2) {
    c = wire.rgb;
    a = wire.a;
  } else if (mode == 1) {
    c = palette(fract(vStrip * 0.618034 + hue));
  } else {
    // The attribute holds the winding number plus a facet in [0, 0.4).
    float w = floor(vWind + 0.5);
    float facet = (vWind - w) * 5.0 - 1.0;
    if (w == 0.0) {
      // Covered although the curve does not wind around it: what forced
      // cuts fill outside the curve. Hatched, so that it reads as wrong.
      float s = step(0.5, fract((gl_FragCoord.x - gl_FragCoord.y) / 14.0));
      c = mix(vec3(0.95, 0.15, 0.25), vec3(1.0, 0.8, 0.1), s);
      a = 0.9;
    } else {
      // On dark, counter-clockwise windings run from indigo through
      // magenta and coral to gold, clockwise ones from deep blue through
      // teal to mint. On light, which multiplies, they run from peach and
      // sky to deep indigo and ink blue. Either way, the more often the curve
      // winds (up to six times), the stronger the color. The facet shades
      // each triangle a little by the direction of its first edge, like a
      // cut stone.
      float t = clamp((abs(w) - 1.0) / max(min(depth, 6.0) - 1.0, 1.0), 0.0, 1.0);
      if (dark > 0.5) {
        c = w > 0.0
          ? ramp(vec3(0.3, 0.22, 0.85), vec3(0.78, 0.22, 0.7),
              vec3(1.0, 0.45, 0.35), vec3(1.0, 0.86, 0.45), t)
          : ramp(vec3(0.06, 0.32, 0.78), vec3(0.05, 0.65, 0.75),
              vec3(0.35, 0.88, 0.72), vec3(0.88, 1.0, 0.82), t);
      } else {
        c = w > 0.0
          ? ramp(vec3(1.0, 0.62, 0.42), vec3(0.93, 0.3, 0.5),
              vec3(0.55, 0.2, 0.72), vec3(0.2, 0.12, 0.5), t)
          : ramp(vec3(0.35, 0.75, 0.95), vec3(0.15, 0.6, 0.65),
              vec3(0.12, 0.42, 0.62), vec3(0.06, 0.2, 0.42), t);
      }
      c = clamp(turnHue(c, hue + 0.2 * vRadius - 0.1), 0.0, 1.0);
      c *= 1.0 + 0.06 * facet;
      if (dark > 0.5) {
        a *= 0.6 + 0.4 * t;
      } else {
        a *= 0.55 + 0.35 * t;
      }
    }
  }
  // Dark adds light (blend one, one); light multiplies like tinted glass
  // (blend dst color, zero).
  color = dark > 0.5 ? vec4(c * a, a) : vec4(mix(vec3(1.0), c, a), 1.0);
}`;

/** A full-screen triangle with a radial gradient, under everything. */
const BACK_VERTEX = `#version 300 es
uniform vec2 aspect;
out vec2 vAt;
void main() {
  vec2 p = vec2(float((gl_VertexID & 1) << 2) - 1.0, float((gl_VertexID & 2) << 1) - 1.0);
  vAt = p * aspect;
  gl_Position = vec4(p, 0.0, 1.0);
}`;

const BACK_FRAGMENT = `#version 300 es
precision highp float;
in vec2 vAt;
uniform vec3 inner;
uniform vec3 outer;
out vec4 color;
void main() {
  color = vec4(mix(inner, outer, smoothstep(0.0, 1.4, length(vAt))), 1.0);
}`;

const MARK_VERTEX = `#version 300 es
layout(location = 0) in vec2 position;
${PLACE}
uniform float size;
uniform float time;
out float vPulse;
void main() {
  // Each point pulses on its own beat, from its index.
  vPulse = 0.5 + 0.5 * sin(4.0 * time + 2.39996 * float(gl_VertexID));
  gl_PointSize = size * (0.65 + 0.7 * vPulse);
  gl_Position = place(position);
}`;

const MARK_FRAGMENT = `#version 300 es
precision highp float;
in float vPulse;
in vec2 vWedge;
${CLIP}
uniform int dots;
uniform vec4 ink;
uniform vec4 glow;
out vec4 color;
void main() {
  if (clipped(vWedge)) discard;
  if (dots == 0) {
    color = ink;
    return;
  }
  // A bright core in a soft halo.
  float r = length(gl_PointCoord - 0.5) * 2.0;
  if (r > 1.0) discard;
  float core = smoothstep(0.45, 0.25, r);
  float halo = (1.0 - r) * (1.0 - r) * (0.35 + 0.65 * vPulse);
  color = ink * core + glow * halo * (1.0 - core);
}`;

const UNIFORMS = ["view", "spin", "zoom", "offset", "copy", "wedge"] as const;
type Place = Record<(typeof UNIFORMS)[number], WebGLUniformLocation | null>;

function locate<K extends string>(
  gl: WebGL2RenderingContext,
  program: WebGLProgram,
  names: readonly K[],
): Record<K, WebGLUniformLocation | null> {
  const out = {} as Record<K, WebGLUniformLocation | null>;
  for (const name of names) out[name] = gl.getUniformLocation(program, name);
  return out;
}

/** What the kaleidoscope draws this frame, besides the mesh. */
export interface View {
  /** Rotational copies, 1 or more. */
  copies: number;
  /** Whether every copy also comes reflected, for a dihedral pattern. */
  mirror: boolean;
  /**
   * Whether each copy shows only its wedge, as in a kaleidoscope, instead of
   * the whole curve over the others.
   */
  wedges: boolean;
  /** How far the copies sit from the center, in [0, 1). */
  spread: number;
  /** The curve's own turn, radians. */
  spin: number;
  /** The whole pattern's turn, radians. */
  turn: number;
  /** Seconds, for the pulse of the crossing points. */
  time: number;
  dark: boolean;
  colorByStrip: boolean;
  wireframe: boolean;
  outline: boolean;
  crossings: boolean;
  /** Turns the palette, in [0, 1). */
  hue: number;
}

/** The winding numbers that the latest update found. */
export interface Windings {
  min: number;
  max: number;
}

/** A Uint32Array 0, 1, 2, … reused for meshes that draw every vertex in order. */
class Iota {
  #a = new Uint32Array(0);
  get(n: number): Uint32Array {
    if (this.#a.length < n) {
      this.#a = new Uint32Array(Math.max(n, Math.ceil(this.#a.length * 1.5)));
      for (let i = 0; i < this.#a.length; i++) this.#a[i] = i;
    }
    return this.#a.subarray(0, n);
  }
}

export class SpiroScene {
  readonly #gl: WebGL2RenderingContext;
  readonly #fill: WebGLProgram;
  readonly #mark: WebGLProgram;
  readonly #back: WebGLProgram;
  readonly #backU: Record<
    "aspect" | "inner" | "outer",
    WebGLUniformLocation | null
  >;
  readonly #fillPlace: Place;
  readonly #markPlace: Place;
  readonly #fillU: Record<
    "mode" | "dark" | "depth" | "alpha" | "hue" | "wire",
    WebGLUniformLocation | null
  >;
  readonly #markU: Record<
    "size" | "time" | "dots" | "ink" | "glow",
    WebGLUniformLocation | null
  >;
  readonly #mesh: Mesh;
  readonly #outline: Mesh;
  readonly #points: Mesh;
  readonly #builder = new StripBuilder();
  readonly #winding = new Winding();
  readonly #iota = new Iota();
  #winds = new Float32Array(0);
  #line = new Float32Array(0);
  #dots = new Float32Array(0);
  #lineCount = 0;
  #dotCount = 0;
  #depth = 1;
  readonly #windings: Windings = { min: 0, max: 0 };
  // The draw calls per copy, made once so that a frame makes no closures.
  readonly #drawFill = () => this.#mesh.draw();
  readonly #drawWire = () => this.#mesh.draw(this.#gl.LINE_STRIP);
  readonly #drawOutline = () =>
    this.#outline.draw(this.#gl.LINE_LOOP, this.#lineCount);
  readonly #drawDots = () => this.#points.draw(this.#gl.POINTS, this.#dotCount);

  constructor(gl: WebGL2RenderingContext) {
    this.#gl = gl;
    this.#fill = createProgram(gl, FILL_VERTEX, FILL_FRAGMENT);
    this.#mark = createProgram(gl, MARK_VERTEX, MARK_FRAGMENT);
    this.#back = createProgram(gl, BACK_VERTEX, BACK_FRAGMENT);
    this.#backU = locate(gl, this.#back, ["aspect", "inner", "outer"]);
    this.#fillPlace = locate(gl, this.#fill, UNIFORMS);
    this.#markPlace = locate(gl, this.#mark, UNIFORMS);
    this.#fillU = locate(gl, this.#fill, [
      "mode",
      "dark",
      "depth",
      "alpha",
      "hue",
      "wire",
    ]);
    this.#markU = locate(gl, this.#mark, [
      "size",
      "time",
      "dots",
      "ink",
      "glow",
    ]);
    this.#mesh = new Mesh(gl);
    this.#outline = new Mesh(gl);
    this.#points = new Mesh(gl);
  }

  /**
   * Uploads the triangulation r of the closed curve through coords' first n
   * points, with the winding number of every triangle, and returns the range
   * of the windings (the same object every time).
   */
  update(r: Result, coords: Float64Array, n: number): Windings {
    const b = this.#builder;
    b.reset();
    b.add(r, coords);
    const positions = b.positions;
    const indices = b.indices;
    const count = positions.length / 2;
    if (this.#winds.length < count) {
      this.#winds = new Float32Array(Math.ceil(count * 1.5));
    }
    const winds = this.#winds;
    const winding = this.#winding;
    winding.build(coords, n);
    let lo = Infinity;
    let hi = -Infinity;
    // Within a strip, the triangle that ends at index j is (j-2, j-1, j).
    let run = 0;
    for (let j = 0; j < indices.length; j++) {
      const v = indices[j];
      if (v === RESTART_INDEX) {
        run = 0;
        continue;
      }
      if (run++ < 2) {
        winds[v] = 0;
        continue;
      }
      const a = indices[j - 2];
      const c = indices[j - 1];
      const ax = positions[2 * a];
      const ay = positions[2 * a + 1];
      const bx = positions[2 * c];
      const by = positions[2 * c + 1];
      const cx = positions[2 * v];
      const cy = positions[2 * v + 1];
      const w = winding.at((ax + bx + cx) / 3, (ay + by + cy) / 3);
      // The facet, cos 2θ of the first edge's direction θ, rides along in
      // [0, 0.4) above the winding number.
      const dx = bx - ax;
      const dy = by - ay;
      const len = dx * dx + dy * dy;
      const facet = len > 0 ? (dx * dx - dy * dy) / len : 0;
      winds[v] = w + 0.2 * (facet + 1) * 0.99;
      // Leave the degenerate first triangle of a clockwise strip out of the
      // range; its centroid lies on an edge.
      const area = (bx - ax) * (cy - ay) - (cx - ax) * (by - ay);
      if (area !== 0) {
        if (w < lo) lo = w;
        if (w > hi) hi = w;
      }
    }
    if (lo > hi) lo = hi = 0;
    this.#depth = Math.max(Math.abs(lo), Math.abs(hi), 1);

    const mesh = this.#mesh;
    mesh.attribute(0, positions, 2);
    mesh.attribute(1, b.strips, 1);
    mesh.attribute(2, winds.subarray(0, count), 1);
    mesh.indices(indices);

    if (this.#line.length < 2 * n) this.#line = new Float32Array(2 * n);
    this.#line.set(coords.subarray(0, 2 * n));
    this.#lineCount = n;
    this.#outline.attribute(0, this.#line.subarray(0, 2 * n), 2);
    this.#outline.indices(this.#iota.get(n));

    const extra = r.extra;
    if (this.#dots.length < extra.length) {
      this.#dots = new Float32Array(Math.ceil(extra.length * 1.5));
    }
    this.#dots.set(extra);
    this.#dotCount = extra.length / 2;
    if (this.#dotCount > 0) {
      this.#points.attribute(0, this.#dots.subarray(0, extra.length), 2);
      this.#points.indices(this.#iota.get(this.#dotCount));
    }
    this.#windings.min = lo;
    this.#windings.max = hi;
    return this.#windings;
  }

  /** Sets the uniforms every copy shares and calls draw once per copy. */
  #copies(v: View, place: Place, draw: () => void): void {
    const gl = this.#gl;
    const aspect = gl.canvas.width / gl.canvas.height;
    gl.uniform2f(
      place.view,
      Math.min(1, 1 / aspect) * 0.97,
      Math.min(1, aspect) * 0.97,
    );
    gl.uniform2f(place.spin, Math.cos(v.spin), Math.sin(v.spin));
    const layers = v.copies * (v.mirror ? 2 : 1);
    const clip = v.wedges && layers > 1;
    const wedge = clip ? (2 * Math.PI) / layers : 7;
    gl.uniform1f(place.wedge, wedge);
    // In wedges, the curve sits on the wedge's middle line and keeps its
    // size; what leaves the wedge is cut off. Over each other, a copy fills
    // the disk that is left beside its distance from the center, and with
    // mirrors it sits in the middle of its half wedge, so that it and its
    // reflection do not overlap more than they must.
    const at = clip ? wedge / 2 : v.mirror ? Math.PI / (2 * v.copies) : 0;
    gl.uniform1f(place.zoom, clip ? 1 : 1 - v.spread);
    gl.uniform2f(
      place.offset,
      v.spread * Math.cos(at),
      v.spread * Math.sin(at),
    );
    for (let i = 0; i < v.copies; i++) {
      const a = v.turn + (2 * Math.PI * i) / v.copies;
      const c = Math.cos(a);
      const s = Math.sin(a);
      gl.uniform3f(place.copy, c, s, 1);
      draw();
      if (v.mirror) {
        gl.uniform3f(place.copy, c, s, -1);
        draw();
      }
    }
  }

  draw(v: View): void {
    const gl = this.#gl;
    gl.viewport(0, 0, gl.canvas.width, gl.canvas.height);
    const aspect = gl.canvas.width / gl.canvas.height;

    gl.disable(gl.BLEND);
    gl.useProgram(this.#back);
    const k = this.#backU;
    gl.uniform2f(k.aspect, Math.max(1, aspect), Math.max(1, 1 / aspect));
    if (v.dark) {
      gl.uniform3f(k.inner, 0.11, 0.1, 0.19);
      gl.uniform3f(k.outer, 0.03, 0.03, 0.06);
    } else {
      gl.uniform3f(k.inner, 1, 1, 1);
      gl.uniform3f(k.outer, 0.9, 0.91, 0.95);
    }
    gl.drawArrays(gl.TRIANGLES, 0, 3);

    gl.enable(gl.BLEND);
    const layers = v.copies * (v.mirror ? 2 : 1);
    // Dark: the copies add up to light. Light: they multiply like layers of
    // tinted glass.
    if (v.dark) gl.blendFunc(gl.ONE, gl.ONE);
    else gl.blendFunc(gl.DST_COLOR, gl.ZERO);

    gl.useProgram(this.#fill);
    const u = this.#fillU;
    gl.uniform1f(u.dark, v.dark ? 1 : 0);
    gl.uniform1f(u.depth, this.#depth);
    gl.uniform1f(u.hue, v.hue);
    gl.uniform1i(u.mode, v.colorByStrip ? 1 : 0);
    // Copies over each other share the light; wedges do not overlap.
    const overlap = v.wedges ? 1 : layers;
    gl.uniform1f(
      u.alpha,
      v.dark
        ? Math.min(0.95, 1.4 / Math.sqrt(overlap))
        : Math.min(0.9, 1.5 / Math.sqrt(overlap)),
    );
    this.#copies(v, this.#fillPlace, this.#drawFill);

    if (v.wireframe) {
      gl.uniform1i(u.mode, 2);
      if (v.dark) gl.uniform4f(u.wire, 0.6, 0.65, 0.9, 0.4);
      else gl.uniform4f(u.wire, 0.2, 0.15, 0.4, 0.4);
      this.#copies(v, this.#fillPlace, this.#drawWire);
    }

    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
    gl.useProgram(this.#mark);
    const m = this.#markU;
    gl.uniform1f(m.time, v.time);
    if (v.outline) {
      gl.uniform1i(m.dots, 0);
      if (v.dark) gl.uniform4f(m.ink, 0.3, 0.32, 0.4, 0.4);
      else gl.uniform4f(m.ink, 0.05, 0.04, 0.12, 0.45);
      this.#copies(v, this.#markPlace, this.#drawOutline);
    }
    if (v.crossings && this.#dotCount > 0) {
      const ratio = pixelRatio();
      gl.uniform1i(m.dots, 1);
      gl.uniform1f(m.size, 7 * ratio);
      if (v.dark) {
        gl.uniform4f(m.ink, 1, 1, 1, 1);
        gl.uniform4f(m.glow, 0.55, 0.85, 1, 0.9);
      } else {
        gl.uniform4f(m.ink, 0.1, 0.05, 0.25, 1);
        gl.uniform4f(m.glow, 0.85, 0.2, 0.5, 0.6);
      }
      this.#copies(v, this.#markPlace, this.#drawDots);
    }
  }

  dispose(): void {
    const gl = this.#gl;
    this.#mesh.dispose();
    this.#outline.dispose();
    this.#points.dispose();
    gl.deleteProgram(this.#fill);
    gl.deleteProgram(this.#mark);
    gl.deleteProgram(this.#back);
  }
}
