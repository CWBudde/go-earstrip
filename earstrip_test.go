package earstrip_test

// A port of earstrip's test/test.cpp: exact covers, flags and strips over hand-made polygons, polygons that snap
// rounding produced from OpenStreetMap tiles, and seeded random polygons; then forced cuts, floating point
// coordinates, the flat interface, index types, triangle lists, restart buffers and the helpers. The constrained
// triangulation's tests are in constrained_test.go. The polygons live in internal/cases, shared with the
// differential tests against the C++ (internal/oracle).

import (
	"fmt"
	"math"
	"math/rand/v2"
	"runtime/debug"
	"slices"
	"testing"
	"time"

	"github.com/cwbudde/go-earstrip"
	"github.com/cwbudde/go-earstrip/internal/cases"
	"github.com/cwbudde/go-earstrip/internal/verify"
)

// clipper is the Clipper of test.cpp: ears only, forced cuts off, so invalid input is refused and the cases see
// exactly what the ears do. One instance serves every case of a test function: buffers are reused.
type clipper struct {
	t earstrip.Triangulator[uint32]
}

func newClipper() *clipper {
	c := &clipper{}
	c.t.SetForcedCuts(false)
	c.t.SetConstrained(false)
	return c
}

// protect turns a panic of f into an error, so that one broken case does not end the whole test binary.
func protect(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	f()
	return nil
}

// check is check() of test.cpp: it runs the clipper and checks the result flags, the exact cover, the strip
// orientation and that no triangle is flat. Either accepts a refusal too, for degenerate input outside the
// guarantees.
func (c *clipper) check(cs cases.Case) (triangles int, err error) {
	var status earstrip.Status
	if err := protect(func() { status = c.t.Run(verify.Float(cs.Rings)) }); err != nil {
		return 0, err
	}
	ok := status != earstrip.Refused
	if (ok != cs.OK && !cs.Either) || c.t.HolesLeftOut() != cs.Dropped {
		return 0, fmt.Errorf("result %v (status %v) dropped %v, expected %v dropped %v", ok, status, c.t.HolesLeftOut(),
			cs.OK, cs.Dropped)
	}
	if !ok {
		return 0, nil
	}
	return verify.Exact(cs.Rings, c.t.Strips(), c.t.StripEnds(), cs.Dropped)
}

// checkAll checks a batch of cases quietly and reports the failures, the first few in detail.
func checkAll(t *testing.T, c *clipper, what string, list []cases.Case) {
	t.Helper()
	failed := 0
	for _, cs := range list {
		if _, err := c.check(cs); err != nil {
			if failed < 10 {
				t.Errorf("%s: %v", cs.Name, err)
			}
			failed++
		}
	}
	if failed > 0 {
		t.Errorf("%s: %d of %d failed", what, failed, len(list))
	}
}

func TestCases(t *testing.T) {
	c := newClipper()
	for _, cs := range cases.Named() {
		t.Run(cs.Name, func(t *testing.T) {
			triangles, err := c.check(cs)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("triangles=%d strips=%d indices=%d", triangles, len(c.t.StripEnds()), len(c.t.Strips()))
		})
	}
}

// Rings whose corners all turn one way but that wind around more than once are not convex: no fast path for them.
// Refusing them is allowed; an accepted one must still be covered exactly by triangles that all turn one way.
func TestNearSpikeVariants(t *testing.T) {
	checkAll(t, newClipper(), "near spike, 192 variants", cases.NearSpikeVariants())
}

func TestStarPolygons(t *testing.T) {
	checkAll(t, newClipper(), "star polygons, square walked twice", cases.Stars())
}

func TestGridHoles(t *testing.T) {
	rng := rand.New(rand.NewPCG(777, 0))
	list := make([]cases.Case, 2000)
	for i := range list {
		list[i] = cases.Case{Name: fmt.Sprintf("grid holes %d", i), Rings: cases.GridHoles(rng), OK: true}
	}
	checkAll(t, newClipper(), "tiny grid-aligned holes", list)
}

