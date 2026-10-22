package earstrip

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

// Internal tests of the ear clipping: the cases of earstrip's test.cpp main(), ears only (forced cuts and the
// constrained triangulation off), checked as test.cpp checks them.

type triPt = [2]float64

func triRing(pts ...int) []triPt {
	r := make([]triPt, 0, len(pts)/2)
	for i := 0; i+1 < len(pts); i += 2 {
		r = append(r, triPt{float64(pts[i]), float64(pts[i+1])})
	}
	return r
}

func triReversed(r []triPt) []triPt {
	out := slices.Clone(r)
	slices.Reverse(out)
	return out
}

func triScaled(r []triPt, s, dx, dy float64) []triPt {
	out := make([]triPt, len(r))
	for i, p := range r {
		out[i] = triPt{p[0]*s + dx, p[1]*s + dy}
	}
	return out
}

// triArea2 is twice the signed area of a ring, exact for the integer coordinates of these tests.
func triArea2(r []triPt) int64 {
	var a int64
	for i := range r {
		p, q := r[i], r[(i+1)%len(r)]
		a += int64(p[0])*int64(q[1]) - int64(q[0])*int64(p[1])
	}
	return a
}

type triExpect struct {
	ok, dropped, either bool
}

// triCheck runs the ears and checks the result flags, exact cover, strip orientation and that no triangle is flat;
// either accepts a refusal too, for degenerate input outside the guarantees.
func triCheck(t *testing.T, tr *Triangulator[uint32], name string, rings [][]triPt, want triExpect) {
	t.Helper()
	s := tr.Run(rings)
	ok := s != Refused
	if (ok != want.ok && !want.either) || tr.HolesLeftOut() != want.dropped {
		t.Errorf("%s: status %v, holes left out %v", name, s, tr.HolesLeftOut())
		return
	}
	if !ok {
		return
	}
	var all []triPt
	var expected int64
	for r, ring := range rings {
		all = append(all, ring...)
		a := triArea2(ring)
		if a < 0 {
			a = -a
		}
		if r == 0 {
			expected += a
		} else {
			expected -= a
		}
	}
	idx, ends, cw := tr.Strips(), tr.StripEnds(), tr.StripClockwise()
	if len(ends) != len(cw) {
		t.Fatalf("%s: %d strip ends, %d clockwise flags", name, len(ends), len(cw))
	}
	var sum int64
	start := uint32(0)
	for s, end := range ends {
		if end < start+3 {
			t.Errorf("%s: strip shorter than a triangle", name)
		}
		sign := 0
		for k := start; k+2 < end; k++ {
			if int(idx[k]) >= len(all) || int(idx[k+1]) >= len(all) || int(idx[k+2]) >= len(all) {
				t.Errorf("%s: index out of range", name)
				continue
			}
			a, b, c := all[idx[k]], all[idx[k+1]], all[idx[k+2]]
			cr := (int64(b[0])-int64(a[0]))*(int64(c[1])-int64(a[1])) - (int64(b[1])-int64(a[1]))*(int64(c[0])-int64(a[0]))
			if (k-start)&1 != 0 {
				cr = -cr
			}
			sg := 0
			if cr > 0 {
				sg = 1
			} else if cr < 0 {
				sg = -1
			}
			if sg == 0 {
				t.Errorf("%s: flat triangle", name)
			}
			if sg != 0 && sign != 0 && sg != sign {
				t.Errorf("%s: strip orientation flips", name)
			}
			if k == start && sg != 0 && (sg < 0) != (cw[s] != 0) {
				t.Errorf("%s: strip %d clockwise flag %d, first triangle sign %d", name, s, cw[s], sg)
			}
			if sg != 0 {
				sign = sg
			}
			if cr < 0 {
				cr = -cr
			}
			sum += cr
		}
		start = end
	}
	// with a dropped hole the triangles cover the outer ring minus the other holes, which this check does not know
	if !want.dropped && sum != expected {
		t.Errorf("%s: area %d, expected %d", name, sum, expected)
	}
	// Triangles winds every triangle counter-clockwise
	tris := tr.Triangles(nil)
	for k := 0; k+2 < len(tris); k += 3 {
		a, b, c := all[tris[k]], all[tris[k+1]], all[tris[k+2]]
		if cross(a[0], a[1], b[0], b[1], c[0], c[1]) <= 0 {
			t.Errorf("%s: triangle %d not counter-clockwise", name, k/3)
			break
		}
	}
}

