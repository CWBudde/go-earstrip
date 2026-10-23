// The Spirograph kaleidoscope: a self-crossing curve whose pen swings out and
// back, triangulated every frame and filled by the non-zero winding rule, then
// drawn as many times as the kaleidoscope has mirrors.

import { Pause, Play, Shuffle } from "lucide-react";
import { useEffect, useMemo, useRef, useState, type RefObject } from "react";

import { Toggle } from "@/components/Toggle";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Slider } from "@/components/ui/slider";
import { formatCount } from "@/lib/format";
import { followSize } from "@/lib/gl";
import { useDark, useReducedMotion } from "@/lib/theme";
import { GLCanvas, type GLSetup } from "@/showcase/GLCanvas";
import { Hud, HudRow } from "@/showcase/Hud";
import { ShowcaseLayout } from "@/showcase/Layout";
import { useAnimationFrame } from "@/showcase/loop";
import type { ShowcaseProps } from "@/showcase/registry";
import { DEFAULT_OPTIONS, FrameStats } from "@/showcase/stats";
import type { Flat, Options } from "@/wasm/earstrip";

import { FAMILIES, type Family } from "./curves";
import { SpiroScene, type View } from "./scene";

/** Seconds of animation time for one swing of the pen, out and back. */
const SWING = 16;

/** What the HUD's own rows show, written by every frame. */
interface Info {
  family: string;
  pen: number;
  crossings: number;
  constrained: boolean;
  /** The smallest and largest winding number of the triangles. */
  windMin: number;
  windMax: number;
}

/** The settings a frame reads. */
interface Settings {
  family: Family;
  p: number;
  q: number;
  reach: number;
  vertices: number;
  copies: number;
  mirror: boolean;
  wedges: boolean;
  spread: number;
  hue: number;
  constrained: boolean;
  forcedCuts: boolean;
  wireframe: boolean;
  outline: boolean;
  crossings: boolean;
  byStrip: boolean;
  dark: boolean;
}

function familyById(id: string): Family {
  return FAMILIES.find((f) => f.id === id) ?? FAMILIES[0];
}

/** The pen at animation time t: from the family's floor out to reach and back. */
function penAt(f: Family, reach: number, t: number): number {
  const top = Math.max(f.penFloor, f.penCeil * reach);
  const swing = 0.5 - 0.5 * Math.cos((2 * Math.PI * t) / SWING);
  return f.penFloor + (top - f.penFloor) * swing;
}

/** The HUD rows of the demo; they poll info like the HUD polls its stats. */
function SpiroRows({ info }: { info: RefObject<Info> }) {
  const [s, setS] = useState<Info>(() => ({ ...info.current }));
  useEffect(() => {
    const id = setInterval(() => setS({ ...info.current }), 250);
    return () => clearInterval(id);
  }, [info]);
  const min = s.windMin;
  const max = s.windMax;
  return (
    <>
      <HudRow label="Family">{s.family}</HudRow>
      <HudRow label="Pen">{s.pen.toFixed(2)}</HudRow>
      <HudRow label="Crossings">
        {s.constrained
          ? s.crossings === 0
            ? "0 (simple)"
            : formatCount(s.crossings)
          : "— (CDT off)"}
      </HudRow>
      <HudRow label="Winding">{min === max ? min : `${min} … ${max}`}</HudRow>
    </>
  );
}

/** A labelled slider over whole numbers or steps. */
function Knob({
  id,
  label,
  value,
  min,
  max,
  step = 1,
  format = String,
  onChange,
}: {
  id: string;
  label: string;
  value: number;
  min: number;
  max: number;
  step?: number;
  format?: (v: number) => string;
  onChange: (v: number) => void;
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex justify-between gap-2 text-sm">
        <Label htmlFor={id}>{label}</Label>
        <span className="text-muted-foreground tabular-nums">
          {format(value)}
        </span>
      </div>
      <Slider
        id={id}
        min={min}
        max={max}
        step={step}
        value={[value]}
        onValueChange={([v]) => onChange(v)}
      />
    </div>
  );
}

