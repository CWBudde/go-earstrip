// The canvas that draws the polygon, its triangles and strips, and edits it:
// drag a vertex, double-click a vertex to delete it or an edge to add one,
// drag the background to pan, scroll to zoom. In draw mode clicks add the
// points of a new ring.
//
// The view (center and zoom), the hover and the drag live in refs, not in
// React state: they change on every pointer move and only need a repaint.

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  type PointerEvent,
  type MouseEvent,
} from "react";

import { bounds, type Point, type Rings } from "@/lib/polygon";
import { orderColor, stripColor, type TriangleStrips } from "@/lib/strips";

export type Mode = "edit" | "draw";

export interface Display {
  edges: boolean;
  order: boolean;
  labels: boolean;
  snap: boolean;
}

export interface ViewportProps {
  rings: Rings;
  /** The input vertices, then the extra points, x, y each. */
  points: Float64Array;
  /** The number of input vertices. */
  vertices: number;
  triangles: Uint32Array;
  strips: TriangleStrips;
  /** How many triangles to draw, in order. */
  shown: number;
  display: Display;
  mode: Mode;
  draft: Point[];
  dark: boolean;
  /** Changes when the view should fit the polygon again. */
  fitKey: number;
  hovered: number;
  onHover: (triangle: number) => void;
  /** Called before an edit; the next onChange is the edit (for undo). */
  onEditStart: () => void;
  onChange: (rings: Rings) => void;
  onDraftChange: (draft: Point[]) => void;
  onFinishDraft: () => void;
}

/** The world point at the canvas center and the pixels per unit. */
interface View {
  cx: number;
  cy: number;
  scale: number;
}

interface Size {
  w: number;
  h: number;
}

type Drag =
  | { kind: "vertex"; ring: number; i: number }
  | { kind: "pan"; sx: number; sy: number; cx: number; cy: number };

interface VertexHit {
  ring: number;
  i: number;
}

/** Pick radius in CSS pixels. */
const HIT = 8;
const MAX_HANDLES = 5000;
const MAX_LABELS = 1500;
const EXTRA_COLOR = "oklch(0.7 0.19 45)";

function toScreen(v: View, s: Size, x: number, y: number): Point {
  return [(x - v.cx) * v.scale + s.w / 2, s.h / 2 - (y - v.cy) * v.scale];
}

function toWorld(v: View, s: Size, sx: number, sy: number): Point {
  return [(sx - s.w / 2) / v.scale + v.cx, (s.h / 2 - sy) / v.scale + v.cy];
}

function local(
  canvas: HTMLCanvasElement,
  e: { clientX: number; clientY: number },
): Point {
  const r = canvas.getBoundingClientRect();
  return [e.clientX - r.left, e.clientY - r.top];
}

function fitView(points: Float64Array, s: Size): View {
  const b = bounds(points);
  if (b === undefined)
    return { cx: 50, cy: 50, scale: Math.min(s.w, s.h) / 120 };
  const bw = b.maxX - b.minX;
  const bh = b.maxY - b.minY;
  const scale =
    bw === 0 && bh === 0
      ? 10
      : 0.9 * Math.min(s.w / Math.max(bw, 1e-300), s.h / Math.max(bh, 1e-300));
  return { cx: (b.minX + b.maxX) / 2, cy: (b.minY + b.maxY) / 2, scale };
}

/** The smallest of 1, 2, 5 times a power of ten that is at least min. */
function niceStep(min: number): number {
  const p = 10 ** Math.floor(Math.log10(min));
  for (const m of [1, 2, 5]) if (m * p >= min) return m * p;
  return 10 * p;
}

/** Decimals that resolve about a pixel at this zoom. */
function decimals(v: View): number {
  return Math.max(0, Math.ceil(Math.log10(v.scale)));
}

function vertexAt(
  rings: Rings,
  v: View,
  s: Size,
  sx: number,
  sy: number,
  skip?: VertexHit,
): VertexHit | null {
  let best: VertexHit | null = null;
  let bestD = HIT * HIT;
  rings.forEach((ring, r) =>
    ring.forEach(([x, y], i) => {
      if (skip !== undefined && skip.ring === r && skip.i === i) return;
      const [px, py] = toScreen(v, s, x, y);
      const d = (px - sx) ** 2 + (py - sy) ** 2;
      if (d <= bestD) {
        bestD = d;
        best = { ring: r, i };
      }
    }),
  );
  return best;
}

