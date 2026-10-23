// The lamp's WebGL 2 scene: a backdrop, the bands of wax drawn straight from
// the triangle strips (or their zig-zag as line strips), and a glow made of
// the same strips: drawn small into a texture, blurred, and laid over the
// picture (added in dark mode, behind the wax as a halo in light mode).

import { createProgram, Mesh, type StripBuilder } from "@/lib/gl";

const FULLSCREEN = `#version 300 es
out vec2 uv;
void main() {
  vec2 p = vec2(gl_VertexID == 1 ? 3.0 : -1.0, gl_VertexID == 2 ? 3.0 : -1.0);
  uv = p * 0.5 + 0.5;
  gl_Position = vec4(p, 0.0, 1.0);
}`;

const BACKDROP = `#version 300 es
precision highp float;
in vec2 uv;
uniform vec3 top;
uniform vec3 bottom;
uniform vec3 heat;
uniform float aspect;
out vec4 color;
void main() {
  vec3 c = mix(bottom, top, smoothstep(0.0, 1.0, uv.y));
  // The lamp's bulb under the middle of the stage.
  vec2 d = vec2((uv.x - 0.5) * aspect, uv.y + 0.15);
  c += heat * exp(-2.2 * dot(d, d));
  // Darker sides, like light through the curved glass.
  float side = abs(uv.x - 0.5) * 2.0;
  c *= 1.0 - 0.18 * side * side;
  color = vec4(c, 1.0);
}`;

const WAX_VERTEX = `#version 300 es
layout(location = 0) in vec2 position;
layout(location = 1) in float strip;
layout(location = 2) in float tag;
uniform vec2 scale;
flat out float vStrip;
flat out float vTag;
out float vHeight;
void main() {
  vStrip = strip;
  vTag = tag;
  vec2 p = position * scale - 1.0;
  vHeight = p.y * 0.5 + 0.5;
  gl_Position = vec4(p, 0.0, 1.0);
}`;

const WAX_FRAGMENT = `#version 300 es
precision highp float;
flat in float vStrip;
flat in float vTag;
in float vHeight;
uniform vec3 band[4];
uniform float bands;
uniform float stripColors;
uniform float glow;
uniform vec4 line;
uniform vec2 stripShade;
uniform sampler2D thickness;
uniform vec2 shade;
uniform vec2 viewport;
uniform vec4 fill;
out vec4 color;
void main() {
  if (line.a > 0.0) {
    color = line;
    return;
  }
  int k = int(vTag + 0.5);
  vec3 c = band[k];
  if (stripColors > 0.5) {
    // Each strip its own shade of the band, by the golden ratio so that
    // neighboring strips differ: a step toward the next band's color and a
    // lightness of its own. It stays in the lamp's palette.
    float h = fract(vStrip * 0.618034);
    float l = fract(vStrip * 0.381966 + 0.5);
    c = mix(c, band[min(k + 1, 3)], 0.35 * h) * (stripShade.x + stripShade.y * l);
  }
  // The blurred wax says how deep in the wax this point is: lighter (or
  // darker) there, so that the flat bands look rounded.
  vec3 t = texture(thickness, gl_FragCoord.xy / viewport).rgb;
  c *= shade.x + shade.y * max(t.r, max(t.g, t.b));
  // Hotter at the bottom, near the bulb.
  c *= 1.12 - 0.24 * vHeight;
  if (glow > 0.0) {
    // The inner bands glow more than the outer ones.
    c *= glow * (vTag + 1.0) / bands;
  }
  color = vec4(mix(c, fill.rgb, fill.a), 1.0);
}`;

const BLUR = `#version 300 es
precision highp float;
in vec2 uv;
uniform sampler2D source;
uniform vec2 step;
out vec4 color;
void main() {
  // A 9-tap Gaussian in 5 samples, by sampling between texels.
  vec4 c = texture(source, uv) * 0.2270270270;
  c += texture(source, uv + step * 1.3846153846) * 0.3162162162;
  c += texture(source, uv - step * 1.3846153846) * 0.3162162162;
  c += texture(source, uv + step * 3.2307692308) * 0.0702702703;
  c += texture(source, uv - step * 3.2307692308) * 0.0702702703;
  color = c;
}`;