func newEarsOnly() *Triangulator[uint32] {
	var tr Triangulator[uint32]
	tr.SetForcedCuts(false)
	tr.SetConstrained(false)
	return &tr
}

func TestEarsCases(t *testing.T) {
	tr := newEarsOnly() // one instance for every case: buffers are reused
	ok := triExpect{ok: true}
	either := triExpect{ok: true, either: true}
	dropped := triExpect{ok: true, dropped: true}
	square := triRing(0, 0, 10, 0, 10, 10, 0, 10)
	cases := []struct {
		name  string
		rings [][]triPt
		want  triExpect
	}{
		{"triangle", [][]triPt{triRing(0, 0, 5, 0, 0, 7)}, ok},
		{"triangle cw", [][]triPt{triRing(0, 0, 0, 7, 5, 0)}, ok},
		{"convex quad", [][]triPt{square}, ok},
		{"convex quad cw closed", [][]triPt{triRing(0, 0, 0, 10, 10, 10, 10, 0, 0, 0)}, ok},
		{"concave quad", [][]triPt{triRing(0, 0, 10, 0, 3, 3, 0, 10)}, ok},
		{"concave quad cw", [][]triPt{triReversed(triRing(0, 0, 10, 0, 3, 3, 0, 10))}, ok},
		{"convex hexagon", [][]triPt{triRing(0, 0, 4, -2, 8, 0, 8, 4, 4, 6, 0, 4)}, ok},
		{"L shape", [][]triPt{triRing(0, 0, 10, 0, 10, 4, 4, 4, 4, 10, 0, 10)}, ok},
		{"square with hole", [][]triPt{square, triRing(3, 3, 7, 3, 7, 7, 3, 7)}, ok},
		{"square with cw hole", [][]triPt{square, triReversed(triRing(3, 3, 7, 3, 7, 7, 3, 7))}, ok},
		{"two holes", [][]triPt{triScaled(square, 2, 0, 0), triRing(2, 2, 8, 2, 8, 8, 2, 8), triRing(12, 12, 18, 12, 15, 18)}, ok},
		{"two holes same rightmost x", [][]triPt{triScaled(square, 2, 0, 0), triRing(2, 2, 8, 2, 8, 8, 2, 8), triRing(4, 12, 8, 12, 8, 18)}, ok},
		{"hole touching outer vertex", [][]triPt{square, triRing(10, 10, 4, 7, 7, 4)}, ok},
		{"hole touching outer, not rightmost", [][]triPt{triRing(0, 0, 10, 0, 10, 10, 0, 10, 0, 5), triRing(0, 5, 5, 3, 5, 7)}, ok},
		{"hole vertex on vertical outer edge", [][]triPt{square, triRing(10, 5, 4, 8, 4, 2)}, ok},
		{"hole vertex on horizontal outer edge", [][]triPt{square, triRing(6, 10, 3, 8, 5, 6)}, ok},
		{"hole touching other hole", [][]triPt{triScaled(square, 2, 0, 0), triRing(2, 2, 8, 2, 8, 8, 2, 8), triRing(8, 8, 14, 9, 9, 14)}, ok},
		{"holes touching outer and each other", [][]triPt{triRing(0, 0, 10, 0, 10, 10, 0, 10, 0, 5), triRing(0, 5, 5, 3, 5, 7), triRing(5, 7, 8, 9, 9, 4)}, ok},
		{"duplicates and collinear", [][]triPt{triRing(0, 0, 0, 0, 5, 0, 10, 0, 10, 0, 10, 5, 10, 10, 5, 10, 5, 10, 0, 10, 0, 5, 0, 0)}, ok},
		{"collinear concave", [][]triPt{triRing(0, 0, 4, 0, 8, 0, 8, 4, 6, 4, 4, 4, 4, 8, 2, 8, 0, 8, 0, 4)}, ok},
		{"zero width spike", [][]triPt{triRing(0, 0, 10, 0, 10, 10, 5, 10, 5, 15, 5, 10, 0, 10)}, either},
		{"zero width notch", [][]triPt{triRing(0, 0, 10, 0, 10, 10, 5, 10, 5, 5, 5, 10, 0, 10)}, either},
		{"all collinear", [][]triPt{triRing(0, 0, 5, 0, 10, 0)}, ok},
		{"two distinct points", [][]triPt{triRing(0, 0, 5, 0, 0, 0, 5, 0)}, ok},
		{"empty", [][]triPt{{}}, ok},
		{"no rings", nil, ok},
		{"bow tie", [][]triPt{triRing(0, 0, 10, 10, 10, 0, 0, 10)}, triExpect{}},
		{"bow tie asymmetric", [][]triPt{triRing(0, 0, 20, 20, 20, 0, 0, 10)}, triExpect{}},
		{"hole outside", [][]triPt{square, triRing(20, 20, 30, 20, 30, 30)}, dropped},
		{"hole left of outer", [][]triPt{square, triRing(-30, 2, -20, 2, -25, 8)}, dropped},
		{"hole outside, other hole kept", [][]triPt{triScaled(square, 2, 0, 0), triRing(2, 2, 8, 2, 5, 8), triRing(30, 30, 40, 30, 35, 40)}, dropped},
		{"single-point holes", [][]triPt{square, triRing(3, 3), triRing(6, 6), triRing(5, 2)}, either},
		{"ring touching itself at a vertex", [][]triPt{triRing(0, 0, 5, 0, 5, 5, 10, 5, 10, 10, 5, 10, 5, 5, 0, 5)}, either},
		{"hole equal to the outer ring", [][]triPt{square, square}, either},
		{"hole along an outer edge", [][]triPt{square, triRing(0, 2, 5, 2, 5, 8, 0, 8)}, either},
		{"touching holes closing a loop", [][]triPt{triScaled(square, 3, 0, 0),
			triRing(6, 3, 12, 6, 6, 9, 3, 6), triRing(12, 6, 18, 3, 24, 6, 18, 9),
			triRing(12, 6, 18, 9, 12, 12, 9, 9), triRing(12, 6, 9, 9, 6, 9, 9, 3)}, either},
		{"collinear tips", [][]triPt{triRing(0, 0, 10, 0, 10, 4, 8, 2, 6, 4, 4, 2, 2, 4, 0, 2)}, ok},
		{"collinear valleys", [][]triPt{triRing(0, 0, 12, 0, 12, 6, 10, 2, 8, 6, 6, 2, 4, 6, 2, 2, 0, 6)}, ok},
		{"holes on one row", [][]triPt{triScaled(square, 4, 0, 0), triRing(4, 10, 8, 10, 6, 14), triRing(14, 10, 18, 10, 16, 14), triRing(24, 10, 28, 10, 26, 14)}, ok},
		{"hole right of a reflex vertex", [][]triPt{triRing(0, 0, 20, 0, 20, 20, 0, 20, 0, 12, 10, 10, 0, 8), triRing(12, 9, 16, 9, 14, 11)}, ok},
	}
	const big = 600000000
	cases = append(cases,
		struct {
			name  string
			rings [][]triPt
			want  triExpect
		}{"world-scale square with hole", [][]triPt{triScaled(square, big/10, -big/2, -big/2), triScaled(triRing(3, 3, 7, 3, 7, 7, 3, 7), big/10, -big/2, -big/2)}, ok},
		struct {
			name  string
			rings [][]triPt
			want  triExpect
		}{"world-scale L", [][]triPt{triScaled(triRing(0, 0, 10, 0, 10, 4, 4, 4, 4, 10, 0, 10), big/10, 0, 0)}, ok},
	)
	for _, c := range cases {
		triCheck(t, tr, c.name, c.rings, c.want)
	}
}

