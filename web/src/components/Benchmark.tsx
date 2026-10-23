// The Performance tab: times go-earstrip, earcut and libtess on the same
// polygons, one size after another, and shows the times as a chart and a
// table with what each triangulator wrote.

import { LoaderCircle, Play, Square } from "lucide-react";
import { useMemo, useRef, useState } from "react";

import { BenchChart, type Series } from "@/components/BenchChart";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  ALGORITHMS,
  Bench,
  type AlgorithmId,
  type Measurement,
  type Outcome,
} from "@/lib/bench";
import { formatTime } from "@/lib/format";
import type { Earstrip, Flat } from "@/wasm/earstrip";

/** The workload value of the polygon of the Playground tab. */
const OWN = "playground";

const SIZES = [100, 1000, 10_000, 100_000];

const BUDGETS = [
  { ms: 20, label: "Quick: 20 ms" },
  { ms: 100, label: "Steady: 100 ms" },
  { ms: 400, label: "Careful: 400 ms" },
];

/** Measurements per triangulator and size, after one to warm up. */
const ROUNDS = 3;

const COLUMNS = [
  "Time per run",
  "Relative",
  "Triangles",
  "Indices",
  "Area",
  "Extra points",
];

interface Cell {
  m?: Measurement;
  error?: string;
}

interface SizeResult {
  vertices: number;
  rings: number;
  /** go-earstrip's result with the default settings. */
  reference: Outcome;
  cells: Partial<Record<AlgorithmId, Cell>>;
}

interface Run {
  /** The polygon of the Playground tab it measured, for OWN. */
  polygon?: Flat;
  budgetMs: number;
  sizes: SizeResult[];
  error?: string;
}

interface Progress {
  done: number;
  total: number;
  what: string;
}

/** Waits until the page has painted, so that the progress shows. */
function nextFrame(): Promise<void> {
  return new Promise((resolve) =>
    requestAnimationFrame(() => setTimeout(resolve, 0)),
  );
}

