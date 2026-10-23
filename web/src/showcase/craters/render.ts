// How the crater sandbox looks, in WebGL 2. Back to front: the sky with far
// ridges and, where ground was blasted away, the dark back wall of the earth;
// the tiles' strips as the ground, shaded as soil, grass and stones (or one
// color per strip), with charred crater rims and flashing tiles; the optional
// wireframe; the balls; the explosions' particles; and their flashes.

import { createProgram, Mesh, StripBuilder } from "@/lib/gl";
import { RESTART_INDEX } from "@/wasm/earstrip";

import {
  BALL_R,
  LIGHT_SECONDS,
  MAX_BALLS,
  MAX_LIGHTS,
  MAX_PARTICLES,
  PARTICLE_FLOATS,
  SCORCH_H,
  SCORCH_W,
  type Sim,
} from "./sim";
import {
  COLS,
  HEIGHT_SAMPLES,
  HEIGHT_STEP,
  ROWS,
  TILE_H,
  TILE_W,
  WORLD_H,
  WORLD_W,
} from "./terrain";

const f = (x: number) => x.toFixed(1);

/** The GLSL every program shares: the world's size, noise, the surface. */
const COMMON = `
precision highp float;
uniform sampler2D heights;
// x, y: where world (0, H) lands, in pixels from the bottom left; z: pixels
// per world unit; w: seconds.
uniform vec4 frame;
uniform float dark;
const float W = ${f(WORLD_W)};
const float H = ${f(WORLD_H)};
float hash(vec2 p) {
  vec3 q = fract(vec3(p.xyx) * 0.1031);
  q += dot(q, q.yzx + 33.33);
  return fract((q.x + q.y) * q.z);
}
float noise(vec2 p) {
  vec2 i = floor(p);
  vec2 f = fract(p);
  vec2 u = f * f * (3.0 - 2.0 * f);
  return mix(mix(hash(i), hash(i + vec2(1, 0)), u.x),
             mix(hash(i + vec2(0, 1)), hash(i + vec2(1, 1)), u.x), u.y);
}
float fbm(vec2 p) {
  float v = 0.0;
  float a = 0.5;
  for (int i = 0; i < 4; i++) {
    v += a * noise(p);
    p = p * 2.03 + 17.0;
    a *= 0.5;
  }
  return v;
}
// The original surface, interpolated like terrain.ts's surfaceY.
float surface(float x) {
  float s = clamp(x / ${f(HEIGHT_STEP)}, 0.0, ${f(HEIGHT_SAMPLES - 1)});
  int i = int(min(floor(s), ${f(HEIGHT_SAMPLES - 2)}));
  float t = s - float(i);
  return mix(texelFetch(heights, ivec2(i, 0), 0).r,
             texelFetch(heights, ivec2(i + 1, 0), 0).r, t);
}
vec2 worldAt(vec2 frag) {
  return vec2((frag.x - frame.x) / frame.z, H - (frag.y - frame.y) / frame.z);
}
// Soil at world point w, dep units under the original surface: banded loam
// and clay, darker in the deep, with stones lit from above.
vec3 soil(vec2 w, float dep) {
  float n = fbm(w * 0.018);
  float band = sin(w.y * 0.05 + n * 5.0 + sin(w.x * 0.004) * 2.0);
  vec3 c = mix(vec3(0.50, 0.32, 0.19), vec3(0.68, 0.47, 0.27),
               smoothstep(0.2, 0.9, band) * 0.55);
  c = mix(c, vec3(0.33, 0.23, 0.20), smoothstep(30.0, 330.0, dep));
  vec2 g = w / 11.0;
  vec2 cell = floor(g);
  vec2 q = fract(g);
  vec2 o = vec2(hash(cell + 17.1), hash(cell + 31.7)) * 0.5 + 0.25;
  float pr = 0.17 + 0.15 * hash(cell + 5.3);
  float pebble = step(0.87, hash(cell)) * smoothstep(pr, pr - 0.07, length(q - o));
  vec3 stone = mix(vec3(0.52, 0.45, 0.38), vec3(0.70, 0.62, 0.52), hash(cell + 9.0));
  c = mix(c, stone * (0.8 + 1.0 * (o.y - q.y)), pebble * 0.85);
  return c * (0.86 + 0.28 * noise(w * 0.4));
}
`;

