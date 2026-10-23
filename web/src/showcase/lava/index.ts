import { lazy } from "react";

import type { ShowcaseDemo } from "@/showcase/registry";

export const demo: ShowcaseDemo = {
  id: "lava",
  title: "Lava lamp",
  blurb:
    "Metaballs rise, merge and split. Every frame, marching squares trace a few iso levels of their field, the bands between the levels become polygons with holes, and each one is triangulated and drawn from its strips, with a glow.",
  order: 40,
  Component: lazy(() =>
    import("./LavaLamp").then((m) => ({ default: m.LavaLamp })),
  ),
};
