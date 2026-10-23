//go:build earstrip_cgo

package oracle_test

// Differential tests: go-earstrip's Triangulator[uint32] against the C++ earstrip::Triangulator<uint32_t>, bit for
// bit (status, holes left out, strips, strip ends, clockwise flags, added points, triangle list, triangles), over
// the hand-made cases, generated polygons, the benchmark files and fuzz input, under several settings.

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"runtime/debug"
	"slices"
	"strings"
	"testing"

	"github.com/cwbudde/go-earstrip"
	"github.com/cwbudde/go-earstrip/internal/cases"
	"github.com/cwbudde/go-earstrip/internal/demo"
	"github.com/cwbudde/go-earstrip/internal/oracle"
	"github.com/cwbudde/go-earstrip/internal/plydata"
	"github.com/cwbudde/go-earstrip/internal/verify"
)

type setting struct {
	name string
	opts oracle.Options
}

var settings = []setting{
	{"default", oracle.Options{}},
	{"no constrained", oracle.Options{NoConstrained: true}},
	{"ears only", oracle.Options{NoConstrained: true, NoForcedCuts: true}},
	{"no extra points", oracle.Options{NoExtraPoints: true}},
	{"triangle list", oracle.Options{TriangleList: true}},
}

// pair is a Go and a C++ triangulator with the same settings, each reused across polygons.
type pair struct {
	opts oracle.Options
	tr   earstrip.Triangulator[uint32]
	o    *oracle.Oracle
	tris []uint32
}

func newPair(t testing.TB, opts oracle.Options) *pair {
	p := &pair{opts: opts, o: oracle.New()}
	t.Cleanup(p.o.Close)
	p.tr.SetConstrained(!opts.NoConstrained)
	p.tr.SetForcedCuts(!opts.NoForcedCuts)
	p.tr.SetExtraPoints(!opts.NoExtraPoints)
	p.tr.SetTriangleList(opts.TriangleList)
	return p
}

// errPanic marks a panic of the Go port.
var errPanic = errors.New("go-earstrip panicked")

// goRun runs the Go triangulator, a panic turned into an error.
func (p *pair) goRun(run func() earstrip.Status) (s earstrip.Status, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v\n%s", errPanic, r, debug.Stack())
		}
	}()
	return run(), nil
}

// compareRings runs both on rings of points.
func (p *pair) compareRings(rings [][][2]float64) error {
	want := p.o.Run(rings, p.opts)
	got, err := p.goRun(func() earstrip.Status { return p.tr.Run(rings) })
	if err != nil {
		return err
	}
	return p.diff(got, want)
}

// compareFlat runs both through the flat interface.
func (p *pair) compareFlat(coords []float64, dim int, holeStarts []int) error {
	want := p.o.RunFlat(coords, dim, holeStarts, p.opts)
	got, err := p.goRun(func() earstrip.Status { return p.tr.RunFlat(coords, dim, holeStarts) })
	if err != nil {
		return err
	}
	return p.diff(got, want)
}

// diff compares the Go triangulator's last result with the C++ one.
func (p *pair) diff(s earstrip.Status, want oracle.Result) error {
	tr := &p.tr
	if int(s) != want.Status {
		return fmt.Errorf("status %v, C++ %v", s, earstrip.Status(want.Status))
	}
	if tr.HolesLeftOut() != want.HolesLeftOut {
		return fmt.Errorf("holes left out %v, C++ %v", tr.HolesLeftOut(), want.HolesLeftOut)
	}
	if err := diffSlice("strips", tr.Strips(), want.Strips); err != nil {
		return err
	}
	if err := diffSlice("strip ends", tr.StripEnds(), want.Ends); err != nil {
		return err
	}
	if err := diffSlice("clockwise", tr.StripClockwise(), want.CW); err != nil {
		return err
	}
	if err := diffSlice("extra points", floatBits(tr.ExtraPoints()), floatBits(want.Extra)); err != nil {
		return fmt.Errorf("%w (Go %v, C++ %v)", err, tr.ExtraPoints(), want.Extra)
	}
	if err := diffSlice("triangle list", tr.TriangleList(), want.List); err != nil {
		return err
	}
	p.tris = tr.Triangles(p.tris[:0])
	return diffSlice("triangles", p.tris, want.Triangles)
}

