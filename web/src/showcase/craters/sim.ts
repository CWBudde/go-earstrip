// Everything that moves in the crater sandbox, written by hand: balls that
// roll on the tiles' triangles and bump into each other, the explosions'
// sparks, dirt and smoke, their flashes, the charred rims of the craters, and
// a short attract mode that blasts a few craters by itself.

import type { FrameStats } from "@/showcase/stats";
import type { Earstrip } from "@/wasm/earstrip";

import {
  makeTerrain,
  random,
  surfaceY,
  WORLD_H,
  WORLD_W,
  type Terrain,
} from "./terrain";
import { Contact, HOLE_CAP, World } from "./world";

export { HOLE_CAP };

export const MAX_BALLS = 90;
export const BALL_R = 8;
const GRAVITY = 900;
const MAX_SPEED = 900;
const SUBSTEPS = 4;
/** The bounce of a ball off the ground and off another ball. */
const RESTITUTION = 0.3;
const BALL_RESTITUTION = 0.5;

export const MAX_PARTICLES = 1600;
/** The floats per particle in Sim.particleData: x, y, size, kind, r, g, b, a. */
export const PARTICLE_FLOATS = 8;
const SPARK = 0;
const DEBRIS = 1;
const SMOKE = 2;

export const MAX_LIGHTS = 6;
/** How long a flash lasts. */
export const LIGHT_SECONDS = 0.55;

/** The scorch map's texels: one per SCORCH_CELL world units. */
export const SCORCH_CELL = 4;
export const SCORCH_W = WORLD_W / SCORCH_CELL;
export const SCORCH_H = WORLD_H / SCORCH_CELL;

/** A blast the attract mode fires, at its time in seconds. */
interface Scripted {
  t: number;
  x: number;
  y: number;
  r: number;
}

export interface Info {
  tiles: number;
  dirty: number;
  craters: number;
  maxHoles: number;
  balls: number;
  statuses: ReturnType<World["statuses"]>;
}

export class Sim {
  terrain: Terrain;
  readonly world: World;

  // Balls, struct of arrays.
  balls = 0;
  readonly bx = new Float32Array(MAX_BALLS);
  readonly by = new Float32Array(MAX_BALLS);
  readonly bvx = new Float32Array(MAX_BALLS);
  readonly bvy = new Float32Array(MAX_BALLS);
  readonly spin = new Float32Array(MAX_BALLS);
  readonly angle = new Float32Array(MAX_BALLS);
  readonly hue = new Float32Array(MAX_BALLS);
  #ballCount = 0;

  // Particles, struct of arrays.
  particles = 0;
  readonly #px = new Float32Array(MAX_PARTICLES);
  readonly #py = new Float32Array(MAX_PARTICLES);
  readonly #pvx = new Float32Array(MAX_PARTICLES);
  readonly #pvy = new Float32Array(MAX_PARTICLES);
  readonly #life = new Float32Array(MAX_PARTICLES);
  readonly #maxLife = new Float32Array(MAX_PARTICLES);
  readonly #size = new Float32Array(MAX_PARTICLES);
  readonly #kind = new Uint8Array(MAX_PARTICLES);
  readonly #stuck = new Uint8Array(MAX_PARTICLES);
  readonly #color = new Float32Array(3 * MAX_PARTICLES);
  /** What the renderer uploads: PARTICLE_FLOATS per live particle. */
  readonly particleData = new Float32Array(PARTICLE_FLOATS * MAX_PARTICLES);

  /** The flashes: x, y, radius and age in seconds each; age < 0 is unused. */
  readonly lights = new Float32Array(4 * MAX_LIGHTS);
  /** The screen shake's strength in world units, decaying. */
  shake = 0;

  /** How charred the ground is, 0..255, at SCORCH_CELL resolution. */
  readonly scorch = new Uint8Array(SCORCH_W * SCORCH_H);
  scorchDirty = true;

  #script: Scripted[] = [];
  /** When the attract mode drops its balls; negative once it has. */
  #dropAt = -1;
  #clock = 0;
  #rand: () => number;
  readonly #out = new Float64Array(3);

  constructor(seed: number) {
    this.terrain = makeTerrain(seed);
    this.world = new World(this.terrain);
    this.#rand = random(seed ^ 0x5bd1e995);
    this.reset(seed);
  }

  /** Starts over with the landscape of seed and replays the attract mode. */
  reset(seed: number): void {
    this.terrain = makeTerrain(seed);
    this.world.reset(this.terrain);
    this.#rand = random(seed ^ 0x5bd1e995);
    this.balls = 0;
    this.particles = 0;
    this.lights.fill(-1);
    this.scorch.fill(0);
    this.scorchDirty = true;
    this.shake = 0;
    this.#clock = 0;
    this.#dropAt = 0.15;
    this.#script = this.#attract();
  }

