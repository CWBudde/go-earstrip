// A canvas with a WebGL 2 context, for the stage of a showcase demo. It shows
// the error instead when the browser has no WebGL 2 or setup throws (a shader
// that does not compile, say).

import { useCallback, useRef, useState, type ComponentProps } from "react";

import { createContext } from "@/lib/gl";

/** Sets the scene up on the context; returns what tears it down. */
export type GLSetup = (
  gl: WebGL2RenderingContext,
  canvas: HTMLCanvasElement,
) => (() => void) | void;

export function GLCanvas({
  setup,
  className,
  ...props
}: {
  /** Called once when the canvas mounts; later changes are not seen. */
  setup: GLSetup;
} & Omit<ComponentProps<"canvas">, "ref">) {
  const [error, setError] = useState<string>();
  const first = useRef(setup);
  // A ref callback, not an effect: React 19 calls its cleanup on unmount.
  const attach = useCallback((canvas: HTMLCanvasElement | null) => {
    if (canvas === null) return;
    let teardown: (() => void) | void;
    try {
      teardown = first.current(createContext(canvas), canvas);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return;
    }
    return () => teardown?.();
  }, []);

  if (error !== undefined) {
    return (
      <p className="p-4 text-sm text-destructive" role="alert">
        {error}
      </p>
    );
  }
  return (
    <canvas
      ref={attach}
      className={`size-full touch-none ${className ?? ""}`}
      {...props}
    />
  );
}
