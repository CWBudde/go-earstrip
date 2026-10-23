// The demos of the Showcase tab. Each lives in its own directory as
// showcase/<id>/index.ts and exports `demo`; this file finds them, so that a
// new demo needs no line here.

import type { ComponentType } from "react";

import type { Earstrip } from "@/wasm/earstrip";

export interface ShowcaseProps {
  api: Earstrip;
  /**
   * Whether the demo can be seen: false while another tab shows or the page
   * is hidden. A demo stops its animation loop while it is false.
   */
  active: boolean;
}

export interface ShowcaseDemo {
  /** The name in the address, #showcase/<id>. */
  id: string;
  title: string;
  /** One or two sentences under the picker. */
  blurb: string;
  /** Where the picker lists it, ascending. */
  order: number;
  Component: ComponentType<ShowcaseProps>;
}

const modules = import.meta.glob<{ demo: ShowcaseDemo }>("./*/index.ts", {
  eager: true,
});

export const DEMOS: ShowcaseDemo[] = Object.values(modules)
  .map((m) => m.demo)
  .sort((a, b) => a.order - b.order);
