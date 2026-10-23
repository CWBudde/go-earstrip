# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A pure-Go, **bit-exact** port of the C++ header-only triangulator [earstrip](https://github.com/GLMap/earstrip)
(vendored at `third_party/earstrip/earstrip.hpp`). For every polygon the Go code must produce the same strips, strip
ends, orientations, status and extra points as the C++. Treat any output difference as a bug, even when the Go
result is "also valid". The README explains the public API and lists the few intended differences from the C++.

## Commands

The `justfile` holds the canonical commands (`just --list`):

```sh
just test                 # go test ./...
just lint                 # golangci-lint, with and without -tags earstrip_cgo
just test-oracle          # differential tests against the C++ via cgo (needs a C++17 compiler, use libc++)
just test-oracle -short   # the same on a sample of z13
just fetch-data           # benchmark polygons into data/ (gh, else curl; not committed)
just conformance          # hash-exact check against the C++ results stored in z13.bin / z9.bin
just fuzz [time]          # FuzzInvariants; `just fuzz-diff` fuzzes Go against the C++
just bench-file data/z13.bin -r 7 --check
just dev-demo             # browser demo: cmd/demo as js/wasm + Vite dev server; `just build-demo` for web/dist
just lint-wasm            # golangci-lint for GOOS=js (cmd/demo is invisible to plain `just lint`)
just lint-web             # eslint + tsc for web/
```

Single test: `go test -run 'TestName/subtest' .` (oracle tests: add `-tags earstrip_cgo ./internal/oracle/`).

- Data-backed tests and benchmarks **skip** when the `.bin` files are missing. They look in `$EARSTRIP_DATA`, then
  in the nearest `data/` directory upwards (so a git worktree finds the main checkout's data). Set
  `EARSTRIP_REQUIRE_DATA=1` to turn the skip into a failure.
- `EARSTRIP_CDT_ORACLE=1` enables a test in `constrained_internal_test.go` that compiles a C++ oracle for the
  constrained triangulation.
- `race_on_test.go` / `race_off_test.go` define `raceEnabled`, so timing checks are skipped under `-race`.
- CI pins golangci-lint v2.13.2 together with Go 1.27.1 for the lint job, because newer Go export data breaks that
  linter version. Runs the oracle only on macOS (libc++).

## Architecture

All of the library is in package `earstrip` at the root. Files map to parts of `earstrip.hpp`:

- `earstrip.go`: public API (`Triangulator[I]`, `Status`, `Rings`/`Polygon`/`FlatRings`, `Run`/`RunFlat`/`RunRings`,
  output accessors, `Triangulate`, `Earcut`).
- `triangulator.go`: the `Triangulator` struct with its reusable buffers, and the control flow. `runRings` →
  `runStrips` → `runOnce` (ears) and `cover` (constrained fallback). Here the status is decided: with more than 128
  rings the CDT goes first; if the ears give `Refused` or `HolesLeftOut`, the CDT replaces their result; otherwise
  forced cuts give `Approximate`.
- `ears.go`: ring classification, the spatial index of reflex "blocker" vertices, ear tests and clipping into
  zig-zag strips.
- `earindex.go`: not in the C++. A kd tree over the blockers that answers large ear tests (a bucket slab of more
  than 64 items) with the same bool as the slab scan, built lazily; it makes large rings near-linear instead of
  quadratic. `checkEarIndex` (set by `earindex_test.go`) compares both answers on every call.
- `holes.go`: bridging holes into the outer ring and joining rings that touch. `bridge` skips blocks of nodes whose
  bounds rule them out (`bridgeBlock`, not in the C++) and scans the rest in the C++ order. `cxxSort` copies
  libc++'s `std::sort` step by step so that ties in the hole order match a clang/libc++ build. Do not replace it
  with `sort`/`slices`.
- `recover.go`: what happens when the ears run out on non-simple boundaries: finding crossings, splitting
  loops/lobes, forced cuts.
- `constrained.go` + `constrained_mesh.go`: port of `detail::Constrained<Index>`, the constrained Delaunay
  triangulation. Input is snapped to a 2³⁰ grid with exact integer predicates; it inserts extra points at crossings
  and fills by non-zero winding. Names and comments follow the C++ (without the leading underscore).

Internal packages (test support only):

- `internal/oracle`: cgo wrapper around the vendored C++ header (build tag `earstrip_cgo`). Holds the differential
  tests, `FuzzDifferential` and the check that the C++ matches the hashes stored in the data files.
- `internal/cases`: the polygons of earstrip's `test/test.cpp` (named cases, fixtures, generators), the
  `Kind`-based generators for the differential tests, and `Decode` for fuzz input.
- `internal/verify`: invariant checks (exact area via int128, strip validity), shared by the package tests and the
  oracle tests.
- `internal/plydata`: reader for the PLY1 benchmark files and the FNV-1a result hash of `bench/bench.cpp`.
- `cmd/earstrip-bench`: Go counterpart of `bench/bench.cpp`.
- `internal/demo` + `cmd/demo`: the browser demo. `internal/demo` holds presets, generators, the benchmark's
  workloads (`workload.go`: polygon families at any size) and a reused `Triangulator` (native, tested); `cmd/demo`
  (`_js.go` files, js/wasm only) publishes it as `globalThis.earstrip`. `web/` is the Vite/React/shadcn front end
  (its own `go.mod` keeps `./...` out of `node_modules`), deployed to GitHub Pages by `.github/workflows/pages.yml`.
  Its Performance tab (`web/src/lib/bench.ts`, `components/Benchmark.tsx`) times go-earstrip against earcut and
  libtess.js in the browser. Its Showcase tab (`web/src/showcase/`) holds WebGL 2 demos, one per
  `showcase/<id>/index.ts` (found by `registry.ts`, loaded lazily), sharing `lib/gl.ts` (`StripBuilder`, `Mesh`),
  `Hud`, `FrameStats` and `Layout`; a demo stops its animation loop while its `active` prop is false. The world map
  uses the batch call `triangulateMany` (`internal/demo/batch.go`) and `web/public/world-*.bin`, written by
  `cmd/naturalearth` (`just naturalearth`).

## Rules for keeping it bit-exact

- **No FMA**: write products that are then added or subtracted as `float64(a*b) - float64(c*d)` (see `cross`).
  Without the explicit conversion Go may fuse them into FMA on arm64 and change the rounding.
- Use `minf`/`maxf`/`min3`/`max3`/`min4`, not the builtins `min`/`max`. They copy `std::min`/`std::max` for NaN and
  signed zeros (`minmax_internal_test.go` checks this).
- Keep the operation order, tie-breaking and iteration order of the C++. When you change the algorithm, compare
  against `earstrip.hpp` first, then run `just test-oracle` and `just conformance`. Plain unit tests do not catch
  these differences.
- Hot paths must not allocate once the buffers have grown. Buffers are reused through `grow`/`assign` and sliced to
  `[:0]`, never reallocated per run.
- A comment in a ported function usually copies the C++ comment. Keep them in sync with the header.
- A speed-up the C++ does not have (the kd tree, the bridge blocks, the radix sort in `constrained.go`) must give
  provably the same answer, not just a valid one; its comment states why. `TestDifferentialWorkloads` (large
  rings up to 100,000 vertices against the C++) and `TestDifferentialBridgeTies` guard them.
