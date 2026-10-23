// The Showcase tab: a picker over the demos of registry.ts and the one picked,
// named in the address as #showcase/<id>. Only the picked demo is mounted.

import { useEffect, useState } from "react";

import { Button } from "@/components/ui/button";
import { useVisible } from "@/lib/theme";
import { DEMOS } from "@/showcase/registry";
import type { Earstrip } from "@/wasm/earstrip";

/** The demo the address names, else the first. */
function idFromHash(): string {
  const m = /^#showcase\/(.+)$/.exec(window.location.hash);
  const named = DEMOS.find((d) => d.id === m?.[1]);
  return (named ?? DEMOS[0])?.id ?? "";
}

export function Showcase({ api, active }: { api: Earstrip; active: boolean }) {
  const visible = useVisible();
  const [id, setId] = useState(idFromHash);

  useEffect(() => {
    const onHash = () => {
      if (window.location.hash.startsWith("#showcase/")) setId(idFromHash());
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  function pick(next: string) {
    setId(next);
    const url = new URL(window.location.href);
    url.hash = `showcase/${next}`;
    window.history.replaceState(null, "", url);
  }

  const demo = DEMOS.find((d) => d.id === id);
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap gap-2" role="group" aria-label="Demo">
        {DEMOS.map((d) => (
          <Button
            key={d.id}
            size="sm"
            variant={d.id === id ? "default" : "outline"}
            aria-pressed={d.id === id}
            onClick={() => pick(d.id)}
          >
            {d.title}
          </Button>
        ))}
      </div>
      {demo !== undefined && (
        <>
          <p className="text-sm text-muted-foreground">{demo.blurb}</p>
          <demo.Component key={demo.id} api={api} active={active && visible} />
        </>
      )}
    </div>
  );
}
