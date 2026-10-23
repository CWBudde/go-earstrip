// The lava lamp: metaballs rise and sink, merge and split. Every frame the
// marching squares trace the contours of a few iso levels of their field, an
// even/odd containment tree turns the contours into the bands between the
// levels, polygons with holes, and each polygon is triangulated by the wasm
// build and drawn straight from its strips, with a glow made of the same
// strips.

import { Pause, Play } from "lucide-react";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type PointerEvent,
  type RefObject,
} from "react";

import { Toggle } from "@/components/Toggle";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { followSize, StripBuilder } from "@/lib/gl";
import { useDark, useReducedMotion } from "@/lib/theme";
import { GLCanvas, type GLSetup } from "@/showcase/GLCanvas";
import { Hud, HudRow } from "@/showcase/Hud";
import { ShowcaseLayout } from "@/showcase/Layout";
import { useAnimationFrame } from "@/showcase/loop";
import type { ShowcaseProps } from "@/showcase/registry";
import { DEFAULT_OPTIONS, FrameStats } from "@/showcase/stats";
import type { Options } from "@/wasm/earstrip";

import { Contours } from "./contours";
import { Field, MAX_BLOBS } from "./field";
import { LavaRenderer, type Palette } from "./renderer";

/** The iso levels by how many there are, outer to inner. */
const LEVELS: Record<number, number[]> = {
  2: [1, 2.2],
  3: [1, 1.8, 3.4],
  4: [1, 1.5, 2.3, 4],
};

/** With the constrained triangulation off, touching contours show what the ears do alone. */
const EARS_ONLY: Options = { ...DEFAULT_OPTIONS, constrained: false };

const DARK: Palette = {
  top: [0.05, 0.025, 0.1],
  bottom: [0.13, 0.03, 0.09],
  heat: [0.22, 0.06, 0.03],
  bands: [
    [0.5, 0.04, 0.24],
    [0.92, 0.2, 0.12],
    [1.0, 0.55, 0.14],
    [1.0, 0.85, 0.5],
  ],
  line: [1, 0.95, 0.85, 0.4],
  additive: true,
  glow: 0.6,
  stripShade: [0.55, 0.8],
  shade: [0.72, 0.75],
  dim: [0.04, 0.02, 0.06, 0.55],
};

const LIGHT: Palette = {
  top: [0.99, 0.965, 0.93],
  bottom: [0.99, 0.87, 0.8],
  heat: [0.0, -0.03, -0.06],
  bands: [
    [0.99, 0.55, 0.45],
    [0.93, 0.25, 0.4],
    [0.6, 0.1, 0.4],
    [0.36, 0.05, 0.3],
  ],
  line: [0.35, 0.03, 0.22, 0.55],
  additive: false,
  glow: 0.7,
  stripShade: [0.72, 0.5],
  shade: [1.12, -0.45],
  dim: [1, 0.97, 0.95, 0.6],
};

/** What the HUD's own rows show, written by the frame and read by LavaRows. */
interface Info {
  levels: number[];
  polygons: number;
  contours: number;
  lakes: number;
  cols: number;
  rows: number;
}

/** The lamp's rows of the HUD; they poll info as the HUD polls FrameStats. */
function LavaRows({ info }: { info: RefObject<Info> }) {
  const [s, setS] = useState<Info>(() => ({ ...info.current }));
  useEffect(() => {
    const id = setInterval(() => setS({ ...info.current }), 250);
    return () => clearInterval(id);
  }, [info]);
  return (
    <>
      <HudRow label="Iso levels">{s.levels.join(" · ")}</HudRow>
      <HudRow label="Polygons">{s.polygons}</HudRow>
      <HudRow label="Contours">
        {s.contours}{" "}
        <span className="text-muted-foreground">({s.lakes} lakes)</span>
      </HudRow>
      <HudRow label="Grid">
        {s.cols}×{s.rows}
      </HudRow>
    </>
  );
}

interface Scene {
  renderer: LavaRenderer;
  width: number;
  height: number;
}

