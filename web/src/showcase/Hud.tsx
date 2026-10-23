// The overlay on a showcase's canvas: what the triangulator did in the latest
// frame and how fast. It reads FrameStats four times a second, so a demo that
// runs at 60 frames per second does not re-render React every frame.

import { useEffect, useState, type ReactNode } from "react";

import { Badge } from "@/components/ui/badge";
import { formatCount } from "@/lib/format";
import { STATUS } from "@/lib/status";
import type { FrameStats, Snapshot } from "@/showcase/stats";
import type { Status } from "@/wasm/earstrip";

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

  const statuses = ORDER.filter((k) => (s.statuses[k] ?? 0) > 0);
  const perTriangle = s.triangles > 0 ? s.stripIndices / s.triangles : 0;
  return (
    <div className="flex w-52 flex-col gap-2 rounded-lg border bg-background/80 p-2.5 text-xs shadow-sm backdrop-blur">
      <div className="flex flex-wrap gap-1">
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
      </div>
      <dl className="grid grid-cols-[1fr_auto] gap-x-3 gap-y-0.5">
        <HudRow label="Vertices">{formatCount(s.vertices)}</HudRow>
        <HudRow label="Rings">{formatCount(s.rings)}</HudRow>
        <HudRow label="Triangles">{formatCount(s.triangles)}</HudRow>
        <HudRow label="Strips">{formatCount(s.strips)}</HudRow>
        <HudRow label="Indices">
          {formatCount(s.stripIndices)}{" "}
          <span className="text-muted-foreground">
            ({perTriangle.toFixed(2)}/tri, list 3)
          </span>
        </HudRow>
        {s.extraPoints > 0 && (
          <HudRow label="Extra points">{formatCount(s.extraPoints)}</HudRow>
        )}
        <HudRow label="Triangulate">{s.triangulateMs.toFixed(2)} ms</HudRow>
        <HudRow label="Frame rate">
          {s.fps > 0 ? `${s.fps.toFixed(0)} fps` : "paused"}
        </HudRow>
        {children}
      </dl>
    </div>
  );
}