var triNearSpike = triRing(68, 1684, 129, 1657, 176, 1626, 196, 1608, 212, 1585, 223, 1559, 228, 1531, 227, 1248, 227, 1249,
	230, 526, 0, 527, 0, 1698)

// Rings whose corners all turn one way but that wind around more than once are not convex: no fast path for them.
// Refusing them is allowed; an accepted one must still be covered exactly by triangles that all turn one way.
func TestEarsWindingRings(t *testing.T) {
	tr := newEarsOnly()
	either := triExpect{ok: true, either: true}
	triCheck(t, tr, "near spike crossing itself (gom4)", [][]triPt{triNearSpike}, either)
	for sym := 0; sym < 8; sym++ {
		for start := range triNearSpike {
			for _, scale := range []float64{1, 100000} {
				// rotations by quarter turns, mirrors, every start point, tile and world scale
				var ring []triPt
				for i := range triNearSpike {
					p := triNearSpike[(start+i)%len(triNearSpike)]
					if sym&1 != 0 {
						p = triPt{-p[0], p[1]}
					}
					for r := 0; r < sym/2; r++ {
						p = triPt{-p[1], p[0]}
					}
					ring = append(ring, triPt{p[0] * scale, p[1] * scale})
				}
				triCheck(t, tr, "near spike variant", [][]triPt{ring}, either)
			}
		}
	}
	gcd := func(a, b int) int {
		for b != 0 {
			a, b = b, a%b
		}
		return a
	}
	for points := 5; points <= 13; points++ {
		for step := 2; 2*step < points; step++ {
			if gcd(points, step) != 1 {
				continue
			}
			// star polygon {points/step}: every corner turns the same way, the ring winds step times
			var star []triPt
			for i := 0; i < points; i++ {
				angle := 2 * math.Pi * float64(step*i) / float64(points)
				star = append(star, triPt{math.Round(1000 * math.Cos(angle)), math.Round(1000 * math.Sin(angle))})
			}
			triCheck(t, tr, "star", [][]triPt{star}, either)
			triCheck(t, tr, "star reversed", [][]triPt{triReversed(star)}, either)
		}
	}
	triCheck(t, tr, "square twice", [][]triPt{triRing(0, 0, 10, 0, 10, 10, 0, 10, 0, 0, 10, 0, 10, 10, 0, 10)}, either)
}