/** The nearest edge within the pick radius: ring, its first vertex, and the closest point on it. */
function edgeAt(
  rings: Rings,
  v: View,
  s: Size,
  sx: number,
  sy: number,
): { ring: number; i: number; point: Point } | null {
  let best: { ring: number; i: number; point: Point } | null = null;
  let bestD = HIT * HIT;
  rings.forEach((ring, r) => {
    if (ring.length < 2) return;
    ring.forEach((a, i) => {
      const b = ring[(i + 1) % ring.length];
      const [ax, ay] = toScreen(v, s, a[0], a[1]);
      const [bx, by] = toScreen(v, s, b[0], b[1]);
      const len = (bx - ax) ** 2 + (by - ay) ** 2;
      const u =
        len === 0
          ? 0
          : Math.min(
              1,
              Math.max(
                0,
                ((sx - ax) * (bx - ax) + (sy - ay) * (by - ay)) / len,
              ),
            );
      const d = (ax + u * (bx - ax) - sx) ** 2 + (ay + u * (by - ay) - sy) ** 2;
      if (d <= bestD) {
        bestD = d;
        best = {
          ring: r,
          i,
          point: [a[0] + u * (b[0] - a[0]), a[1] + u * (b[1] - a[1])],
        };
      }
    });
  });
  return best;
}

/** The topmost drawn triangle under the world point, or -1. */
function triangleAt(
  points: Float64Array,
  tri: Uint32Array,
  shown: number,
  x: number,
  y: number,
): number {
  for (let t = Math.min(shown, tri.length / 3) - 1; t >= 0; t--) {
    const a = 2 * tri[3 * t];
    const b = 2 * tri[3 * t + 1];
    const c = 2 * tri[3 * t + 2];
    const d1 =
      (points[b] - points[a]) * (y - points[a + 1]) -
      (points[b + 1] - points[a + 1]) * (x - points[a]);
    const d2 =
      (points[c] - points[b]) * (y - points[b + 1]) -
      (points[c + 1] - points[b + 1]) * (x - points[b]);
    const d3 =
      (points[a] - points[c]) * (y - points[c + 1]) -
      (points[a + 1] - points[c + 1]) * (x - points[c]);
    if ((d1 >= 0 && d2 >= 0 && d3 >= 0) || (d1 <= 0 && d2 <= 0 && d3 <= 0))
      return t;
  }
  return -1;
}

/** Rounds to the pixel's decimals, or to integers when snapping. */
function roundPoint([x, y]: Point, v: View, snap: boolean): Point {
  if (snap) return [Math.round(x), Math.round(y)];
  const d = decimals(v);
  return [Number(x.toFixed(d)), Number(y.toFixed(d))];
}

/** The point for the cursor: a vertex under it (to make rings touch), else the rounded cursor position. */
function snapPoint(
  p: ViewportProps,
  v: View,
  s: Size,
  sx: number,
  sy: number,
  skip?: VertexHit,
): Point {
  const hit = vertexAt(p.rings, v, s, sx, sy, skip);
  if (hit !== null) {
    const [x, y] = p.rings[hit.ring][hit.i];
    return [x, y];
  }
  return roundPoint(toWorld(v, s, sx, sy), v, p.display.snap);
}

