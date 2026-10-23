// The world map's WebGL 2 side: the camera, a hand-written 3 × 3 matrix from
// the grid to clip space, and the renderer, which draws the ocean, the
// countries (one drawElements per copy of the world in view, usually one) and
// their borders.

import { createProgram, Mesh } from "@/lib/gl";
import type { BatchResult } from "@/wasm/earstrip";

import type { Geometry } from "./geometry";

/**
 * Where the map is looked at, in device pixels: the grid point in the middle
 * of the canvas and the pixels per grid unit. The world repeats east and west.
 */
export class Camera {
  readonly size: number;
  cx: number;
  cy: number;
  zoom = 1;
  width = 1;
  height = 1;

  constructor(size: number) {
    this.size = size;
    this.cx = size / 2;
    this.cy = size / 2;
  }

  get minZoom(): number {
    return Math.min(this.width, this.height) / this.size;
  }

  get maxZoom(): number {
    return (this.width / this.size) * 96;
  }

  /**
   * The world's width across the canvas, from about 60° south to 80° north
   * (Antarctica mostly below the edge).
   */
  fit(): void {
    this.zoom = this.width / this.size;
    this.cx = this.size / 2;
    this.cy = this.size * 0.585;
    this.clamp();
  }

  resize(width: number, height: number): void {
    const first = this.width === 1 && this.height === 1;
    // Keep what is seen across the width.
    if (!first) this.zoom *= width / this.width;
    this.width = width;
    this.height = height;
    if (first) this.fit();
    else this.clamp();
  }

  clamp(): void {
    this.zoom = Math.min(this.maxZoom, Math.max(this.minZoom, this.zoom));
    const half = this.height / (2 * this.zoom);
    this.cy =
      2 * half >= this.size
        ? this.size / 2
        : Math.min(this.size - half, Math.max(half, this.cy));
    this.cx = ((this.cx % this.size) + this.size) % this.size;
  }

  /** The grid point under the pixel (x, y), y down. */
  toWorld(x: number, y: number): [number, number] {
    return [
      this.cx + (x - this.width / 2) / this.zoom,
      this.cy - (y - this.height / 2) / this.zoom,
    ];
  }

  pan(dx: number, dy: number): void {
    this.cx -= dx / this.zoom;
    this.cy += dy / this.zoom;
    this.clamp();
  }

  /** Zooms by factor, keeping the grid point under the pixel (x, y) there. */
  zoomAt(x: number, y: number, factor: number): void {
    const [wx, wy] = this.toWorld(x, y);
    this.zoom = Math.min(
      this.maxZoom,
      Math.max(this.minZoom, this.zoom * factor),
    );
    this.cx = wx - (x - this.width / 2) / this.zoom;
    this.cy = wy + (y - this.height / 2) / this.zoom;
    this.clamp();
  }

  /** The copies of the world in view, as multiples of the world's width. */
  copies(): number[] {
    const half = this.width / (2 * this.zoom);
    const out: number[] = [];
    // A copy only touching the edge is not in view.
    const last = Math.ceil((this.cx + half) / this.size) - 1;
    for (let k = Math.floor((this.cx - half) / this.size); k <= last; k++) {
      out.push(k);
    }
    return out;
  }

  /**
   * The matrix from the grid to clip space for the copy of the world k
   * widths east, column-major: scale, then the translation.
   */
  matrix(k: number, out: Float32Array): Float32Array {
    const sx = (2 * this.zoom) / this.width;
    const sy = (2 * this.zoom) / this.height;
    out.set([
      sx,
      0,
      0,
      0,
      sy,
      0,
      (k * this.size - this.cx) * sx,
      -this.cy * sy,
      1,
    ]);
    return out;
  }
}

const FILL_VERTEX = `#version 300 es
layout(location = 0) in vec2 position;
layout(location = 1) in vec2 info;   // country, MAPCOLOR7
layout(location = 2) in vec2 place;  // strip, place in the strip
uniform mat3 view;
out vec3 vBary;
flat out vec4 vInfo;
void main() {
  // Three places in a row of a strip differ mod 3, so every triangle of a
  // strip gets one corner each: barycentric coordinates for the wireframe.
  float k = mod(place.y, 3.0);
  vBary = vec3(k < 0.5, abs(k - 1.0) < 0.5, k > 1.5);
  // A flat value comes from a triangle's last vertex: its parity alternates
  // along the strip.
  vInfo = vec4(info, place.x, mod(place.y, 2.0));
  gl_Position = vec4((view * vec3(position, 1.0)).xy, 0.0, 1.0);
}`;