// Combs: hundreds of reflex corners go through the bucket index.
func TestEarsCombs(t *testing.T) {
	tr := newEarsOnly()
	ok := triExpect{ok: true}
	const teeth = 120
	var spikes []triPt
	for i := 0; i < teeth; i++ {
		spikes = append(spikes, triRing(i*10, 0, i*10+4, 0, i*10+5, 60+(i*37)%30, i*10+6, 0)...)
	}
	spikes = append(spikes, triRing(teeth*10, 0, teeth*10, 100, 0, 100)...)
	triCheck(t, tr, "comb, 483 points, notches", [][]triPt{spikes}, ok)
	var slots []triPt
	for i := 0; i < teeth; i++ {
		slots = append(slots, triRing(i*10, 0, i*10+5, 0, i*10+5, 90, i*10+8, 90, i*10+8, 0)...)
	}
	slots = append(slots, triRing(teeth*10, 0, teeth*10, 100, 0, 100)...)
	triCheck(t, tr, "comb, 603 points, slots", [][]triPt{slots}, ok)
	holes := [][]triPt{triScaled(triRing(0, 0, 10, 0, 10, 10, 0, 10), 100, 0, 0)}
	for i := 0; i < 9; i++ {
		for j := 0; j < 9; j++ {
			holes = append(holes, triRing(i*100+20, j*100+20, i*100+80, j*100+30, i*100+70, j*100+80, i*100+50, j*100+50))
		}
	}
	triCheck(t, tr, "81 holes", holes, ok)
}

