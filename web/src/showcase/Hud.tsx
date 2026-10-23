// The overlay on a showcase's canvas: what the triangulator did in the latest
// frame and how fast. It folds to its first line, and starts so on a phone. It
// reads FrameStats four times a second, so a demo that runs at 60 frames per
// second does not re-render React every frame.

import { ChevronDown } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { formatCount } from "@/lib/format";
import { STATUS } from "@/lib/status";
import type { FrameStats, Snapshot } from "@/showcase/stats";
import type { Status } from "@/wasm/earstrip";

/** Where the HUD starts open: tailwind's sm breakpoint and up. */
const wideQuery = window.matchMedia("(min-width: 640px)");

const ORDER: Status[] = [
  "Exact",
  "Constrained",
  "HolesLeftOut",
  "Approximate",
  "Refused",
];

/** A row of the HUD: a label and its value. */
export function HudRow({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="text-right tabular-nums">{children}</dd>
    </>
  );
}

export function Hud({
  stats,
  children,
}: {
  stats: FrameStats;
  /** More rows (HudRow) after the standard ones. */
  children?: ReactNode;
}) {
  const [s, setS] = useState<Snapshot>(() => stats.snapshot());
  useEffect(() => {
    const id = setInterval(() => setS(stats.snapshot()), 250);
    return () => clearInterval(id);
  }, [stats]);

  // On a phone the open HUD would hide most of the stage, so it starts folded
  // to its first line there.
  const [open, setOpen] = useState(() => wideQuery.matches);

  const statuses = ORDER.filter((k) => (s.statuses[k] ?? 0) > 0);
  const perTriangle = s.triangles > 0 ? s.stripIndices / s.triangles : 0;
  const fps = s.fps > 0 ? `${s.fps.toFixed(0)} fps` : "paused";
  return (
    <div
      className={`flex flex-col gap-2 rounded-lg border bg-background/80 p-2.5 text-xs shadow-sm backdrop-blur ${open ? "w-52" : ""}`}
    >
      {/* The Layout makes the HUD ignore the pointer, so that the stage gets
          it; the fold button takes it back. */}
      <button
        type="button"
        className="pointer-events-auto flex items-start gap-2 text-left"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
      >
        <span className="sr-only">Statistics</span>
        <span className="flex flex-1 flex-wrap gap-1">
          {statuses.length === 0 && <Badge variant="outline">Waiting</Badge>}
          {statuses.map((k) => (
            <Badge
              key={k}
              variant={STATUS[k].good ? "default" : "destructive"}
              title={STATUS[k].text}
            >
              {k}
              {s.calls > 1 && ` ×${s.statuses[k]}`}
            </Badge>
          ))}
          {!open && (
            <span className="self-center tabular-nums text-muted-foreground">
              {s.triangulateMs.toFixed(1)} ms · {fps}
            </span>
          )}
        </span>
        <ChevronDown
          className={`mt-0.5 size-4 shrink-0 text-muted-foreground transition-transform ${open ? "rotate-180" : ""}`}
        />
      </button>
      <dl
        className={`grid grid-cols-[1fr_auto] gap-x-3 gap-y-0.5 ${open ? "" : "hidden"}`}
      >
        <HudRow label="Vertices">{formatCount(s.vertices)}</HudRow>
        <HudRow label="Rings">{formatCount(s.rings)}</HudRow>
        <HudRow label="Triangles">{formatCount(s.triangles)}</HudRow>
        <HudRow label="Strips">{formatCount(s.strips)}</HudRow>
        <HudRow label="Index buffer">
          {formatCount(s.stripIndices)}{" "}
          <span className="text-muted-foreground">
            ({perTriangle.toFixed(2)}/tri, list 3)
          </span>
        </HudRow>
        {s.extraPoints > 0 && (
          <HudRow label="Extra points">{formatCount(s.extraPoints)}</HudRow>
        )}
        <HudRow label="Triangulate">{s.triangulateMs.toFixed(2)} ms</HudRow>
        <HudRow label="Frame rate">{fps}</HudRow>
        {children}
      </dl>
    </div>
  );
}