func floatBits(v []float64) []uint64 {
	out := make([]uint64, len(v))
	for i, f := range v {
		out[i] = math.Float64bits(f)
	}
	return out
}

func diffSlice[T comparable](what string, got, want []T) error {
	if slices.Equal(got, want) {
		return nil
	}
	n := min(len(got), len(want))
	i := 0
	for i < n && got[i] == want[i] {
		i++
	}
	window := func(s []T) []T { return s[max(i-4, 0):min(i+8, len(s))] }
	return fmt.Errorf("%s differ at %d of %d (C++ %d), from %d: Go %v, C++ %v", what, i, len(got), len(want),
		max(i-4, 0), window(got), window(want))
}

// dump prints a polygon as a Go literal when it is small, its size otherwise.
func dump(rings [][][2]float64) string {
	points := 0
	for _, r := range rings {
		points += len(r)
	}
	if points > 80 {
		return fmt.Sprintf("%d rings, %d points", len(rings), points)
	}
	var b strings.Builder
	b.WriteString("[][][2]float64{")
	for k, r := range rings {
		if k > 0 {
			b.WriteString(", ")
		}
		b.WriteString("{")
		for i, q := range r {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "{%v, %v}", q[0], q[1])
		}
		b.WriteString("}")
	}
	b.WriteString("}")
	return b.String()
}

// mismatches counts differences and reports the first few, stopping a run after too many panics.
type mismatches struct {
	t      testing.TB
	n      int
	panics int
	first  []string
}

func (m *mismatches) add(name string, rings [][][2]float64, err error) bool {
	m.n++
	if errors.Is(err, errPanic) {
		m.panics++
	}
	if len(m.first) < 10 {
		m.first = append(m.first, name)
	}
	if m.n == 1 {
		m.t.Errorf("%s: %v\npolygon: %s", name, err, dump(rings))
	}
	return m.panics < 20
}

func (m *mismatches) report(total int) {
	m.t.Helper()
	if m.panics >= 20 {
		m.t.Errorf("stopped after %d panics", m.panics)
	}
	if m.n > 0 {
		m.t.Errorf("%d of %d differ from the C++; first: %s", m.n, total, strings.Join(m.first, "; "))
	}
}

// forSettings runs f once per setting as a subtest with a fresh pair.
func forSettings(t *testing.T, f func(t *testing.T, p *pair)) {
	for _, s := range settings {
		t.Run(s.name, func(t *testing.T) { f(t, newPair(t, s.opts)) })
	}
}