// Random valid polygons at tile and at world scale.
func TestRandomPolygons(t *testing.T) {
	rng := rand.New(rand.NewPCG(12345, 0))
	list := make([]cases.Case, 3000)
	for i := range list {
		scale := int64(1)
		if i%3 == 2 {
			scale = 40000
		}
		list[i] = cases.Case{Name: fmt.Sprintf("random %d", i), Rings: cases.RandomPolygon(rng, scale), OK: true}
	}
	checkAll(t, newClipper(), "random star polygons with holes", list)
}

// Forced cuts (the default without the constrained triangulation): invalid rings still give a triangle per vertex
// beyond two, never a refusal.
func TestForcedCuts(t *testing.T) {
	var tr earstrip.Triangulator[uint32]
	tr.SetConstrained(false)
	invalid := []struct {
		name        string
		rings       cases.Rings
		figureEight bool // has lobes to fill; a square walked twice is empty under the even-odd rule
	}{
		{"bow tie", cases.Rings{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}}, true},
		{"bow tie asymmetric", cases.Rings{{{0, 0}, {20, 20}, {20, 0}, {0, 10}}}, true},
		{"square twice", cases.Rings{{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}, {10, 0}, {10, 10}, {0, 10}}}, false},
		{"near spike", cases.Rings{cases.NearSpike}, false},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			var s earstrip.Status
			if err := protect(func() { s = tr.Run(verify.Float(tc.rings)) }); err != nil {
				t.Fatal(err)
			}
			if s != earstrip.Approximate && s != earstrip.Exact {
				t.Errorf("status %v, want Approximate or Exact", s)
			}
			triangles, begin := 0, uint32(0)
			for _, end := range tr.StripEnds() {
				triangles += int(end - begin - 2)
				begin = end
			}
			for _, i := range tr.Strips() {
				if int(i) >= len(tc.rings[0]) {
					t.Errorf("index %d out of range", i)
				}
			}
			if tc.figureEight && triangles == 0 {
				t.Error("no triangles for a figure eight")
			}
		})
	}
}

// Longitude and latitude: close doubles must not share hash buckets, or joining holes turns quadratic.
func TestLonLatVertices(t *testing.T) {
	const n = 20000
	poly := make([][][2]float64, 2)
	for i := range n {
		a := 2 * math.Pi * float64(i) / n
		poly[0] = append(poly[0], [2]float64{21.0122 + 0.01*math.Cos(a), 52.2297 + 0.006*math.Sin(a)})
		poly[1] = append(poly[1], [2]float64{21.0122 + 0.004*math.Cos(-a), 52.2297 + 0.002*math.Sin(-a)})
	}
	var tr earstrip.Triangulator[uint32]
	var s earstrip.Status
	start := time.Now()
	if err := protect(func() { s = tr.Run(poly) }); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	tris := tr.Triangles(nil)
	all := verify.Points(poly)
	area, expected := 0.0, 0.0
	for k := 0; k+2 < len(tris); k += 3 {
		a, b, c := all[tris[k]], all[tris[k+1]], all[tris[k+2]]
		area += ((b[0]-a[0])*(c[1]-a[1]) - (b[1]-a[1])*(c[0]-a[0])) / 2
	}
	for r := range poly {
		a := 0.0
		for i, p := range poly[r] {
			q := poly[r][(i+1)%len(poly[r])]
			a += p[0]*q[1] - q[0]*p[1]
		}
		sign := 1.0
		if r > 0 {
			sign = -1
		}
		expected += sign * math.Abs(a) / 2
	}
	t.Logf("%v, area error %.2e", elapsed, math.Abs(area-expected)/expected)
	if s != earstrip.Exact {
		t.Errorf("status %v, want Exact", s)
	}
	if math.Abs(area-expected) > 1e-6*expected {
		t.Errorf("area %g, expected %g", area, expected)
	}
	if len(tris) != 3*2*n {
		t.Errorf("%d triangles, want %d", len(tris)/3, 2*n)
	}
	// test.cpp allows 500 ms; a quadratic join takes far longer, a slower machine or the race detector somewhat
	if elapsed > 2*time.Second {
		t.Errorf("took %v: joining the hole turned quadratic?", elapsed)
	}
}

// intPairs is a polygon of integer pairs through the Rings interface, as test.cpp's std::pair<int, int> points.
type intPairs [][][2]int

