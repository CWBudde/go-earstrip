import type { ShowcaseDemo } from "@/showcase/registry";

import { Pinwheel } from "./Pinwheel";

export const demo: ShowcaseDemo = {
  id: "hello",
  title: "Pinwheel",
  blurb:
    "A placeholder: a star with orbiting holes, triangulated every frame and drawn as triangle strips with WebGL 2, one color per strip.",
  order: 99,
  Component: Pinwheel,
};
