import { lazy } from "react";

import type { ShowcaseDemo } from "@/showcase/registry";

export const demo: ShowcaseDemo = {
  id: "spiro",
  title: "Spirograph",
  blurb:
    "Spirograph curves that cross themselves hundreds of times, filled every frame by the constrained triangulation by the non-zero winding rule and colored by winding number, then mirrored into a kaleidoscope. The dots are the points it inserts at the crossings.",
  order: 30,
  Component: lazy(() => import("./Spiro").then((m) => ({ default: m.Spiro }))),
};