/** A triangle that covers the screen, from gl_VertexID alone. */
const FULLSCREEN = `#version 300 es
void main() {
  vec2 p = vec2(float((gl_VertexID & 1) << 2) - 1.0, float((gl_VertexID & 2) << 1) - 1.0);
  gl_Position = vec4(p, 0.0, 1.0);
}`;

const SKY = `#version 300 es
${COMMON}
out vec4 o;
void main() {
  vec2 w = worldAt(gl_FragCoord.xy);
  float t = clamp(w.y / H, -0.6, 1.0);
  vec3 day = mix(vec3(0.33, 0.58, 0.92), vec3(0.99, 0.85, 0.68), smoothstep(-0.3, 0.8, t));
  vec3 night = mix(vec3(0.02, 0.04, 0.11), vec3(0.22, 0.14, 0.32), smoothstep(-0.3, 0.9, t));
  vec3 col = mix(day, night, dark);

  vec2 sp = vec2(W * 0.8, H * 0.16);
  float d = length(w - sp);
  vec3 sun = vec3(1.0, 0.95, 0.75) * smoothstep(30.0, 27.5, d)
           + vec3(1.0, 0.72, 0.42) * exp(-d / 120.0) * 0.5;
  float crescent = smoothstep(24.0, 22.0, d) * smoothstep(19.0, 22.5, length(w - sp - vec2(-10.0, -6.0)));
  vec3 moon = vec3(0.93, 0.93, 1.0) * crescent + vec3(0.45, 0.5, 0.95) * exp(-d / 90.0) * 0.22;
  col += mix(sun, moon, dark);

  vec2 sg = floor(gl_FragCoord.xy / 3.0);
  float sh = hash(sg);
  float star = step(0.9965, sh) * (0.55 + 0.45 * sin(frame.w * (1.0 + 3.0 * hash(sg + 1.0)) + sh * 40.0));
  col += vec3(star) * dark * smoothstep(0.75, 0.1, t);

  float cl = fbm(vec2(w.x * 0.0035 + frame.w * 0.006, w.y * 0.011));
  float cloud = smoothstep(0.5, 0.72, cl) * smoothstep(0.55, 0.0, t);
  col = mix(col, mix(vec3(1.0, 0.98, 0.96), vec3(0.30, 0.27, 0.45), dark), cloud * mix(0.8, 0.45, dark));

  float r1 = H * 0.40 + (fbm(vec2(w.x * 0.0028, 1.7)) - 0.5) * 280.0;
  float r2 = H * 0.53 + (fbm(vec2(w.x * 0.0045 + 7.0, 4.1)) - 0.5) * 220.0;
  vec3 far = mix(vec3(0.62, 0.72, 0.90), vec3(0.13, 0.12, 0.27), dark);
  vec3 near = mix(vec3(0.47, 0.60, 0.74), vec3(0.09, 0.09, 0.19), dark);
  col = mix(col, far, smoothstep(r1 - 1.0, r1 + 1.0, w.y) * 0.75);
  col = mix(col, near, smoothstep(r2 - 1.0, r2 + 1.0, w.y) * 0.9);

  // The back wall where the ground is gone, darker towards the old surface's
  // edge, so that craters and caves read as holes into the earth.
  float dep = w.y - surface(w.x);
  if (dep > 0.0) {
    vec3 wall = soil(w * 0.85 + 300.0, dep) * mix(vec3(0.58, 0.52, 0.52), vec3(0.24, 0.22, 0.32), dark);
    // Faint vertical grooves, as if dug by hand.
    wall *= 0.9 + 0.1 * noise(vec2(w.x * 0.25, w.y * 0.02));
    col = mix(col, wall, smoothstep(0.0, 1.5, dep));
  }
  o = vec4(col, 1.0);
}`;

const GROUND_VERTEX = `#version 300 es
layout(location = 0) in vec2 position;
layout(location = 1) in float strip;
layout(location = 2) in float tag;
uniform vec4 view;
// Moves the mesh, in world units: the shadow pass draws it offset.
uniform vec2 offset;
out vec2 vWorld;
flat out float vStrip;
flat out float vTag;
void main() {
  vec2 p = position + offset;
  vWorld = p;
  vStrip = strip;
  vTag = tag;
  gl_Position = vec4(p.x * view.x + view.y, p.y * view.z + view.w, 0.0, 1.0);
}`;

