// What the page follows of the system and the browser: the color scheme, the
// wish for less motion, and whether the page is visible at all.

import { useSyncExternalStore } from "react";

function useMedia(query: MediaQueryList): boolean {
  return useSyncExternalStore(
    (onChange) => {
      query.addEventListener("change", onChange);
      return () => query.removeEventListener("change", onChange);
    },
    () => query.matches,
  );
}

const darkQuery = window.matchMedia("(prefers-color-scheme: dark)");
const motionQuery = window.matchMedia("(prefers-reduced-motion: reduce)");

/** Whether the page is dark; main.tsx follows the system the same way. */
export function useDark(): boolean {
  return useMedia(darkQuery);
}

/** Whether the system asks for less motion: animations then start paused. */
export function useReducedMotion(): boolean {
  return useMedia(motionQuery);
}

/** Whether the page is visible, not in a background tab or minimized. */
export function useVisible(): boolean {
  return useSyncExternalStore(
    (onChange) => {
      document.addEventListener("visibilitychange", onChange);
      return () => document.removeEventListener("visibilitychange", onChange);
    },
    () => !document.hidden,
  );
}
