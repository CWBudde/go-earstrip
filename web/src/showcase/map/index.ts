import { lazy } from "react";

import type { ShowcaseDemo } from "@/showcase/registry";

export const demo: ShowcaseDemo = {
  id: "map",
  title: "World map",
  blurb:
    "What earstrip is for: it is the triangulator of the GLMap map engine. Every country of Natural Earth goes through one batch call, and the GPU draws the whole world from one restart index buffer of triangle strips, with fewer indices than a triangle list.",
  order: 10,
  Component: lazy(() =>
    import("./WorldMap").then((m) => ({ default: m.WorldMap })),
  ),
};