export function Spiro({ api, active }: ShowcaseProps) {
  const dark = useDark();
  const reduced = useReducedMotion();
  const [playing, setPlaying] = useState(!reduced);
  const [familyId, setFamilyId] = useState("hypo");
  const family = familyById(familyId);
  const [p, setP] = useState(family.p.value);
  const [qWanted, setQ] = useState(family.q.value);
  const qMax = Math.min(family.q.max, family.qLimit?.(p) ?? family.q.max);
  const q = Math.max(family.q.min, Math.min(qWanted, qMax));
  const [reach, setReach] = useState(1);
  const [speed, setSpeed] = useState(1);
  const [vertices, setVertices] = useState(1400);
  const [copies, setCopies] = useState(3);
  const [mirror, setMirror] = useState(true);
  const [wedges, setWedges] = useState(false);
  const [spread, setSpread] = useState(0.2);
  const [hue, setHue] = useState(0);
  const [constrained, setConstrained] = useState(true);
  const [forcedCuts, setForcedCuts] = useState(true);
  const [wireframe, setWireframe] = useState(false);
  const [outline, setOutline] = useState(true);
  const [crossings, setCrossings] = useState(true);
  const [byStrip, setByStrip] = useState(false);
  // On a phone the HUD would hide most of the stage, so it starts hidden
  // there.
  const [hud, setHud] = useState(
    () => window.matchMedia("(min-width: 640px)").matches,
  );

  const scene = useRef<SpiroScene>(undefined);
  const time = useRef(SWING * 0.4);
  const stats = useMemo(() => new FrameStats(), []);
  const info = useRef<Info>({
    family: family.name,
    pen: 0,
    crossings: 0,
    constrained: true,
    windMin: 0,
    windMax: 0,
  });
  // The kaleidoscope's settings for the scene, refilled every frame.
  const view = useRef<View>({
    copies: 1,
    mirror: false,
    wedges: false,
    spread: 0,
    spin: 0,
    turn: 0,
    time: 0,
    dark: false,
    colorByStrip: false,
    wireframe: false,
    outline: false,
    crossings: false,
    hue: 0,
  });
  // The polygon's buffer, replaced only when the vertex count changes.
  const polygon = useRef<Flat>({ coords: new Float64Array(0), holeStarts: [] });
  const options = useRef<Options>({ ...DEFAULT_OPTIONS });

  // The latest settings for render, which the loop and the resize call.
  const settings = useRef<Settings>(undefined);
  useEffect(() => {
    settings.current = {
      family,
      p,
      q,
      reach,
      vertices,
      copies,
      mirror,
      wedges,
      spread,
      hue,
      constrained,
      forcedCuts,
      wireframe,
      outline,
      crossings,
      byStrip,
      dark,
    };
  });

  function render() {
    const sc = scene.current;
    const s = settings.current;
    if (sc === undefined || s === undefined) return;
    const t = time.current;
    const f = s.family;
    const n = f.vertices(s.vertices, s.p, s.q);
    let poly = polygon.current;
    if (poly.coords.length !== 2 * n) {
      poly = polygon.current = {
        coords: new Float64Array(2 * n),
        holeStarts: [],
      };
    }
    const pen = penAt(f, s.reach, t);
    f.sample(poly.coords, n, s.p, s.q, pen, 0.3 * t);
    const o = options.current;
    o.constrained = s.constrained;
    o.forcedCuts = s.forcedCuts;
    const r = stats.run(api, poly, o);
    const windings = sc.update(r, poly.coords, n);
    const i = info.current;
    i.family = f.name;
    i.pen = pen;
    i.crossings = r.extra.length / 2;
    i.constrained = s.constrained;
    i.windMin = windings.min;
    i.windMax = windings.max;

    const v = view.current;
    v.copies = s.copies;
    v.mirror = s.mirror;
    v.wedges = s.wedges;
    v.spread = s.spread;
    // In wedges, the curve turns faster: the mirrors make the motion.
    v.spin = (s.wedges ? 0.12 : 0.05) * t;
    v.turn = -0.02 * t;
    v.time = t;
    v.dark = s.dark;
    v.colorByStrip = s.byStrip;
    v.wireframe = s.wireframe;
    v.outline = s.outline;
    v.crossings = s.crossings;
    v.hue = s.hue;
    sc.draw(v);
  }
  const renderRef = useRef(render);
  useEffect(() => {
    renderRef.current = render;
  });

  const setup: GLSetup = (gl, canvas) => {
    const sc = new SpiroScene(gl);
    scene.current = sc;
    const stop = followSize(canvas, () => renderRef.current());
    return () => {
      stop();
      sc.dispose();
      scene.current = undefined;
    };
  };

  const running = active && playing;
  // Paused, every change of a setting draws one frame.
  useEffect(() => {
    if (!running) {
      stats.pause();
      renderRef.current();
    }
  }, [
    running,
    stats,
    familyId,
    p,
    q,
    reach,
    vertices,
    copies,
    mirror,
    wedges,
    spread,
    hue,
    constrained,
    forcedCuts,
    wireframe,
    outline,
    crossings,
    byStrip,
    dark,
  ]);

  useAnimationFrame(running, (now, dt) => {
    time.current += (speed * dt) / 1000;
    stats.begin(now);
    renderRef.current();
    stats.end();
  });

  function pickFamily(id: string) {
    const f = familyById(id);
    setFamilyId(id);
    setP(f.p.value);
    setQ(f.q.value);
  }

  /** A random curve and kaleidoscope. */
  function shuffle() {
    const f = FAMILIES[Math.floor(Math.random() * FAMILIES.length)];
    const between = (lo: number, hi: number) =>
      lo + Math.floor(Math.random() * (hi - lo + 1));
    const np = between(f.p.min, Math.min(f.p.max, 2 * f.p.value));
    const limit = Math.min(f.q.max, f.qLimit?.(np) ?? f.q.max);
    setFamilyId(f.id);
    setP(np);
    setQ(between(f.q.min, Math.max(f.q.min, limit)));
    setReach(0.5 + 0.5 * Math.random());
    setCopies(between(1, 9));
    setMirror(Math.random() < 0.6);
    setWedges(Math.random() < 0.7);
    setSpread(Math.round(Math.random() * 10) / 20);
    setHue(Math.round(Math.random() * 20) / 20);
  }

  return (
    <ShowcaseLayout
      stage={<GLCanvas setup={setup} />}
      hud={
        hud ? (
          <Hud stats={stats}>
            <SpiroRows info={info} />
          </Hud>
        ) : undefined
      }
      controls={
        <>
          <Card>
            <CardHeader>
              <CardTitle>Spirograph</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="flex-1"
                  onClick={() => setPlaying((v) => !v)}
                >
                  {playing ? <Pause /> : <Play />} {playing ? "Pause" : "Play"}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  className="flex-1"
                  onClick={shuffle}
                >
                  <Shuffle /> Random
                </Button>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="spiro-family">Curve</Label>
                <Select value={familyId} onValueChange={pickFamily}>
                  <SelectTrigger id="spiro-family" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {FAMILIES.map((f) => (
                      <SelectItem key={f.id} value={f.id}>
                        {f.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs text-muted-foreground">{family.about}</p>
              </div>
              <Knob
                id="spiro-p"
                label={family.p.label}
                value={p}
                min={family.p.min}
                max={family.p.max}
                onChange={setP}
              />
              <Knob
                id="spiro-q"
                label={family.q.label}
                value={q}
                min={family.q.min}
                max={Math.max(family.q.min, qMax)}
                onChange={setQ}
              />
              <Knob
                id="spiro-reach"
                label="Pen reach"
                value={reach}
                min={0.1}
                max={1}
                step={0.05}
                format={(v) => v.toFixed(2)}
                onChange={setReach}
              />
              <Knob
                id="spiro-speed"
                label="Speed"
                value={speed}
                min={0.1}
                max={3}
                step={0.1}
                format={(v) => `${v.toFixed(1)}×`}
                onChange={setSpeed}
              />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Kaleidoscope</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <Knob
                id="spiro-copies"
                label="Copies"
                value={copies}
                min={1}
                max={12}
                onChange={setCopies}
              />
              <Toggle
                id="spiro-mirror"
                label="Mirrored"
                hint="Reflects every copy too, like the mirrors of a kaleidoscope."
                checked={mirror}
                onChange={setMirror}
              />
              <Toggle
                id="spiro-wedges"
                label="Wedges"
                hint="Each copy shows only its slice, as in a real kaleidoscope. Off, the whole curves lie over each other."
                checked={wedges}
                onChange={setWedges}
              />
              <Knob
                id="spiro-spread"
                label="Spread"
                value={spread}
                min={0}
                max={0.6}
                step={0.05}
                format={(v) => v.toFixed(2)}
                onChange={setSpread}
              />
              <Knob
                id="spiro-hue"
                label="Hue"
                value={hue}
                min={0}
                max={0.95}
                step={0.05}
                format={(v) => `${Math.round(v * 360)}°`}
                onChange={setHue}
              />
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>Triangulator</CardTitle>
            </CardHeader>
            <CardContent className="flex flex-col gap-4">
              <Toggle
                id="spiro-constrained"
                label="Constrained triangulation"
                hint="Off, the ears run out at the crossings and the status turns Approximate: hatched triangles cover what the curve does not wind around."
                checked={constrained}
                onChange={setConstrained}
              />
              <Toggle
                id="spiro-forced"
                label="Forced cuts"
                hint="Off as well, the status is Refused and the strips cover only what the ears clipped."
                checked={forcedCuts}
                disabled={constrained}
                onChange={setForcedCuts}
              />
              <Toggle
                id="spiro-crossings"
                label="Crossing points"
                hint="The extra points the constrained triangulation inserts where the curve crosses itself."
                checked={crossings}
                onChange={setCrossings}
              />
              <Toggle
                id="spiro-outline"
                label="Outline"
                checked={outline}
                onChange={setOutline}
              />
              <Toggle
                id="spiro-wire"
                label="Strip wireframe"
                hint="The restart index buffer drawn as line strips: the zig-zag of every triangle strip."
                checked={wireframe}
                onChange={setWireframe}
              />
              <Toggle
                id="spiro-strip"
                label="Color by strip"
                hint="Instead of by winding number."
                checked={byStrip}
                onChange={setByStrip}
              />
              <Knob
                id="spiro-vertices"
                label="Vertices"
                value={vertices}
                min={300}
                max={2400}
                step={100}
                format={(v) => v.toLocaleString()}
                onChange={setVertices}
              />
              <Toggle
                id="spiro-hud"
                label="Statistics"
                hint="The overlay with the status, counts and times."
                checked={hud}
                onChange={setHud}
              />
            </CardContent>
          </Card>
        </>
      }
    />
  );
}