const FILL_FRAGMENT = `#version 300 es
precision highp float;
in vec3 vBary;
flat in vec4 vInfo;
uniform vec3 palette[7];
uniform float byStrip;
uniform float hover;
uniform float head;
uniform float glow;
uniform float wire;
uniform float dark;
uniform vec3 wireColor;
uniform vec3 dimColor;
out vec4 color;

uniform vec3 vivid[7];

float hash(float n) {
  return fract(sin(n * 12.9898 + 4.1414) * 43758.5453);
}

// A strip's color: its country's hue, so that the countries stay apart, in a
// lightness and a slight shift of hue of its own, so that the strips do.
vec3 stripColor(float s, float mapColor) {
  vec3 c = vivid[int(mapColor) - 1];
  float h = hash(s);
  c *= 0.72 + 0.5 * h;
  c = mix(c, c.gbr, 0.1 * hash(s + 7.0));
  return clamp(c, 0.0, 1.0);
}

void main() {
  bool hovered = vInfo.x == hover;
  vec3 c;
  if (byStrip > 0.5 || hovered) {
    c = stripColor(vInfo.z, vInfo.y);
    // The zig-zag: every other triangle of a strip a shade darker.
    c *= 1.0 - 0.07 * vInfo.w;
  } else {
    c = palette[int(vInfo.y) - 1];
  }
  if (hover >= 0.0 && !hovered) c = mix(c, dimColor, 0.45);
  if (glow > 0.0) {
    float age = head - vInfo.z;
    float g = age >= 0.0 ? 1.0 - clamp(age / glow, 0.0, 1.0) : 0.0;
    c = mix(c, vec3(1.0, 0.96, 0.82), g * g * 0.85);
  }
  if (wire > 0.5) {
    vec3 a = smoothstep(vec3(0.0), fwidth(vBary) * 1.25, vBary);
    float edge = 1.0 - min(min(a.x, a.y), a.z);
    c = mix(c, wireColor, edge * 0.75);
  }
  color = vec4(c, 1.0);
}`;

const BORDER_VERTEX = `#version 300 es
layout(location = 0) in vec2 position;
layout(location = 1) in vec2 info;
uniform mat3 view;
flat out float vCountry;
void main() {
  vCountry = info.x;
  gl_Position = vec4((view * vec3(position, 1.0)).xy, 0.0, 1.0);
}`;

const BORDER_FRAGMENT = `#version 300 es
precision highp float;
flat in float vCountry;
uniform vec4 line;
uniform vec4 hoverLine;
uniform float hover;
out vec4 color;
void main() {
  color = vCountry == hover ? hoverLine : line;
}`;

// One triangle over the whole canvas; the fragment shader finds the grid
// point, and from it longitude and latitude, of every pixel.
const OCEAN_VERTEX = `#version 300 es
out vec2 vClip;
void main() {
  vec2 p = vec2(gl_VertexID == 1 ? 3.0 : -1.0, gl_VertexID == 2 ? 3.0 : -1.0);
  vClip = p;
  gl_Position = vec4(p, 0.0, 1.0);
}`;

const OCEAN_FRAGMENT = `#version 300 es
precision highp float;
in vec2 vClip;
uniform vec4 inverse;  // the middle, x, y, and half the canvas in grid units
uniform float size;
uniform vec3 deep;
uniform vec3 shallow;
uniform vec3 space;
uniform vec4 graticule;
out vec4 color;
void main() {
  vec2 w = inverse.xy + vClip * inverse.zw;
  vec2 m = w / size * 2.0 - 1.0;
  if (abs(m.y) > 1.0) {
    color = vec4(space, 1.0);
    return;
  }
  // Web Mercator backwards.
  float lat = degrees(atan(sinh(m.y * 3.14159265)));
  float lon = m.x * 180.0;
  vec3 c = mix(shallow, deep, smoothstep(10.0, 75.0, abs(lat)));
  vec2 g = vec2(lon, lat) / 15.0;
  vec2 d = abs(fract(g + 0.5) - 0.5) / fwidth(g);
  float on = 1.0 - min(min(d.x, d.y), 1.0);
  color = vec4(mix(c, graticule.rgb, on * graticule.a), 1.0);
}`;

type RGB = [number, number, number];

