// WebGL 2 for the showcase: a context that follows its canvas's size, programs
// with readable errors, strips copied into per-strip vertices, and meshes whose
// buffers grow but never shrink, so that a demo that triangulates every frame
// uploads without reallocating.
//
// WebGL 2 always restarts a strip at index 0xFFFFFFFF when it draws with 32-bit
// indices, which is the layout of Result.restart.

import { RESTART_INDEX, type Result } from "@/wasm/earstrip";

export function createContext(
  canvas: HTMLCanvasElement,
  options?: WebGLContextAttributes,
): WebGL2RenderingContext {
  const gl = canvas.getContext("webgl2", { antialias: true, ...options });
  if (gl === null) throw new Error("This browser has no WebGL 2.");
  return gl;
}

/** The device pixel ratio the showcase renders at, at most 2. */
export function pixelRatio(): number {
  return Math.min(window.devicePixelRatio || 1, 2);
}

/**
 * Keeps the canvas's drawing buffer at its CSS size times pixelRatio and calls
 * onResize with the size in pixels after every change. Returns the function
 * that stops it.
 */
export function followSize(
  canvas: HTMLCanvasElement,
  onResize: (width: number, height: number) => void,
): () => void {
  const observer = new ResizeObserver((entries) => {
    const rect = entries[entries.length - 1].contentRect;
    const ratio = pixelRatio();
    const width = Math.max(1, Math.round(rect.width * ratio));
    const height = Math.max(1, Math.round(rect.height * ratio));
    if (canvas.width !== width || canvas.height !== height) {
      canvas.width = width;
      canvas.height = height;
    }
    onResize(width, height);
  });
  observer.observe(canvas);
  return () => observer.disconnect();
}

function compile(
  gl: WebGL2RenderingContext,
  type: GLenum,
  source: string,
): WebGLShader {
  const shader = gl.createShader(type);
  if (shader === null) throw new Error("The WebGL context is lost.");
  gl.shaderSource(shader, source);
  gl.compileShader(shader);
  if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
    const log = gl.getShaderInfoLog(shader);
    gl.deleteShader(shader);
    const kind = type === gl.VERTEX_SHADER ? "vertex" : "fragment";
    throw new Error(`The ${kind} shader does not compile: ${log}`);
  }
  return shader;
}

/** Compiles and links a program; the error says which shader failed and why. */
export function createProgram(
  gl: WebGL2RenderingContext,
  vertex: string,
  fragment: string,
): WebGLProgram {
  const vs = compile(gl, gl.VERTEX_SHADER, vertex);
  const fs = compile(gl, gl.FRAGMENT_SHADER, fragment);
  const program = gl.createProgram();
  gl.attachShader(program, vs);
  gl.attachShader(program, fs);
  gl.linkProgram(program);
  gl.deleteShader(vs);
  gl.deleteShader(fs);
  if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
    const log = gl.getProgramInfoLog(program);
    gl.deleteProgram(program);
    throw new Error(`The program does not link: ${log}`);
  }
  return program;
}

function grown<T extends Float32Array | Uint32Array>(
  a: T,
  n: number,
  make: (n: number) => T,
): T {
  if (a.length >= n) return a;
  const b = make(Math.max(n, Math.ceil(a.length * 1.5)));
  b.set(a);
  return b;
}

/**
 * Collects the strips of one or more results as vertices of their own: every
 * index of Result.restart gets a vertex with its position, the number of its
 * strip and a tag, so that a shader can color by strip (strips share vertices,
 * so a shared vertex could not carry one color per strip). The indices count
 * up, with RESTART_INDEX between strips and between results.
 *
 * Its arrays are reused from one reset to the next.
 */
export class StripBuilder {
  #positions = new Float32Array(0);
  #strips = new Float32Array(0);
  #tags = new Float32Array(0);
  #indices = new Uint32Array(0);
  #vertices = 0;
  #count = 0;
  #strip = 0;

  reset(): void {
    this.#vertices = 0;
    this.#count = 0;
    this.#strip = 0;
  }