const COMPOSITE = `#version 300 es
precision highp float;
in vec2 uv;
uniform sampler2D source;
uniform float strength;
out vec4 color;
void main() {
  vec3 g = texture(source, uv).rgb * strength;
  // Premultiplied: added in dark mode (alpha is ignored), over in light mode.
  color = vec4(g, clamp(max(g.r, max(g.g, g.b)), 0.0, 1.0));
}`;

/** How the glow texture's size relates to the canvas's. */
const GLOW_DIVISOR = 4;

/** The colors of a theme: the backdrop and the bands, outer to inner. */
export interface Palette {
  top: [number, number, number];
  bottom: [number, number, number];
  heat: [number, number, number];
  bands: [number, number, number][];
  line: [number, number, number, number];
  /** Whether the glow is added over the picture (else laid behind the wax). */
  additive: boolean;
  glow: number;
  /** A strip's lightness in strip colors: base + range × [0, 1). */
  stripShade: [number, number];
  /** What the wax fades to under the wireframe, and how far. */
  dim: [number, number, number, number];
  /** The wax's lightness by its depth in the glow: base + gain × glow. */
  shade: [number, number];
}

export interface DrawSettings {
  palette: Palette;
  bands: number;
  stripColors: boolean;
  wireframe: boolean;
  /** The stage in cells, which the wax's positions are in. */
  cols: number;
  rows: number;
}

interface Target {
  texture: WebGLTexture;
  framebuffer: WebGLFramebuffer;
}

type Uniforms<K extends string> = Record<K, WebGLUniformLocation | null>;

function uniforms<K extends string>(
  gl: WebGL2RenderingContext,
  program: WebGLProgram,
  names: K[],
): Uniforms<K> {
  const out = {} as Uniforms<K>;
  for (const n of names) out[n] = gl.getUniformLocation(program, n);
  return out;
}

