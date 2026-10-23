// The world map: Natural Earth's countries, triangulated in one call and drawn
// with WebGL 2 straight from the restart index buffer, as the GLMap engine
// (whose triangulator earstrip is) draws its polygons. An intro draws the
// strips in their order; pan, zoom, hover and the toggles redraw on demand.

import { LocateFixed, RotateCcw } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { Toggle } from "@/components/Toggle";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { formatCount } from "@/lib/format";
import { pixelRatio, followSize } from "@/lib/gl";
import { STATUS } from "@/lib/status";
import { useDark, useReducedMotion } from "@/lib/theme";
import { GLCanvas, type GLSetup } from "@/showcase/GLCanvas";
import { Hud, HudRow } from "@/showcase/Hud";
import { ShowcaseLayout } from "@/showcase/Layout";
import type { ShowcaseProps } from "@/showcase/registry";
import { FrameStats } from "@/showcase/stats";
import type { BatchResult, Status } from "@/wasm/earstrip";

import { buildGeometry, countryAt, type Geometry } from "./geometry";
import { Camera, MapRenderer, type View } from "./renderer";
import { LEVELS, loadWorld, type Level, type World } from "./world";

/** How long the intro takes, in milliseconds. */
const INTRO_MS = 2200;

interface Loaded {
  world: World;
  result: BatchResult;
  geometry: Geometry;
  stats: FrameStats;
}

interface Scene {
  renderer: MapRenderer;
  camera?: Camera;
  canvas: HTMLCanvasElement;
  width: number;
  height: number;
}

function bytes(n: number): string {
  if (n < 1e6) return `${(n / 1e3).toFixed(0)} KB`;
  return `${(n / 1e6).toFixed(2)} MB`;
}

/** Two or more buttons of which one is on. */
function Segmented<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
}) {
  return (
    <div className="flex flex-col gap-2">
      <Label>{label}</Label>
      <div className="grid grid-flow-col gap-1" role="group" aria-label={label}>
        {options.map((o) => (
          <Button
            key={o.value}
            size="sm"
            variant={o.value === value ? "default" : "outline"}
            aria-pressed={o.value === value}
            onClick={() => onChange(o.value)}
          >
            {o.label}
          </Button>
        ))}
      </div>
    </div>
  );
}

/** What the map shows of the highlighted country. */
function CountryInfo({ data, country }: { data: Loaded; country: number }) {
  const c = data.world.countries[country];
  const s = data.geometry.countries[country];
  const statuses = Object.entries(s.statuses) as [Status, number][];
  return (
    <div className="flex max-w-64 flex-col gap-1 rounded-lg border bg-background/85 px-3 py-2 text-xs shadow-sm backdrop-blur">
      <div className="flex items-center justify-between gap-2">
        <span className="truncate text-sm font-medium">{c.name}</span>
        {statuses
          .filter(([k]) => k !== "Exact")
          .map(([k, n]) => (
            <Badge
              key={k}
              variant={STATUS[k].good ? "secondary" : "destructive"}
              title={STATUS[k].text}
            >
              {k}
              {s.polygons > 1 && ` ×${n}`}
            </Badge>
          ))}
      </div>
      <p className="text-muted-foreground tabular-nums">
        {s.polygons > 1 && `${formatCount(s.polygons)} polygons · `}
        {s.triangles.toLocaleString("en")} triangles in{" "}
        {s.strips.toLocaleString("en")} {s.strips === 1 ? "strip" : "strips"}
        {" · "}
        {s.indices.toLocaleString("en")} indices (
        {(s.indices / Math.max(1, s.triangles)).toFixed(2)}/tri)
      </p>
    </div>
  );
}

