import {
  BookOpen,
  Check,
  Code,
  Copy,
  Dices,
  FileText,
  LoaderCircle,
  Maximize,
  MousePointer2,
  Pause,
  PenLine,
  Play,
  Trash2,
  Undo2,
} from "lucide-react";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";

import { Viewport, type Display, type Mode } from "@/components/Viewport";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  allPoints,
  formatJSON,
  fromFlat,
  parseJSON,
  toFlat,
  type Point,
  type Rings,
} from "@/lib/polygon";
import { restartLength, triangleStrips } from "@/lib/strips";
import {
  loadEarstrip,
  type Earstrip,
  type Flat,
  type Options,
  type Result,
  type Status,
  type Timing,
} from "@/wasm/earstrip";

const REPO = "https://github.com/CWBudde/go-earstrip";
const DOCS = "https://pkg.go.dev/github.com/cwbudde/go-earstrip";
const BLOG = "https://globus.software/blog/2026/10/16/cutting-map-polygons/";

const DEFAULT_OPTIONS: Options = {
  forcedCuts: true,
  constrained: true,
  extraPoints: true,
  triangleList: false,
};

const DEFAULT_DISPLAY: Display = {
  edges: true,
  order: true,
  labels: false,
  snap: true,
};

/** How long the timing runs, in milliseconds, after the polygon settles. */
const TIME_BUDGET = 40;

const UNDO_DEPTH = 100;

/** The status table of the README. */
const STATUS: Record<Status, { text: string; good: boolean }> = {
  Exact: { text: "The triangles cover the polygon exactly.", good: true },
  HolesLeftOut: {
    text: "A hole could not be joined to the boundary (it lies outside or crosses it) and is left out.",
    good: false,
  },
  Constrained: {
    text: "Covered by the constrained triangulation: the rings cross, a hole could not be joined, or there are more than 128 rings. It fills what lies inside the outer ring by the non-zero winding rule and outside every hole.",
    good: true,
  },
  Approximate: {
    text: "The ears ran out (the polygon is not simple) and the rest was cut by forced cuts.",
    good: false,
  },
  Refused: {
    text: "The ears ran out and forced cuts are off: the strips cover only what the ears did.",
    good: false,
  },
};

type LoadState =
  | { status: "loading" }
  | { status: "ready"; api: Earstrip }
  | { status: "error"; message: string };

const darkQuery = window.matchMedia("(prefers-color-scheme: dark)");

/** Whether the page is dark; main.tsx follows the system the same way. */
function useDark(): boolean {
  return useSyncExternalStore(
    (onChange) => {
      darkQuery.addEventListener("change", onChange);
      return () => darkQuery.removeEventListener("change", onChange);
    },
    () => darkQuery.matches,
  );
}

function formatTime(ns: number): string {
  if (ns < 1e3) return `${ns.toFixed(0)} ns`;
  if (ns < 1e6) return `${(ns / 1e3).toPrecision(3)} µs`;
  return `${(ns / 1e6).toPrecision(3)} ms`;
}

function Header({ goVersion }: { goVersion?: string }) {
  return (
    <header className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="font-heading text-3xl font-semibold tracking-tight">
            go-earstrip
          </h1>
          <p className="text-muted-foreground">
            Polygon triangulation into triangle strips, in pure Go, compiled to
            WebAssembly and running in your browser.
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" asChild>
            <a href={REPO} target="_blank" rel="noreferrer">
              <Code /> GitHub
            </a>
          </Button>
          <Button variant="outline" size="sm" asChild>
            <a href={DOCS} target="_blank" rel="noreferrer">
              <BookOpen /> pkg.go.dev
            </a>
          </Button>
          <Button variant="outline" size="sm" asChild>
            <a href={BLOG} target="_blank" rel="noreferrer">
              <FileText /> How it works
            </a>
          </Button>
        </div>
      </div>
      <p className="text-sm text-muted-foreground">
        A bit-exact port of{" "}
        <a
          className="underline underline-offset-4"
          href="https://github.com/GLMap/earstrip"
          target="_blank"
          rel="noreferrer"
        >
          earstrip
        </a>
        , the triangulator of the GLMap map engine. It cuts ears along zig-zag
        strips, about 1.7 indices per triangle instead of 3, and covers polygons
        whose rings cross with a constrained Delaunay triangulation. Here it
        runs as{" "}
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge variant="outline" className="cursor-help">
              js/wasm
            </Badge>
          </TooltipTrigger>
          <TooltipContent>
            {goVersion === undefined
              ? "GOOS=js GOARCH=wasm"
              : `Built with ${goVersion}, GOOS=js GOARCH=wasm`}
          </TooltipContent>
        </Tooltip>{" "}
        on one thread; native builds are faster.
      </p>
    </header>
  );
}

