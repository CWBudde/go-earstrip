// Package demo holds what the browser demo (cmd/demo) computes: the preset polygons, the generated ones and a
// triangulation run with its result and timing. cmd/demo only converts between JavaScript and these functions, so
// they build and test natively.
package demo

import (
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"github.com/cwbudde/go-earstrip"
	"github.com/cwbudde/go-earstrip/internal/cases"
)

// Flat is a polygon as RunFlat takes it: x, y per vertex of all rings in order, and the vertex index where each hole
// starts.
type Flat struct {
	Coords     []float64
	HoleStarts []int
}

// Preset is a named polygon of the demo's list.
type Preset struct {
	Group string
	Name  string
	Rings cases.Rings
}

// Presets returns the demo's polygons: a few to start with, then the cases of earstrip's test/test.cpp.
func Presets() []Preset {
	named := map[string]cases.Rings{}
	for _, c := range cases.Named() {
		named[c.Name] = c.Rings
	}
	stars := map[string]cases.Rings{}
	for _, c := range cases.Stars() {
		stars[c.Name] = c.Rings
	}
	combs := cases.Combs()
	sq := cases.Square
	const start = "Start here"
	out := []Preset{
		{start, "random polygon with holes", cases.RandomPolygon(rand.New(rand.NewPCG(34, 0)), 1)},
		{start, "square with hole", named["square with hole"]},
		{start, "81 holes", combs[2].Rings},
		{start, "comb, notches", combs[0].Rings},
		{start, "holes on a grid", cases.GridHoles(rand.New(rand.NewPCG(3, 0)))},
		{start, "bow tie (crosses itself)", cases.Rings{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}}},
		{start, "hole across the boundary", cases.Rings{cases.Scaled(sq, 2, 0, 0), {{12, 4}, {26, 6}, {24, 14}, {12, 16}}}},
		{start, "overlapping holes", cases.Rings{cases.Scaled(sq, 3, 0, 0), {{4, 4}, {18, 4}, {18, 18}, {4, 18}}, {{12, 12}, {26, 12}, {26, 26}, {12, 26}}}},
		{start, "star 7/3 (winds three times)", stars["star 7/3"]},
		{start, "near spike", cases.Rings{cases.NearSpike}},
	}
	for _, c := range cases.Named() {
		out = append(out, Preset{"test.cpp", c.Name, c.Rings})
	}
	for _, c := range cases.Stars() {
		out = append(out, Preset{"test.cpp: stars", c.Name, c.Rings})
	}
	return out
}

// FlatInts flattens integer rings.
func FlatInts(rings cases.Rings) Flat {
	var f Flat
	for r, ring := range rings {
		if r > 0 {
			f.HoleStarts = append(f.HoleStarts, len(f.Coords)/2)
		}
		for _, p := range ring {
			f.Coords = append(f.Coords, float64(p[0]), float64(p[1]))
		}
	}
	return f
}

// FlatFloats flattens float rings.
func FlatFloats(rings [][][2]float64) Flat {
	var f Flat
	for r, ring := range rings {
		if r > 0 {
			f.HoleStarts = append(f.HoleStarts, len(f.Coords)/2)
		}
		for _, p := range ring {
			f.Coords = append(f.Coords, p[0], p[1])
		}
	}
	return f
}

// Kinds returns the names of the generated polygon families, as Generate takes them.
func Kinds() []string {
	var out []string
	for _, k := range cases.Kinds() {
		out = append(out, k.String())
	}
	return out
}

// Generate returns a random polygon of the named kind (see Kinds) for the seed.
func Generate(kind string, seed uint64) (Flat, error) {
	for _, k := range cases.Kinds() {
		if k.String() == kind {
			return FlatFloats(cases.Wild(rand.New(rand.NewPCG(seed, 0)), k)), nil
		}
	}
	return Flat{}, fmt.Errorf("unknown kind %q", kind)
}

// Options are the settings of the Triangulator.
type Options struct {
	ForcedCuts   bool
	Constrained  bool
	ExtraPoints  bool
	TriangleList bool
}

// Result is what a run gives. The slices alias the Runner's buffers until its next run.
type Result struct {
	Status         earstrip.Status
	HolesLeftOut   bool
	Strips         []uint32
	StripEnds      []uint32
	StripClockwise []uint8
	// Triangles are three indices per counter-clockwise triangle: from the strips, strip by strip, or the triangle
	// list in triangle list mode.
	Triangles []uint32
	// Restart is the index buffer of Triangulator.Restart: all strips with RestartIndex between them, for one
	// TRIANGLE_STRIP draw call in WebGL 2. Empty in triangle list mode.
	Restart []uint32
	// Extra are the points the constrained triangulation added, x, y each.
	Extra []float64
}

// RestartIndex is the primitive restart index of Result.Restart, the one WebGL 2 uses for 32-bit indices.
const RestartIndex = 0xFFFFFFFF

// Runner runs one reused Triangulator, as an application would.
type Runner struct {
	t         earstrip.Triangulator[uint32]
	triangles []uint32
	restart   []uint32
}

// check returns an error for input RunFlat would panic on, or that the triangulator does not take.
func check(f Flat) error {
	if len(f.Coords)%2 != 0 {
		return fmt.Errorf("%d coordinates: want x, y pairs", len(f.Coords))
	}
	for i, v := range f.Coords {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("coordinate %d is %v: want finite numbers", i, v)
		}
	}
	prev := 0
	for _, s := range f.HoleStarts {
		if s < prev || s > len(f.Coords)/2 {
			return fmt.Errorf("hole starts %v: want them ascending, at most %d", f.HoleStarts, len(f.Coords)/2)
		}
		prev = s
	}
	return nil
}

func (r *Runner) set(o Options) {
	r.t.SetForcedCuts(o.ForcedCuts)
	r.t.SetConstrained(o.Constrained)
	r.t.SetExtraPoints(o.ExtraPoints)
	r.t.SetTriangleList(o.TriangleList)
}

// Run triangulates the polygon.
func (r *Runner) Run(f Flat, o Options) (Result, error) {
	if err := check(f); err != nil {
		return Result{}, err
	}
	r.set(o)
	status := r.t.RunFlat(f.Coords, 2, f.HoleStarts)
	res := Result{
		Status:         status,
		HolesLeftOut:   r.t.HolesLeftOut(),
		Strips:         r.t.Strips(),
		StripEnds:      r.t.StripEnds(),
		StripClockwise: r.t.StripClockwise(),
		Extra:          r.t.ExtraPoints(),
	}
	if o.TriangleList {
		res.Triangles = r.t.TriangleList()
	} else {
		r.triangles = r.t.Triangles(r.triangles[:0])
		res.Triangles = r.triangles
		r.restart = r.t.Restart(r.restart[:0], RestartIndex)
		res.Restart = r.restart
	}
	return res, nil
}

// Timing is the result of Time.
type Timing struct {
	Runs   int
	PerRun time.Duration
}

// Time runs the triangulation over and over, doubling the count until a batch takes at least budget, and returns
// the time per run of that batch. The browser's clock is coarse (often 0.1 ms), so a single run tells little.
func (r *Runner) Time(f Flat, o Options, budget time.Duration) (Timing, error) {
	if err := check(f); err != nil {
		return Timing{}, err
	}
	r.set(o)
	for n := 1; ; n *= 2 {
		start := time.Now()
		for range n {
			r.t.RunFlat(f.Coords, 2, f.HoleStarts)
		}
		elapsed := time.Since(start)
		if elapsed >= budget || n >= 1<<24 {
			return Timing{Runs: n, PerRun: elapsed / time.Duration(n)}, nil
		}
	}
}