function errorText(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function LineKey({ color }: { color: string }) {
  return (
    <span
      aria-hidden="true"
      className="inline-block h-0.5 w-3.5 shrink-0 rounded-full"
      style={{ background: color }}
    />
  );
}

/** The triangles' total area against go-earstrip's. */
function formatArea(area: number, reference: number): string {
  if (reference === 0) return area === 0 ? "100%" : "–";
  const ratio = area / reference;
  const diff = Math.abs(ratio - 1);
  if (diff < 1e-9) return "100%";
  return `${(100 * ratio).toFixed(diff < 1e-3 ? 4 : diff < 0.1 ? 2 : 0)}%`;
}

function SizeRows({ size, ids }: { size: SizeResult; ids: AlgorithmId[] }) {
  const times = ids.flatMap((id) => {
    const ns = size.cells[id]?.m?.nsPerRun;
    return ns === undefined ? [] : [ns];
  });
  const fastest = Math.min(...times);
  const slowest = Math.max(...times);
  return (
    <tbody className="border-t">
      <tr>
        <th
          colSpan={COLUMNS.length + 1}
          scope="colgroup"
          className="px-2 pt-4 pb-1 text-left font-medium"
        >
          {size.vertices.toLocaleString()} vertices
          <span className="font-normal text-muted-foreground">
            {" "}
            · {size.rings.toLocaleString()} ring{size.rings === 1 ? "" : "s"}
            {size.reference.status !== undefined &&
              ` · go-earstrip: ${size.reference.status}`}
          </span>
        </th>
      </tr>
      {ids.map((id) => {
        const a = ALGORITHMS.find((b) => b.id === id)!;
        const cell = size.cells[id];
        const m = cell?.m;
        const error = cell?.error ?? m?.error;
        return (
          <tr key={id} className="hover:bg-muted/50">
            <th scope="row" className="px-2 py-1.5 text-left font-normal">
              <span className="flex flex-wrap items-center gap-2">
                <LineKey color={a.color} />
                {a.name}
                {m?.status !== undefined &&
                  m.status !== size.reference.status && (
                    <Badge variant="outline">{m.status}</Badge>
                  )}
                {error !== undefined && (
                  <Badge variant="destructive">{error}</Badge>
                )}
              </span>
            </th>
            {m === undefined ? (
              <td
                colSpan={COLUMNS.length}
                className="px-2 py-1.5 text-muted-foreground"
              >
                failed
              </td>
            ) : (
              <>
                <td className="px-2 py-1.5">
                  <span className="flex items-center justify-end gap-2">
                    <span className="tabular-nums">
                      {formatTime(m.nsPerRun)}
                    </span>
                    <span
                      aria-hidden="true"
                      className="hidden h-1.5 w-20 shrink-0 sm:block"
                    >
                      <span
                        className="block h-full rounded-r-full"
                        style={{
                          width: `${Math.max(2, (100 * m.nsPerRun) / slowest)}%`,
                          background: a.color,
                        }}
                      />
                    </span>
                  </span>
                </td>
                <td
                  className={`px-2 py-1.5 text-right tabular-nums ${m.nsPerRun === fastest ? "font-semibold" : "text-muted-foreground"}`}
                >
                  {(m.nsPerRun / fastest).toFixed(2)}×
                </td>
                <td className="px-2 py-1.5 text-right tabular-nums">
                  {m.triangles.toLocaleString()}
                </td>
                <td className="px-2 py-1.5 text-right tabular-nums">
                  {m.indices.toLocaleString()}
                  <span className="text-muted-foreground">
                    {m.triangles > 0 &&
                      ` (${(m.indices / m.triangles).toFixed(2)})`}
                  </span>
                </td>
                <td className="px-2 py-1.5 text-right tabular-nums">
                  {formatArea(m.area, size.reference.area)}
                </td>
                <td className="px-2 py-1.5 text-right tabular-nums">
                  {m.extraPoints.toLocaleString()}
                </td>
              </>
            )}
          </tr>
        );
      })}
    </tbody>
  );
}

export function Benchmark({
  api,
  own,
}: {
  api: Earstrip;
  /** The polygon of the Playground tab. */
  own: Flat | undefined;
}) {
  const bench = useMemo(() => new Bench(api), [api]);
  const workloads = useMemo(() => api.workloads(), [api]);
  const [workload, setWorkload] = useState(() => workloads[0]?.name ?? OWN);
  const [largest, setLargest] = useState(10_000);
  const [budget, setBudget] = useState(100);
  const [enabled, setEnabled] = useState(
    () =>
      Object.fromEntries(ALGORITHMS.map((a) => [a.id, true])) as Record<
        AlgorithmId,
        boolean
      >,
  );
  const [results, setResults] = useState<Record<string, Run>>({});
  const [progress, setProgress] = useState<Progress | null>(null);
  const stop = useRef<{ stopped: boolean } | null>(null);

  const running = progress !== null;
  const ownVertices = own === undefined ? 0 : own.coords.length / 2;
  const about =
    workload === OWN
      ? "The polygon as you left it in the Playground tab."
      : workloads.find((w) => w.name === workload)?.about;
  const runnable =
    !running &&
    ALGORITHMS.some((a) => enabled[a.id]) &&
    (workload !== OWN || ownVertices >= 3);

  async function run() {
    const name = workload;
    const polygon = own;
    const ids = ALGORITHMS.filter((a) => enabled[a.id]).map((a) => a.id);
    const polygons: (() => Flat)[] =
      name === OWN
        ? [() => polygon!]
        : SIZES.filter((s) => s <= largest).map(
            (s) => () => api.workload(name, s),
          );
    const token = { stopped: false };
    stop.current = token;
    const entry: Run = { polygon, budgetMs: budget, sizes: [] };
    const publish = () =>
      setResults((r) => ({
        ...r,
        [name]: {
          ...entry,
          sizes: entry.sizes.map((s) => ({ ...s, cells: { ...s.cells } })),
        },
      }));
    const total = polygons.length * ids.length;
    let done = 0;
    setProgress({ done, total, what: "starting" });
    publish();
    try {
      for (const make of polygons) {
        await nextFrame();
        if (token.stopped) return;
        const p = make();
        const size: SizeResult = {
          vertices: p.coords.length / 2,
          rings: p.holeStarts.length + 1,
          reference: bench.reference(p),
          cells: {},
        };
        entry.sizes.push(size);
        for (const id of ids) {
          const a = ALGORITHMS.find((b) => b.id === id)!;
          setProgress({
            done,
            total,
            what: `${a.name}, ${size.vertices.toLocaleString()} vertices`,
          });
          await nextFrame();
          if (token.stopped) return;
          try {
            size.cells[id] = { m: bench.measure(id, p, budget, ROUNDS) };
          } catch (err) {
            size.cells[id] = { error: errorText(err) };
          }
          done++;
          publish();
        }
      }
    } catch (err) {
      entry.error = errorText(err);
      publish();
    } finally {
      stop.current = null;
      setProgress(null);
    }
  }

  const shown = results[workload];
  const ids = ALGORITHMS.map((a) => a.id).filter((id) =>
    shown?.sizes.some((s) => s.cells[id] !== undefined),
  );
  const series: Series[] = ALGORITHMS.filter((a) => ids.includes(a.id)).map(
    (a) => ({
      id: a.id,
      name: a.name,
      color: a.color,
      points: (shown?.sizes ?? []).flatMap((s) => {
        const ns = s.cells[a.id]?.m?.nsPerRun;
        return ns === undefined ? [] : [{ x: s.vertices, y: ns }];
      }),
    }),
  );
  const chartable = (shown?.sizes.length ?? 0) >= 2;
  const title =
    workload === OWN ? "Your polygon from the Playground tab" : workload;

  return (
    <div className="grid gap-6 lg:grid-cols-[300px_minmax(0,1fr)]">
      <aside className="flex flex-col gap-6">
        <Card>
          <CardHeader>
            <CardTitle>Benchmark</CardTitle>
            <CardDescription>
              Times each triangulator on the same polygons, here in your
              browser, one after another on the main thread.
            </CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <div className="flex flex-col gap-2">
              <Label htmlFor="workload">Polygons</Label>
              <Select
                value={workload}
                onValueChange={setWorkload}
                disabled={running}
              >
                <SelectTrigger id="workload" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {workloads.map((w) => (
                    <SelectItem key={w.name} value={w.name}>
                      {w.name}
                    </SelectItem>
                  ))}
                  <SelectSeparator />
                  <SelectItem value={OWN}>
                    your polygon ({ownVertices.toLocaleString()} vertices)
                  </SelectItem>
                </SelectContent>
              </Select>
              {about !== undefined && (
                <p className="text-xs text-muted-foreground">{about}</p>
              )}
            </div>
            {workload !== OWN && (
              <div className="flex flex-col gap-2">
                <Label htmlFor="largest">Sizes</Label>
                <Select
                  value={String(largest)}
                  onValueChange={(v) => setLargest(Number(v))}
                  disabled={running}
                >
                  <SelectTrigger id="largest" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SIZES.slice(1).map((s) => (
                      <SelectItem key={s} value={String(s)}>
                        100 to {s.toLocaleString()} vertices
                        {s === SIZES.at(-1) && " (slow)"}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
            <div className="flex flex-col gap-2">
              <Label htmlFor="budget">Measurement</Label>
              <Select
                value={String(budget)}
                onValueChange={(v) => setBudget(Number(v))}
                disabled={running}
              >
                <SelectTrigger id="budget" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {BUDGETS.map((b) => (
                    <SelectItem key={b.ms} value={String(b.ms)}>
                      {b.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <p className="text-xs text-muted-foreground">
                Runs repeat until a batch takes this long; the best of {ROUNDS}{" "}
                batches counts, after one to warm up. A run longer than twice
                that counts once, one longer than a second right away.
              </p>
            </div>
            {running ? (
              <Button
                variant="outline"
                onClick={() => {
                  if (stop.current !== null) stop.current.stopped = true;
                }}
              >
                <Square /> Stop
              </Button>
            ) : (
              <Button disabled={!runnable} onClick={() => void run()}>
                <Play /> Run
              </Button>
            )}
            {progress !== null && (
              <div className="flex flex-col gap-1.5" role="status">
                <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                  <div
                    className="h-full rounded-full bg-primary transition-[width]"
                    style={{
                      width: `${(100 * progress.done) / Math.max(progress.total, 1)}%`,
                    }}
                  />
                </div>
                <p className="flex items-center gap-2 text-xs text-muted-foreground">
                  <LoaderCircle className="size-3 shrink-0 animate-spin" />
                  {progress.done} of {progress.total}: {progress.what}
                </p>
              </div>
            )}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Triangulators</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            {ALGORITHMS.map((a) => (
              <Toggle
                key={a.id}
                id={`algo-${a.id}`}
                label={
                  <span className="flex items-center gap-2">
                    <LineKey color={a.color} />
                    {a.name}
                  </span>
                }
                hint={a.about}
                checked={enabled[a.id]}
                disabled={running}
                onChange={(on) => setEnabled({ ...enabled, [a.id]: on })}
              />
            ))}
          </CardContent>
        </Card>
      </aside>

      <main className="flex min-w-0 flex-col gap-6">
        {shown === undefined ? (
          <Card>
            <CardHeader>
              <CardTitle>No results yet</CardTitle>
              <CardDescription>
                Pick the polygons and press Run. The page stays usable between
                measurements but holds still during each one. At 100,000
                vertices a single run of libtess can take seconds: it slows
                down faster than linearly on large rings.
              </CardDescription>
            </CardHeader>
          </Card>
        ) : (
          <>
            {chartable && (
              <Card>
                <CardHeader>
                  <CardTitle>Time per run</CardTitle>
                  <CardDescription>{title}</CardDescription>
                </CardHeader>
                <CardContent>
                  <BenchChart series={series} />
                </CardContent>
              </Card>
            )}
            <Card>
              <CardHeader>
                <CardTitle>Results</CardTitle>
                <CardDescription>
                  {title}, best of {ROUNDS} measurements of at least{" "}
                  {shown.budgetMs} ms each. go-earstrip is timed inside
                  WebAssembly, the others in JavaScript. Indices: the length of
                  the index buffer, per triangle in brackets. Area: the
                  triangles' total area against go-earstrip's with the default
                  settings, which is the exact polygon area where its status is
                  Exact; where rings cross, the triangulators fill different
                  parts.
                  {workload === OWN &&
                    shown.polygon !== own &&
                    " Measured on an earlier version of your polygon."}
                </CardDescription>
              </CardHeader>
              <CardContent>
                {shown.error !== undefined && (
                  <p className="mb-3 text-sm text-destructive" role="alert">
                    {shown.error}
                  </p>
                )}
                <div className="overflow-x-auto">
                  <table className="w-full min-w-[640px] text-sm">
                    <thead className="text-xs text-muted-foreground">
                      <tr>
                        <th
                          scope="col"
                          className="px-2 py-1 text-left font-medium"
                        >
                          Triangulator
                        </th>
                        {COLUMNS.map((c) => (
                          <th
                            key={c}
                            scope="col"
                            className="px-2 py-1 text-right font-medium"
                          >
                            {c}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    {shown.sizes.map((s) => (
                      <SizeRows
                        key={s.vertices}
                        size={s}
                        ids={ids.filter((id) => s.cells[id] !== undefined)}
                      />
                    ))}
                  </table>
                </div>
              </CardContent>
            </Card>
          </>
        )}
      </main>
    </div>
  );
}