// handMade returns every polygon of test/test.cpp, named.
func handMade() (names []string, polys [][][][2]float64) {
	add := func(name string, poly [][][2]float64) {
		names = append(names, name)
		polys = append(polys, poly)
	}
	all := cases.Named()
	all = append(all, cases.NearSpikeVariants()...)
	all = append(all, cases.Stars()...)
	for _, cs := range all {
		add(cs.Name, verify.Float(cs.Rings))
	}
	rng := rand.New(rand.NewPCG(777, 0))
	for i := range 2000 {
		add(fmt.Sprintf("grid holes %d", i), verify.Float(cases.GridHoles(rng)))
	}
	rng = rand.New(rand.NewPCG(12345, 0))
	for i := range 3000 {
		scale := int64(1)
		if i%3 == 2 {
			scale = 40000
		}
		add(fmt.Sprintf("random %d", i), verify.Float(cases.RandomPolygon(rng, scale)))
	}
	// constrainedTests()
	add("constrained: bow tie", [][][2]float64{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}})
	add("constrained: bow tie asymmetric", [][][2]float64{{{0, 0}, {20, 20}, {20, 0}, {0, 10}}})
	add("constrained: square walked twice", [][][2]float64{{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}, {10, 0}, {10, 10}, {0, 10}}})
	add("constrained: hole outside", [][][2]float64{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}, {{20, 0}, {24, 0}, {24, 4}, {20, 4}}})
	star := [][][2]float64{nil}
	for k := range 5 {
		a := math.Pi/2 + 4*math.Pi*float64(k)/5
		star[0] = append(star[0], [2]float64{math.Round(1000 * math.Cos(a)), math.Round(1000 * math.Sin(a))})
	}
	add("constrained: pentagram", star)
	grid := [][][2]float64{{{0, 0}, {410, 0}, {410, 210}, {0, 210}}}
	for i := range 20 {
		for j := range 10 {
			x, y := float64(10+20*i), float64(10+20*j)
			grid = append(grid, [][2]float64{{x, y}, {x, y + 10}, {x + 10, y + 10}, {x + 10, y}})
		}
	}
	add("constrained: 200 holes", grid)
	add("constrained: lon/lat bow tie", [][][2]float64{{{21.0, 52.2}, {21.01, 52.21}, {21.01, 52.2}, {21.0, 52.21}}})
	big := [][][2]float64{nil}
	for k := range 70000 {
		a := 2 * math.Pi * float64(k) / 70000
		big[0] = append(big[0], [2]float64{math.Round(1e6 * math.Cos(a)), math.Round(1e6 * math.Sin(a))})
	}
	big[0] = append(big[0], [2]float64{0, 0}, [2]float64{2e6, 1})
	add("constrained: 70,002 points", big)
	lonLat := make([][][2]float64, 2)
	for i := range 20000 {
		a := 2 * math.Pi * float64(i) / 20000
		lonLat[0] = append(lonLat[0], [2]float64{21.0122 + 0.01*math.Cos(a), 52.2297 + 0.006*math.Sin(a)})
		lonLat[1] = append(lonLat[1], [2]float64{21.0122 + 0.004*math.Cos(-a), 52.2297 + 0.002*math.Sin(-a)})
	}
	add("40,000 lon/lat vertices with a hole", lonLat)
	return names, polys
}

func TestDifferentialHandMade(t *testing.T) {
	names, polys := handMade()
	forSettings(t, func(t *testing.T, p *pair) {
		m := mismatches{t: t}
		for i, poly := range polys {
			if err := p.compareRings(poly); err != nil && !m.add(names[i], poly, err) {
				break
			}
		}
		m.report(len(polys))
	})
}

func TestDifferentialRandom(t *testing.T) {
	perKind := 3000
	if testing.Short() {
		perKind = 300
	}
	forSettings(t, func(t *testing.T, p *pair) {
		m := mismatches{t: t}
		total := 0
		for _, kind := range cases.Kinds() {
			rng := rand.New(rand.NewPCG(uint64(kind), 42))
			for i := range perKind {
				poly := cases.Wild(rng, kind)
				total++
				if err := p.compareRings(poly); err != nil && !m.add(fmt.Sprintf("%v %d", kind, i), poly, err) {
					break
				}
			}
		}
		m.report(total)
	})
}

// TestDifferentialFlat compares the flat interface, two and three numbers per vertex, with holes.
func TestDifferentialFlat(t *testing.T) {
	forSettings(t, func(t *testing.T, p *pair) {
		m := mismatches{t: t}
		rng := rand.New(rand.NewPCG(3, 3))
		total := 0
		for i := range 2000 {
			poly := cases.Wild(rng, cases.Kinds()[i%int(len(cases.Kinds()))])
			for _, dim := range []int{2, 3} {
				var coords []float64
				var holes []int
				for r, ring := range poly {
					if r > 0 {
						holes = append(holes, len(coords)/dim)
					}
					for _, q := range ring {
						coords = append(coords, q[0], q[1])
						if dim == 3 {
							coords = append(coords, float64(i))
						}
					}
				}
				total++
				if err := p.compareFlat(coords, dim, holes); err != nil && !m.add(fmt.Sprintf("flat %d dim %d", i, dim), poly, err) {
					break
				}
			}
		}
		m.report(total)
	})
}

