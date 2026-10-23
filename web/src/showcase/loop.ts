// The animation loop of a showcase demo.

import { useEffect, useRef } from "react";

/**
 * Calls frame once per animation frame while running, with the time and the
 * milliseconds since the previous frame (at most 100, so that a stall does not
 * make the animation jump). It stops when running turns false or the
 * component unmounts.
 */
export function useAnimationFrame(
  running: boolean,
  frame: (now: number, dt: number) => void,
): void {
  const ref = useRef(frame);
  useEffect(() => {
    ref.current = frame;
  });

  useEffect(() => {
    if (!running) return;
    let id = 0;
    let last = -1;
    const step = (now: number) => {
      const dt = last < 0 ? 0 : Math.min(now - last, 100);
      last = now;
      ref.current(now, dt);
      id = requestAnimationFrame(step);
    };
    id = requestAnimationFrame(step);
    return () => cancelAnimationFrame(id);
  }, [running]);
}