function hex(h: string): RGB {
  const n = parseInt(h.slice(1), 16);
  return [(n >> 16) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
}

/** The colors of a theme. */
interface Theme {
  /** The fills of MAPCOLOR7 1 to 7. */
  palette: RGB[];
  /** The same hues stronger, for the strips. */
  vivid: RGB[];
  deep: RGB;
  shallow: RGB;
  space: RGB;
  graticule: [number, number, number, number];
  border: [number, number, number, number];
  hoverBorder: [number, number, number, number];
  wire: RGB;
  dim: RGB;
}

const LIGHT: Theme = {
  palette: [
    "#f2cd85",
    "#bcdd9b",
    "#f2b0aa",
    "#cbbbe8",
    "#f1e08a",
    "#a6d8c9",
    "#f0bb8f",
  ].map(hex),
  vivid: [
    "#f0a73c",
    "#7cc15a",
    "#ef6f6c",
    "#9a7be0",
    "#e9cc3f",
    "#3fb5a0",
    "#e8834d",
  ].map(hex),
  deep: hex("#c9dcec"),
  shallow: hex("#e2edf6"),
  space: hex("#f4f6f9"),
  graticule: [...hex("#7f9bb8"), 0.22],
  border: [...hex("#5b6577"), 0.55],
  hoverBorder: [...hex("#111827"), 1],
  wire: hex("#24304a"),
  dim: hex("#e9eef4"),
};

const DARK: Theme = {
  palette: [
    "#5d5034",
    "#3f5636",
    "#5d3a3d",
    "#463e5e",
    "#5b5631",
    "#2f524b",
    "#5e4430",
  ].map(hex),
  vivid: [
    "#c7862c",
    "#5f9c45",
    "#c4524f",
    "#7a5fc0",
    "#bfa52f",
    "#2f9181",
    "#c0653a",
  ].map(hex),
  deep: hex("#08101f"),
  shallow: hex("#0e1a2e"),
  space: hex("#0a0a0b"),
  graticule: [...hex("#5b7aa6"), 0.2],
  border: [...hex("#c9d4e5"), 0.4],
  hoverBorder: [...hex("#ffffff"), 1],
  wire: hex("#e7edf7"),
  dim: hex("#0d1526"),
};

/** What a frame shows. */
export interface View {
  byStrip: boolean;
  /** Draws the list instead of the strips: the same pixels. */
  list: boolean;
  wireframe: boolean;
  /** The highlighted country, or -1. */
  hover: number;
  /** How many strips the intro has shown, fractional; Infinity for all. */
  reveal: number;
  dark: boolean;
}

function uniforms(gl: WebGL2RenderingContext, program: WebGLProgram) {
  return (name: string) => gl.getUniformLocation(program, name);
}

export class MapRenderer {
  readonly #gl: WebGL2RenderingContext;
  readonly #fill: WebGLProgram;
  readonly #border: WebGLProgram;
  readonly #ocean: WebGLProgram;
  readonly #u: Record<string, WebGLUniformLocation | null> = {};
  readonly #shared: Mesh;
  readonly #strips: Mesh;
  readonly #borders: Mesh;
  readonly #matrix = new Float32Array(9);
  #geometry?: Geometry;
  #result?: BatchResult;
  /** Which index buffer each mesh holds: the list or the strips. */
  #sharedList = false;
  #stripsList = false;

  constructor(gl: WebGL2RenderingContext) {
    this.#gl = gl;
    this.#fill = createProgram(gl, FILL_VERTEX, FILL_FRAGMENT);
    this.#border = createProgram(gl, BORDER_VERTEX, BORDER_FRAGMENT);
    this.#ocean = createProgram(gl, OCEAN_VERTEX, OCEAN_FRAGMENT);
    const f = uniforms(gl, this.#fill);
    for (const name of [
      "view",
      "palette",
      "vivid",
      "byStrip",
      "hover",
      "head",
      "glow",
      "wire",
      "dark",
      "wireColor",
      "dimColor",
    ]) {
      this.#u[`fill.${name}`] = f(name);
    }
    const b = uniforms(gl, this.#border);
    for (const name of ["view", "line", "hoverLine", "hover"]) {
      this.#u[`border.${name}`] = b(name);
    }
    const o = uniforms(gl, this.#ocean);
    for (const name of [
      "inverse",
      "size",
      "deep",
      "shallow",
      "space",
      "graticule",
    ]) {
      this.#u[`ocean.${name}`] = o(name);
    }
    this.#shared = new Mesh(gl);
    this.#strips = new Mesh(gl);
    this.#borders = new Mesh(gl);
  }

  /** Uploads the meshes of a new triangulation. */
  setGeometry(g: Geometry, r: BatchResult, list: boolean): void {
    this.#geometry = g;
    this.#result = r;
    this.#shared.attribute(0, g.sharedPositions, 2);
    this.#shared.attribute(1, g.sharedInfo, 2);
    this.#shared.indices(list ? r.list : r.restart);
    this.#sharedList = list;
    this.#strips.attribute(0, g.stripPositions, 2);
    this.#strips.attribute(1, g.stripInfo, 2);
    this.#strips.attribute(2, g.stripPlace, 2);
    this.#strips.indices(list ? g.stripList : g.stripIndices);
    this.#stripsList = list;
    this.#borders.attribute(0, g.sharedPositions, 2);
    this.#borders.attribute(1, g.sharedInfo, 2);
    this.#borders.indices(g.borders);
  }

  /** Draws the frame; returns the draw calls of the countries. */
  render(camera: Camera, view: View): number {
    const gl = this.#gl;
    const u = this.#u;
    const theme = view.dark ? DARK : LIGHT;
    gl.viewport(0, 0, gl.canvas.width, gl.canvas.height);
    gl.disable(gl.BLEND);

    gl.useProgram(this.#ocean);
    gl.uniform4f(
      u["ocean.inverse"],
      camera.cx,
      camera.cy,
      camera.width / (2 * camera.zoom),
      camera.height / (2 * camera.zoom),
    );
    gl.uniform1f(u["ocean.size"], camera.size);
    gl.uniform3fv(u["ocean.deep"], theme.deep);
    gl.uniform3fv(u["ocean.shallow"], theme.shallow);
    gl.uniform3fv(u["ocean.space"], theme.space);
    gl.uniform4fv(u["ocean.graticule"], theme.graticule);
    gl.bindVertexArray(null);
    gl.drawArrays(gl.TRIANGLES, 0, 3);

    const g = this.#geometry;
    const r = this.#result;
    if (g === undefined || r === undefined) return 0;

    const strips = g.strips;
    const revealing = view.reveal < strips;
    const shown = Math.max(0, Math.min(strips, Math.floor(view.reveal)));
    const ends = view.list ? r.stripListEnds : r.stripRestartEnds;
    const count = shown === 0 ? 0 : ends[shown - 1];
    // The shared mesh is what an engine draws; the strip mesh, the same
    // triangles with a vertex per index, can tell the strips apart.
    const byPlace =
      view.byStrip || view.wireframe || view.hover >= 0 || revealing;
    const mesh = byPlace ? this.#strips : this.#shared;
    if (byPlace && this.#stripsList !== view.list) {
      this.#strips.indices(view.list ? g.stripList : g.stripIndices);
      this.#stripsList = view.list;
    }
    if (!byPlace && this.#sharedList !== view.list) {
      this.#shared.indices(view.list ? r.list : r.restart);
      this.#sharedList = view.list;
    }

    gl.useProgram(this.#fill);
    gl.uniform3fv(u["fill.palette"], theme.palette.flat());
    gl.uniform3fv(u["fill.vivid"], theme.vivid.flat());
    gl.uniform1f(u["fill.byStrip"], view.byStrip ? 1 : 0);
    gl.uniform1f(u["fill.hover"], view.hover);
    gl.uniform1f(u["fill.head"], view.reveal);
    gl.uniform1f(u["fill.glow"], revealing ? Math.max(8, strips * 0.06) : 0);
    gl.uniform1f(u["fill.wire"], view.wireframe ? 1 : 0);
    gl.uniform1f(u["fill.dark"], view.dark ? 1 : 0);
    gl.uniform3fv(u["fill.wireColor"], theme.wire);
    gl.uniform3fv(u["fill.dimColor"], theme.dim);
    const copies = camera.copies();
    const mode = view.list ? gl.TRIANGLES : gl.TRIANGLE_STRIP;
    for (const k of copies) {
      gl.uniformMatrix3fv(
        u["fill.view"],
        false,
        camera.matrix(k, this.#matrix),
      );
      mesh.draw(mode, count);
    }

    gl.enable(gl.BLEND);
    gl.blendFuncSeparate(
      gl.SRC_ALPHA,
      gl.ONE_MINUS_SRC_ALPHA,
      gl.ONE,
      gl.ONE_MINUS_SRC_ALPHA,
    );
    gl.useProgram(this.#border);
    const fade = revealing ? 0.35 + 0.65 * (shown / strips) ** 4 : 1;
    const [lr, lg, lb, la] = theme.border;
    gl.uniform4f(u["border.line"], lr, lg, lb, la * fade);
    gl.uniform4fv(u["border.hoverLine"], theme.hoverBorder);
    gl.uniform1f(u["border.hover"], view.hover);
    for (const k of copies) {
      gl.uniformMatrix3fv(
        u["border.view"],
        false,
        camera.matrix(k, this.#matrix),
      );
      this.#borders.draw(gl.LINE_STRIP);
    }
    gl.disable(gl.BLEND);
    return copies.length;
  }

  dispose(): void {
    const gl = this.#gl;
    this.#shared.dispose();
    this.#strips.dispose();
    this.#borders.dispose();
    gl.deleteProgram(this.#fill);
    gl.deleteProgram(this.#border);
    gl.deleteProgram(this.#ocean);
  }
}