// TestDifferentialWorkloads compares the demo's benchmark polygons, large single rings among them, which neither the
// generators nor the data files have (100,000 vertices not under -short).
func TestDifferentialWorkloads(t *testing.T) {
	sizes := []int{100, 1000, 10_000}
	if !testing.Short() {
		sizes = append(sizes, 100_000)
	}
	forSettings(t, func(t *testing.T, p *pair) {
		for _, w := range demo.Workloads() {
			for _, n := range sizes {
				f, err := demo.Workload(w.Name, n)
				if err != nil {
					t.Fatal(err)
				}
				if err := p.compareFlat(f.Coords, 2, f.HoleStarts); err != nil {
					t.Errorf("%s, %d vertices: %v", w.Name, n, err)
				}
			}
		}
	})
}

// TestDifferentialShrink compares the results around Shrink on both sides.
func TestDifferentialShrink(t *testing.T) {
	forSettings(t, func(t *testing.T, p *pair) {
		rng := rand.New(rand.NewPCG(9, 9))
		for i := range 200 {
			poly := cases.Wild(rng, cases.Kinds()[i%int(len(cases.Kinds()))])
			if i%7 == 0 {
				p.o.Shrink()
				if _, err := p.goRun(func() earstrip.Status { p.tr.Shrink(); return 0 }); err != nil {
					t.Fatal(err)
				}
			}
			if err := p.compareRings(poly); err != nil {
				t.Fatalf("polygon %d: %v\n%s", i, err, dump(poly))
			}
		}
	})
}

// TestDifferentialData compares every polygon of the benchmark files (z13 sampled under -short).
func TestDifferentialData(t *testing.T) {
	for _, name := range plydata.Files {
		t.Run(name, func(t *testing.T) {
			f := loadData(t, name)
			n := limit(f.Len(), name == "z13.bin")
			forSettings(t, func(t *testing.T, p *pair) {
				m := mismatches{t: t}
				var coords []float64
				var sizes []uint32
				var poly plydata.Poly
				for i := range n {
					poly = f.Polygon(i)
					coords, sizes = poly.Flat(coords, sizes)
					want := p.o.RunSized(coords, sizes, p.opts)
					got, err := p.goRun(func() earstrip.Status { return p.tr.RunRings(&poly) })
					if err == nil {
						err = p.diff(got, want)
					}
					if err != nil && !m.add(fmt.Sprintf("polygon %d", i), poly.Nested(), err) {
						break
					}
				}
				m.report(n)
			})
		})
	}
}

// FuzzDifferential compares both on polygons decoded from fuzz bytes (cases.Decode), under every setting.
//
//	go test -tags earstrip_cgo -run '^$' -fuzz FuzzDifferential -fuzztime 2m ./internal/oracle/
func FuzzDifferential(f *testing.F) {
	for _, s := range []cases.Rings{
		{cases.Square},
		{cases.Square, {{3, 3}, {7, 3}, {7, 7}, {3, 7}}},
		{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}},
		{cases.Square, {{10, 10}, {4, 7}, {7, 4}}},
		{cases.Square, {{20, 20}, {30, 20}, {30, 30}}},
		{{{0, 0}, {10, 0}, {10, 10}, {5, 10}, {5, 15}, {5, 10}, {0, 10}}},
	} {
		f.Add(cases.Encode(s, 1))
	}
	pairs := make([]*pair, len(settings))
	for i, s := range settings {
		pairs[i] = newPair(f, s.opts)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		poly := verify.Float(cases.Decode(data))
		for i, p := range pairs {
			if err := p.compareRings(poly); err != nil {
				t.Fatalf("%s: %v\npolygon: %s", settings[i].name, err, dump(poly))
			}
		}
	})
}
