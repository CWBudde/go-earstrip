import type { ShowcaseDemo } from "@/showcase/registry";

import { WorldMap } from "./WorldMap";

export const demo: ShowcaseDemo = {
  id: "map",
  title: "World map",
  blurb:
    "What earstrip is for: it is the triangulator of the GLMap map engine. Every country of Natural Earth goes through one batch call, and the GPU draws the whole world from one restart index buffer of triangle strips, with fewer indices than a triangle list.",
  order: 30,
  Component: WorldMap,
};
