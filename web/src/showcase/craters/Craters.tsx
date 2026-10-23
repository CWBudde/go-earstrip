// The crater sandbox: a side view of hills and caves, cut into 8 × 4 tiles.
// A click or tap blasts a crater, a drag digs; every crater is a hole of the
// tiles it touches, and the tiles whose holes changed are triangulated again
// in the frame. Balls roll on the triangles into the craters and caves.

import { CircleDot, Pause, Play, RotateCcw, Shuffle } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type PointerEvent } from "react";

import { Toggle } from "@/components/Toggle";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { followSize } from "@/lib/gl";
import { useDark, useReducedMotion } from "@/lib/theme";
import { GLCanvas, type GLSetup } from "@/showcase/GLCanvas";
import { Hud, HudRow } from "@/showcase/Hud";
import { ShowcaseLayout } from "@/showcase/Layout";
import { useAnimationFrame } from "@/showcase/loop";
import type { ShowcaseProps } from "@/showcase/registry";
import { FrameStats } from "@/showcase/stats";
import type { Status } from "@/wasm/earstrip";

import { Renderer } from "./render";
import { HOLE_CAP, MAX_BALLS, Sim, type Info } from "./sim";
import { COLS, ROWS, WORLD_H, WORLD_W } from "./terrain";

/** The landscape the page opens with; New world draws another. */
const FIRST_SEED = 7;

/** The short names of the statuses in the HUD's tile row. */
const SHORT: Record<Status, string> = {
  Exact: "exact",
  Constrained: "CDT",
  HolesLeftOut: "holes out",
  Approximate: "approx.",
  Refused: "refused",
};

/** The sandbox's own HUD rows, read four times a second like the Hud's. */
function CraterRows({ sim }: { sim: Sim }) {
  const [info, setInfo] = useState<Info>(() => sim.info());
  useEffect(() => {
    const id = setInterval(() => setInfo(sim.info()), 250);
    return () => clearInterval(id);
  }, [sim]);
  const statuses = (Object.keys(info.statuses) as Status[])
    .map((k) => `${info.statuses[k]} ${SHORT[k]}`)
    .join(" · ");
  return (
    <>
      <HudRow label="Tiles">
        {info.tiles}/{COLS * ROWS}{" "}
        <span className="text-muted-foreground">({info.dirty} redone)</span>
      </HudRow>
      <HudRow label="Craters">{info.craters}</HudRow>
      <HudRow label="Tile runs">{statuses || "–"}</HudRow>
      <HudRow label="Holes/tile">
        {info.maxHoles}{" "}
        <span className="text-muted-foreground">max of {HOLE_CAP}</span>
      </HudRow>
      <HudRow label="Balls">
        {info.balls}/{MAX_BALLS}
      </HudRow>
    </>
  );
}