func (p intPairs) Count() int         { return len(p) }
func (p intPairs) Size(r int) int     { return len(p[r]) }
func (p intPairs) X(r, i int) float64 { return float64(p[r][i][0]) }
func (p intPairs) Y(r, i int) float64 { return float64(p[r][i][1]) }

// The flat interface gives the same strips as the nested one; uint16 indices; pairs as points.
func TestFlatInterface(t *testing.T) {
	flat := []float64{0, 0, 10, 0, 10, 10, 0, 10, 3, 3, 7, 3, 7, 7, 3, 7}
	nested := [][][2]float64{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}, {{3, 3}, {7, 3}, {7, 7}, {3, 7}}}
	var a earstrip.Triangulator[uint32]
	var b earstrip.Triangulator[uint16]
	var fromFlat, fromNested, fromPairs []uint32
	var from16 []uint16
	err := protect(func() {
		a.RunFlat(flat, 2, []int{4})
		fromFlat = slices.Clone(a.Strips())
		a.Run(nested)
		fromNested = slices.Clone(a.Strips())
		a.RunRings(intPairs{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}, {{3, 3}, {7, 3}, {7, 7}, {3, 7}}})
		fromPairs = slices.Clone(a.Strips())
		b.Run(nested)
		from16 = slices.Clone(b.Strips())
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fromFlat) == 0 || !slices.Equal(fromFlat, fromNested) || !slices.Equal(fromPairs, fromNested) {
		t.Errorf("flat %v, nested %v, pairs %v", fromFlat, fromNested, fromPairs)
	}
	if len(from16) != len(fromNested) {
		t.Fatalf("uint16 %v, uint32 %v", from16, fromNested)
	}
	for k := range from16 {
		if uint32(from16[k]) != fromNested[k] {
			t.Fatalf("uint16 %v, uint32 %v", from16, fromNested)
		}
	}

	// three numbers per vertex, x and y first: the third is ignored
	flat3 := []float64{0, 0, 5, 10, 0, 5, 10, 10, 5, 0, 10, 5, 3, 3, 9, 7, 3, 9, 7, 7, 9, 3, 7, 9}
	var s earstrip.Status
	if err := protect(func() { s = a.RunFlat(flat3, 3, []int{4}) }); err != nil {
		t.Fatal(err)
	}
	if s != earstrip.Exact || !slices.Equal(a.Strips(), fromNested) {
		t.Errorf("dim 3: status %v strips %v, want Exact %v", s, a.Strips(), fromNested)
	}
}

// Triangles winds every triangle counter-clockwise, whichever way the input rings run; list mode writes exactly
// what Triangles makes of the strips.
func TestTriangleList(t *testing.T) {
	t.Run("winding", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(99, 0))
		var tr earstrip.Triangulator[uint32]
		var tris []uint32
		wrong := 0
		for range 200 {
			rings := cases.RandomPolygon(rng, 1)
			if err := protect(func() { tr.Run(verify.Float(rings)) }); err != nil {
				t.Fatal(err)
			}
			tris = tr.Triangles(tris[:0])
			all := verify.Points(rings)
			for k := 0; k+2 < len(tris); k += 3 {
				if int(max(tris[k], tris[k+1], tris[k+2])) >= len(all) {
					t.Fatalf("index out of range")
				}
				a, b, c := all[tris[k]], all[tris[k+1]], all[tris[k+2]]
				if (b[0]-a[0])*(c[1]-a[1])-(b[1]-a[1])*(c[0]-a[0]) <= 0 {
					wrong++
				}
			}
		}
		if wrong > 0 {
			t.Errorf("%d triangles wound the wrong way", wrong)
		}
	})
	t.Run("mode", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(99, 0))
		var strips, list earstrip.Triangulator[uint32]
		list.SetTriangleList(true)
		var tris []uint32
		differ := 0
		for range 500 {
			rings := verify.Float(cases.RandomPolygon(rng, 1))
			err := protect(func() {
				strips.Run(rings)
				tris = strips.Triangles(tris[:0])
				list.Run(rings)
			})
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(tris, list.TriangleList()) || len(list.Strips()) != 0 || len(list.StripEnds()) != 0 {
				differ++
			}
		}
		if differ > 0 {
			t.Errorf("%d of 500 differ", differ)
		}
	})
	t.Run("independent decoding", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(5, 0))
		var tr earstrip.Triangulator[uint32]
		for range 200 {
			rings := verify.Float(cases.RandomPolygon(rng, 1))
			if err := protect(func() { tr.Run(rings) }); err != nil {
				t.Fatal(err)
			}
			want := verify.TrianglesFromStrips(tr.Strips(), tr.StripEnds(), tr.StripClockwise())
			if got := tr.Triangles(nil); !slices.Equal(got, want) {
				t.Fatalf("Triangles %v, decoded from the strips %v", got, want)
			}
		}
	})
}