function paint(
  canvas: HTMLCanvasElement,
  p: ViewportProps,
  v: View,
  s: Size,
  hover: VertexHit | null,
  cursor: Point | null,
  drag: Drag | null,
) {
  const { w, h } = s;
  if (w === 0 || h === 0) return;
  const dpr = window.devicePixelRatio || 1;
  const cw = Math.round(w * dpr);
  const ch = Math.round(h * dpr);
  if (canvas.width !== cw || canvas.height !== ch) {
    canvas.width = cw;
    canvas.height = ch;
  }
  const ctx = canvas.getContext("2d");
  if (ctx === null) return;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
  ctx.clearRect(0, 0, w, h);
  ctx.lineJoin = "round";

  const css = getComputedStyle(canvas);
  const fg = css.getPropertyValue("--foreground").trim();
  const muted = css.getPropertyValue("--muted-foreground").trim();
  const gridColor = css.getPropertyValue("--border").trim();
  const bg = css.getPropertyValue("--background").trim();
  const primary = css.getPropertyValue("--primary").trim();
  const X = (x: number) => (x - v.cx) * v.scale + w / 2;
  const Y = (y: number) => h / 2 - (y - v.cy) * v.scale;
  const pts = p.points;

  // grid
  const step = niceStep(40 / v.scale);
  const x0 = Math.ceil((v.cx - w / 2 / v.scale) / step) * step;
  const y0 = Math.ceil((v.cy - h / 2 / v.scale) / step) * step;
  ctx.strokeStyle = gridColor;
  ctx.lineWidth = 1;
  ctx.beginPath();
  for (let k = 0; k * step * v.scale <= w + 1; k++) {
    const sx = Math.round(X(x0 + k * step)) + 0.5;
    ctx.moveTo(sx, 0);
    ctx.lineTo(sx, h);
  }
  for (let k = 0; k * step * v.scale <= h + 1; k++) {
    const sy = Math.round(Y(y0 + k * step)) + 0.5;
    ctx.moveTo(0, sy);
    ctx.lineTo(w, sy);
  }
  ctx.stroke();

  // triangles, colored by strip (by order in triangle list mode)
  const tri = p.triangles;
  const total = tri.length / 3;
  const n = Math.min(p.shown, total);
  const list = p.strips.counts.length === 0;
  const hovered = p.hovered >= 0 && p.hovered < n ? p.hovered : -1;
  const hoveredStrip = hovered >= 0 ? p.strips.strip[hovered] : -1;
  const trianglePath = (t: number) => {
    const a = 2 * tri[3 * t];
    const b = 2 * tri[3 * t + 1];
    const c = 2 * tri[3 * t + 2];
    ctx.moveTo(X(pts[a]), Y(pts[a + 1]));
    ctx.lineTo(X(pts[b]), Y(pts[b + 1]));
    ctx.lineTo(X(pts[c]), Y(pts[c + 1]));
    ctx.closePath();
  };
  for (let t = 0; t < n; t++) {
    const st = p.strips.strip[t];
    const dim = hovered >= 0 && (list ? t !== hovered : st !== hoveredStrip);
    ctx.globalAlpha = dim ? 0.35 : 1;
    ctx.fillStyle = list
      ? orderColor(t, total, p.dark)
      : stripColor(st, p.dark);
    ctx.beginPath();
    trianglePath(t);
    ctx.fill();
  }
  if (p.display.edges && n > 0) {
    ctx.globalAlpha = 0.3;
    ctx.strokeStyle = fg;
    ctx.lineWidth = 0.75;
    ctx.beginPath();
    for (let t = 0; t < n; t++) trianglePath(t);
    ctx.stroke();
  }

  // the order of the triangles in each strip, through their centroids
  if (p.display.order && !list && n > 0) {
    const centroid = (t: number): Point => {
      const a = 2 * tri[3 * t];
      const b = 2 * tri[3 * t + 1];
      const c = 2 * tri[3 * t + 2];
      return [
        X((pts[a] + pts[b] + pts[c]) / 3),
        Y((pts[a + 1] + pts[b + 1] + pts[c + 1]) / 3),
      ];
    };
    ctx.globalAlpha = 0.7;
    ctx.strokeStyle = fg;
    ctx.fillStyle = fg;
    ctx.lineWidth = 1.25;
    ctx.beginPath();
    for (let t = 0; t < n; t++) {
      const [cx, cy] = centroid(t);
      if (p.strips.pos[t] === 0) ctx.moveTo(cx, cy);
      else ctx.lineTo(cx, cy);
    }
    ctx.stroke();
    ctx.beginPath();
    for (let t = 0; t < n; t++) {
      if (p.strips.pos[t] !== 0) continue;
      const [cx, cy] = centroid(t);
      ctx.moveTo(cx + 3, cy);
      ctx.arc(cx, cy, 3, 0, 2 * Math.PI);
    }
    ctx.fill();
  }

  if (hovered >= 0) {
    ctx.globalAlpha = 1;
    ctx.strokeStyle = fg;
    ctx.lineWidth = 2;
    ctx.beginPath();
    trianglePath(hovered);
    ctx.stroke();
  }

  // rings
  ctx.globalAlpha = 1;
  ctx.strokeStyle = fg;
  ctx.lineWidth = 1.5;
  ctx.beginPath();
  for (const ring of p.rings) {
    ring.forEach(([x, y], i) =>
      i === 0 ? ctx.moveTo(X(x), Y(y)) : ctx.lineTo(X(x), Y(y)),
    );
    if (ring.length > 0) ctx.closePath();
  }
  ctx.stroke();

  // vertices
  if (p.vertices <= MAX_HANDLES) {
    const r = p.vertices > 1000 ? 1.5 : 2.5;
    ctx.fillStyle = fg;
    ctx.beginPath();
    for (const ring of p.rings) {
      for (const [x, y] of ring) {
        ctx.moveTo(X(x) + r, Y(y));
        ctx.arc(X(x), Y(y), r, 0, 2 * Math.PI);
      }
    }
    ctx.fill();
  }
  const active = drag?.kind === "vertex" ? drag : hover;
  if (active !== null && p.rings[active.ring]?.[active.i] !== undefined) {
    const [x, y] = p.rings[active.ring][active.i];
    ctx.strokeStyle = fg;
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.arc(X(x), Y(y), 6, 0, 2 * Math.PI);
    ctx.stroke();
  }

  // the points the constrained triangulation added
  ctx.fillStyle = EXTRA_COLOR;
  ctx.strokeStyle = bg;
  ctx.lineWidth = 1;
  for (let i = 2 * p.vertices; i + 1 < pts.length; i += 2) {
    const x = X(pts[i]);
    const y = Y(pts[i + 1]);
    ctx.beginPath();
    ctx.moveTo(x, y - 5);
    ctx.lineTo(x + 5, y);
    ctx.lineTo(x, y + 5);
    ctx.lineTo(x - 5, y);
    ctx.closePath();
    ctx.fill();
    ctx.stroke();
  }

  // indices, as in the strips
  if (p.display.labels && pts.length / 2 <= MAX_LABELS) {
    ctx.font = "11px ui-monospace, SFMono-Regular, Menlo, monospace";
    ctx.textBaseline = "bottom";
    for (let i = 0; 2 * i + 1 < pts.length; i++) {
      const x = X(pts[2 * i]);
      const y = Y(pts[2 * i + 1]);
      if (x < -20 || x > w + 20 || y < -20 || y > h + 20) continue;
      ctx.fillStyle = i >= p.vertices ? EXTRA_COLOR : muted;
      ctx.fillText(String(i), x + 5, y - 3);
    }
  }

  // the ring being drawn
  if (p.mode === "draw" && p.draft.length > 0) {
    ctx.strokeStyle = primary;
    ctx.fillStyle = primary;
    ctx.lineWidth = 1.5;
    ctx.setLineDash([5, 4]);
    ctx.beginPath();
    p.draft.forEach(([x, y], i) =>
      i === 0 ? ctx.moveTo(X(x), Y(y)) : ctx.lineTo(X(x), Y(y)),
    );
    if (cursor !== null) ctx.lineTo(X(cursor[0]), Y(cursor[1]));
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.beginPath();
    for (const [x, y] of p.draft) {
      ctx.moveTo(X(x) + 3, Y(y));
      ctx.arc(X(x), Y(y), 3, 0, 2 * Math.PI);
    }
    ctx.fill();
    if (p.draft.length >= 3) {
      const [x, y] = p.draft[0];
      ctx.beginPath();
      ctx.arc(X(x), Y(y), HIT, 0, 2 * Math.PI);
      ctx.stroke();
    }
  }

  ctx.font = "12px ui-monospace, SFMono-Regular, Menlo, monospace";
  ctx.textBaseline = "alphabetic";
  ctx.fillStyle = muted;
  if (cursor !== null) {
    const d = p.display.snap ? 0 : decimals(v);
    ctx.fillText(`${cursor[0].toFixed(d)}, ${cursor[1].toFixed(d)}`, 8, h - 8);
  }
  if (p.rings.length === 0 && p.draft.length === 0) {
    ctx.textAlign = "center";
    ctx.fillText(
      p.mode === "draw"
        ? "Click to place the points of the outer ring"
        : "Empty: pick a preset or draw a ring",
      w / 2,
      h / 2,
    );
    ctx.textAlign = "start";
  }
}