  /**
   * The attract mode: a few blasts that break the surface and overlap, a
   * shaft dug down into the ground and some balls dropped onto it all, so
   * that a visitor sees what the sandbox does before touching it.
   */
  #attract(): Scripted[] {
    const h = this.terrain.heights;
    const at = (u: number, depth: number) => ({
      x: u * WORLD_W,
      y: surfaceY(h, u * WORLD_W) + depth,
    });
    const s: Scripted[] = [];
    const add = (t: number, u: number, depth: number, r: number) =>
      s.push({ t, ...at(u, depth), r });
    add(0.35, 0.6, 8, 40);
    add(0.6, 0.645, 14, 36);
    add(0.85, 0.69, 6, 42);
    for (let i = 0; i < 9; i++) add(1.15 + i * 0.06, 0.3 + i * 0.006, 4 + i * 22, 30);
    add(1.9, 0.47, 30, 58);
    add(2.25, 0.84, 10, 38);
    add(2.45, 0.865, 30, 34);
    add(2.7, 0.52, 70, 44);
    return s;
  }

  /** Blasts a crater at (x, y) with its explosion. */
  blast(x: number, y: number, r: number, motion = true): void {
    if (this.world.blast(x, y, r)) this.#char(x, y, r);
    if (!motion) return;
    this.#explode(x, y, r);
    let slot = 0;
    for (let i = 0; i < MAX_LIGHTS; i++) {
      const age = this.lights[4 * i + 3];
      if (age < 0 || age > this.lights[4 * slot + 3]) slot = i;
      if (age < 0) break;
    }
    this.lights.set([x, y, r, 0], 4 * slot);
    this.shake = Math.min(14, this.shake + r * 0.12);
  }

  /** Paints the charred rim around a crater into the scorch map. */
  #char(x: number, y: number, r: number): void {
    const reach = r * 1.7;
    const i0 = Math.max(0, Math.floor((x - reach) / SCORCH_CELL));
    const i1 = Math.min(SCORCH_W - 1, Math.ceil((x + reach) / SCORCH_CELL));
    const j0 = Math.max(0, Math.floor((y - reach) / SCORCH_CELL));
    const j1 = Math.min(SCORCH_H - 1, Math.ceil((y + reach) / SCORCH_CELL));
    for (let j = j0; j <= j1; j++) {
      for (let i = i0; i <= i1; i++) {
        const d = Math.hypot((i + 0.5) * SCORCH_CELL - x, (j + 0.5) * SCORCH_CELL - y) / r;
        if (d >= 1.7) continue;
        const v = d < 1 ? 1 : 1 - (d - 1) / 0.7;
        const n = 0.7 + 0.3 * this.#rand();
        const k = j * SCORCH_W + i;
        this.scorch[k] = Math.max(this.scorch[k], Math.round(255 * v * v * n));
      }
    }
    this.scorchDirty = true;
  }

  #spawn(
    kind: number,
    x: number,
    y: number,
    vx: number,
    vy: number,
    life: number,
    size: number,
    r: number,
    g: number,
    b: number,
  ): void {
    if (this.particles >= MAX_PARTICLES) return;
    const i = this.particles++;
    this.#px[i] = x;
    this.#py[i] = y;
    this.#pvx[i] = vx;
    this.#pvy[i] = vy;
    this.#life[i] = life;
    this.#maxLife[i] = life;
    this.#size[i] = size;
    this.#kind[i] = kind;
    this.#stuck[i] = 0;
    this.#color[3 * i] = r;
    this.#color[3 * i + 1] = g;
    this.#color[3 * i + 2] = b;
  }

  #explode(x: number, y: number, r: number): void {
    const rand = this.#rand;
    const depth = y - surfaceY(this.terrain.heights, x);
    const sparks = 26 + Math.round(r * 0.5);
    for (let i = 0; i < sparks; i++) {
      const a = rand() * 2 * Math.PI;
      const v = (180 + 520 * rand()) * (r / 40);
      const hot = rand();
      this.#spawn(
        SPARK,
        x + Math.cos(a) * r * 0.3,
        y + Math.sin(a) * r * 0.3,
        Math.cos(a) * v,
        Math.sin(a) * v - 120,
        0.25 + 0.45 * rand(),
        3 + 4 * rand(),
        1,
        0.45 + 0.45 * hot,
        0.12 + 0.3 * hot * hot,
      );
    }
    const chunks = 16 + Math.round(r * 0.3);
    for (let i = 0; i < chunks; i++) {
      const a = -Math.PI / 2 + (rand() - 0.5) * 2.6;
      const v = 160 + 380 * rand();
      const grass = depth < 16 && rand() < 0.5;
      const k = 0.7 + 0.5 * rand();
      this.#spawn(
        DEBRIS,
        x + (rand() - 0.5) * r,
        y + (rand() - 0.5) * r,
        Math.cos(a) * v,
        Math.sin(a) * v,
        1.1 + 0.9 * rand(),
        2.5 + 3.5 * rand(),
        (grass ? 0.3 : 0.45) * k,
        (grass ? 0.55 : 0.29) * k,
        (grass ? 0.2 : 0.16) * k,
      );
    }
    for (let i = 0; i < 9; i++) {
      const a = rand() * 2 * Math.PI;
      const v = 20 + 50 * rand();
      const g = 0.35 + 0.25 * rand();
      this.#spawn(
        SMOKE,
        x + Math.cos(a) * r * 0.4,
        y + Math.sin(a) * r * 0.4,
        Math.cos(a) * v,
        Math.sin(a) * v - 30,
        1.0 + 0.9 * rand(),
        r * (0.45 + 0.3 * rand()),
        g,
        g * 0.96,
        g * 0.92,
      );
    }
  }

  /** Drops count balls from above the world, the oldest leaving past the cap. */
  dropBalls(count: number): void {
    const rand = this.#rand;
    for (let k = 0; k < count; k++) {
      let i = this.balls;
      if (i >= MAX_BALLS) {
        // Past the cap the oldest ball makes room: shift the rest down.
        for (const a of [this.bx, this.by, this.bvx, this.bvy, this.spin, this.angle, this.hue])
          a.copyWithin(0, 1, MAX_BALLS);
        i = MAX_BALLS - 1;
      } else {
        this.balls++;
      }
      this.bx[i] = WORLD_W * (0.08 + 0.84 * rand());
      this.by[i] = -BALL_R - 60 * rand() - k * 6;
      this.bvx[i] = (rand() - 0.5) * 80;
      this.bvy[i] = 0;
      this.spin[i] = 0;
      this.angle[i] = rand() * 6.28;
      this.hue[i] = this.#ballCount++ % 8;
    }
  }

  /** Advances by dt seconds; triangulates the tiles that changed. */
  step(dt: number, api: Earstrip, stats: FrameStats): void {
    this.#clock += dt;
    if (this.#dropAt >= 0 && this.#clock >= this.#dropAt) {
      this.#dropAt = -1;
      this.dropBalls(16);
    }
    while (this.#script.length > 0 && this.#script[0].t <= this.#clock) {
      const b = this.#script.shift()!;
      this.blast(b.x, b.y, b.r);
    }
    this.world.update(api, stats);
    for (const t of this.world.tiles) t.flash = Math.max(0, t.flash - dt / 0.7);
    if (dt <= 0) return;
    this.#stepBalls(dt);
    this.#stepParticles(dt);
    for (let i = 0; i < MAX_LIGHTS; i++) {
      const k = 4 * i + 3;
      if (this.lights[k] >= 0) {
        this.lights[k] += dt;
        if (this.lights[k] > LIGHT_SECONDS) this.lights[k] = -1;
      }
    }
    this.shake *= Math.exp(-dt * 9);
    if (this.shake < 0.05) this.shake = 0;
  }

  /** Triangulates the dirty tiles without moving anything, for a pause. */
  settle(api: Earstrip, stats: FrameStats): void {
    this.world.update(api, stats);
  }

  #stepBalls(dt: number): void {
    const h = dt / SUBSTEPS;
    const { bx, by, bvx, bvy, spin, angle, world } = this;
    const out = this.#out;
    const R = BALL_R;
    for (let s = 0; s < SUBSTEPS; s++) {
      for (let i = 0; i < this.balls; i++) {
        let vx = bvx[i];
        let vy = bvy[i] + GRAVITY * h;
        const speed = Math.hypot(vx, vy);
        if (speed > MAX_SPEED) {
          vx *= MAX_SPEED / speed;
          vy *= MAX_SPEED / speed;
        }
        const px = bx[i];
        const py = by[i];
        let x = px + vx * h;
        let y = py + vy * h;
        if (x < R) {
          x = R;
          vx = Math.abs(vx) * 0.5;
        } else if (x > WORLD_W - R) {
          x = WORLD_W - R;
          vx = -Math.abs(vx) * 0.5;
        }
        let rolling = false;
        for (let iter = 0; iter < 2; iter++) {
          const c = world.contact(x, y, R, out);
          if (c === Contact.None) break;
          if (c === Contact.Inside) {
            x = px;
            y = py;
            vx *= -0.3;
            vy *= -0.3;
            break;
          }
          const d = out[2];
          if (d < 1e-6) break;
          const nx = (x - out[0]) / d;
          const ny = (y - out[1]) / d;
          x = out[0] + nx * R;
          y = out[1] + ny * R;
          const vn = vx * nx + vy * ny;
          if (vn < 0) {
            vx -= (1 + RESTITUTION) * vn * nx;
            vy -= (1 + RESTITUTION) * vn * ny;
          }
          // Rolling friction along the ground, and the spin that rolling gives.
          const vt = -vx * ny + vy * nx;
          vx += ny * vt * 0.004;
          vy -= nx * vt * 0.004;
          spin[i] = vt / R;
          rolling = true;
        }
        if (!rolling) spin[i] *= 0.999;
        angle[i] += spin[i] * h;
        bx[i] = x;
        by[i] = y;
        bvx[i] = vx;
        bvy[i] = vy;
      }
      // Balls push each other apart and trade the speed along the push.
      for (let i = 0; i < this.balls; i++) {
        for (let j = i + 1; j < this.balls; j++) {
          const dx = bx[j] - bx[i];
          const dy = by[j] - by[i];
          const d2 = dx * dx + dy * dy;
          if (d2 >= 4 * R * R || d2 < 1e-9) continue;
          const d = Math.sqrt(d2);
          const nx = dx / d;
          const ny = dy / d;
          const push = (2 * R - d) / 2;
          bx[i] -= nx * push;
          by[i] -= ny * push;
          bx[j] += nx * push;
          by[j] += ny * push;
          const rel = (bvx[j] - bvx[i]) * nx + (bvy[j] - bvy[i]) * ny;
          if (rel < 0) {
            const k = ((1 + BALL_RESTITUTION) * rel) / 2;
            bvx[i] += k * nx;
            bvy[i] += k * ny;
            bvx[j] -= k * nx;
            bvy[j] -= k * ny;
          }
        }
      }
    }
    // Balls that fell out of the world are gone.
    let n = 0;
    for (let i = 0; i < this.balls; i++) {
      if (by[i] > WORLD_H + 40) continue;
      if (n !== i) {
        for (const a of [bx, by, bvx, bvy, spin, angle, this.hue]) a[n] = a[i];
      }
      n++;
    }
    this.balls = n;
  }

  #stepParticles(dt: number): void {
    const out = this.#out;
    const data = this.particleData;
    let n = 0;
    for (let i = 0; i < this.particles; i++) {
      const life = this.#life[i] - dt;
      if (life <= 0) continue;
      const kind = this.#kind[i];
      let x = this.#px[i];
      let y = this.#py[i];
      let vx = this.#pvx[i];
      let vy = this.#pvy[i];
      let size = this.#size[i];
      if (kind === SPARK) {
        vy += 400 * dt;
        const drag = Math.exp(-dt * 3);
        vx *= drag;
        vy *= drag;
      } else if (kind === DEBRIS) {
        if (this.#stuck[i] === 0) {
          vy += GRAVITY * dt;
          if (this.world.contact(x, y, size * 0.5, out) !== Contact.None) {
            this.#stuck[i] = 1;
            vx = 0;
            vy = 0;
          }
        }
      } else {
        const drag = Math.exp(-dt * 1.5);
        vx *= drag;
        vy = vy * drag - 25 * dt;
        size += dt * 22;
      }
      x += vx * dt;
      y += vy * dt;
      // Keep the particle, compacted to slot n.
      this.#px[n] = x;
      this.#py[n] = y;
      this.#pvx[n] = vx;
      this.#pvy[n] = vy;
      this.#life[n] = life;
      this.#maxLife[n] = this.#maxLife[i];
      this.#size[n] = size;
      this.#kind[n] = kind;
      this.#stuck[n] = this.#stuck[i];
      this.#color[3 * n] = this.#color[3 * i];
      this.#color[3 * n + 1] = this.#color[3 * i + 1];
      this.#color[3 * n + 2] = this.#color[3 * i + 2];
      const f = life / this.#maxLife[n];
      const o = PARTICLE_FLOATS * n;
      data[o] = x;
      data[o + 1] = y;
      data[o + 2] = kind === SPARK ? size * (0.4 + 0.6 * f) : size;
      data[o + 3] = kind;
      data[o + 4] = this.#color[3 * n];
      data[o + 5] = this.#color[3 * n + 1];
      data[o + 6] = this.#color[3 * n + 2];
      data[o + 7] =
        kind === SMOKE ? 0.45 * f * Math.min(1, (1 - f) * 6) : kind === DEBRIS ? Math.min(1, f * 3) : f;
      n++;
    }
    this.particles = n;
  }

  info(): Info {
    let tiles = 0;
    let maxHoles = 0;
    for (const t of this.world.tiles) {
      if (t.status !== undefined) tiles++;
      maxHoles = Math.max(maxHoles, t.holes);
    }
    return {
      tiles,
      dirty: this.world.lastDirty,
      craters: this.world.craters.length,
      maxHoles,
      balls: this.balls,
      statuses: this.world.statuses(),
    };
  }
}