// triRandomPolygon is a random star-shaped outer ring with small holes inside its kernel, with repeated and
// collinear points sprinkled in and random ring orientations (after test.cpp's randomPolygon).
func triRandomPolygon(rng *rand.Rand, scale float64) [][]triPt {
	const radius = 10000.0
	points := 24 + rng.IntN(300)
	rings := make([][]triPt, 1)
	for i := 0; i < points; i++ {
		angle := 2 * math.Pi * (float64(i) + 0.4*rng.Float64()) / float64(points)
		r := radius * (0.4 + 0.6*rng.Float64())
		rings[0] = append(rings[0], triPt{math.Round(r * math.Cos(angle)), math.Round(r * math.Sin(angle))})
	}
	// holes on a grid of cells inside the disk of radius 0.35 * radius, which the star always contains
	cells := rng.IntN(7)
	cell := 0.7 * radius / float64(max(cells, 1)) / math.Sqrt2
	grid := func(k int) float64 { return math.Floor(-0.35*radius/math.Sqrt2 + float64(k)*cell) }
	for cx := 0; cx < cells; cx++ {
		for cy := 0; cy < cells; cy++ {
			if rng.Float64() < 0.3 {
				continue
			}
			x0, y0 := grid(cx), grid(cy)
			var hole []triPt
			n := 3 + rng.IntN(6)
			for i := 0; i < n; i++ {
				angle := 2 * math.Pi * (float64(i) + 0.5*rng.Float64()) / float64(n)
				r := cell * (0.15 + 0.3*rng.Float64())
				hole = append(hole, triPt{math.Round(x0 + cell/2 + r*math.Cos(angle)), math.Round(y0 + cell/2 + r*math.Sin(angle))})
			}
			rings = append(rings, hole)
		}
	}
	for k, ring := range rings {
		// repeated points and collinear midpoints
		var out []triPt
		for i, p := range ring {
			out = append(out, p)
			q := ring[(i+1)%len(ring)]
			if rng.Float64() < 0.05 {
				out = append(out, p)
			}
			if rng.Float64() < 0.05 && math.Mod(q[0]-p[0], 2) == 0 && math.Mod(q[1]-p[1], 2) == 0 {
				out = append(out, triPt{(p[0] + q[0]) / 2, (p[1] + q[1]) / 2})
			}
		}
		if rng.Float64() < 0.5 {
			out = triReversed(out)
		}
		rings[k] = triScaled(out, scale, 0, 0)
	}
	return rings
}

func TestEarsRandomPolygons(t *testing.T) {
	tr := newEarsOnly()
	rng := rand.New(rand.NewPCG(12345, 1))
	for i := 0; i < 1000; i++ {
		scale := 1.0
		if i%3 == 2 {
			scale = 40000
		}
		triCheck(t, tr, "random", triRandomPolygon(rng, scale), triExpect{ok: true})
	}
}

// Forced cuts: invalid rings still give strips over their own vertices, never a refusal.
func TestForcedCuts(t *testing.T) {
	var tr Triangulator[uint32]
	tr.SetConstrained(false)
	invalid := [][]triPt{
		triRing(0, 0, 10, 10, 10, 0, 0, 10),
		triRing(0, 0, 20, 20, 20, 0, 0, 10),
		triRing(0, 0, 10, 0, 10, 10, 0, 10, 0, 0, 10, 0, 10, 10, 0, 10),
		triNearSpike,
	}
	for k, ring := range invalid {
		s := tr.Run([][]triPt{ring})
		if s != Approximate && s != Exact {
			t.Errorf("invalid ring %d: status %v", k, s)
		}
		triangles := 0
		begin := uint32(0)
		for _, end := range tr.StripEnds() {
			triangles += int(end - begin - 2)
			begin = end
		}
		for _, i := range tr.Strips() {
			if int(i) >= len(ring) {
				t.Errorf("invalid ring %d: index %d out of range", k, i)
			}
		}
		// a square walked twice is empty under the even-odd rule; the figure eights have lobes to fill
		if k < 2 && triangles == 0 {
			t.Errorf("invalid ring %d: no triangles", k)
		}
	}
}