export class LavaRenderer {
  readonly #gl: WebGL2RenderingContext;
  readonly #mesh: Mesh;
  readonly #backdrop: WebGLProgram;
  readonly #wax: WebGLProgram;
  readonly #blur: WebGLProgram;
  readonly #composite: WebGLProgram;
  readonly #u: {
    backdrop: Uniforms<"top" | "bottom" | "heat" | "aspect">;
    wax: Uniforms<
      | "scale"
      | "band"
      | "bands"
      | "stripColors"
      | "glow"
      | "line"
      | "stripShade"
      | "thickness"
      | "shade"
      | "viewport"
      | "fill"
    >;
    blur: Uniforms<"source" | "step">;
    composite: Uniforms<"source" | "strength">;
  };
  readonly #band = new Float32Array(12);
  #targets: [Target, Target] | undefined;
  #glowWidth = 0;
  #glowHeight = 0;

  constructor(gl: WebGL2RenderingContext) {
    this.#gl = gl;
    this.#mesh = new Mesh(gl);
    this.#backdrop = createProgram(gl, FULLSCREEN, BACKDROP);
    this.#wax = createProgram(gl, WAX_VERTEX, WAX_FRAGMENT);
    this.#blur = createProgram(gl, FULLSCREEN, BLUR);
    this.#composite = createProgram(gl, FULLSCREEN, COMPOSITE);
    this.#u = {
      backdrop: uniforms(gl, this.#backdrop, [
        "top",
        "bottom",
        "heat",
        "aspect",
      ]),
      wax: uniforms(gl, this.#wax, [
        "scale",
        "band",
        "bands",
        "stripColors",
        "glow",
        "line",
        "stripShade",
        "thickness",
        "shade",
        "viewport",
        "fill",
      ]),
      blur: uniforms(gl, this.#blur, ["source", "step"]),
      composite: uniforms(gl, this.#composite, ["source", "strength"]),
    };
  }

  /** Uploads the strips of the frame. */
  upload(builder: StripBuilder): void {
    this.#mesh.attribute(0, builder.positions, 2);
    this.#mesh.attribute(1, builder.strips, 1);
    this.#mesh.attribute(2, builder.tags, 1);
    this.#mesh.indices(builder.indices);
  }

  /** The two glow textures, (re)made at a quarter of the canvas's size. */
  #glowTargets(): [Target, Target] {
    const gl = this.#gl;
    const w = Math.max(1, Math.ceil(gl.canvas.width / GLOW_DIVISOR));
    const h = Math.max(1, Math.ceil(gl.canvas.height / GLOW_DIVISOR));
    if (
      this.#targets !== undefined &&
      w === this.#glowWidth &&
      h === this.#glowHeight
    )
      return this.#targets;
    this.#disposeTargets();
    const make = (): Target => {
      const texture = gl.createTexture();
      gl.bindTexture(gl.TEXTURE_2D, texture);
      gl.texImage2D(
        gl.TEXTURE_2D,
        0,
        gl.RGBA8,
        w,
        h,
        0,
        gl.RGBA,
        gl.UNSIGNED_BYTE,
        null,
      );
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE);
      gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE);
      const framebuffer = gl.createFramebuffer();
      gl.bindFramebuffer(gl.FRAMEBUFFER, framebuffer);
      gl.framebufferTexture2D(
        gl.FRAMEBUFFER,
        gl.COLOR_ATTACHMENT0,
        gl.TEXTURE_2D,
        texture,
        0,
      );
      gl.bindFramebuffer(gl.FRAMEBUFFER, null);
      return { texture, framebuffer };
    };
    this.#targets = [make(), make()];
    this.#glowWidth = w;
    this.#glowHeight = h;
    return this.#targets;
  }

  #disposeTargets(): void {
    if (this.#targets === undefined) return;
    const gl = this.#gl;
    for (const t of this.#targets) {
      gl.deleteFramebuffer(t.framebuffer);
      gl.deleteTexture(t.texture);
    }
    this.#targets = undefined;
  }

  /**
   * Sets the wax program up. thickness is the glow texture it shades by, on
   * unit 1; while the glow itself is drawn, it is the other texture and the
   * shading is off, so that no texture is read while it is drawn into.
   */
  #useWax(
    s: DrawSettings,
    glow: number,
    thickness: Target,
    line?: [number, number, number, number],
  ): void {
    const gl = this.#gl;
    const u = this.#u.wax;
    gl.useProgram(this.#wax);
    gl.uniform2f(u.scale, 2 / s.cols, 2 / s.rows);
    for (let k = 0; k < 4; k++) {
      const c = s.palette.bands[Math.min(k, s.palette.bands.length - 1)];
      this.#band.set(c, 3 * k);
    }
    gl.uniform3fv(u.band, this.#band);
    gl.uniform1f(u.bands, s.bands);
    gl.uniform1f(u.stripColors, s.stripColors ? 1 : 0);
    gl.uniform1f(u.glow, glow);
    const p = s.palette;
    gl.uniform2f(u.stripShade, p.stripShade[0], p.stripShade[1]);
    gl.uniform2f(u.shade, glow > 0 ? 1 : p.shade[0], glow > 0 ? 0 : p.shade[1]);
    gl.uniform2f(u.viewport, gl.canvas.width, gl.canvas.height);
    // Under the wireframe the wax is dimmed, so that the zig-zag stands out.
    const d = p.dim;
    gl.uniform4f(
      u.fill,
      d[0],
      d[1],
      d[2],
      s.wireframe && glow === 0 ? d[3] : 0,
    );
    gl.activeTexture(gl.TEXTURE1);
    gl.bindTexture(gl.TEXTURE_2D, thickness.texture);
    gl.uniform1i(u.thickness, 1);
    if (line === undefined) gl.uniform4f(u.line, 0, 0, 0, 0);
    else gl.uniform4f(u.line, line[0], line[1], line[2], line[3]);
  }

  /** Blurs the wax into the glow texture; returns the one that holds it. */
  #renderGlow(s: DrawSettings): Target {
    const gl = this.#gl;
    const [a, b] = this.#glowTargets();
    const w = this.#glowWidth;
    const h = this.#glowHeight;
    gl.viewport(0, 0, w, h);
    gl.bindFramebuffer(gl.FRAMEBUFFER, a.framebuffer);
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT);
    this.#useWax(s, s.palette.glow, b);
    this.#mesh.draw();

    gl.useProgram(this.#blur);
    gl.uniform1i(this.#u.blur.source, 0);
    gl.activeTexture(gl.TEXTURE0);
    // Two rounds of a horizontal and a vertical pass, the second twice as
    // wide, for a soft and wide halo.
    for (const spread of [1, 2.5]) {
      gl.bindFramebuffer(gl.FRAMEBUFFER, b.framebuffer);
      gl.bindTexture(gl.TEXTURE_2D, a.texture);
      gl.uniform2f(this.#u.blur.step, spread / w, 0);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
      gl.bindFramebuffer(gl.FRAMEBUFFER, a.framebuffer);
      gl.bindTexture(gl.TEXTURE_2D, b.texture);
      gl.uniform2f(this.#u.blur.step, 0, spread / h);
      gl.drawArrays(gl.TRIANGLES, 0, 3);
    }
    gl.bindFramebuffer(gl.FRAMEBUFFER, null);
    return a;
  }

  #compositeGlow(glow: Target): void {
    const gl = this.#gl;
    gl.useProgram(this.#composite);
    gl.activeTexture(gl.TEXTURE0);
    gl.bindTexture(gl.TEXTURE_2D, glow.texture);
    gl.uniform1i(this.#u.composite.source, 0);
    gl.uniform1f(this.#u.composite.strength, 1);
    gl.drawArrays(gl.TRIANGLES, 0, 3);
  }

  draw(s: DrawSettings): void {
    const gl = this.#gl;
    const p = s.palette;
    gl.disable(gl.BLEND);
    const glow = this.#renderGlow(s);

    gl.viewport(0, 0, gl.canvas.width, gl.canvas.height);
    gl.useProgram(this.#backdrop);
    const ub = this.#u.backdrop;
    gl.uniform3f(ub.top, ...p.top);
    gl.uniform3f(ub.bottom, ...p.bottom);
    gl.uniform3f(ub.heat, ...p.heat);
    gl.uniform1f(ub.aspect, gl.canvas.width / gl.canvas.height);
    gl.drawArrays(gl.TRIANGLES, 0, 3);

    gl.enable(gl.BLEND);
    if (!p.additive) {
      gl.blendFunc(gl.ONE, gl.ONE_MINUS_SRC_ALPHA);
      this.#compositeGlow(glow);
    }
    gl.disable(gl.BLEND);
    this.#useWax(s, 0, glow);
    this.#mesh.draw();
    gl.enable(gl.BLEND);
    if (s.wireframe) {
      gl.blendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA);
      this.#useWax(s, 0, glow, p.line);
      this.#mesh.draw(gl.LINE_STRIP);
    }
    if (p.additive) {
      gl.blendFunc(gl.ONE, gl.ONE);
      this.#compositeGlow(glow);
    }
    gl.disable(gl.BLEND);
  }

  dispose(): void {
    const gl = this.#gl;
    this.#disposeTargets();
    this.#mesh.dispose();
    gl.deleteProgram(this.#backdrop);
    gl.deleteProgram(this.#wax);
    gl.deleteProgram(this.#blur);
    gl.deleteProgram(this.#composite);
  }
}
