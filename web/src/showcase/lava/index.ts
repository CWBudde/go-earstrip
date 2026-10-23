import type { ShowcaseDemo } from "@/showcase/registry";

import { LavaLamp } from "./LavaLamp";

export const demo: ShowcaseDemo = {
  id: "lava",
  title: "Lava lamp",
  blurb:
    "Metaballs rise, merge and split. Every frame, marching squares trace a few iso levels of their field, the bands between the levels become polygons with holes, and each one is triangulated and drawn from its strips, with a glow.",
  order: 10,
  Component: LavaLamp,
};
