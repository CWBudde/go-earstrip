// A placeholder demo and the smallest example of the showcase parts: a star
// with orbiting holes, triangulated every frame and drawn with WebGL 2 straight
// from the restart index buffer, one color per strip.

import { Pause, Play } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Slider } from "@/components/ui/slider";
import { createProgram, followSize, Mesh, StripBuilder } from "@/lib/gl";
import { useDark, useReducedMotion } from "@/lib/theme";
import { GLCanvas, type GLSetup } from "@/showcase/GLCanvas";
import { Hud } from "@/showcase/Hud";
import { ShowcaseLayout } from "@/showcase/Layout";
import { useAnimationFrame } from "@/showcase/loop";
import type { ShowcaseProps } from "@/showcase/registry";
import { FrameStats } from "@/showcase/stats";
import type { Flat } from "@/wasm/earstrip";

const VERTEX = `#version 300 es
layout(location = 0) in vec2 position;
layout(location = 1) in float strip;
uniform vec2 scale;
flat out float vStrip;
void main() {
  vStrip = strip;
  gl_Position = vec4(position * scale, 0.0, 1.0);
}`;

const FRAGMENT = `#version 300 es
precision highp float;
flat in float vStrip;
uniform float dark;
out vec4 color;
void main() {
  float h = fract(vStrip * 0.618034);
  vec3 c = 0.5 + 0.5 * cos(6.28318 * (h + vec3(0.0, 0.33, 0.67)));
  color = vec4(mix(mix(c, vec3(1.0), 0.35), c * 0.75, dark), 1.0);
}`;

/** The star with spikes points and its holes at time t, in [-1, 1]². */
function star(spikes: number, t: number): Flat {
  const coords: number[] = [];
  const n = spikes * 2;
  for (let i = 0; i < n; i++) {
    const a = (i / n) * 2 * Math.PI + 0.2 * t;
    const r = i % 2 === 0 ? 0.95 : 0.55 + 0.1 * Math.sin(3 * t + i);
    coords.push(r * Math.cos(a), r * Math.sin(a));
  }
  const holeStarts: number[] = [];
  const holes = Math.max(2, Math.floor(spikes / 3));
  for (let h = 0; h < holes; h++) {
    holeStarts.push(coords.length / 2);
    const c = (h / holes) * 2 * Math.PI - 0.5 * t;
    const cx = 0.3 * Math.cos(c);
    const cy = 0.3 * Math.sin(c);
    // Clockwise, as a hole; the triangulator takes either orientation.
    for (let k = 24; k > 0; k--) {
      const a = (k / 24) * 2 * Math.PI;
      coords.push(cx + 0.12 * Math.cos(a), cy + 0.12 * Math.sin(a));
    }
  }
  return { coords: new Float64Array(coords), holeStarts };
}

interface Scene {
  gl: WebGL2RenderingContext;
  mesh: Mesh;
  scale: WebGLUniformLocation | null;
  dark: WebGLUniformLocation | null;
}

export function Pinwheel({ api, active }: ShowcaseProps) {
  const dark = useDark();
  const reduced = useReducedMotion();
  const [playing, setPlaying] = useState(!reduced);
  const [spikes, setSpikes] = useState(12);
  const scene = useRef<Scene>(undefined);
  const time = useRef(0);
  const stats = useMemo(() => new FrameStats(), []);
  const builder = useMemo(() => new StripBuilder(), []);

  // The latest settings for render, which the loop and the resize call.
  const settings = useRef({ spikes, dark });
  useEffect(() => {
    settings.current = { spikes, dark };
  });

  function render() {
    const s = scene.current;
    if (s === undefined) return;
    const polygon = star(settings.current.spikes, time.current);
    const r = stats.run(api, polygon);
    builder.reset();
    builder.add(r, polygon.coords);
    s.mesh.attribute(0, builder.positions, 2);
    s.mesh.attribute(1, builder.strips, 1);
    s.mesh.indices(builder.indices);

    const { gl } = s;
    gl.viewport(0, 0, gl.canvas.width, gl.canvas.height);
    gl.clearColor(0, 0, 0, 0);
    gl.clear(gl.COLOR_BUFFER_BIT);
    const aspect = gl.canvas.width / gl.canvas.height;
    gl.uniform2f(
      s.scale,
      Math.min(1, 1 / aspect) * 0.95,
      Math.min(1, aspect) * 0.95,
    );
    gl.uniform1f(s.dark, settings.current.dark ? 1 : 0);
    s.mesh.draw();
  }
  const renderRef = useRef(render);
  useEffect(() => {
    renderRef.current = render;
  });

  const setup: GLSetup = (gl, canvas) => {
    const program = createProgram(gl, VERTEX, FRAGMENT);
    gl.useProgram(program);
    const mesh = new Mesh(gl);
    scene.current = {
      gl,
      mesh,
      scale: gl.getUniformLocation(program, "scale"),
      dark: gl.getUniformLocation(program, "dark"),
    };
    const stop = followSize(canvas, () => renderRef.current());
    return () => {
      stop();
      mesh.dispose();
      gl.deleteProgram(program);
      scene.current = undefined;
    };
  };

  const running = active && playing;
  useEffect(() => {
    if (!running) {
      stats.pause();
      renderRef.current();
    }
  }, [running, spikes, dark, stats]);

  useAnimationFrame(running, (now, dt) => {
    time.current += dt / 1000;
    stats.begin(now);
    render();
    stats.end();
  });

  return (
    <ShowcaseLayout
      stage={<GLCanvas setup={setup} />}
      hud={<Hud stats={stats} />}
      controls={
        <Card>
          <CardHeader>
            <CardTitle>Pinwheel</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-4">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPlaying((p) => !p)}
            >
              {playing ? <Pause /> : <Play />} {playing ? "Pause" : "Play"}
            </Button>
            <div className="flex flex-col gap-2">
              <Label htmlFor="pinwheel-spikes">Spikes: {spikes}</Label>
              <Slider
                id="pinwheel-spikes"
                min={3}
                max={200}
                value={[spikes]}
                onValueChange={([v]) => setSpikes(v)}
              />
            </div>
          </CardContent>
        </Card>
      }
    />
  );
}
