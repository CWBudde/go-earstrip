# go-earstrip

[![Go Reference](https://pkg.go.dev/badge/github.com/cwbudde/go-earstrip.svg)](https://pkg.go.dev/github.com/cwbudde/go-earstrip)
[![CI](https://github.com/CWBudde/go-earstrip/actions/workflows/ci.yml/badge.svg)](https://github.com/CWBudde/go-earstrip/actions/workflows/ci.yml)
[![Demo](https://img.shields.io/badge/demo-live-blue)](https://cwbudde.github.io/go-earstrip/)

Polygon triangulation for map data, in pure Go. A port of [earstrip](https://github.com/GLMap/earstrip) by Evgen
Bodunov, the triangulator of the [GLMap](https://globus.software) map engine.

earstrip cuts ears along zig-zag strips, so what it writes is **triangle strips**, about 1.7 indices per triangle
instead of 3. When the rings of a polygon cross or touch, as rounding to a tile grid makes them do, it covers the
polygon with a **constrained Delaunay triangulation** instead of keeping whatever triangles the ears left. The ideas
behind it are told step by step in
[Cutting map polygons into triangles](https://globus.software/blog/2026/10/16/cutting-map-polygons/).

The port is **bit-exact**: for every polygon it writes the same strips, strip ends, orientations, status and added
points as the C++ header (see [Testing](#testing)).

**[Live demo](https://cwbudde.github.io/go-earstrip/)**: draw or edit a polygon and watch it triangulated in the
browser, by the Go code compiled to WebAssembly (see [Web demo](#web-demo)).

## Install

```sh
go get github.com/cwbudde/go-earstrip
```

Pure Go, no dependencies, Go 1.24 or later.

## Usage

```go
import "github.com/cwbudde/go-earstrip"

var t earstrip.Triangulator[uint32] // reuse it: buffers only grow
status := t.Run([][][2]float64{
	{{0, 0}, {10, 0}, {10, 10}, {0, 10}}, // outer ring
	{{3, 3}, {7, 3}, {7, 7}, {3, 7}},     // a hole
})
t.Strips()         // [1 5 0 4 3 7 2 6 1 5]: indices of all strips, one after another
t.StripEnds()      // [10]: where each strip ends
t.StripClockwise() // [0]: per strip, whether its first triangle runs clockwise
t.Triangles(nil)   // or as a list, three indices per counter-clockwise triangle
t.Restart(nil, 0xFFFFFFFF) // one index buffer for TRIANGLE_STRIP with primitive restart (WebGL 2, WebGPU)
```

- **Input**: rings of points (`Run`), flat coordinates with hole starts as earcut takes them (`RunFlat`), or any type
  implementing `Rings` (`RunRings`). Ring 0 is the outer ring; either orientation is fine, a repeated closing point too.
  Indices count the vertices of all rings in order.
- **Index type**: `Triangulator[uint16]`, `[uint32]` or `[uint64]`. A polygon with more vertices than the type can
  count panics instead of wrapping.
- **One-shot helpers**: `earstrip.Triangulate(polygon)` returns a triangle list;
  `earstrip.Earcut(coords, holeIndices, dim)` takes and returns what [earcut](https://github.com/mapbox/earcut) does,
  every index an input vertex.
- **Triangle lists**: `SetTriangleList(true)` makes `Run` fill `TriangleList()` instead of the strips.
- A `Triangulator` is not safe for concurrent use; use one per goroutine. The returned slices alias its buffers and
  stay valid until the next run. `Shrink()` gives the memory back.

## Web demo

[**Try it in the browser**](https://cwbudde.github.io/go-earstrip/): the library compiled to WebAssembly. Pick one of
the cases of earstrip's `test.cpp` or a random polygon, drag, add and delete vertices or draw rings and holes, and
watch the strips, the status and the extra points change; the triangles can be stepped through in output order.
The **Performance** tab times go-earstrip, in several settings, against [earcut](https://github.com/mapbox/earcut)
and [libtess.js](https://github.com/brendankenny/libtess.js) (the GLU tessellator) on generated polygons from 100 to
100,000 vertices or on your own, and compares what they write: triangles, indices, area and extra points.
The **Showcase** tab triangulates live and draws the strips with WebGL 2, straight from the restart index buffer
(`Triangulator.Restart`):

- **World map**: every country of [Natural Earth](https://www.naturalearthdata.com) (public domain;
  `just naturalearth` converts it into `web/public/world-*.bin`) in one batch call, drawn in one draw call, strip
  by strip on load.
- **Crater sandbox**: blast craters into a tiled terrain. A tile is a square minus every crater that touches it,
  and the constrained triangulation takes the overlapping holes apart; only changed tiles run again.
- **Spirograph**: curves that cross themselves hundreds of times, filled by the non-zero winding rule every frame,
  with the extra points at the crossings, as a kaleidoscope.
- **Lava lamp**: metaballs whose iso-bands become polygons with holes, about 20 calls per frame.

```sh
just dev-demo     # cmd/demo as js/wasm, then Vite's dev server at http://localhost:5173/go-earstrip/
just build-demo   # the static site in web/dist (needs Node.js 22)
```

`cmd/demo` is the js/wasm entry point, `internal/demo` what it computes, `web/` the React front end.

## Polygons that are not simple

`Run` reports how a polygon was covered:

| Status | Meaning |
| --- | --- |
| `Exact` | the triangles cover the polygon exactly |
| `HolesLeftOut` | a hole could not be joined to the boundary (it lies outside or crosses it) and is left out |
| `Constrained` | covered by the constrained triangulation: the rings cross, a hole could not be joined, or there are more than 128 rings. It fills what lies inside the outer ring by the non-zero winding rule and outside every hole |
| `Approximate` | the ears ran out (the polygon is not simple) and the rest was cut by forced cuts |
| `Refused` | as `Approximate`, with forced cuts switched off: the strips cover only what the ears did |

Where two edges cross, the constrained triangulation adds a point next to the crossing: `ExtraPoints()` has their
coordinates, and index `n + k` in the strips is point `k`, `n` being the number of input vertices.
`SetExtraPoints(false)` keeps every index an input vertex, `SetConstrained(false)` turns the constrained
triangulation off (such polygons are then untangled and cut by forced cuts), and `SetForcedCuts(false)` stops there
instead (`Refused`). The predicates are exact for integer coordinates within ±2²⁵ in the ears and for 32-bit
integers in the constrained triangulation, which rounds other input to a 2³⁰ grid over the polygon's bounding box.
Coordinates must be finite.

## Performance

OpenStreetMap polygons in tile coordinates, best of 7 runs on one thread of an Apple M5 Pro, default settings,
measured with `go run ./cmd/earstrip-bench data/<file> -r 7` and the C++ `bench/bench.cpp` (clang -O3):

| Data | Polygons | go-earstrip | earstrip (C++) |
| --- | ---: | ---: | ---: |
| Masovian Voivodeship, zoom 13 (`z13.bin`) | 3,740,425 | 686 ms | 447 ms |
| the same at zoom 9 (`z9.bin`) | 683,742 | 93 ms | 65 ms |
| world map tiles, up to 11,675 holes in one (`world.bin`) | 56,472 | 530 ms | 432 ms |
| an older build with crossing rings (`gom4old.bin`) | 1,212,030 | 564 ms | 378 ms |

On these files the Go port runs at about two thirds of the C++ speed (four fifths on `world.bin`). For
comparison, the C++ earstrip is 13% faster than earcut.hpp on `z13.bin`.

Large rings are another matter. The C++ tests each ear against a slab of its spatial index that spans one axis,
and the zig-zag strips of a large ring cut long thin ears whose slab holds much of the ring, so the time grows
with the square of the vertices: 1.15 s for a smooth ring of 100,000 vertices. go-earstrip answers these tests
from a kd tree instead, and bridges holes and sorts the constrained triangulation's vertices with faster
structures, each giving exactly the answer of the C++ code it replaces. The same ring takes 23 ms
(`go test ./internal/demo -bench Workloads` times the demo's polygon families from 1,000 to 100,000 vertices).

A reused `Triangulator` allocates nothing once it has seen the largest polygon.

## Testing

```sh
just test          # unit tests: test.cpp ported, constrained tests, examples
just fetch-data    # the benchmark polygons (OpenStreetMap, ODbL; 147 MB packed) into data/
just conformance   # 4.4 million polygons hash-identical to the C++ results stored with the data
just test-oracle   # bit-exact differential tests against the C++ header through cgo (needs a C++17 compiler)
just fuzz          # invariants fuzzing; `just fuzz-diff` fuzzes Go against the C++
just bench         # Go benchmarks; `just bench-file data/z13.bin -r 7` mirrors bench/bench.cpp
```

The C++ header is vendored in `third_party/earstrip` for the oracle only; the library itself is pure Go. The
benchmark data comes from the [bench-data-1](https://github.com/GLMap/earstrip/releases/tag/bench-data-1) release of
earstrip and is not redistributed here.

Differences from the C++:

- A polygon with more vertices than the index type can count panics; the C++ lets the indices wrap.
- `RunFlat` panics on hole starts that decrease or lie past the vertices.
- The triangle list is cleared on every run (the C++ keeps a stale list after switching from list mode to strips on a
  polygon with more than 128 rings).
- Ties in the hole order follow libc++'s `std::sort`, so results are identical to a C++ build against libc++
  (Clang/Apple); a libstdc++ build may order holes with the same rightmost point differently.

## License

ISC, see [LICENSE](LICENSE). Original C++ © 2026 Evgen Bodunov; Go port © 2026 Christian Budde. The benchmark
polygons are © OpenStreetMap contributors under the [ODbL](https://opendatacommons.org/licenses/odbl/1-0/).
