package earstrip_test

// constrainedTests() of earstrip's test/test.cpp: what the constrained triangulation covers (inside ring 0 by
// non-zero winding, outside every hole), the points it adds at crossings, and when it is used.

import (
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/cwbudde/go-earstrip"
	"github.com/cwbudde/go-earstrip/internal/verify"
)

// gridPoly is a 410 x 210 rectangle with 200 square holes: past 128 rings the ears are skipped.
func gridPoly() (poly [][][2]float64, area2 float64) {
	poly = [][][2]float64{{{0, 0}, {410, 0}, {410, 210}, {0, 210}}}
	area2 = 2.0 * 410 * 210
	for i := range 20 {
		for j := range 10 {
			x, y := float64(10+20*i), float64(10+20*j)
			poly = append(poly, [][2]float64{{x, y}, {x, y + 10}, {x + 10, y + 10}, {x + 10, y}})
			area2 -= 200
		}
	}
	return poly, area2
}

func TestConstrained(t *testing.T) {
	// expect runs a default triangulator and checks status, doubled area (within tolerance) and number of added
	// points; it returns the added points.
	expect := func(t *testing.T, poly [][][2]float64, status earstrip.Status, area2 float64, extra int, tolerance float64) []float64 {
		t.Helper()
		var tr earstrip.Triangulator[uint32]
		var s earstrip.Status
		if err := protect(func() { s = tr.Run(poly) }); err != nil {
			t.Fatal(err)
		}
		area, err := verify.Cover(tr.Strips(), tr.StripEnds(), tr.StripClockwise(), verify.Points(poly), tr.ExtraPoints())
		switch {
		case err != nil:
			t.Fatal(err)
		case s != status:
			t.Fatalf("status %v, want %v", s, status)
		case math.Abs(area-area2) > tolerance:
			t.Fatalf("area %g expected %g", area/2, area2/2)
		case len(tr.ExtraPoints()) != 2*extra:
			t.Fatalf("%d added points, expected %d", len(tr.ExtraPoints())/2, extra)
		}
		return slices.Clone(tr.ExtraPoints())
	}
	bowTie := [][][2]float64{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}}

	t.Run("bow tie", func(t *testing.T) {
		extra := expect(t, bowTie, earstrip.Constrained, 100, 1, 0)
		if !slices.Equal(extra, []float64{5, 5}) {
			t.Errorf("crossing at %v, not (5, 5)", extra)
		}
	})
	t.Run("square walked twice", func(t *testing.T) {
		expect(t, [][][2]float64{{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}, {10, 0}, {10, 10}, {0, 10}}}, earstrip.Constrained, 200, 0, 0)
	})
	t.Run("hole outside", func(t *testing.T) {
		expect(t, [][][2]float64{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}, {{20, 0}, {24, 0}, {24, 4}, {20, 4}}}, earstrip.Constrained, 200, 0, 0)
	})
	// a pentagram: its centre pentagon has winding 2 and stays filled, so the cover is the star's outline through
	// the five added crossings
	t.Run("pentagram", func(t *testing.T) {
		star := [][][2]float64{nil}
		for k := range 5 {
			a := math.Pi/2 + 4*math.Pi*float64(k)/5
			star[0] = append(star[0], [2]float64{math.Round(1000 * math.Cos(a)), math.Round(1000 * math.Sin(a))})
		}
		var tr earstrip.Triangulator[uint32]
		var s earstrip.Status
		if err := protect(func() { s = tr.Run(star) }); err != nil {
			t.Fatal(err)
		}
		area, err := verify.Cover(tr.Strips(), tr.StripEnds(), tr.StripClockwise(), star[0], tr.ExtraPoints())
		if err != nil || s != earstrip.Constrained || len(tr.ExtraPoints()) != 10 {
			t.Fatalf("status %v, %d added points, %v", s, len(tr.ExtraPoints())/2, err)
		}
		ring := slices.Clone(star[0])
		for k := 0; k < 10; k += 2 {
			ring = append(ring, [2]float64{tr.ExtraPoints()[k], tr.ExtraPoints()[k+1]})
		}
		slices.SortFunc(ring, func(a, b [2]float64) int {
			return cmpFloat(math.Atan2(a[1], a[0]), math.Atan2(b[1], b[0]))
		})
		outline := 0.0
		for i, p := range ring {
			q := ring[(i+1)%len(ring)]
			outline += p[0]*q[1] - q[0]*p[1]
		}
		if math.Abs(area-outline) >= 1e-6 {
			t.Errorf("area %.0f, outline %.0f", area/2, outline/2)
		}
	})
	// 200 holes: past 128 rings the ears are skipped
	t.Run("200 holes", func(t *testing.T) {
		grid, area2 := gridPoly()
		expect(t, grid, earstrip.Constrained, area2, 0, 0)
	})
	t.Run("ears alone: 200 holes", func(t *testing.T) {
		grid, _ := gridPoly()
		var ears earstrip.Triangulator[uint32]
		ears.SetConstrained(false)
		var s earstrip.Status
		if err := protect(func() { s = ears.Run(grid) }); err != nil {
			t.Fatal(err)
		}
		if s != earstrip.Exact {
			t.Errorf("status %v, want Exact", s)
		}
	})
	// lon/lat: rounded to the grid, the crossing comes back within a grid step
	t.Run("lon/lat bow tie", func(t *testing.T) {
		poly := [][][2]float64{{{21.0, 52.2}, {21.01, 52.21}, {21.01, 52.2}, {21.0, 52.21}}}
		extra := expect(t, poly, earstrip.Constrained, 1e-4, 1, 1e-12)
		if len(extra) != 2 || math.Abs(extra[0]-21.005) >= 1e-9 || math.Abs(extra[1]-52.205) >= 1e-9 {
			t.Errorf("crossing at %v, want (21.005, 52.205)", extra)
		}
	})
	// added points refused: the bow tie goes the ears' way and every index is an input vertex
	t.Run("no added points allowed", func(t *testing.T) {
		var tr earstrip.Triangulator[uint32]
		tr.SetExtraPoints(false)
		var s earstrip.Status
		if err := protect(func() { s = tr.Run(bowTie) }); err != nil {
			t.Fatal(err)
		}
		if s != earstrip.Approximate || len(tr.ExtraPoints()) != 0 {
			t.Errorf("status %v, %d added points; want Approximate, none", s, len(tr.ExtraPoints())/2)
		}
		for _, i := range tr.Strips() {
			if i >= 4 {
				t.Errorf("index %d is not an input vertex", i)
			}
		}
	})
	// list mode gives what Triangles makes of the strips, for constrained polygons too
	t.Run("triangle list mode", func(t *testing.T) {
		var strips, list earstrip.Triangulator[uint32]
		list.SetTriangleList(true)
		var tris []uint32
		for _, poly := range [][][][2]float64{bowTie, {{{0, 0}, {20, 20}, {20, 0}, {0, 10}}}} {
			var s earstrip.Status
			err := protect(func() {
				strips.Run(poly)
				tris = strips.Triangles(tris[:0])
				s = list.Run(poly)
			})
			if err != nil {
				t.Fatal(err)
			}
			if s != earstrip.Constrained || !slices.Equal(tris, list.TriangleList()) {
				t.Errorf("%v: status %v, list %v, triangles %v", poly, s, list.TriangleList(), tris)
			}
		}
	})
	// more points than uint16 can index: the C++ keeps the constrained triangulation out and lets the ears' indices
	// wrap; the Go port refuses the input instead (the constrained limit itself is tested in constrained_internal_test.go)
	t.Run("past the index range", func(t *testing.T) {
		big := [][][2]float64{nil}
		for k := range 70000 {
			a := 2 * math.Pi * float64(k) / 70000
			big[0] = append(big[0], [2]float64{math.Round(1e6 * math.Cos(a)), math.Round(1e6 * math.Sin(a))})
		}
		big[0] = append(big[0], [2]float64{0, 0}, [2]float64{2e6, 1})
		var tr earstrip.Triangulator[uint16]
		err := protect(func() { tr.Run(big) })
		if err == nil || !strings.Contains(err.Error(), "do not fit the index type") {
			t.Errorf("want a panic about the index type, got %v", err)
		}
	})
	// Shrink frees the buffers and keeps the settings: the same polygon gives the same result again
	t.Run("shrink keeps the settings", func(t *testing.T) {
		grid, _ := gridPoly()
		var tr earstrip.Triangulator[uint32]
		tr.SetConstrained(false)
		var before []uint32
		var s earstrip.Status
		err := protect(func() {
			tr.Run(grid)
			before = slices.Clone(tr.Strips())
			tr.Shrink()
			if len(tr.Strips()) != 0 || len(tr.StripEnds()) != 0 {
				t.Error("Shrink kept the strips")
			}
			s = tr.Run(grid)
		})
		if err != nil {
			t.Fatal(err)
		}
		if s != earstrip.Exact || !slices.Equal(tr.Strips(), before) {
			t.Errorf("status %v after Shrink, strips equal %v", s, slices.Equal(tr.Strips(), before))
		}
	})
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
