// The world map's data: Natural Earth's countries (public domain) as
// cmd/naturalearth writes them to web/public/world-<level>.bin, projected to
// Web Mercator and quantized to a grid, x east and y north. The format is
// documented in cmd/naturalearth; internal/demo.DecodeWorld reads it in Go.

import type { Batch } from "@/wasm/earstrip";

/** Natural Earth's scales: 1:110 million and 1:50 million. */
export type Level = "110m" | "50m";

export const LEVELS: Level[] = ["110m", "50m"];

export interface Country {
  name: string;
  /** Natural Earth's MAPCOLOR7, 1 to 7: no two neighbours share one. */
  color: number;
}

export interface World {
  level: Level;
  /** The grid's size: coordinates run from 0 to size - 1. */
  size: number;
  countries: Country[];
  /** The country of each polygon. */
  polygonCountry: Uint16Array;
  /** All polygons, for triangulateMany. */
  batch: Batch;
  /** minX, minY, maxX, maxY of each polygon's outer ring. */
  bounds: Float32Array;
  /** The size of the file. */
  bytes: number;
}

/** Reads the uvarints and zig-zag varints of encoding/binary. */
class Reader {
  #data: Uint8Array;
  pos = 0;

  constructor(data: Uint8Array) {
    this.#data = data;
  }

  byte(): number {
    if (this.pos >= this.#data.length) throw new Error("world file: too short");
    return this.#data[this.pos++];
  }

  uvarint(): number {
    let v = 0;
    let scale = 1;
    for (;;) {
      const b = this.byte();
      v += (b & 0x7f) * scale;
      if (b < 0x80) return v;
      scale *= 128;
      if (scale > 2 ** 49) throw new Error("world file: varint too long");
    }
  }

  varint(): number {
    const u = this.uvarint();
    // Zig-zag: 0, -1, 1, -2, …; the values are small enough for floats.
    return u % 2 === 0 ? u / 2 : -(u + 1) / 2;
  }

  bytes(n: number): Uint8Array {
    if (this.pos + n > this.#data.length) {
      throw new Error("world file: too short");
    }
    const out = this.#data.subarray(this.pos, this.pos + n);
    this.pos += n;
    return out;
  }

  get left(): number {
    return this.#data.length - this.pos;
  }
}

export function decodeWorld(level: Level, data: Uint8Array): World {
  const r = new Reader(data);
  const magic = new TextDecoder().decode(r.bytes(4));
  if (magic !== "ESNE") throw new Error("not a world file");
  const version = r.byte();
  if (version !== 1) throw new Error(`world file version ${version}, want 1`);
  const bits = r.byte();

  const names = new TextDecoder();
  const countries: Country[] = [];
  for (let i = r.uvarint(); i > 0; i--) {
    const name = names.decode(r.bytes(r.uvarint()));
    countries.push({ name, color: r.byte() });
  }

  const polygons = r.uvarint();
  const polygonCountry = new Uint16Array(polygons);
  const polygonStarts = new Uint32Array(polygons);
  const ringSizes: number[] = [];
  for (let p = 0; p < polygons; p++) {
    polygonCountry[p] = r.uvarint();
    polygonStarts[p] = ringSizes.length;
    for (let k = r.uvarint(); k > 0; k--) ringSizes.push(r.uvarint());
  }

  const ringStarts = new Uint32Array(ringSizes.length);
  let vertices = 0;
  ringSizes.forEach((n, i) => {
    ringStarts[i] = vertices;
    vertices += n;
  });
  const coords = new Float64Array(2 * vertices);
  let x = 0;
  let y = 0;
  for (let i = 0; i < vertices; i++) {
    x += r.varint();
    y += r.varint();
    coords[2 * i] = x;
    coords[2 * i + 1] = y;
  }
  if (r.left > 0) throw new Error(`world file: ${r.left} bytes too many`);

  const bounds = new Float32Array(4 * polygons);
  for (let p = 0; p < polygons; p++) {
    const first = ringStarts[polygonStarts[p]];
    const ring = polygonStarts[p] + 1;
    const end = ring < ringStarts.length ? ringStarts[ring] : vertices;
    let x0 = Infinity;
    let y0 = Infinity;
    let x1 = -Infinity;
    let y1 = -Infinity;
    for (let i = first; i < end; i++) {
      x0 = Math.min(x0, coords[2 * i]);
      y0 = Math.min(y0, coords[2 * i + 1]);
      x1 = Math.max(x1, coords[2 * i]);
      y1 = Math.max(y1, coords[2 * i + 1]);
    }
    bounds.set([x0, y0, x1, y1], 4 * p);
  }

  return {
    level,
    size: 2 ** bits,
    countries,
    polygonCountry,
    batch: { coords, ringStarts, polygonStarts },
    bounds,
    bytes: data.length,
  };
}

const loading = new Map<Level, Promise<World>>();

/** Fetches and decodes a level once; later calls get the same promise. */
export function loadWorld(level: Level): Promise<World> {
  let p = loading.get(level);
  if (p === undefined) {
    p = (async () => {
      const url = `${import.meta.env.BASE_URL}world-${level}.bin`;
      const response = await fetch(url);
      if (!response.ok) {
        throw new Error(`${url}: HTTP ${response.status}`);
      }
      return decodeWorld(level, new Uint8Array(await response.arrayBuffer()));
    })();
    p.catch(() => loading.delete(level));
    loading.set(level, p);
  }
  return p;
}