// The flat interface gives the same strips as the nested one; uint16 indices.
func TestFlatAndUint16(t *testing.T) {
	tr := newEarsOnly()
	tr.RunFlat([]float64{0, 0, 10, 0, 10, 10, 0, 10, 3, 3, 7, 3, 7, 7, 3, 7}, 2, []int{4})
	fromFlat := slices.Clone(tr.Strips())
	rings := [][]triPt{triRing(0, 0, 10, 0, 10, 10, 0, 10), triRing(3, 3, 7, 3, 7, 7, 3, 7)}
	tr.Run(rings)
	if !slices.Equal(fromFlat, tr.Strips()) {
		t.Errorf("flat %v, nested %v", fromFlat, tr.Strips())
	}
	var t16 Triangulator[uint16]
	t16.SetConstrained(false)
	t16.Run(rings)
	if len(t16.Strips()) != len(fromFlat) {
		t.Fatalf("uint16 strips %v", t16.Strips())
	}
	for k, i := range t16.Strips() {
		if uint32(i) != fromFlat[k] {
			t.Errorf("uint16 strips %v, uint32 %v", t16.Strips(), fromFlat)
			break
		}
	}
}

// List mode writes exactly what Triangles makes of the strips.
func TestTriangleListMode(t *testing.T) {
	strips, list := newEarsOnly(), newEarsOnly()
	list.SetTriangleList(true)
	rng := rand.New(rand.NewPCG(99, 2))
	for i := 0; i < 200; i++ {
		rings := triRandomPolygon(rng, 1)
		strips.Run(rings)
		tris := strips.Triangles(nil)
		list.Run(rings)
		if !slices.Equal(tris, list.TriangleList()) || len(list.Strips()) != 0 {
			t.Fatalf("polygon %d: list mode differs", i)
		}
	}
}

// A reused Triangulator allocates nothing once its buffers have grown.
func TestReuseAllocatesNothing(t *testing.T) {
	tr := newEarsOnly()
	rng := rand.New(rand.NewPCG(7, 7))
	polys := make([]Rings, 20) // converted once: a slice in an interface is allocated
	for i := range polys {
		polys[i] = Polygon(triRandomPolygon(rng, 1))
	}
	for _, p := range polys {
		tr.RunRings(p)
	}
	allocs := testing.AllocsPerRun(5, func() {
		for _, p := range polys {
			tr.RunRings(p)
		}
	})
	if allocs != 0 {
		t.Errorf("%v allocations per run", allocs)
	}
}

// cxxSort orders ties as libc++'s std::sort; the expected orders were printed by std::sort of Apple clang 21's
// libc++ for indices 0..n-1 under key[i] = 7i mod 5, decreasing.
func TestCxxSortTies(t *testing.T) {
	want := map[int][]uint32{
		30: {7, 22, 2, 17, 12, 27, 24, 9, 4, 14, 29, 19, 6, 11, 16, 26, 21, 1, 8, 28, 13, 3, 18, 23, 15, 25, 20, 10, 5, 0},
		150: {62, 57, 2, 67, 72, 142, 52, 7, 137, 147, 77, 132, 12, 47, 82, 127, 42, 17, 122, 87, 92, 117, 22, 37, 97, 112,
			32, 27, 107, 102, 104, 59, 109, 99, 34, 24, 94, 114, 119, 39, 89, 19, 124, 84, 44, 14, 79, 129, 134, 49, 149, 9,
			139, 4, 54, 69, 144, 74, 64, 29, 1, 136, 21, 116, 16, 76, 66, 56, 111, 146, 126, 71, 26, 106, 51, 61, 141, 31,
			101, 46, 6, 81, 131, 96, 11, 36, 86, 41, 91, 121, 133, 18, 38, 93, 8, 43, 83, 138, 98, 33, 13, 78, 48, 103, 128,
			143, 73, 28, 108, 53, 3, 68, 58, 113, 23, 123, 63, 148, 118, 88, 105, 75, 20, 115, 25, 125, 110, 15, 30, 100,
			130, 35, 95, 90, 10, 135, 40, 85, 45, 80, 140, 0, 5, 50, 70, 145, 55, 65, 60, 120},
	}
	for n, w := range want {
		key := make([]uint32, n)
		order := make([]uint32, n)
		for i := range order {
			key[i] = uint32(i*7) % 5
			order[i] = uint32(i)
		}
		cxxSort(order, func(a, b uint32) bool { return key[a] > key[b] })
		if !slices.Equal(order, w) {
			t.Errorf("n = %d: %v, want %v", n, order, w)
		}
	}
}