  /**
   * Adds the strips of r. coords are the x, y of the polygon's vertices as
   * they went into the run; indices past them are r.extra's points. The
   * positions are coords' through scale and offset: x * scale + dx.
   */
  add(
    r: Result,
    coords: ArrayLike<number>,
    tag = 0,
    scale = 1,
    dx = 0,
    dy = 0,
  ): void {
    const restart = r.restart;
    if (restart.length === 0) return;
    const n = coords.length / 2;
    const extra = r.extra;
    let count = this.#count;
    let v = this.#vertices;
    const indices = (this.#indices = grown(
      this.#indices,
      count + restart.length + 1,
      (k) => new Uint32Array(k),
    ));
    const positions = (this.#positions = grown(
      this.#positions,
      2 * (v + restart.length),
      (k) => new Float32Array(k),
    ));
    const strips = (this.#strips = grown(
      this.#strips,
      v + restart.length,
      (k) => new Float32Array(k),
    ));
    const tags = (this.#tags = grown(
      this.#tags,
      v + restart.length,
      (k) => new Float32Array(k),
    ));
    if (count > 0) indices[count++] = RESTART_INDEX;
    let strip = this.#strip;
    for (const k of restart) {
      if (k === RESTART_INDEX) {
        indices[count++] = RESTART_INDEX;
        strip++;
        continue;
      }
      let x: number;
      let y: number;
      if (k < n) {
        x = coords[2 * k];
        y = coords[2 * k + 1];
      } else {
        x = extra[2 * (k - n)];
        y = extra[2 * (k - n) + 1];
      }
      positions[2 * v] = x * scale + dx;
      positions[2 * v + 1] = y * scale + dy;
      strips[v] = strip;
      tags[v] = tag;
      indices[count++] = v++;
    }
    this.#strip = strip + 1;
    this.#vertices = v;
    this.#count = count;
  }

  /** x, y per vertex. */
  get positions(): Float32Array {
    return this.#positions.subarray(0, 2 * this.#vertices);
  }

  /** The strip of each vertex, counting over all results since the reset. */
  get strips(): Float32Array {
    return this.#strips.subarray(0, this.#vertices);
  }

  /** The tag of each vertex, as add got it. */
  get tags(): Float32Array {
    return this.#tags.subarray(0, this.#vertices);
  }

  /** The index buffer for one TRIANGLE_STRIP draw call. */
  get indices(): Uint32Array {
    return this.#indices.subarray(0, this.#count);
  }

  /** How many strips were added since the reset. */
  get stripCount(): number {
    return this.#strip;
  }
}

interface Buffer {
  buffer: WebGLBuffer;
  bytes: number;
}

/**
 * A vertex array with float attributes and 32-bit indices. Uploads reuse the
 * buffers and only reallocate them, by half again, when the data outgrows
 * them.
 */
export class Mesh {
  readonly #gl: WebGL2RenderingContext;
  readonly #vao: WebGLVertexArrayObject;
  readonly #attributes = new Map<number, Buffer>();
  #index?: Buffer;
  #count = 0;

  constructor(gl: WebGL2RenderingContext) {
    this.#gl = gl;
    this.#vao = gl.createVertexArray();
  }

  #upload(
    target: GLenum,
    b: Buffer | undefined,
    data: Float32Array | Uint32Array,
  ): Buffer {
    const gl = this.#gl;
    b ??= { buffer: gl.createBuffer(), bytes: 0 };
    gl.bindBuffer(target, b.buffer);
    if (data.byteLength > b.bytes) {
      b.bytes = Math.max(data.byteLength, Math.ceil(b.bytes * 1.5));
      gl.bufferData(target, b.bytes, gl.DYNAMIC_DRAW);
    }
    gl.bufferSubData(target, 0, data);
    return b;
  }

  /** Uploads the attribute at location, size floats per vertex. */
  attribute(location: number, data: Float32Array, size: number): void {
    const gl = this.#gl;
    gl.bindVertexArray(this.#vao);
    const b = this.#upload(
      gl.ARRAY_BUFFER,
      this.#attributes.get(location),
      data,
    );
    this.#attributes.set(location, b);
    gl.enableVertexAttribArray(location);
    gl.vertexAttribPointer(location, size, gl.FLOAT, false, 0, 0);
    gl.bindVertexArray(null);
  }

  /** Uploads the indices; draw draws all of them by default. */
  indices(data: Uint32Array): void {
    const gl = this.#gl;
    gl.bindVertexArray(this.#vao);
    this.#index = this.#upload(gl.ELEMENT_ARRAY_BUFFER, this.#index, data);
    gl.bindVertexArray(null);
    this.#count = data.length;
  }

  /** Draws the first count indices, as triangle strips by default. */
  draw(mode: GLenum = this.#gl.TRIANGLE_STRIP, count = this.#count): void {
    if (count === 0) return;
    const gl = this.#gl;
    gl.bindVertexArray(this.#vao);
    gl.drawElements(mode, count, gl.UNSIGNED_INT, 0);
    gl.bindVertexArray(null);
  }

  dispose(): void {
    const gl = this.#gl;
    for (const b of this.#attributes.values()) gl.deleteBuffer(b.buffer);
    if (this.#index !== undefined) gl.deleteBuffer(this.#index.buffer);
    gl.deleteVertexArray(this.#vao);
  }
}