export function Craters({ api, active }: ShowcaseProps) {
  const dark = useDark();
  const reduced = useReducedMotion();
  const [playing, setPlaying] = useState(!reduced);
  const [radius, setRadius] = useState(40);
  const [strips, setStrips] = useState(false);
  const [wireframe, setWireframe] = useState(false);
  const [seed, setSeed] = useState(FIRST_SEED);
  const stats = useMemo(() => new FrameStats(), []);
  const sim = useMemo(() => new Sim(FIRST_SEED), []);
  const renderer = useRef<Renderer>(undefined);
  const time = useRef(0);
  /** The pointer's last blast while it is down, for digging by dragging. */
  const drag = useRef<{ x: number; y: number } | undefined>(undefined);

  const running = active && playing;
  // The latest settings for paint and the pointer, which run outside React.
  const settings = useRef({
    radius,
    strips,
    wireframe,
    dark,
    running,
    reduced,
  });
  useEffect(() => {
    settings.current = { radius, strips, wireframe, dark, running, reduced };
  });

  function paint() {
    const s = settings.current;
    renderer.current?.draw(sim, {
      dark: s.dark,
      strips: s.strips,
      wireframe: s.wireframe,
      // No screen shake for those who asked for less motion.
      shake: !s.reduced,
      time: time.current,
    });
  }
  const paintRef = useRef(paint);
  useEffect(() => {
    paintRef.current = paint;
  });

  /** Triangulates what changed and draws once, for when the loop is off. */
  function still() {
    stats.begin(performance.now());
    sim.settle(api, stats);
    stats.end();
    paintRef.current();
  }
  const stillRef = useRef(still);
  useEffect(() => {
    stillRef.current = still;
  });

  const setup: GLSetup = (gl, canvas) => {
    const r = new Renderer(gl);
    renderer.current = r;
    const stop = followSize(canvas, () => paintRef.current());
    return () => {
      stop();
      r.dispose();
      renderer.current = undefined;
    };
  };

  useEffect(() => {
    if (!running) {
      stats.pause();
      stillRef.current();
    }
  }, [running, strips, wireframe, dark, stats]);

  useAnimationFrame(running, (now, dt) => {
    time.current += dt / 1000;
    stats.begin(now);
    sim.step(dt / 1000, api, stats);
    paint();
    stats.end();
  });

  /** The world point under the pointer, with the renderer's view. */
  function toWorld(e: PointerEvent<HTMLCanvasElement>): [number, number] {
    const rect = e.currentTarget.getBoundingClientRect();
    const s = Math.max(rect.width / WORLD_W, rect.height / WORLD_H);
    const ox = (rect.width - WORLD_W * s) / 2;
    return [
      (e.clientX - rect.left - ox) / s,
      WORLD_H - (rect.bottom - e.clientY) / s,
    ];
  }

  function blastAt(x: number, y: number) {
    const s = settings.current;
    sim.blast(x, y, s.radius, s.running);
    if (!s.running) stillRef.current();
  }

  function onPointerDown(e: PointerEvent<HTMLCanvasElement>) {
    e.currentTarget.setPointerCapture(e.pointerId);
    const [x, y] = toWorld(e);
    drag.current = { x, y };
    blastAt(x, y);
  }

  function onPointerMove(e: PointerEvent<HTMLCanvasElement>) {
    const d = drag.current;
    if (d === undefined) return;
    const [x, y] = toWorld(e);
    // A new crater once the pointer has moved half a radius: a trench.
    if (Math.hypot(x - d.x, y - d.y) < settings.current.radius * 0.5) return;
    drag.current = { x, y };
    blastAt(x, y);
  }

  function onPointerUp() {
    drag.current = undefined;
  }

  function restart(next: number) {
    setSeed(next);
    sim.reset(next);
    if (!settings.current.running) stillRef.current();
  }

  return (
    <ShowcaseLayout
      stage={
        <GLCanvas
          setup={setup}
          className="cursor-crosshair"
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerUp}
          aria-label="Crater sandbox: click or tap to blast a crater, drag to dig"
          role="img"
        />
      }
      hud={
        <Hud stats={stats}>
          <CraterRows sim={sim} />
        </Hud>
      }
      controls={
        <Card>
          <CardHeader>
            <CardTitle>Crater sandbox</CardTitle>
            <CardDescription>
              Click or tap to blast a crater, drag to dig a trench.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="grid grid-cols-2 gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPlaying((p) => !p)}
              >
                {playing ? <Pause /> : <Play />} {playing ? "Pause" : "Play"}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => sim.dropBalls(12)}
              >
                <CircleDot /> Drop balls
              </Button>
              <Button variant="outline" size="sm" onClick={() => restart(seed)}>
                <RotateCcw /> Reset
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => restart(1 + Math.floor(Math.random() * 99999))}
              >
                <Shuffle /> New world
              </Button>
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="craters-radius">Crater radius: {radius}</Label>
              <Slider
                id="craters-radius"
                min={16}
                max={90}
                value={[radius]}
                onValueChange={([v]) => setRadius(v)}
              />
            </div>
            <Toggle
              id="craters-strips"
              label="Color by strip"
              hint="Off: by material, soil, grass and stone."
              checked={strips}
              onChange={setStrips}
            />
            <Toggle
              id="craters-wireframe"
              label="Wireframe"
              checked={wireframe}
              onChange={setWireframe}
            />
            <p className="text-xs text-muted-foreground">
              Each of the {COLS} × {ROWS} tiles is one polygon: its square minus
              the sky, the caves and every crater that touches it. Holes that
              overlap or cross the square go to the constrained triangulation,
              which subtracts them by the non-zero winding rule, no clipping
              library involved. Only the tiles whose holes changed are
              triangulated again; they flash. Past {HOLE_CAP} craters a tile
              merges its oldest crater with the nearest one into a circle around
              both, so ground never grows back. World {seed}.
            </p>
          </CardContent>
        </Card>
      }
    />
  );
}