/**
 * The ground's shadow, cast down and right onto the back wall only (not onto
 * the sky): it makes craters and caves read as cavities.
 */
const SHADOW = `#version 300 es
${COMMON}
uniform float strength;
in vec2 vWorld;
out vec4 o;
void main() {
  if (vWorld.y - surface(vWorld.x) <= 0.0) discard;
  o = vec4(0.0, 0.0, 0.0, strength);
}`;

const GROUND = `#version 300 es
${COMMON}
uniform sampler2D scorch;
uniform float flash[${COLS * ROWS}];
uniform float stripMode;
uniform float grid;
in vec2 vWorld;
flat in float vStrip;
flat in float vTag;
out vec4 o;
void main() {
  vec2 w = vWorld;
  float dep = w.y - surface(w.x);
  vec3 c = soil(w, dep);

  float blade = noise(vec2(w.x * 0.9, 0.0));
  float gw = 7.0 + 3.0 * noise(vec2(w.x * 0.08, 3.0)) + 4.0 * blade * blade;
  float sc = texture(scorch, w / vec2(W, H)).r;
  float g = smoothstep(gw, gw - 1.5, dep) * (1.0 - smoothstep(0.15, 0.5, sc));
  vec3 grass = mix(vec3(0.50, 0.78, 0.28), vec3(0.22, 0.46, 0.15), clamp(dep / gw, 0.0, 1.0));
  c *= mix(1.0, 0.72, smoothstep(gw + 7.0, gw, dep));
  c = mix(c, grass, g);
  c = mix(c, vec3(0.13, 0.085, 0.06), sc * 0.88);
  c = mix(c, c * vec3(0.52, 0.55, 0.74), dark);

  float hue = fract(vStrip * 0.618034);
  vec3 sh = 0.5 + 0.5 * cos(6.28318 * (hue + vec3(0.0, 0.33, 0.67)));
  float l = dot(c, vec3(0.3, 0.59, 0.11));
  sh = mix(sh, vec3(dot(sh, vec3(0.33))), 0.25);
  c = mix(c, sh * mix(0.45 + 0.9 * l, 0.3 + 0.8 * l, dark), stripMode);

  float fl = flash[int(vTag + 0.5)];
  vec2 cellSize = vec2(${f(TILE_W)}, ${f(TILE_H)});
  vec2 e = abs(fract(w / cellSize + 0.5) - 0.5) * cellSize;
  float px = fwidth(w.x);
  float line = 1.0 - smoothstep(0.0, 1.6 * px, min(e.x, e.y));
  c = mix(c, vec3(1.0, 0.88, 0.5), 0.22 * fl * fl);
  c = mix(c, mix(vec3(1.0), vec3(1.0, 0.8, 0.3), fl), line * max(0.45 * grid, fl));
  o = vec4(c, 1.0);
}`;

const LINE_VERTEX = `#version 300 es
layout(location = 0) in vec2 position;
uniform vec4 view;
void main() {
  gl_Position = vec4(position.x * view.x + view.y, position.y * view.z + view.w, 0.0, 1.0);
}`;

const LINE = `#version 300 es
precision mediump float;
uniform vec4 color;
out vec4 o;
void main() { o = color; }`;

const BALL_VERTEX = `#version 300 es
layout(location = 0) in vec4 ball; // x, y, angle, color
uniform vec4 view;
uniform float size;
out float vAngle;
flat out int vColor;
void main() {
  vAngle = ball.z;
  vColor = int(ball.w + 0.5);
  gl_PointSize = size;
  gl_Position = vec4(ball.x * view.x + view.y, ball.y * view.z + view.w, 0.0, 1.0);
}`;