export function WorldMap({ api, active }: ShowcaseProps) {
  const dark = useDark();
  const reduced = useReducedMotion();
  const [level, setLevel] = useState<Level>("50m");
  const [byStrip, setByStrip] = useState(true);
  const [list, setList] = useState(false);
  const [wireframe, setWireframe] = useState(false);
  const [hover, setHover] = useState(-1);
  const [data, setData] = useState<Loaded>();
  const [error, setError] = useState<string>();
  const [drawCalls, setDrawCalls] = useState(1);
  const [hudOpen, setHudOpen] = useState(false);
  const empty = useMemo(() => new FrameStats(), []);
  const stats = data?.stats ?? empty;

  const scene = useRef<Scene>(undefined);
  /** The intro's progress in milliseconds, or -1 when it is done. */
  const intro = useRef(-1);
  const frame = useRef(0);
  const lastFrame = useRef(0);
  const idle = useRef(0);

  // The latest state for the frame callback and the pointer handlers.
  const latest = useRef({
    data,
    active,
    byStrip,
    list,
    wireframe,
    hover,
    dark,
  });
  useEffect(() => {
    latest.current = { data, active, byStrip, list, wireframe, hover, dark };
  });

  useEffect(() => {
    let live = true;
    loadWorld(level).then(
      (world) => {
        if (!live) return;
        const s = new FrameStats();
        s.begin(performance.now());
        const result = s.runMany(api, world.batch);
        s.end();
        setData({
          world,
          result,
          geometry: buildGeometry(world, result),
          stats: s,
        });
        setHover(-1);
        intro.current = reduced ? -1 : 0;
      },
      (err: unknown) => {
        if (live) setError(err instanceof Error ? err.message : String(err));
      },
    );
    return () => {
      live = false;
    };
    // The intro reads reduced when a level arrives; a change later does not
    // load the level again.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [level, api]);

  /** Draws one frame, and asks for the next while the intro runs. */
  function draw(now: number) {
    frame.current = 0;
    const sc = scene.current;
    const l = latest.current;
    if (sc === undefined || l.data === undefined || sc.camera === undefined) {
      return;
    }
    const { stats: s, geometry, result } = l.data;
    const dt = lastFrame.current > 0 ? now - lastFrame.current : 0;
    if (dt > 250) s.pause();
    lastFrame.current = now;
    s.begin(now);

    let reveal = Infinity;
    if (intro.current >= 0) {
      intro.current += Math.min(dt, 50);
      const t = Math.min(1, intro.current / INTRO_MS);
      // Even in triangles: the strip that holds the triangle eased in.
      const target = (1 - (1 - t) ** 2.2) * result.list.length;
      const ends = result.stripListEnds;
      let lo = 0;
      let hi = ends.length;
      while (lo < hi) {
        const mid = (lo + hi) >> 1;
        if (ends[mid] < target) lo = mid + 1;
        else hi = mid;
      }
      reveal = t >= 1 ? Infinity : lo;
      if (t >= 1) intro.current = -1;
    }
    const view: View = {
      byStrip: l.byStrip,
      list: l.list,
      wireframe: l.wireframe,
      hover: l.hover,
      reveal: reveal < geometry.strips ? reveal : Infinity,
      dark: l.dark,
    };
    const calls = sc.renderer.render(sc.camera, view);
    s.end();
    setDrawCalls(calls);

    window.clearTimeout(idle.current);
    if (intro.current >= 0) {
      requestDraw();
    } else {
      idle.current = window.setTimeout(() => s.pause(), 300);
    }
  }
  const drawRef = useRef(draw);
  useEffect(() => {
    drawRef.current = draw;
  });

  function requestDraw() {
    if (!latest.current.active || frame.current !== 0) return;
    frame.current = requestAnimationFrame((now) => drawRef.current(now));
  }
  const requestRef = useRef(requestDraw);
  useEffect(() => {
    requestRef.current = requestDraw;
  });

  // Uploads a new triangulation and starts its intro.
  useEffect(() => {
    const sc = scene.current;
    if (sc === undefined || data === undefined) return;
    upload(sc, data, latest.current.list);
    requestRef.current();
  }, [data]);

  // Redraws on any change; stops the frames when the demo is hidden.
  useEffect(() => {
    if (!active) {
      cancelAnimationFrame(frame.current);
      frame.current = 0;
      return;
    }
    requestRef.current();
  }, [active, byStrip, list, wireframe, hover, dark]);

  useEffect(
    () => () => {
      cancelAnimationFrame(frame.current);
      window.clearTimeout(idle.current);
    },
    [],
  );

  const setup: GLSetup = (gl, canvas) => {
    const renderer = new MapRenderer(gl);
    const sc: Scene = { renderer, canvas, width: 1, height: 1 };
    scene.current = sc;
    if (latest.current.data !== undefined) {
      upload(sc, latest.current.data, latest.current.list);
    }
    const stopSize = followSize(canvas, (width, height) => {
      sc.width = width;
      sc.height = height;
      sc.camera?.resize(width, height);
      requestRef.current();
    });
    const stopPointers = attachPointers(canvas, sc, {
      redraw: () => requestRef.current(),
      hoverAt: (x, y, touch) => {
        const l = latest.current;
        if (l.data === undefined || sc.camera === undefined) return;
        const [wx, wy] = sc.camera.toWorld(x, y);
        const size = sc.camera.size;
        const c = countryAt(
          l.data.world,
          l.data.result,
          ((wx % size) + size) % size,
          wy,
        );
        // A tap on the highlighted country, or on the sea, lets go.
        setHover(touch && c === l.hover ? -1 : c);
      },
      leave: () => setHover(-1),
    });
    return () => {
      stopSize();
      stopPointers();
      renderer.dispose();
      scene.current = undefined;
    };
  };

  function replay() {
    intro.current = reduced ? -1 : 0;
    requestDraw();
  }

  function resetView() {
    scene.current?.camera?.fit();
    requestDraw();
  }

  const r = data?.result;
  const stripBytes = r === undefined ? 0 : 4 * r.restart.length;
  const listBytes = r === undefined ? 0 : 4 * r.list.length;

  return (
    <ShowcaseLayout
      stage={
        <div className="relative size-full">
          <GLCanvas
            setup={setup}
            className="cursor-grab active:cursor-grabbing"
            aria-label="World map: drag to pan, scroll or pinch to zoom"
          />
          {error !== undefined && (
            <p
              className="absolute inset-x-0 top-1/2 text-center text-sm text-destructive"
              role="alert"
            >
              {error}
            </p>
          )}
          <div className="pointer-events-none absolute right-2 bottom-2 left-2 flex items-end justify-between gap-2">
            {data !== undefined && hover >= 0 ? (
              <CountryInfo data={data} country={hover} />
            ) : (
              <span className="rounded-md bg-background/70 px-2 py-1 text-xs text-muted-foreground backdrop-blur">
                Hover or tap a country
              </span>
            )}
            <span className="hidden rounded-md bg-background/70 px-2 py-1 text-[10px] text-muted-foreground backdrop-blur sm:block">
              Natural Earth 1:{level}
            </span>
          </div>
        </div>
      }
      hud={
        // On a phone the HUD would cover the map: it folds into a button.
        <div className="flex flex-col items-start gap-1">
          <button
            type="button"
            className="pointer-events-auto rounded-md border bg-background/80 px-2 py-1 text-xs tabular-nums shadow-sm backdrop-blur sm:hidden"
            aria-expanded={hudOpen}
            onClick={() => setHudOpen((o) => !o)}
          >
            {data === undefined
              ? "Loading…"
              : `${formatCount(data.result.list.length / 3)} triangles · ${data.result.ms.toFixed(0)} ms`}{" "}
            <span className="text-muted-foreground">
              {hudOpen ? "▴" : "▾"}
            </span>
          </button>
          <div className={hudOpen ? "" : "max-sm:hidden"}>
            <Hud stats={stats}>
              {data !== undefined && (
                <>
                  <HudRow label="Polygons">
                    {formatCount(data.world.polygonCountry.length)}
                  </HudRow>
                  <HudRow label="In Go">{data.result.ms.toFixed(1)} ms</HudRow>
                  <HudRow label="Buffer">
                    {bytes(list ? listBytes : stripBytes)}
                  </HudRow>
                  <HudRow label="Draws">{drawCalls}</HudRow>
                </>
              )}
            </Hud>
          </div>
        </div>
      }
      controls={
        <Card>
          <CardHeader>
            <CardTitle>World map</CardTitle>
            <CardDescription>
              Every country in one triangulateMany call, drawn by one
              drawElements as triangle strips.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-5">
            <Segmented
              label="Detail"
              value={level}
              options={LEVELS.map((l) => ({ value: l, label: `1:${l}` }))}
              onChange={setLevel}
            />
            <Segmented
              label="Color by"
              value={byStrip ? "strip" : "country"}
              options={[
                { value: "strip", label: "Strip" },
                { value: "country", label: "Country" },
              ]}
              onChange={(v) => setByStrip(v === "strip")}
            />
            <div className="flex flex-col gap-2">
              <Segmented
                label="Draw as"
                value={list ? "list" : "strips"}
                options={[
                  { value: "strips", label: "Strips" },
                  { value: "list", label: "Triangle list" },
                ]}
                onChange={(v) => setList(v === "list")}
              />
              {r !== undefined && (
                <dl className="grid grid-cols-[auto_1fr_auto] gap-x-3 text-xs tabular-nums">
                  <dt
                    className={list ? "text-muted-foreground" : "font-medium"}
                  >
                    Strips
                  </dt>
                  <dd className="text-right text-muted-foreground">
                    {r.restart.length.toLocaleString("en")} indices
                  </dd>
                  <dd
                    className={list ? "text-muted-foreground" : "font-medium"}
                  >
                    {bytes(stripBytes)}
                  </dd>
                  <dt
                    className={list ? "font-medium" : "text-muted-foreground"}
                  >
                    List
                  </dt>
                  <dd className="text-right text-muted-foreground">
                    {r.list.length.toLocaleString("en")} indices
                  </dd>
                  <dd
                    className={list ? "font-medium" : "text-muted-foreground"}
                  >
                    {bytes(listBytes)}
                  </dd>
                </dl>
              )}
              <p className="text-xs text-muted-foreground">
                The same pixels either way; the strips need{" "}
                {r === undefined
                  ? "fewer"
                  : `${Math.round(100 * (1 - stripBytes / listBytes))}% fewer`}{" "}
                index bytes.
              </p>
            </div>
            <Toggle
              id="map-wireframe"
              label="Wireframe"
              hint="The edges of every triangle."
              checked={wireframe}
              onChange={setWireframe}
            />
            <div className="grid grid-cols-2 gap-2">
              <Button variant="outline" size="sm" onClick={replay}>
                <RotateCcw /> Replay
              </Button>
              <Button variant="outline" size="sm" onClick={resetView}>
                <LocateFixed /> Reset view
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Drag to pan, scroll or pinch to zoom. Data: Natural Earth, public
              domain.
            </p>
          </CardContent>
        </Card>
      }
    />
  );
}

/** Uploads the triangulation, with a camera for its grid. */
function upload(sc: Scene, data: Loaded, list: boolean) {
  sc.renderer.setGeometry(data.geometry, data.result, list);
  if (sc.camera === undefined || sc.camera.size !== data.world.size) {
    sc.camera = new Camera(data.world.size);
    sc.camera.resize(sc.width, sc.height);
  }
}

/**
 * Pans on a drag, zooms on the wheel, a pinch or a double click, and reports
 * hovers and taps in device pixels. Returns the function that detaches it.
 */
function attachPointers(
  canvas: HTMLCanvasElement,
  sc: Scene,
  on: {
    redraw: () => void;
    hoverAt: (x: number, y: number, touch: boolean) => void;
    leave: () => void;
  },
): () => void {
  const pointers = new Map<number, { x: number; y: number }>();
  let moved = 0;
  let downAt = 0;
  const local = (e: PointerEvent | WheelEvent | MouseEvent) => {
    const rect = canvas.getBoundingClientRect();
    const ratio = canvas.width / Math.max(1, rect.width);
    return {
      x: (e.clientX - rect.left) * ratio,
      y: (e.clientY - rect.top) * ratio,
    };
  };
  const pinch = () => {
    const [a, b] = [...pointers.values()];
    return {
      x: (a.x + b.x) / 2,
      y: (a.y + b.y) / 2,
      d: Math.hypot(a.x - b.x, a.y - b.y),
    };
  };

  const down = (e: PointerEvent) => {
    canvas.setPointerCapture(e.pointerId);
    pointers.set(e.pointerId, local(e));
    if (pointers.size === 1) {
      moved = 0;
      downAt = performance.now();
    }
  };
  const move = (e: PointerEvent) => {
    const p = local(e);
    const prev = pointers.get(e.pointerId);
    const camera = sc.camera;
    if (prev === undefined || camera === undefined) {
      if (e.pointerType === "mouse") on.hoverAt(p.x, p.y, false);
      return;
    }
    if (pointers.size === 1) {
      camera.pan(p.x - prev.x, p.y - prev.y);
      moved += Math.hypot(p.x - prev.x, p.y - prev.y);
      pointers.set(e.pointerId, p);
    } else {
      const before = pinch();
      pointers.set(e.pointerId, p);
      const after = pinch();
      camera.zoomAt(before.x, before.y, after.d / Math.max(1, before.d));
      camera.pan(after.x - before.x, after.y - before.y);
      moved = Infinity;
    }
    on.redraw();
  };
  const up = (e: PointerEvent) => {
    if (!pointers.delete(e.pointerId)) return;
    const tap =
      pointers.size === 0 &&
      moved < 8 * pixelRatio() &&
      performance.now() - downAt < 500;
    if (tap) {
      const p = local(e);
      on.hoverAt(p.x, p.y, e.pointerType !== "mouse");
    }
  };
  const leave = (e: PointerEvent) => {
    if (e.pointerType === "mouse" && pointers.size === 0) on.leave();
  };
  const wheel = (e: WheelEvent) => {
    e.preventDefault();
    const camera = sc.camera;
    if (camera === undefined) return;
    const p = local(e);
    const lines = e.deltaMode === WheelEvent.DOM_DELTA_LINE ? 16 : 1;
    camera.zoomAt(p.x, p.y, Math.exp(-e.deltaY * lines * 0.002));
    on.redraw();
  };
  const dblclick = (e: MouseEvent) => {
    const p = local(e);
    sc.camera?.zoomAt(p.x, p.y, 2);
    on.redraw();
  };

  canvas.addEventListener("pointerdown", down);
  canvas.addEventListener("pointermove", move);
  canvas.addEventListener("pointerup", up);
  canvas.addEventListener("pointercancel", up);
  canvas.addEventListener("pointerleave", leave);
  canvas.addEventListener("wheel", wheel, { passive: false });
  canvas.addEventListener("dblclick", dblclick);
  return () => {
    canvas.removeEventListener("pointerdown", down);
    canvas.removeEventListener("pointermove", move);
    canvas.removeEventListener("pointerup", up);
    canvas.removeEventListener("pointercancel", up);
    canvas.removeEventListener("pointerleave", leave);
    canvas.removeEventListener("wheel", wheel);
    canvas.removeEventListener("dblclick", dblclick);
  };
}
