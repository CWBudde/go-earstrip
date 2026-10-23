// The layout every showcase demo shares: the stage with the HUD on it, and the
// controls beside it (below it on narrow screens).

import type { ReactNode } from "react";

export function ShowcaseLayout({
  stage,
  hud,
  controls,
}: {
  /** The canvas, filling the stage. */
  stage: ReactNode;
  hud?: ReactNode;
  controls: ReactNode;
}) {
  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-[minmax(0,1fr)_300px]">
      <div className="relative aspect-[4/3] min-w-0 overflow-hidden rounded-xl border bg-card sm:aspect-video">
        {/* Absolute, so that the canvas's size in pixels does not widen the
            grid column. */}
        <div className="absolute inset-0">{stage}</div>
        {hud !== undefined && (
          <div className="pointer-events-none absolute top-2 left-2">{hud}</div>
        )}
      </div>
      <aside className="flex flex-col gap-6">{controls}</aside>
    </div>
  );
}