export function Viewport(props: ViewportProps) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const propsRef = useRef(props);
  const viewRef = useRef<View>({ cx: 0, cy: 0, scale: 1 });
  const sizeRef = useRef<Size>({ w: 0, h: 0 });
  const dragRef = useRef<Drag | null>(null);
  const hoverRef = useRef<VertexHit | null>(null);
  const cursorRef = useRef<Point | null>(null);
  const frameRef = useRef(0);
  const fittedRef = useRef<number | null>(null);

  const requestPaint = useCallback(() => {
    if (frameRef.current !== 0) return;
    frameRef.current = requestAnimationFrame(() => {
      frameRef.current = 0;
      const canvas = canvasRef.current;
      if (canvas === null) return;
      paint(
        canvas,
        propsRef.current,
        viewRef.current,
        sizeRef.current,
        hoverRef.current,
        cursorRef.current,
        dragRef.current,
      );
    });
  }, []);

  // Fit the view when the canvas first gets a size and whenever fitKey changes.
  const fitIfNeeded = useCallback(() => {
    const size = sizeRef.current;
    if (size.w === 0 || fittedRef.current === propsRef.current.fitKey) return;
    fittedRef.current = propsRef.current.fitKey;
    viewRef.current = fitView(propsRef.current.points, size);
  }, []);

  useLayoutEffect(() => {
    propsRef.current = props;
    fitIfNeeded();
    requestPaint();
  });

  useEffect(() => {
    const wrap = wrapRef.current;
    if (wrap === null) return;
    const observer = new ResizeObserver(([entry]) => {
      sizeRef.current = {
        w: entry.contentRect.width,
        h: entry.contentRect.height,
      };
      fitIfNeeded();
      requestPaint();
    });
    observer.observe(wrap);
    return () => observer.disconnect();
  }, [fitIfNeeded, requestPaint]);

  // React's onWheel is passive and cannot stop the page from scrolling.
  useEffect(() => {
    const canvas = canvasRef.current;
    if (canvas === null) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      const [sx, sy] = local(canvas, e);
      const v = viewRef.current;
      const s = sizeRef.current;
      const [wx, wy] = toWorld(v, s, sx, sy);
      const scale =
        v.scale * Math.exp(-e.deltaY * (e.deltaMode === 1 ? 0.05 : 0.002));
      // keep the world point under the cursor
      viewRef.current = {
        cx: wx - (sx - s.w / 2) / scale,
        cy: wy - (s.h / 2 - sy) / scale,
        scale,
      };
      requestPaint();
    };
    canvas.addEventListener("wheel", onWheel, { passive: false });
    return () => canvas.removeEventListener("wheel", onWheel);
  }, [requestPaint]);

  function onPointerDown(e: PointerEvent<HTMLCanvasElement>) {
    if (e.button !== 0) return;
    const canvas = e.currentTarget;
    const [sx, sy] = local(canvas, e);
    const v = viewRef.current;
    const s = sizeRef.current;
    if (props.mode === "draw") {
      const d = props.draft;
      if (d.length >= 3) {
        const [fx, fy] = toScreen(v, s, d[0][0], d[0][1]);
        if ((fx - sx) ** 2 + (fy - sy) ** 2 <= HIT * HIT) {
          props.onFinishDraft();
          return;
        }
      }
      props.onDraftChange([...d, snapPoint(props, v, s, sx, sy)]);
      return;
    }
    canvas.setPointerCapture(e.pointerId);
    const hit = vertexAt(props.rings, v, s, sx, sy);
    if (hit !== null) {
      props.onEditStart();
      dragRef.current = { kind: "vertex", ...hit };
      canvas.style.cursor = "grabbing";
    } else {
      dragRef.current = { kind: "pan", sx, sy, cx: v.cx, cy: v.cy };
      canvas.style.cursor = "move";
    }
    requestPaint();
  }

  function onPointerMove(e: PointerEvent<HTMLCanvasElement>) {
    const canvas = e.currentTarget;
    const [sx, sy] = local(canvas, e);
    const v = viewRef.current;
    const s = sizeRef.current;
    const drag = dragRef.current;
    if (drag?.kind === "vertex") {
      const p = snapPoint(props, v, s, sx, sy, drag);
      cursorRef.current = p;
      const old = props.rings[drag.ring]?.[drag.i];
      if (old !== undefined && (old[0] !== p[0] || old[1] !== p[1])) {
        props.onChange(
          props.rings.map((ring, r) =>
            r === drag.ring ? ring.map((q, i) => (i === drag.i ? p : q)) : ring,
          ),
        );
      }
    } else if (drag?.kind === "pan") {
      viewRef.current = {
        ...v,
        cx: drag.cx - (sx - drag.sx) / v.scale,
        cy: drag.cy + (sy - drag.sy) / v.scale,
      };
      cursorRef.current = toWorld(viewRef.current, s, sx, sy);
    } else {
      const world = toWorld(v, s, sx, sy);
      cursorRef.current =
        props.mode === "draw" ? snapPoint(props, v, s, sx, sy) : world;
      hoverRef.current =
        props.mode === "edit" ? vertexAt(props.rings, v, s, sx, sy) : null;
      canvas.style.cursor =
        props.mode === "draw"
          ? "crosshair"
          : hoverRef.current !== null
            ? "grab"
            : "default";
      const t = triangleAt(
        props.points,
        props.triangles,
        props.shown,
        world[0],
        world[1],
      );
      if (t !== props.hovered) props.onHover(t);
    }
    requestPaint();
  }

  function onPointerUp(e: PointerEvent<HTMLCanvasElement>) {
    if (dragRef.current === null) return;
    dragRef.current = null;
    e.currentTarget.releasePointerCapture(e.pointerId);
    e.currentTarget.style.cursor = "default";
    requestPaint();
  }

  function onPointerLeave() {
    cursorRef.current = null;
    hoverRef.current = null;
    if (props.hovered !== -1) props.onHover(-1);
    requestPaint();
  }

  function onDoubleClick(e: MouseEvent<HTMLCanvasElement>) {
    if (props.mode !== "edit") return;
    const [sx, sy] = local(e.currentTarget, e);
    const v = viewRef.current;
    const s = sizeRef.current;
    const rings = props.rings;
    const hit = vertexAt(rings, v, s, sx, sy);
    if (hit !== null) {
      props.onEditStart();
      const ring = rings[hit.ring];
      props.onChange(
        ring.length <= 3
          ? rings.filter((_, r) => r !== hit.ring)
          : rings.map((q, r) =>
              r === hit.ring ? ring.filter((_, i) => i !== hit.i) : q,
            ),
      );
      hoverRef.current = null;
      return;
    }
    const edge = edgeAt(rings, v, s, sx, sy);
    if (edge !== null) {
      props.onEditStart();
      const p = roundPoint(edge.point, v, props.display.snap);
      props.onChange(
        rings.map((ring, r) =>
          r === edge.ring
            ? [...ring.slice(0, edge.i + 1), p, ...ring.slice(edge.i + 1)]
            : ring,
        ),
      );
    }
  }

  return (
    <div
      ref={wrapRef}
      className="relative h-[min(70svh,640px)] min-h-80 w-full"
    >
      <canvas
        ref={canvasRef}
        className="absolute inset-0 size-full touch-none select-none"
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
        onPointerLeave={onPointerLeave}
        onDoubleClick={onDoubleClick}
      />
    </div>
  );
}