const BALL = `#version 300 es
precision highp float;
uniform float size;
in float vAngle;
flat in int vColor;
out vec4 o;
const vec3 PALETTE[8] = vec3[8](
  vec3(0.95, 0.26, 0.30), vec3(0.99, 0.74, 0.16), vec3(0.20, 0.60, 0.96),
  vec3(0.32, 0.80, 0.42), vec3(0.68, 0.42, 0.96), vec3(1.00, 0.52, 0.20),
  vec3(0.14, 0.80, 0.78), vec3(0.98, 0.44, 0.72));
void main() {
  vec2 p = gl_PointCoord * 2.0 - 1.0;
  float r = length(p);
  float edge = 1.0 - smoothstep(1.0 - 3.0 / size, 1.0, r);
  if (edge <= 0.0) discard;
  vec3 n = vec3(p.x, -p.y, sqrt(max(0.0, 1.0 - r * r)));
  float ca = cos(vAngle);
  float sa = sin(vAngle);
  vec2 q = vec2(ca * p.x - sa * p.y, sa * p.x + ca * p.y);
  vec3 base = mix(PALETTE[vColor], vec3(0.97), smoothstep(0.3, 0.2, abs(q.y)) * 0.92);
  vec3 L = normalize(vec3(-0.45, 0.6, 0.7));
  float diff = max(dot(n, L), 0.0);
  float spec = pow(max(dot(reflect(-L, n), vec3(0.0, 0.0, 1.0)), 0.0), 28.0);
  vec3 c = base * (0.28 + 0.8 * diff) + vec3(0.65 * spec);
  c *= mix(0.45, 1.0, smoothstep(1.0, 0.8, r));
  o = vec4(c * edge, edge);
}`;

const PARTICLE_VERTEX = `#version 300 es
layout(location = 0) in vec4 shape; // x, y, size, kind
layout(location = 1) in vec4 color;
uniform vec4 view;
uniform float scale;
out vec4 vColor;
flat out int vKind;
void main() {
  vColor = color;
  vKind = int(shape.w + 0.5);
  gl_PointSize = max(1.5, shape.z * scale);
  gl_Position = vec4(shape.x * view.x + view.y, shape.y * view.z + view.w, 0.0, 1.0);
}`;

const PARTICLE = `#version 300 es
precision mediump float;
in vec4 vColor;
flat in int vKind;
out vec4 o;
void main() {
  vec2 p = gl_PointCoord * 2.0 - 1.0;
  float d = length(p);
  if (vKind == 0) {
    // A spark: a hot glow, added to what is behind it.
    float g = max(0.0, 1.0 - d);
    o = vec4(vColor.rgb * g * g * vColor.a * 1.7, 0.0);
  } else if (vKind == 1) {
    // A clod of dirt: a little square.
    float a = 1.0 - smoothstep(0.65, 0.85, max(abs(p.x), abs(p.y)));
    o = vec4(vColor.rgb, 1.0) * a * vColor.a;
  } else {
    // Smoke: a soft puff.
    float g = 1.0 - smoothstep(0.0, 1.0, d);
    o = vec4(vColor.rgb, 1.0) * g * g * vColor.a;
  }
}`;

const FX = `#version 300 es
${COMMON}
uniform vec4 lights[${MAX_LIGHTS}];
out vec4 o;
void main() {
  vec2 w = worldAt(gl_FragCoord.xy);
  vec3 c = vec3(0.0);
  for (int i = 0; i < ${MAX_LIGHTS}; i++) {
    vec4 L = lights[i];
    if (L.w < 0.0) continue;
    float a = L.w / ${LIGHT_SECONDS.toFixed(3)};
    float k = 1.0 - a;
    float d = length(w - L.xy);
    float rr = L.z * L.z;
    float glow = exp(-d * d / (rr * 4.0)) * k * k * k * 0.8;
    float core = exp(-d * d / (rr * 0.45)) * k * k * k * k * 0.9;
    float R = L.z * (0.9 + 3.2 * sqrt(a));
    float ring = exp(-pow((d - R) / (L.z * 0.1 + 3.0), 2.0)) * k * k * 0.55;
    c += vec3(1.0, 0.52, 0.18) * glow + vec3(1.0, 0.95, 0.8) * core + vec3(1.0, 0.86, 0.62) * ring;
  }
  o = vec4(c * mix(0.7, 1.1, dark), 0.0);
}`;

interface Program {
  program: WebGLProgram;
  u: Record<string, WebGLUniformLocation | null>;
}

