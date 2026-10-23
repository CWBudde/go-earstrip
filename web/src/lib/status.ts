// What each Status means, as the README's status table says it.

import type { Status } from "@/wasm/earstrip";

/** The status table of the README; good marks the statuses that cover the polygon. */
export const STATUS: Record<Status, { text: string; good: boolean }> = {
  Exact: { text: "The triangles cover the polygon exactly.", good: true },
  HolesLeftOut: {
    text: "A hole could not be joined to the boundary (it lies outside or crosses it) and is left out.",
    good: false,
  },
  Constrained: {
    text: "Covered by the constrained triangulation: the rings cross, a hole could not be joined, or there are more than 128 rings. It fills what lies inside the outer ring by the non-zero winding rule and outside every hole.",
    good: true,
  },
  Approximate: {
    text: "The ears ran out (the polygon is not simple) and the rest was cut by forced cuts.",
    good: false,
  },
  Refused: {
    text: "The ears ran out and forced cuts are off: the strips cover only what the ears did.",
    good: false,
  },
};