function Toggle({
  id,
  label,
  hint,
  checked,
  onChange,
}: {
  id: string;
  label: string;
  hint?: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <div className="flex flex-col gap-0.5">
        <Label htmlFor={id}>{label}</Label>
        {hint !== undefined && (
          <p className="text-xs text-muted-foreground">{hint}</p>
        )}
      </div>
      <Switch id={id} checked={checked} onCheckedChange={onChange} />
    </div>
  );
}

function Stat({
  label,
  value,
  sub,
}: {
  label: string;
  value: ReactNode;
  sub?: ReactNode;
}) {
  return (
    <div className="flex flex-col">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-lg font-semibold tabular-nums">{value}</dd>
      {sub !== undefined && (
        <dd className="text-xs text-muted-foreground tabular-nums">{sub}</dd>
      )}
    </div>
  );
}

function CopyButton({ text, label }: { text: () => string; label: string }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return;
    const id = setTimeout(() => setCopied(false), 1500);
    return () => clearTimeout(id);
  }, [copied]);
  return (
    <Button
      variant="outline"
      size="sm"
      onClick={() => {
        navigator.clipboard.writeText(text()).then(() => setCopied(true));
      }}
    >
      {copied ? <Check /> : <Copy />} {label}
    </Button>
  );
}

function Playground({ api }: { api: Earstrip }) {
  const presets = useMemo(() => api.presets(), [api]);
  const kinds = useMemo(() => api.kinds(), [api]);
  const groups = useMemo(() => {
    const m = new Map<string, { index: number; name: string }[]>();
    presets.forEach(({ group, name }, index) => {
      if (!m.has(group)) m.set(group, []);
      m.get(group)!.push({ index, name });
    });
    return [...m];
  }, [presets]);

  const [rings, setRings] = useState<Rings>(() => fromFlat(api.preset(0)));
  const [preset, setPreset] = useState("0");
  const [kind, setKind] = useState(() => kinds[0] ?? "");
  const [seed, setSeed] = useState(1);
  const [options, setOptions] = useState(DEFAULT_OPTIONS);
  const [display, setDisplay] = useState(DEFAULT_DISPLAY);
  const [mode, setMode] = useState<Mode>("edit");
  const [draft, setDraft] = useState<Point[]>([]);
  const [fitKey, setFitKey] = useState(0);
  const [hovered, setHovered] = useState(-1);
  const [shown, setShown] = useState<number | null>(null);
  const [playing, setPlaying] = useState(false);
  const [undo, setUndo] = useState<Rings[]>([]);
  const [timing, setTiming] = useState<{
    flat: Flat;
    options: Options;
    timing: Timing;
  } | null>(null);
  const [jsonOpen, setJsonOpen] = useState(false);
  const [jsonText, setJsonText] = useState<string | null>(null);
  const [jsonError, setJsonError] = useState<string>();
  const editBase = useRef<Rings | null>(null);
  const dark = useDark();

  const flat = useMemo(() => toFlat(rings), [rings]);
  const run = useMemo((): { result?: Result; error?: string } => {
    try {
      return { result: api.triangulate(flat, options) };
    } catch (err) {
      return { error: String(err) };
    }
  }, [api, flat, options]);
  const result = run.result;
  const strips = useMemo(() => triangleStrips(result), [result]);
  const points = useMemo(
    () => allPoints(rings, result?.extra),
    [rings, result],
  );
  const vertices = flat.coords.length / 2;
  const total = strips.strip.length;
  const shownCount = shown === null ? total : Math.min(shown, total);
  const list = options.triangleList;

  // Time the run once the polygon has settled for a moment.
  useEffect(() => {
    const id = setTimeout(() => {
      try {
        setTiming({
          flat,
          options,
          timing: api.time(flat, options, TIME_BUDGET),
        });
      } catch {
        setTiming(null);
      }
    }, 300);
    return () => clearTimeout(id);
  }, [api, flat, options]);
  const timed =
    timing !== null && timing.flat === flat && timing.options === options
      ? timing.timing
      : undefined;

  // Step through the triangles in output order.
  useEffect(() => {
    if (!playing) return;
    const duration = Math.min(8000, Math.max(1500, total * 30));
    const start = performance.now();
    let frame = requestAnimationFrame(function tick(now) {
      const k = Math.floor(((now - start) / duration) * (total + 1));
      if (k >= total) {
        setShown(null);
        setPlaying(false);
        return;
      }
      setShown(k);
      frame = requestAnimationFrame(tick);
    });
    return () => cancelAnimationFrame(frame);
  }, [playing, total]);

  function pushUndo(base: Rings) {
    setUndo((u) => [...u.slice(-UNDO_DEPTH + 1), base]);
  }

  /** Replaces the polygon, as a new one: the view fits it. */
  function replace(next: Rings) {
    pushUndo(rings);
    setRings(next);
    setDraft([]);
    setShown(null);
    setPlaying(false);
    setHovered(-1);
    setJsonText(null);
    setFitKey((k) => k + 1);
  }

  function loadPreset(value: string) {
    setPreset(value);
    replace(fromFlat(api.preset(Number(value))));
  }

  function generate(s: number) {
    setSeed(s);
    setPreset("");
    replace(fromFlat(api.generate(kind, s)));
  }

  function change(next: Rings) {
    if (editBase.current !== null) {
      pushUndo(editBase.current);
      editBase.current = null;
    }
    setRings(next);
    setJsonText(null);
  }

  function undoLast() {
    const prev = undo.at(-1);
    if (prev === undefined) return;
    setUndo((u) => u.slice(0, -1));
    setRings(prev);
    setDraft([]);
    setJsonText(null);
  }

  function finishDraft() {
    if (draft.length >= 3) {
      pushUndo(rings);
      setRings([...rings, draft]);
      setPreset("");
      setJsonText(null);
    }
    setDraft([]);
  }

  function changeMode(next: Mode) {
    if (mode === "draw" && next !== "draw") finishDraft();
    setMode(next);
  }

  // Keyboard shortcuts, through a ref so the listener sees the current state.
  const keys = useRef<(e: KeyboardEvent) => void>(() => {});
  useEffect(() => {
    keys.current = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      if (target?.closest("input, textarea, select, [role=listbox]")) return;
      if ((e.metaKey || e.ctrlKey) && e.key === "z") {
        e.preventDefault();
        undoLast();
      } else if (mode === "draw" && e.key === "Enter") {
        finishDraft();
      } else if (mode === "draw" && e.key === "Escape") {
        setDraft([]);
      } else if (mode === "draw" && e.key === "Backspace") {
        setDraft((d) => d.slice(0, -1));
      }
    };
  });
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => keys.current(e);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  function applyJSON() {
    try {
      replace(parseJSON(jsonText ?? formatJSON(rings)));
      setPreset("");
      setJsonError(undefined);
    } catch (err) {
      setJsonError(err instanceof Error ? err.message : String(err));
    }
  }

  function stripsJSON(): string {
    if (result === undefined) return "";
    if (list) {
      return JSON.stringify({ triangles: [...result.triangles] });
    }
    return JSON.stringify({
      strips: [...result.strips],
      stripEnds: [...result.stripEnds],
      stripClockwise: [...result.stripClockwise],
    });
  }

  const status = result !== undefined ? STATUS[result.status] : undefined;
  const indices =
    result === undefined ? 0 : list ? 3 * total : result.strips.length;
  const hoverInfo = (() => {
    if (result === undefined || hovered < 0 || hovered >= total) return null;
    const [a, b, c] = result.triangles.subarray(3 * hovered, 3 * hovered + 3);
    const s = strips.strip[hovered];
    const where =
      s < 0
        ? `triangle ${hovered + 1} of ${total} in the list`
        : `triangle ${strips.pos[hovered] + 1} of ${strips.counts[s]} in strip ${s + 1} of ${strips.counts.length}, which starts ${result.stripClockwise[s] !== 0 ? "clockwise" : "counter-clockwise"}`;
    return `${where} · vertices ${a}, ${b}, ${c}`;
  })();

  return (
    <div className="grid gap-6 lg:grid-cols-[300px_minmax(0,1fr)]">
      <aside className="order-2 flex flex-col gap-6 lg:order-1">
        <Card>
          <CardHeader>
            <CardTitle>Polygon</CardTitle>
            <CardDescription>
              Start from a preset, the cases of earstrip's test.cpp, or a random
              polygon of the differential tests.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="preset">Preset</Label>
              <Select value={preset} onValueChange={loadPreset}>
                <SelectTrigger id="preset" className="w-full">
                  <SelectValue placeholder="Your own" />
                </SelectTrigger>
                <SelectContent>
                  {groups.map(([group, items]) => (
                    <SelectGroup key={group}>
                      <SelectLabel>{group}</SelectLabel>
                      {items.map(({ index, name }) => (
                        <SelectItem key={index} value={String(index)}>
                          {name}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="kind">Random</Label>
              <Select value={kind} onValueChange={setKind}>
                <SelectTrigger id="kind" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {kinds.map((k) => (
                    <SelectItem key={k} value={k}>
                      {k}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <div className="flex gap-2">
                <Input
                  aria-label="Seed"
                  type="number"
                  min={0}
                  step={1}
                  value={seed}
                  onChange={(e) =>
                    setSeed(Math.max(0, Math.floor(Number(e.target.value))))
                  }
                />
                <Button onClick={() => generate(seed)}>Generate</Button>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="outline"
                      size="icon"
                      aria-label="Random seed"
                      onClick={() =>
                        generate(Math.floor(Math.random() * 1_000_000))
                      }
                    >
                      <Dices />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>Random seed</TooltipContent>
                </Tooltip>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Triangulator</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Toggle
              id="constrained"
              label="Constrained triangulation"
              hint="Cover polygons the ears cannot finish by a constrained Delaunay triangulation."
              checked={options.constrained}
              onChange={(constrained) =>
                setOptions({ ...options, constrained })
              }
            />
            <Toggle
              id="extra"
              label="Extra points"
              hint="Let it add points where edges cross."
              checked={options.extraPoints}
              onChange={(extraPoints) =>
                setOptions({ ...options, extraPoints })
              }
            />
            <Toggle
              id="forced"
              label="Forced cuts"
              hint="When the ears run out, cut the corners that do the least damage."
              checked={options.forcedCuts}
              onChange={(forcedCuts) => setOptions({ ...options, forcedCuts })}
            />
            <Toggle
              id="list"
              label="Triangle list"
              hint="Write three indices per triangle instead of strips."
              checked={options.triangleList}
              onChange={(triangleList) =>
                setOptions({ ...options, triangleList })
              }
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Display</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Toggle
              id="order"
              label="Strip order"
              hint="A line through the triangles of each strip, a dot at its start."
              checked={display.order}
              onChange={(order) => setDisplay({ ...display, order })}
            />
            <Toggle
              id="edges"
              label="Triangle edges"
              checked={display.edges}
              onChange={(edges) => setDisplay({ ...display, edges })}
            />
            <Toggle
              id="labels"
              label="Vertex indices"
              checked={display.labels}
              onChange={(labels) => setDisplay({ ...display, labels })}
            />
            <Toggle
              id="snap"
              label="Snap to integers"
              hint="New and moved points land on whole coordinates, where the predicates are exact."
              checked={display.snap}
              onChange={(snap) => setDisplay({ ...display, snap })}
            />
          </CardContent>
        </Card>
      </aside>

      <main className="order-1 flex min-w-0 flex-col gap-6 lg:order-2">
        <Card className="gap-0 overflow-hidden py-0">
          <div className="flex flex-wrap items-center gap-2 border-b p-2">
            <div className="flex gap-1">
              <Button
                variant={mode === "edit" ? "secondary" : "ghost"}
                size="sm"
                onClick={() => changeMode("edit")}
              >
                <MousePointer2 /> Edit
              </Button>
              <Button
                variant={mode === "draw" ? "secondary" : "ghost"}
                size="sm"
                onClick={() => changeMode("draw")}
              >
                <PenLine />{" "}
                {rings.length === 0 ? "Draw outer ring" : "Draw hole"}
              </Button>
            </div>
            <Separator orientation="vertical" className="h-6" />
            <Button
              variant="ghost"
              size="sm"
              disabled={undo.length === 0}
              onClick={undoLast}
            >
              <Undo2 /> Undo
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setFitKey((k) => k + 1)}
            >
              <Maximize /> Fit
            </Button>
            <Button
              variant="ghost"
              size="sm"
              disabled={rings.length === 0}
              onClick={() => {
                setPreset("");
                replace([]);
              }}
            >
              <Trash2 /> Clear
            </Button>
          </div>
          <Viewport
            rings={rings}
            points={points}
            vertices={vertices}
            triangles={result?.triangles ?? new Uint32Array()}
            strips={strips}
            shown={shownCount}
            display={display}
            mode={mode}
            draft={draft}
            dark={dark}
            fitKey={fitKey}
            hovered={hovered}
            onHover={setHovered}
            onEditStart={() => {
              editBase.current = rings;
            }}
            onChange={change}
            onDraftChange={setDraft}
            onFinishDraft={finishDraft}
          />
          <div className="flex flex-col gap-2 border-t p-3">
            <div className="flex items-center gap-3">
              <Button
                variant="outline"
                size="icon-sm"
                aria-label={playing ? "Pause" : "Play the triangles in order"}
                disabled={total === 0}
                onClick={() => {
                  if (!playing && shownCount >= total) setShown(0);
                  setPlaying(!playing);
                }}
              >
                {playing ? <Pause /> : <Play />}
              </Button>
              <Slider
                aria-label="Triangles shown"
                min={0}
                max={Math.max(total, 1)}
                step={1}
                value={[shownCount]}
                disabled={total === 0}
                onValueChange={([v]) => {
                  setPlaying(false);
                  setShown(v >= total ? null : v);
                }}
              />
              <span className="w-28 shrink-0 text-right text-sm text-muted-foreground tabular-nums">
                {shownCount} / {total}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {mode === "draw"
                ? "Click to place points; click the first point or press Enter to close the ring, Backspace removes the last point, Esc cancels. The first ring is the outer ring, the others are holes."
                : "Drag a vertex to move it (it sticks to other vertices), double-click a vertex to delete it or an edge to add one. Drag the background to pan, scroll to zoom. Hover a triangle to see its strip."}
            </p>
          </div>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex flex-wrap items-center gap-2">
              Result
              {result !== undefined && (
                <Badge variant={status?.good ? "default" : "destructive"}>
                  {result.status}
                </Badge>
              )}
              {result?.holesLeftOut && result.status !== "HolesLeftOut" && (
                <Badge variant="outline">a hole was left out</Badge>
              )}
            </CardTitle>
            <CardDescription>{run.error ?? status?.text}</CardDescription>
          </CardHeader>
          {result !== undefined && (
            <CardContent className="flex flex-col gap-4">
              <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
                <Stat
                  label="Vertices"
                  value={vertices.toLocaleString()}
                  sub={`${rings.length} ring${rings.length === 1 ? "" : "s"}`}
                />
                <Stat
                  label="Triangles"
                  value={total.toLocaleString()}
                  sub={
                    list
                      ? "triangle list"
                      : `${strips.counts.length.toLocaleString()} strip${strips.counts.length === 1 ? "" : "s"}`
                  }
                />
                <Stat
                  label="Indices"
                  value={indices.toLocaleString()}
                  sub={
                    total > 0
                      ? `${(indices / total).toFixed(2)} per triangle${list ? "" : ` (list: ${(3 * total).toLocaleString()})`}`
                      : undefined
                  }
                />
                <Stat
                  label="Time per run"
                  value={
                    timed !== undefined ? (
                      formatTime(timed.nsPerRun)
                    ) : (
                      <LoaderCircle className="size-4 animate-spin text-muted-foreground" />
                    )
                  }
                  sub={
                    timed !== undefined
                      ? `mean of ${timed.runs.toLocaleString()} runs`
                      : "measuring"
                  }
                />
                {!list && (
                  <Stat
                    label="With primitive restart"
                    value={restartLength(result).toLocaleString()}
                    sub="indices in one buffer"
                  />
                )}
                <Stat
                  label="Extra points"
                  value={(result.extra.length / 2).toLocaleString()}
                  sub={
                    result.extra.length > 0
                      ? `indices ${vertices} and up`
                      : "every index an input vertex"
                  }
                />
              </dl>
              <p className="min-h-5 text-sm text-muted-foreground">
                {hoverInfo ?? "Hover a triangle for its strip and vertices."}
              </p>
              <div className="flex flex-wrap gap-2">
                <CopyButton
                  label={list ? "Copy triangles" : "Copy strips"}
                  text={stripsJSON}
                />
                <CopyButton
                  label="Copy polygon"
                  text={() => formatJSON(rings)}
                />
              </div>
            </CardContent>
          )}
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center justify-between gap-2">
              Polygon as JSON
              <Button
                variant="outline"
                size="sm"
                onClick={() => setJsonOpen(!jsonOpen)}
              >
                {jsonOpen ? "Hide" : "Show"}
              </Button>
            </CardTitle>
            <CardDescription>
              Rings of [x, y] points, the outer ring first; a single ring or a
              GeoJSON Polygon works too.
            </CardDescription>
          </CardHeader>
          {jsonOpen && (
            <CardContent className="flex flex-col gap-3">
              <textarea
                aria-label="Polygon as JSON"
                spellCheck={false}
                className="h-64 w-full resize-y rounded-md border border-input bg-transparent p-2 font-mono text-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
                value={jsonText ?? formatJSON(rings)}
                onChange={(e) => setJsonText(e.target.value)}
              />
              {jsonError !== undefined && (
                <p className="text-sm text-destructive" role="alert">
                  {jsonError}
                </p>
              )}
              <div className="flex gap-2">
                <Button size="sm" onClick={applyJSON}>
                  Apply
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={jsonText === null}
                  onClick={() => {
                    setJsonText(null);
                    setJsonError(undefined);
                  }}
                >
                  Reset
                </Button>
              </div>
            </CardContent>
          )}
        </Card>
      </main>
    </div>
  );
}

export default function App() {
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<LoadState>({ status: "loading" });

  useEffect(() => {
    let cancelled = false;
    loadEarstrip().then(
      (api) => {
        if (!cancelled) setState({ status: "ready", api });
      },
      (err: unknown) => {
        if (!cancelled) setState({ status: "error", message: String(err) });
      },
    );
    return () => {
      cancelled = true;
    };
  }, [attempt]);

  function retry() {
    setState({ status: "loading" });
    setAttempt((a) => a + 1);
  }

  return (
    <TooltipProvider>
      <div className="mx-auto flex min-h-svh max-w-7xl flex-col gap-6 px-4 py-8">
        <Header
          goVersion={state.status === "ready" ? state.api.goVersion : undefined}
        />
        <Separator />
        {state.status === "loading" && (
          <p
            className="flex items-center gap-2 text-muted-foreground"
            role="status"
          >
            <LoaderCircle className="size-4 animate-spin" /> Loading the
            WebAssembly module…
          </p>
        )}
        {state.status === "error" && (
          <Card className="border-destructive" role="alert">
            <CardHeader>
              <CardTitle>The WebAssembly module did not load</CardTitle>
              <CardDescription>{state.message}</CardDescription>
            </CardHeader>
            <CardContent>
              <Button variant="outline" size="sm" onClick={retry}>
                Try again
              </Button>
            </CardContent>
          </Card>
        )}
        {state.status === "ready" && <Playground api={state.api} />}
        <footer className="mt-auto text-xs text-muted-foreground">
          go-earstrip © 2026 Christian Budde, a port of earstrip © 2026 Evgen
          Bodunov; ISC license.
        </footer>
      </div>
    </TooltipProvider>
  );
}