function program(
  gl: WebGL2RenderingContext,
  vertex: string,
  fragment: string,
  uniforms: string[],
): Program {
  const p = createProgram(gl, vertex, fragment);
  const u: Program["u"] = {};
  for (const name of uniforms) u[name] = gl.getUniformLocation(p, name);
  return { program: p, u };
}

/** A vertex array of points from one interleaved buffer of fixed size. */
class Points {
  readonly #gl: WebGL2RenderingContext;
  readonly vao: WebGLVertexArrayObject;
  readonly buffer: WebGLBuffer;

  constructor(
    gl: WebGL2RenderingContext,
    floats: number,
    /** location, size and offset in floats of each attribute. */
    layout: [number, number, number][],
    stride: number,
  ) {
    this.#gl = gl;
    this.vao = gl.createVertexArray();
    this.buffer = gl.createBuffer();
    gl.bindVertexArray(this.vao);
    gl.bindBuffer(gl.ARRAY_BUFFER, this.buffer);
    gl.bufferData(gl.ARRAY_BUFFER, floats * 4, gl.DYNAMIC_DRAW);
    for (const [loc, size, offset] of layout) {
      gl.enableVertexAttribArray(loc);
      gl.vertexAttribPointer(
        loc,
        size,
        gl.FLOAT,
        false,
        stride * 4,
        offset * 4,
      );
    }
    gl.bindVertexArray(null);
  }

  draw(data: Float32Array, count: number): void {
    if (count === 0) return;
    const gl = this.#gl;
    gl.bindBuffer(gl.ARRAY_BUFFER, this.buffer);
    gl.bufferSubData(gl.ARRAY_BUFFER, 0, data);
    gl.bindVertexArray(this.vao);
    gl.drawArrays(gl.POINTS, 0, count);
    gl.bindVertexArray(null);
  }

  dispose(): void {
    this.#gl.deleteBuffer(this.buffer);
    this.#gl.deleteVertexArray(this.vao);
  }
}

export interface Look {
  dark: boolean;
  /** Color by strip instead of by material. */
  strips: boolean;
  wireframe: boolean;
  /** Whether blasts shake the view. */
  shake: boolean;
  /** Seconds, for the twinkling and the clouds. */
  time: number;
}

export class Renderer {
  readonly #gl: WebGL2RenderingContext;
  readonly #sky: Program;
  readonly #ground: Program;
  readonly #shadow: Program;
  readonly #line: Program;
  readonly #ball: Program;
  readonly #particle: Program;
  readonly #fx: Program;
  readonly #heights: WebGLTexture;
  readonly #scorch: WebGLTexture;
  readonly #mesh: Mesh;
  readonly #lines: Mesh;
  readonly #balls: Points;
  readonly #particles: Points;
  readonly #builder = new StripBuilder();
  readonly #ballData = new Float32Array(4 * MAX_BALLS);
  readonly #flash = new Float32Array(COLS * ROWS);
  #lineIndices = new Uint32Array(0);
  #lineCount = 0;
  #version = -1;
  #linesVersion = -1;
  #terrain: unknown;
  /** An empty vertex array for the full-screen passes. */
  readonly #empty: WebGLVertexArrayObject;