// Restart writes every strip into one buffer, restart between strips; decoded, it gives the triangles of
// Triangles, each counter-clockwise.
func TestRestart(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 0))
	var tr earstrip.Triangulator[uint32]
	const restart = math.MaxUint32
	for i := range 300 {
		rings := cases.Wild(rng, cases.Kinds()[i%3])
		if err := protect(func() { tr.Run(rings) }); err != nil {
			t.Fatal(err)
		}
		buf := tr.Restart(nil, restart)
		restarts := 0
		for _, v := range buf {
			if v == restart {
				restarts++
			}
		}
		if want := max(len(tr.StripEnds())-1, 0); restarts != want {
			t.Fatalf("polygon %d: %d restarts for %d strips", i, restarts, len(tr.StripEnds()))
		}
		got, want := verify.DecodeRestart(buf, restart), tr.Triangles(nil)
		if !sameTriangles(got, want) {
			t.Fatalf("polygon %d: restart buffer gives %v, Triangles %v", i, got, want)
		}
	}
	// appends to dst
	pre := []uint32{1, 2}
	if out := tr.Restart(pre, restart); !slices.Equal(out[:2], []uint32{1, 2}) {
		t.Errorf("Restart does not append: %v", out)
	}
}

// sameTriangles compares triangle lists up to the rotation of each triangle's corners.
func sameTriangles(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for k := 0; k+2 < len(a); k += 3 {
		x, y, z := a[k], a[k+1], a[k+2]
		p, q, r := b[k], b[k+1], b[k+2]
		same := (x == p && y == q && z == r) || (x == q && y == r && z == p) || (x == r && y == p && z == q)
		if !same {
			return false
		}
	}
	return true
}

// Shrink frees the buffers and the last result and keeps every setting.
func TestShrink(t *testing.T) {
	rings := [][][2]float64{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}} // a bow tie: what it gives depends on the settings
	for _, setup := range []struct {
		name string
		set  func(*earstrip.Triangulator[uint32])
	}{
		{"default", func(*earstrip.Triangulator[uint32]) {}},
		{"no constrained", func(t *earstrip.Triangulator[uint32]) { t.SetConstrained(false) }},
		{"no forced cuts", func(t *earstrip.Triangulator[uint32]) { t.SetConstrained(false); t.SetForcedCuts(false) }},
		{"no extra points", func(t *earstrip.Triangulator[uint32]) { t.SetExtraPoints(false) }},
		{"triangle list", func(t *earstrip.Triangulator[uint32]) { t.SetTriangleList(true) }},
	} {
		t.Run(setup.name, func(t *testing.T) {
			var tr earstrip.Triangulator[uint32]
			setup.set(&tr)
			var s1, s2 earstrip.Status
			var strips1, list1, strips2, list2 []uint32
			var extra1 []float64
			err := protect(func() {
				s1 = tr.Run(rings)
				strips1, list1, extra1 = slices.Clone(tr.Strips()), slices.Clone(tr.TriangleList()), slices.Clone(tr.ExtraPoints())
				tr.Shrink()
				if len(tr.Strips())+len(tr.StripEnds())+len(tr.StripClockwise())+len(tr.TriangleList())+len(tr.ExtraPoints()) != 0 || tr.HolesLeftOut() {
					t.Error("Shrink kept a result")
				}
				s2 = tr.Run(rings)
				strips2, list2 = slices.Clone(tr.Strips()), slices.Clone(tr.TriangleList())
			})
			if err != nil {
				t.Fatal(err)
			}
			if s1 != s2 || !slices.Equal(strips1, strips2) || !slices.Equal(list1, list2) || !slices.Equal(extra1, tr.ExtraPoints()) {
				t.Errorf("before Shrink %v %v %v, after %v %v %v", s1, strips1, list1, s2, strips2, list2)
			}
		})
	}
}