export function LavaLamp({ api, active }: ShowcaseProps) {
  const dark = useDark();
  const reduced = useReducedMotion();
  const [playing, setPlaying] = useState(!reduced);
  const [count, setCount] = useState(8);
  const [speed, setSpeed] = useState(1);
  const [cols, setCols] = useState(160);
  const [levels, setLevels] = useState(3);
  const [stripColors, setStripColors] = useState(false);
  const [wireframe, setWireframe] = useState(false);
  const [constrained, setConstrained] = useState(true);

  const scene = useRef<Scene>(undefined);
  // Lamp time in seconds; it starts where the blobs are spread over the height.
  const time = useRef(244);
  const pointer = useRef<{ x: number; y: number }>(undefined);
  const info = useRef<Info>({
    levels: LEVELS[3],
    polygons: 0,
    contours: 0,
    lakes: 0,
    cols: 0,
    rows: 0,
  });
  const stats = useMemo(() => new FrameStats(), []);
  const builder = useMemo(() => new StripBuilder(), []);
  const field = useMemo(() => new Field(), []);
  const contours = useMemo(() => new Contours(), []);

  // The latest settings for render, which the loop, the resize and the
  // pointer call.
  const settings = useRef({
    count,
    speed,
    cols,
    levels,
    stripColors,
    wireframe,
    constrained,
    dark,
  });
  useEffect(() => {
    settings.current = {
      count,
      speed,
      cols,
      levels,
      stripColors,
      wireframe,
      constrained,
      dark,
    };
  });

  /** The grid's rows for the stage's shape: square cells. */
  function gridRows(s: Scene, c: number): number {
    return Math.max(8, Math.round((c * s.height) / s.width));
  }

  function render() {
    const s = scene.current;
    if (s === undefined) return;
    const set = settings.current;
    const rows = gridRows(s, set.cols);
    field.resize(set.cols, rows);
    field.sample(time.current, set.count, pointer.current);

    const isos = LEVELS[set.levels];
    contours.reset();
    for (let k = 0; k < isos.length; k++) contours.march(field, isos[k], k);

    const options = set.constrained ? DEFAULT_OPTIONS : EARS_ONLY;
    builder.reset();
    let polygons = 0;
    for (let k = 0; k < isos.length; k++) {
      const hi = k + 1 < isos.length ? k + 1 : -1;
      contours.assemble(k, hi, (coords, holeStarts) => {
        const r = stats.run(api, { coords, holeStarts }, options);
        builder.add(r, coords, k);
        polygons++;
      });
    }
    s.renderer.upload(builder);

    let lakes = 0;
    for (const a of contours.area) if (a < 0) lakes++;
    const i = info.current;
    i.levels = isos;
    i.polygons = polygons;
    i.contours = contours.count;
    i.lakes = lakes;
    i.cols = set.cols;
    i.rows = rows;

    s.renderer.draw({
      palette: set.dark ? DARK : LIGHT,
      bands: isos.length,
      stripColors: set.stripColors,
      wireframe: set.wireframe,
      cols: set.cols,
      rows,
    });
  }
  const renderRef = useRef(render);
  useEffect(() => {
    renderRef.current = render;
  });

  const setup: GLSetup = (gl, canvas) => {
    const renderer = new LavaRenderer(gl);
    const s: Scene = { renderer, width: canvas.width, height: canvas.height };
    scene.current = s;
    const stop = followSize(canvas, (width, height) => {
      s.width = width;
      s.height = height;
      renderRef.current();
    });
    return () => {
      stop();
      renderer.dispose();
      scene.current = undefined;
    };
  };

  const running = active && playing;
  useEffect(() => {
    if (!running) {
      stats.pause();
      renderRef.current();
    }
  }, [
    running,
    count,
    cols,
    levels,
    stripColors,
    wireframe,
    constrained,
    dark,
    stats,
  ]);

  useAnimationFrame(running, (now, dt) => {
    time.current += (dt / 1000) * settings.current.speed;
    stats.begin(now);
    render();
    stats.end();
  });

  /** Puts the pointer's heat where the event is, in cells; redraws when paused. */
  function heat(e: PointerEvent<HTMLCanvasElement>) {
    const s = scene.current;
    if (s === undefined) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const c = settings.current.cols;
    pointer.current = {
      x: ((e.clientX - rect.left) / rect.width) * c,
      y: (1 - (e.clientY - rect.top) / rect.height) * gridRows(s, c),
    };
    if (!running) renderRef.current();
  }
  function cool() {
    pointer.current = undefined;
    if (!running) renderRef.current();
  }

  return (
    <ShowcaseLayout
      stage={
        <GLCanvas
          setup={setup}
          className="cursor-crosshair"
          aria-label="Lava lamp: drag to add a blob of heat"
          onPointerDown={(e) => {
            e.currentTarget.setPointerCapture(e.pointerId);
            heat(e);
          }}
          onPointerMove={(e) => {
            if (pointer.current !== undefined) heat(e);
          }}
          onPointerUp={cool}
          onPointerCancel={cool}
        />
      }
      hud={
        <Hud stats={stats}>
          <LavaRows info={info} />
        </Hud>
      }
      controls={
        <Card>
          <CardHeader>
            <CardTitle>Lava lamp</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-5">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPlaying((p) => !p)}
            >
              {playing ? <Pause /> : <Play />} {playing ? "Pause" : "Play"}
            </Button>
            <div className="flex flex-col gap-2">
              <Label htmlFor="lava-count">Blobs: {count}</Label>
              <Slider
                id="lava-count"
                min={1}
                max={MAX_BLOBS}
                value={[count]}
                onValueChange={([v]) => setCount(v)}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="lava-speed">Speed: {speed.toFixed(1)}×</Label>
              <Slider
                id="lava-speed"
                min={0.2}
                max={4}
                step={0.1}
                value={[speed]}
                onValueChange={([v]) => setSpeed(v)}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="lava-cols">Grid: {cols} columns</Label>
              <Slider
                id="lava-cols"
                min={40}
                max={320}
                step={10}
                value={[cols]}
                onValueChange={([v]) => setCols(v)}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="lava-levels">Iso levels: {levels}</Label>
              <Slider
                id="lava-levels"
                min={2}
                max={4}
                value={[levels]}
                onValueChange={([v]) => setLevels(v)}
              />
            </div>
            <Toggle
              id="lava-strips"
              label="Strip colors"
              hint="A shade per triangle strip; off: one color per level."
              checked={stripColors}
              onChange={setStripColors}
            />
            <Toggle
              id="lava-wireframe"
              label="Wireframe"
              hint="The restart buffer as line strips: the zig-zag of each strip."
              checked={wireframe}
              onChange={setWireframe}
            />
            <Toggle
              id="lava-constrained"
              label="Constrained"
              hint="Off: the ears alone, without the fallback. Marching squares contours never cross, so it stays Exact."
              checked={constrained}
              onChange={setConstrained}
            />
            <p className="text-xs text-muted-foreground">
              Drag on the lamp to add a blob of heat.
            </p>
          </CardContent>
        </Card>
      }
    />
  );
}
