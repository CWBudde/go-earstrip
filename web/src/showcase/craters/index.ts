import { lazy } from "react";

import type { ShowcaseDemo } from "@/showcase/registry";

export const demo: ShowcaseDemo = {
  id: "craters",
  title: "Crater sandbox",
  blurb:
    "Blast craters into hills and caves cut into 8 × 4 tiles. Each tile is a square minus every hole that touches it, overlapping and crossing its edge, so the constrained triangulation does the boolean difference; only changed tiles are triangulated again, and balls roll on the triangles.",
  order: 20,
  Component: lazy(() =>
    import("./Craters").then((m) => ({ default: m.Craters })),
  ),
};