// The package-level helpers: Triangulate (the strips' triangles, extra points allowed) and Earcut (earcut's
// arguments and result, every index an input vertex).
func TestHelpers(t *testing.T) {
	square := []float64{0, 0, 10, 0, 10, 10, 0, 10}
	withHole := append(slices.Clone(square), 3, 3, 7, 3, 7, 7, 3, 7)
	area := func(data []float64, tri []uint32, dim int) float64 {
		sum := 0.0
		for k := 0; k+2 < len(tri); k += 3 {
			a, b, c := int(tri[k])*dim, int(tri[k+1])*dim, int(tri[k+2])*dim
			sum += ((data[b]-data[a])*(data[c+1]-data[a+1]) - (data[b+1]-data[a+1])*(data[c]-data[a])) / 2
		}
		return sum
	}
	tests := []struct {
		name  string
		run   func() []uint32
		data  []float64
		dim   int
		n     int // number of indices
		area  float64
		below uint32 // every index below
	}{
		{"Earcut square", func() []uint32 { return earstrip.Earcut(square, nil, 2) }, square, 2, 6, 100, 4},
		{"Earcut dim 0", func() []uint32 { return earstrip.Earcut(square, nil, 0) }, square, 2, 6, 100, 4},
		{"Earcut square with a hole", func() []uint32 { return earstrip.Earcut(withHole, []int{4}, 2) }, withHole, 2, 24, 84, 8},
		{"Earcut clockwise", func() []uint32 { return earstrip.Earcut([]float64{0, 0, 0, 10, 10, 10, 10, 0}, nil, 2) },
			[]float64{0, 0, 0, 10, 10, 10, 10, 0}, 2, 6, 100, 4},
		{"Earcut three dimensions", func() []uint32 { return earstrip.Earcut([]float64{0, 0, 5, 10, 0, 5, 10, 10, 5, 0, 10, 5}, nil, 3) },
			[]float64{0, 0, 5, 10, 0, 5, 10, 10, 5, 0, 10, 5}, 3, 6, 100, 4},
		{"Triangulate square with a hole", func() []uint32 {
			return earstrip.Triangulate([][][2]float64{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}, {{3, 3}, {7, 3}, {7, 7}, {3, 7}}})
		}, withHole, 2, 24, 84, 8},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var tri []uint32
			if err := protect(func() { tri = tc.run() }); err != nil {
				t.Fatal(err)
			}
			if len(tri) != tc.n || area(tc.data, tri, tc.dim) != tc.area {
				t.Errorf("%v: %d indices, area %g; want %d, %g", tri, len(tri), area(tc.data, tri, tc.dim), tc.n, tc.area)
			}
			for _, i := range tri {
				if i >= tc.below {
					t.Errorf("index %d out of range", i)
				}
			}
		})
	}
	// the bow tie: the earcut-compatible call keeps to the input vertices, Triangulate adds the crossing
	t.Run("bow tie", func(t *testing.T) {
		var ec, tri []uint32
		err := protect(func() {
			ec = earstrip.Earcut([]float64{0, 0, 10, 10, 10, 0, 0, 10}, nil, 2)
			tri = earstrip.Triangulate([][][2]float64{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}})
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(ec) == 0 || slices.Max(ec) >= 4 {
			t.Errorf("Earcut %v: want triangles over the four input vertices", ec)
		}
		if len(tri) == 0 || slices.Max(tri) != 4 {
			t.Errorf("Triangulate %v: want the added crossing, index 4", tri)
		}
	})
}

func TestStatusString(t *testing.T) {
	want := map[earstrip.Status]string{earstrip.Exact: "Exact", earstrip.HolesLeftOut: "HolesLeftOut",
		earstrip.Approximate: "Approximate", earstrip.Refused: "Refused", earstrip.Constrained: "Constrained", 9: "Status(?)"}
	for s, w := range want {
		if s.String() != w {
			t.Errorf("%d: %q, want %q", s, s.String(), w)
		}
	}
}
