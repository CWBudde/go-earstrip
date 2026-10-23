// The time per run against the number of vertices, both on log scales, one
// line per triangulator. A crosshair snaps to the nearest size and lists every
// triangulator there; the arrow keys move it too. The table below the chart
// has every value.

import {
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type RefObject,
} from "react";

import { formatCount, formatTime, formatTimeTick } from "@/lib/format";

export interface Series {
  id: string;
  name: string;
  /** A CSS color. */
  color: string;
  /** x: vertices, y: nanoseconds per run. */
  points: { x: number; y: number }[];
}

const HEIGHT = 300;
const MARGIN = { top: 12, right: 16, bottom: 28, left: 56 };
/** Room for the direct labels at the line ends. */
const LABEL_WIDTH = 176;
/** Direct labels for at most this many series; the legend always. */
const MAX_LABELED = 4;

function useWidth(): [RefObject<HTMLDivElement | null>, number] {
  const ref = useRef<HTMLDivElement>(null);
  const [width, setWidth] = useState(0);
  useLayoutEffect(() => {
    const el = ref.current;
    if (el === null) return;
    const observer = new ResizeObserver(([entry]) =>
      setWidth(entry.contentRect.width),
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, []);
  return [ref, width];
}

/** A log scale from [lo, hi] to [a, b]. */
function logScale(lo: number, hi: number, a: number, b: number) {
  const l0 = Math.log10(lo);
  const span = Math.log10(hi) - l0 || 1;
  return (v: number) => a + ((Math.log10(v) - l0) / span) * (b - a);
}

/** A short stroke of the series color, as legend and tooltip keys. */
function LineKey({ color, dot }: { color: string; dot?: boolean }) {
  return (
    <svg width="18" height="10" aria-hidden="true" className="shrink-0">
      <line
        x1="2"
        y1="5"
        x2="16"
        y2="5"
        stroke={color}
        strokeWidth="2"
        strokeLinecap="round"
      />
      {dot && <circle cx="9" cy="5" r="3" fill={color} />}
    </svg>
  );
}

export function BenchChart({ series }: { series: Series[] }) {
  const [ref, width] = useWidth();
  const [active, setActive] = useState<number | null>(null);

  const xs = [...new Set(series.flatMap((s) => s.points.map((p) => p.x)))];
  xs.sort((a, b) => a - b);
  const ys = series.flatMap((s) => s.points.map((p) => p.y));
  const labeled = series.length <= MAX_LABELED && width >= 480;
  const plotRight = Math.max(
    width - MARGIN.right - (labeled ? LABEL_WIDTH : 0),
    MARGIN.left + 1,
  );
  const plotBottom = HEIGHT - MARGIN.bottom;

  const xLo = xs[0] ?? 1;
  const xHi = xs.at(-1) ?? 10;
  const x = logScale(
    xLo === xHi ? xLo / 2 : xLo,
    xLo === xHi ? xHi * 2 : xHi,
    MARGIN.left + 16,
    plotRight - 16,
  );
  const dLo = Math.floor(Math.log10(Math.min(...ys, 1e9)));
  const dHi = Math.max(Math.ceil(Math.log10(Math.max(...ys, 1))), dLo + 1);
  const y = logScale(10 ** dLo, 10 ** dHi, plotBottom, MARGIN.top);
  const decades: number[] = [];
  for (let d = dLo; d <= dHi; d++) decades.push(10 ** d);

  // End labels, top to bottom; one that would touch the one above is left to
  // the legend.
  const ends: { s: Series; px: number; py: number }[] = [];
  if (labeled) {
    const all = series
      .filter((s) => s.points.length > 0)
      .map((s) => {
        const last = s.points.reduce((a, b) => (b.x > a.x ? b : a));
        return { s, px: x(last.x), py: y(last.y) };
      })
      .sort((a, b) => a.py - b.py);
    for (const e of all) {
      const above = ends.at(-1);
      if (above === undefined || e.py - above.py >= 14) ends.push(e);
    }
  }

  const at = active === null ? undefined : xs[active];
  const rows =
    at === undefined
      ? []
      : series
          .flatMap((s) => {
            const p = s.points.find((q) => q.x === at);
            return p === undefined ? [] : [{ s, ns: p.y }];
          })
          .sort((a, b) => a.ns - b.ns);

  function nearest(px: number): number | null {
    let best: number | null = null;
    let dist = Infinity;
    xs.forEach((v, i) => {
      const d = Math.abs(x(v) - px);
      if (d < dist) [best, dist] = [i, d];
    });
    return best;
  }

  function onKey(e: KeyboardEvent) {
    if (xs.length === 0) return;
    if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
      e.preventDefault();
      const step = e.key === "ArrowRight" ? 1 : -1;
      setActive((a) => {
        const next = a === null ? (step > 0 ? 0 : xs.length - 1) : a + step;
        return Math.min(Math.max(next, 0), xs.length - 1);
      });
    } else if (e.key === "Escape") {
      setActive(null);
    }
  }

  const tipX = at === undefined ? 0 : x(at);
  const flip = tipX > width / 2;

  return (
    <div className="flex flex-col gap-3">
      <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
        {series.map((s) => (
          <li key={s.id} className="flex items-center gap-1.5">
            <LineKey color={s.color} dot />
            {s.name}
          </li>
        ))}
      </ul>
      <div ref={ref} className="relative">
        {width > 0 && (
          <svg
            width={width}
            height={HEIGHT}
            role="img"
            aria-label="Time per run against the number of vertices, both on log scales. The arrow keys step through the sizes; the table below has every value."
            tabIndex={0}
            className="block rounded-md outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
            onKeyDown={onKey}
            onBlur={() => setActive(null)}
            onPointerMove={(e) => {
              const box = e.currentTarget.getBoundingClientRect();
              setActive(nearest(e.clientX - box.left));
            }}
            onPointerLeave={() => setActive(null)}
          >
            {decades.map((d) => (
              <g key={d}>
                <line
                  x1={MARGIN.left}
                  x2={plotRight}
                  y1={y(d)}
                  y2={y(d)}
                  className="stroke-border"
                  strokeWidth="1"
                />
                <text
                  x={MARGIN.left - 8}
                  y={y(d)}
                  dy="0.32em"
                  textAnchor="end"
                  className="fill-muted-foreground text-xs tabular-nums"
                >
                  {formatTimeTick(d)}
                </text>
              </g>
            ))}
            {xs.map((v) => (
              <text
                key={v}
                x={x(v)}
                y={plotBottom + 18}
                textAnchor="middle"
                className="fill-muted-foreground text-xs tabular-nums"
              >
                {formatCount(v)}
              </text>
            ))}
            {at !== undefined && (
              <line
                x1={x(at)}
                x2={x(at)}
                y1={MARGIN.top}
                y2={plotBottom}
                className="stroke-muted-foreground"
                strokeWidth="1"
              />
            )}
            {series.map((s) => {
              const pts = [...s.points].sort((a, b) => a.x - b.x);
              return (
                <g key={s.id}>
                  <path
                    d={pts
                      .map(
                        (p, i) => `${i === 0 ? "M" : "L"}${x(p.x)},${y(p.y)}`,
                      )
                      .join("")}
                    fill="none"
                    stroke={s.color}
                    strokeWidth="2"
                    strokeLinejoin="round"
                    strokeLinecap="round"
                  />
                  {pts.map((p) => (
                    <circle
                      key={p.x}
                      cx={x(p.x)}
                      cy={y(p.y)}
                      r={p.x === at ? 5 : 4}
                      fill={s.color}
                      className="stroke-card"
                      strokeWidth="2"
                    />
                  ))}
                </g>
              );
            })}
            {ends.map(({ s, px, py }) => (
              <text
                key={s.id}
                x={px + 10}
                y={py}
                dy="0.32em"
                className="fill-foreground text-xs"
              >
                {s.name}
              </text>
            ))}
          </svg>
        )}
        {at !== undefined && rows.length > 0 && (
          <div
            className="pointer-events-none absolute top-2 z-10 flex min-w-52 flex-col gap-1 rounded-md border bg-popover p-2 text-xs text-popover-foreground shadow-md"
            style={flip ? { right: width - tipX + 12 } : { left: tipX + 12 }}
          >
            <p className="text-muted-foreground">
              {at.toLocaleString()} vertices
            </p>
            {rows.map(({ s, ns }) => (
              <p key={s.id} className="flex items-center gap-2">
                <LineKey color={s.color} />
                <span className="font-semibold tabular-nums">
                  {formatTime(ns)}
                </span>
                <span className="text-muted-foreground">{s.name}</span>
              </p>
            ))}
          </div>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        Vertices across, time per run up, both on log scales: a line rising one
        decade per decade is linear time, a steeper one is worse.
      </p>
    </div>
  );
}