  constructor(gl: WebGL2RenderingContext) {
    this.#gl = gl;
    const common = ["heights", "frame", "dark"];
    this.#sky = program(gl, FULLSCREEN, SKY, common);
    this.#ground = program(gl, GROUND_VERTEX, GROUND, [
      ...common,
      "view",
      "offset",
      "scorch",
      "flash",
      "stripMode",
      "grid",
    ]);
    this.#shadow = program(gl, GROUND_VERTEX, SHADOW, [
      ...common,
      "view",
      "offset",
      "strength",
    ]);
    this.#line = program(gl, LINE_VERTEX, LINE, ["view", "color"]);
    this.#ball = program(gl, BALL_VERTEX, BALL, ["view", "size"]);
    this.#particle = program(gl, PARTICLE_VERTEX, PARTICLE, ["view", "scale"]);
    this.#fx = program(gl, FULLSCREEN, FX, [...common, "lights"]);

    this.#heights = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, this.#heights);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.NEAREST);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    this.#scorch = gl.createTexture();
    gl.bindTexture(gl.TEXTURE_2D, this.#scorch);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
    gl.pixelStorei(gl.UNPACK_ALIGNMENT, 1);
    gl.texImage2D(
      gl.TEXTURE_2D,
      0,
      gl.R8,
      SCORCH_W,
      SCORCH_H,
      0,
      gl.RED,
      gl.UNSIGNED_BYTE,
      null,
    );

    this.#empty = gl.createVertexArray();
    this.#mesh = new Mesh(gl);
    this.#lines = new Mesh(gl);
    this.#balls = new Points(gl, 4 * MAX_BALLS, [[0, 4, 0]], 4);
    this.#particles = new Points(
      gl,
      PARTICLE_FLOATS * MAX_PARTICLES,
      [
        [0, 4, 0],
        [1, 4, 4],
      ],
      PARTICLE_FLOATS,
    );
  }

  /** Uploads what changed since the last frame. */
  #sync(sim: Sim, wireframe: boolean): void {
    const gl = this.#gl;
    if (this.#terrain !== sim.terrain) {
      this.#terrain = sim.terrain;
      gl.bindTexture(gl.TEXTURE_2D, this.#heights);
      gl.texImage2D(
        gl.TEXTURE_2D,
        0,
        gl.R32F,
        HEIGHT_SAMPLES,
        1,
        0,
        gl.RED,
        gl.FLOAT,
        sim.terrain.heights,
      );
    }
    if (sim.scorchDirty) {
      sim.scorchDirty = false;
      gl.bindTexture(gl.TEXTURE_2D, this.#scorch);
      gl.texSubImage2D(
        gl.TEXTURE_2D,
        0,
        0,
        0,
        SCORCH_W,
        SCORCH_H,
        gl.RED,
        gl.UNSIGNED_BYTE,
        sim.scorch,
      );
    }
    const b = this.#builder;
    if (this.#version !== sim.world.version) {
      this.#version = sim.world.version;
      b.reset();
      for (const t of sim.world.tiles) {
        if (t.result !== undefined) b.add(t.result, t.points, t.index);
      }
      this.#mesh.attribute(0, b.positions, 2);
      this.#mesh.attribute(1, b.strips, 1);
      this.#mesh.attribute(2, b.tags, 1);
      this.#mesh.indices(b.indices);
    }
    if (wireframe && this.#linesVersion !== this.#version) {
      this.#linesVersion = this.#version;
      this.#buildLines(b.indices);
      this.#lines.attribute(0, b.positions, 2);
      this.#lines.indices(this.#lineIndices.subarray(0, this.#lineCount));
    }
  }

  /**
   * The edges of the strips' triangles as line pairs: in a strip the vertex
   * i meets i + 1 and i + 2.
   */
  #buildLines(indices: Uint32Array): void {
    const need = 4 * indices.length;
    if (this.#lineIndices.length < need)
      this.#lineIndices = new Uint32Array(Math.ceil(need * 1.5));
    const out = this.#lineIndices;
    let n = 0;
    for (let i = 0; i < indices.length; i++) {
      const a = indices[i];
      if (a === RESTART_INDEX) continue;
      for (let k = 1; k <= 2; k++) {
        const b = indices[i + k];
        if (b === undefined || b === RESTART_INDEX) break;
        out[n++] = a;
        out[n++] = b;
      }
    }
    this.#lineCount = n;
  }

  draw(sim: Sim, look: Look): void {
    const gl = this.#gl;
    this.#sync(sim, look.wireframe);
    const cw = gl.canvas.width;
    const ch = gl.canvas.height;
    // The world covers the stage, bottom up and centred: a stage narrower
    // than the world (a phone's 4:3) crops its sides, a wider one the sky.
    const s = Math.max(cw / WORLD_W, ch / WORLD_H);
    const shake = look.shake ? sim.shake : 0;
    const shakeX = shake * Math.sin(look.time * 97);
    const shakeY = shake * Math.cos(look.time * 71) * 0.6;
    const ox = (cw - WORLD_W * s) / 2 + shakeX * s;
    const oy = -shakeY * s;
    const view: [number, number, number, number] = [
      (2 * s) / cw,
      (2 * ox) / cw - 1,
      (-2 * s) / ch,
      (2 * (WORLD_H * s + oy)) / ch - 1,
    ];
    const dark = look.dark ? 1 : 0;

    gl.viewport(0, 0, cw, ch);
    gl.disable(gl.BLEND);
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, this.#heights);
    gl.activeTexture(gl.TEXTURE1);
    gl.bindTexture(gl.TEXTURE_2D, this.#scorch);

    const common = (p: Program) => {
      gl.useProgram(p.program);
      gl.uniform1i(p.u.heights, 0);
      gl.uniform4f(p.u.frame, ox, oy, s, look.time);
      gl.uniform1f(p.u.dark, dark);
    };

    common(this.#sky);
    gl.bindVertexArray(this.#empty);
    gl.drawArrays(gl.TRIANGLES, 0, 3);

    // The shadow first, in three steps for a soft edge.
    gl.enable(gl.BLEND);
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
    const sh = this.#shadow;
    common(sh);
    gl.uniform4f(sh.u.view, ...view);
    for (const k of [1, 2, 3]) {
      gl.uniform2f(sh.u.offset, 3.5 * k, 4.5 * k);
      gl.uniform1f(sh.u.strength, look.dark ? 0.22 : 0.17);
      this.#mesh.draw();
    }
    gl.disable(gl.BLEND);

    const g = this.#ground;
    common(g);
    gl.uniform2f(g.u.offset, 0, 0);
    gl.uniform4f(g.u.view, ...view);
    gl.uniform1i(g.u.scorch, 1);
    for (const t of sim.world.tiles) this.#flash[t.index] = t.flash;
    gl.uniform1fv(g.u.flash, this.#flash);
    gl.uniform1f(g.u.stripMode, look.strips ? 1 : 0);
    gl.uniform1f(g.u.grid, look.strips || look.wireframe ? 1 : 0);
    this.#mesh.draw();

    gl.enable(gl.BLEND);
    gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
    if (look.wireframe) {
      gl.useProgram(this.#line.program);
      gl.uniform4f(this.#line.u.view, ...view);
      // White over the strip colors, else a shade that suits the soil.
      const a = look.strips ? 0.55 : look.dark ? 0.3 : 0.4;
      const v = look.strips || look.dark ? 1 : 0.1;
      gl.uniform4f(this.#line.u.color, v * a, v * a, v * a, a);
      this.#lines.draw(gl.LINES, this.#lineCount);
    }

    if (sim.balls > 0) {
      const d = this.#ballData;
      for (let i = 0; i < sim.balls; i++) {
        d[4 * i] = sim.bx[i];
        d[4 * i + 1] = sim.by[i];
        d[4 * i + 2] = sim.angle[i];
        d[4 * i + 3] = sim.hue[i];
      }
      gl.useProgram(this.#ball.program);
      gl.uniform4f(this.#ball.u.view, ...view);
      gl.uniform1f(this.#ball.u.size, 2 * BALL_R * s + 2);
      this.#balls.draw(d, sim.balls);
    }

    if (sim.particles > 0) {
      gl.useProgram(this.#particle.program);
      gl.uniform4f(this.#particle.u.view, ...view);
      gl.uniform1f(this.#particle.u.scale, s);
      this.#particles.draw(sim.particleData, sim.particles);
    }

    let lit = false;
    for (let i = 0; i < MAX_LIGHTS; i++)
      if (sim.lights[4 * i + 3] >= 0) lit = true;
    if (lit) {
      common(this.#fx);
      gl.uniform4fv(this.#fx.u.lights, sim.lights);
      gl.bindVertexArray(this.#empty);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
    }
    gl.disable(gl.BLEND);
  }

  dispose(): void {
    const gl = this.#gl;
    for (const p of [
      this.#sky,
      this.#ground,
      this.#shadow,
      this.#line,
      this.#ball,
      this.#particle,
      this.#fx,
    ])
      gl.deleteProgram(p.program);
    gl.deleteTexture(this.#heights);
    gl.deleteTexture(this.#scorch);
    this.#mesh.dispose();
    this.#lines.dispose();
    this.#balls.dispose();
    this.#particles.dispose();
    gl.deleteVertexArray(this.#empty);
  }
}
