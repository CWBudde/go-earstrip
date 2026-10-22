package earstrip

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/big"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ---------- int128 ----------

func int128Big(x int128) *big.Int {
	r := new(big.Int).SetInt64(x.hi)
	r.Lsh(r, 64)
	return r.Add(r, new(big.Int).SetUint64(x.lo))
}

var two128 = new(big.Int).Lsh(big.NewInt(1), 128)

// wrap128 reduces v to a signed 128-bit value, as the wrapping arithmetic does.
func wrap128(v *big.Int) *big.Int {
	r := new(big.Int).Mod(v, two128)
	if r.Cmp(new(big.Int).Rsh(two128, 1)) >= 0 {
		r.Sub(r, two128)
	}
	return r
}

func randInt64(rng *rand.Rand) int64 {
	switch rng.IntN(6) {
	case 0:
		return []int64{0, 1, -1, math.MaxInt64, math.MinInt64, math.MinInt64 + 1, 1 << 32, -(1 << 32)}[rng.IntN(8)]
	case 1:
		return rng.Int64N(1<<20) - 1<<19
	case 2:
		return rng.Int64N(1<<40) - 1<<39
	default:
		return int64(rng.Uint64())
	}
}

func randInt128(rng *rand.Rand) int128 {
	switch rng.IntN(4) {
	case 0:
		return mul128(randInt64(rng), randInt64(rng))
	case 1:
		return int128{lo: rng.Uint64(), hi: int64(rng.Uint64())}
	default:
		return mul128(randInt64(rng), 1)
	}
}

func TestInt128VsBig(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 200000; i++ {
		a, b := randInt64(rng), randInt64(rng)
		if got, want := int128Big(mul128(a, b)), new(big.Int).Mul(big.NewInt(a), big.NewInt(b)); got.Cmp(want) != 0 {
			t.Fatalf("mul128(%d, %d) = %v, want %v", a, b, got, want)
		}
		x, y := randInt128(rng), randInt128(rng)
		bx, by := int128Big(x), int128Big(y)
		if got, want := int128Big(x.add(y)), wrap128(new(big.Int).Add(bx, by)); got.Cmp(want) != 0 {
			t.Fatalf("%v + %v = %v, want %v", bx, by, got, want)
		}
		if got, want := int128Big(x.sub(y)), wrap128(new(big.Int).Sub(bx, by)); got.Cmp(want) != 0 {
			t.Fatalf("%v - %v = %v, want %v", bx, by, got, want)
		}
		if got, want := int128Big(x.neg()), wrap128(new(big.Int).Neg(bx)); got.Cmp(want) != 0 {
			t.Fatalf("-%v = %v, want %v", bx, got, want)
		}
		if got, want := x.less(y), bx.Cmp(by) < 0; got != want {
			t.Fatalf("%v < %v = %v", bx, by, got)
		}
		if got, want := x.lessEq(y), bx.Cmp(by) <= 0; got != want {
			t.Fatalf("%v <= %v = %v", bx, by, got)
		}
		if got, want := x.sign(), bx.Sign(); got != want {
			t.Fatalf("sign %v = %d", bx, got)
		}
		if x.hi != math.MinInt64 || x.lo != 0 {
			if got, want := int128Big(x.abs()), new(big.Int).Abs(bx); got.Cmp(want) != 0 {
				t.Fatalf("abs %v = %v", bx, got)
			}
			f, _ := new(big.Float).SetInt(bx).Float64()
			if d := x.toDouble(); math.Abs(d-f) > math.Abs(f)*1e-15 {
				t.Fatalf("toDouble %v = %v, want %v", bx, d, f)
			}
		}
		// times: the product must fit 127 bits
		m := randInt64(rng)
		if p := new(big.Int).Mul(bx, big.NewInt(m)); p.BitLen() < 127 {
			if got := int128Big(x.times(m)); got.Cmp(p) != 0 {
				t.Fatalf("%v * %d = %v, want %v", bx, m, got, p)
			}
		}
		// floorDiv: n = d * q + r with 0 <= r < d and q of 62 bits
		d := mul128(rng.Int64N(math.MaxInt64)+1, rng.Int64N(1<<40)+1)
		if rng.IntN(3) == 0 {
			d = mul128(rng.Int64N(1<<30)+1, 1)
		}
		q := rng.Int64N(1<<62) - 1<<61
		bd := int128Big(d)
		r := new(big.Int).Lsh(new(big.Int).SetUint64(rng.Uint64()), 64)
		r.Or(r, new(big.Int).SetUint64(rng.Uint64()))
		r.Mod(r, bd)
		n := new(big.Int).Add(new(big.Int).Mul(bd, big.NewInt(q)), r)
		if n.BitLen() >= 127 {
			continue
		}
		nn := int128{lo: new(big.Int).And(n, new(big.Int).SetUint64(math.MaxUint64)).Uint64(), hi: new(big.Int).Rsh(n, 64).Int64()}
		gq, gr := floorDiv(nn, d)
		if gq != q || int128Big(gr).Cmp(r) != 0 {
			t.Fatalf("floorDiv(%v, %v) = %d, %v; want %d, %v", n, bd, gq, int128Big(gr), q, r)
		}
	}
}

// ---------- predicates ----------

func bigOrient(a, b, c cdtVertex) int {
	abx, aby := big.NewInt(b.x-a.x), big.NewInt(b.y-a.y)
	acx, acy := big.NewInt(c.x-a.x), big.NewInt(c.y-a.y)
	return new(big.Int).Sub(new(big.Int).Mul(abx, acy), new(big.Int).Mul(aby, acx)).Sign()
}

func bigInCircle(a, b, c, d cdtVertex) int {
	sub := func(p, q int64) *big.Int { return new(big.Int).Sub(big.NewInt(p), big.NewInt(q)) }
	mul := func(x, y *big.Int) *big.Int { return new(big.Int).Mul(x, y) }
	add := func(x, y *big.Int) *big.Int { return new(big.Int).Add(x, y) }
	adx, ady, bdx, bdy, cdx, cdy := sub(a.x, d.x), sub(a.y, d.y), sub(b.x, d.x), sub(b.y, d.y), sub(c.x, d.x), sub(c.y, d.y)
	al, bl, cl := add(mul(adx, adx), mul(ady, ady)), add(mul(bdx, bdx), mul(bdy, bdy)), add(mul(cdx, cdx), mul(cdy, cdy))
	det := add(add(mul(al, new(big.Int).Sub(mul(bdx, cdy), mul(bdy, cdx))), mul(bl, new(big.Int).Sub(mul(cdx, ady), mul(cdy, adx)))),
		mul(cl, new(big.Int).Sub(mul(adx, bdy), mul(ady, bdx))))
	return det.Sign()
}

func TestPredicatesVsBig(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	coord := func(span int64) int64 { return rng.Int64N(span+1) - span/2 }
	for _, span := range []int64{4, 64, smallSpan, 1 << 20, 1<<23 - 1, math.MaxInt32, 1 << 32} {
		c := &constrained[uint32]{wide: span > math.MaxInt32, exactCircle: span <= smallSpan}
		c.vertices = make([]cdtVertex, 4)
		for i := 0; i < 50000; i++ {
			for k := range c.vertices {
				c.vertices[k] = cdtVertex{coord(span), coord(span)}
			}
			if rng.IntN(4) == 0 { // collinear
				c.vertices[2] = cdtVertex{2*c.vertices[1].x - c.vertices[0].x, 2*c.vertices[1].y - c.vertices[0].y}
			}
			if math.Abs(float64(c.vertices[2].x)) > float64(span) || math.Abs(float64(c.vertices[2].y)) > float64(span) {
				c.vertices[2] = c.vertices[1]
			}
			a, b, cc, d := c.vertices[0], c.vertices[1], c.vertices[2], c.vertices[3]
			if got, want := c.orient(0, 1, 2), bigOrient(a, b, cc); got != want {
				t.Fatalf("span %d: orient(%v, %v, %v) = %d, want %d", span, a, b, cc, got, want)
			}
			if span > math.MaxInt32 {
				continue
			}
			o := bigOrient(a, b, cc)
			if o == 0 {
				continue
			}
			if o < 0 {
				b, cc = cc, b
				c.vertices[1], c.vertices[2] = b, cc
			}
			got, want := c.inCircle(0, 1, 2, 3), bigInCircle(a, b, cc, d)
			if c.exactCircle && got != (want > 0) {
				t.Fatalf("span %d: inCircle(%v, %v, %v, %v) = %v, want %d", span, a, b, cc, d, got, want)
			}
			if !c.exactCircle && got && want <= 0 {
				t.Fatalf("span %d: inexact inCircle(%v, %v, %v, %v) = true, want %d", span, a, b, cc, d, want)
			}
		}
	}
}

// ---------- simple triangulations ----------

func square(x0, y0, x1, y1 float64) [][2]float64 {
	return [][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
}

// runCDT triangulates the polygon and checks the indices and the orientation of the triangles.
func runCDT[I Index](t *testing.T, c *constrained[I], poly Polygon) {
	t.Helper()
	if !c.load(poly) {
		t.Fatalf("load failed")
	}
	if !c.build() {
		t.Fatalf("build failed")
	}
	limit := uint64(c.pointCount) + uint64(c.extraCount())
	if len(c.stripEnds) != len(c.stripCW) {
		t.Fatalf("%d strip ends, %d orientations", len(c.stripEnds), len(c.stripCW))
	}
	begin := uint32(0)
	for s, end := range c.stripEnds {
		if end < begin+3 {
			t.Fatalf("strip %d has %d vertices", s, end-begin)
		}
		for k := begin; k < end; k++ {
			if uint64(c.strips[k]) >= limit {
				t.Fatalf("index %d out of range %d", c.strips[k], limit)
			}
		}
		for k := begin; k+2 < end; k++ {
			flip := ((k - begin) & 1) ^ uint32(c.stripCW[s])
			p := c.point(uint32(c.strips[k+flip]))
			q := c.point(uint32(c.strips[k+1-flip]))
			r := c.point(uint32(c.strips[k+2]))
			if crossPoints(p, q, r) <= 0 {
				t.Fatalf("strip %d triangle %d not counterclockwise", s, k-begin)
			}
		}
		begin = end
	}
}

func TestConstrainedSimple(t *testing.T) {
	grid := Polygon{{{0, 0}, {410, 0}, {410, 210}, {0, 210}}}
	gridArea := 2.0 * 410 * 210
	for i := 0; i < 20; i++ {
		for j := 0; j < 10; j++ {
			x, y := 10+20*float64(i), 10+20*float64(j)
			grid = append(grid, [][2]float64{{x, y}, {x, y + 10}, {x + 10, y + 10}, {x + 10, y}})
			gridArea -= 200
		}
	}
	var pentagram Polygon = [][][2]float64{{}}
	for k := 0; k < 5; k++ {
		a := math.Pi/2 + 4*math.Pi*float64(k)/5
		pentagram[0] = append(pentagram[0], [2]float64{math.Round(1000 * math.Cos(a)), math.Round(1000 * math.Sin(a))})
	}
	cases := []struct {
		name  string
		poly  Polygon
		area2 float64 // doubled area
		extra int
		tol   float64
	}{
		{"square", Polygon{square(0, 0, 10, 10)}, 200, 0, 0},
		{"square clockwise", Polygon{{{0, 0}, {0, 10}, {10, 10}, {10, 0}}}, 200, 0, 0},
		{"square with hole", Polygon{square(0, 0, 10, 10), square(2, 2, 4, 4)}, 192, 0, 0},
		{"bow tie", Polygon{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}}, 100, 1, 0},
		{"square walked twice", Polygon{{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}, {10, 0}, {10, 10}, {0, 10}}}, 200, 0, 0},
		{"hole outside", Polygon{square(0, 0, 10, 10), square(20, 0, 24, 4)}, 200, 0, 0},
		{"self-touching", Polygon{{{0, 0}, {5, 5}, {10, 0}, {10, 10}, {5, 5}, {0, 10}}}, 100, 0, 0},
		{"touching rings", Polygon{square(0, 0, 10, 10), {{0, 0}, {5, 2}, {5, 5}}}, 200 - 15, 0, 0},
		{"hole crossing the outer ring", Polygon{square(0, 0, 10, 10), square(5, 5, 15, 15)}, 150, 2, 0},
		{"closing point repeated", Polygon{{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}}}, 200, 0, 0},
		{"200 holes", grid, gridArea, 0, 0},
		{"lon/lat bow tie", Polygon{{{21.0, 52.2}, {21.01, 52.21}, {21.01, 52.2}, {21.0, 52.21}}}, 1e-4, 1, 1e-12},
		{"float square with hole", Polygon{square(0.1, 0.2, 0.7, 0.9), square(0.3, 0.3, 0.4, 0.5)}, 2 * (0.6*0.7 - 0.1*0.2), 0, 1e-12},
	}
	var c constrained[uint32]
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runCDT(t, &c, tc.poly)
			if got := c.covered(c.strips, c.stripEnds); math.Abs(got-tc.area2) > tc.tol {
				t.Errorf("covered %v, want %v", got, tc.area2)
			}
			if got := int(c.extraCount()); got != tc.extra {
				t.Errorf("%d extra points, want %d", got, tc.extra)
			}
		})
	}
	t.Run("bow tie crossing", func(t *testing.T) {
		runCDT(t, &c, Polygon{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}})
		if got := c.extraPoints(nil); len(got) != 2 || got[0] != 5 || got[1] != 5 {
			t.Errorf("extra points %v, want [5 5]", got)
		}
	})
	t.Run("lon/lat crossing", func(t *testing.T) {
		runCDT(t, &c, Polygon{{{21.0, 52.2}, {21.01, 52.21}, {21.01, 52.2}, {21.0, 52.21}}})
		if got := c.extraPoints(nil); len(got) != 2 || math.Abs(got[0]-21.005) > 1e-9 || math.Abs(got[1]-52.205) > 1e-9 {
			t.Errorf("extra points %v, want [21.005 52.205]", got)
		}
	})
	t.Run("pentagram", func(t *testing.T) {
		runCDT(t, &c, pentagram)
		if c.extraCount() != 5 {
			t.Fatalf("%d extra points, want 5", c.extraCount())
		}
		// the centre pentagon has winding 2 and stays filled: the cover is the outline through the crossings
		ring := append([][2]float64{}, pentagram[0]...)
		extra := c.extraPoints(nil)
		for k := 0; k < 10; k += 2 {
			ring = append(ring, [2]float64{extra[k], extra[k+1]})
		}
		slicesSortByAngle(ring)
		outline := 0.0
		for i := range ring {
			p, q := ring[i], ring[(i+1)%len(ring)]
			outline += p[0]*q[1] - q[0]*p[1]
		}
		if got := c.covered(c.strips, c.stripEnds); math.Abs(got-outline) > 1e-6 {
			t.Errorf("covered %v, outline %v", got, outline)
		}
	})
	t.Run("ring area", func(t *testing.T) {
		if !c.load(Polygon{square(0, 0, 10, 10), square(2, 2, 4, 4)}) {
			t.Fatal("load failed")
		}
		if got := c.inputRingsArea(); got != 192 {
			t.Errorf("inputRingsArea %v, want 192", got)
		}
	})
	t.Run("degenerate", func(t *testing.T) {
		for _, poly := range []Polygon{{}, {{}}, {{{0, 0}, {1, 1}}}, {{{0, 0}, {1, 1}, {2, 2}, {3, 3}}}, {{{1, 1}, {1, 1}, {1, 1}}}} {
			runCDT(t, &c, poly)
			if len(c.strips) != 0 || c.extraCount() != 0 {
				t.Errorf("%v: %d indices, %d extra points", poly, len(c.strips), c.extraCount())
			}
		}
	})
	t.Run("past the index range", func(t *testing.T) {
		big := Polygon{{}}
		for k := 0; k < 70000; k++ {
			a := 2 * math.Pi * float64(k) / 70000
			big[0] = append(big[0], [2]float64{math.Round(1e6 * math.Cos(a)), math.Round(1e6 * math.Sin(a))})
		}
		var c16 constrained[uint16]
		if c16.load(big) {
			t.Fatal("70000 points loaded for 16-bit indices")
		}
		// 65530 points and crossings: the added points run out of indices
		big[0] = big[0][:65530]
		big = append(big, [][2]float64{{-2e6, -10}, {2e6, -10}, {2e6, 10}, {-2e6, 10}})
		if !c16.load(big) {
			t.Fatal("load failed")
		}
		if c16.build() {
			t.Fatal("build succeeded past the index range")
		}
		if c16.extraCount() != 0 || len(c16.strips) != 0 {
			t.Fatal("failed build left a result")
		}
	})
}

func slicesSortByAngle(ring [][2]float64) {
	for i := 1; i < len(ring); i++ {
		for j := i; j > 0 && math.Atan2(ring[j][1], ring[j][0]) < math.Atan2(ring[j-1][1], ring[j-1][0]); j-- {
			ring[j], ring[j-1] = ring[j-1], ring[j]
		}
	}
}

// TestConstrainedNoAlloc checks that a reused instance does not allocate in steady state.
func TestConstrainedNoAlloc(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	poly := Polygon{randomRing(rng, 60, 1000, true), starRing(rng, 20, 300, 300, 50, 200, true)}
	var c constrained[uint32]
	var extra []float64
	var rings Rings = poly // the conversion to the interface allocates
	run := func() {
		c.load(rings)
		c.build()
		extra = c.extraPoints(extra)
	}
	run()
	if n := testing.AllocsPerRun(20, run); n != 0 {
		t.Errorf("%v allocations per run", n)
	}
}

// ---------- random polygons ----------

func roundIf(v float64, integral bool) float64 {
	if integral {
		return math.Round(v)
	}
	return v
}

// starRing returns a ring around (cx, cy) with radii in [r0, r1]: simple when the angles differ.
func starRing(rng *rand.Rand, n int, cx, cy, r0, r1 float64, integral bool) [][2]float64 {
	angles := make([]float64, n)
	for i := range angles {
		angles[i] = rng.Float64() * 2 * math.Pi
	}
	slicesSortFloat(angles)
	ring := make([][2]float64, n)
	for i, a := range angles {
		r := r0 + (r1-r0)*rng.Float64()
		ring[i] = [2]float64{roundIf(cx+r*math.Cos(a), integral), roundIf(cy+r*math.Sin(a), integral)}
	}
	if rng.IntN(2) == 0 { // clockwise
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			ring[i], ring[j] = ring[j], ring[i]
		}
	}
	return ring
}

func slicesSortFloat(v []float64) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

// randomRing returns n random points in [0, size)^2: a ring crossing itself.
func randomRing(rng *rand.Rand, n int, size float64, integral bool) [][2]float64 {
	ring := make([][2]float64, n)
	for i := range ring {
		ring[i] = [2]float64{roundIf(rng.Float64()*size, integral), roundIf(rng.Float64()*size, integral)}
	}
	return ring
}

func transform(p Polygon, scale, ox, oy float64, integral bool) Polygon {
	out := make(Polygon, len(p))
	for r, ring := range p {
		out[r] = make([][2]float64, len(ring))
		for i, q := range ring {
			out[r][i] = [2]float64{roundIf(q[0]*scale+ox, integral), roundIf(q[1]*scale+oy, integral)}
		}
	}
	return out
}

// randomPolygon returns polygon number k of the random set.
func randomPolygon(rng *rand.Rand) Polygon {
	integral := rng.IntN(3) != 0
	var p Polygon
	switch rng.IntN(7) {
	case 0: // star
		p = Polygon{starRing(rng, 3+rng.IntN(60), 0, 0, 10+rng.Float64()*100, 200, integral)}
	case 1: // self-crossing ring
		p = Polygon{randomRing(rng, 3+rng.IntN(40), 100, integral)}
	case 2: // star with holes, some crossing it or each other
		p = Polygon{starRing(rng, 3+rng.IntN(80), 0, 0, 100, 400, integral)}
		for h := rng.IntN(12); h > 0; h-- {
			cx, cy := rng.Float64()*600-300, rng.Float64()*600-300
			p = append(p, starRing(rng, 3+rng.IntN(10), cx, cy, 5, 10+rng.Float64()*80, integral))
		}
	case 3: // tiny grid: duplicates, collinear points, touching rings
		p = Polygon{randomRing(rng, 3+rng.IntN(12), 8, true)}
		for h := rng.IntN(4); h > 0; h-- {
			p = append(p, randomRing(rng, 3+rng.IntN(6), 8, true))
		}
	case 4: // many holes
		p = Polygon{starRing(rng, 3+rng.IntN(30), 0, 0, 900, 1000, integral)}
		for h := 100 + rng.IntN(100); h > 0; h-- {
			cx, cy := rng.Float64()*1200-600, rng.Float64()*1200-600
			p = append(p, starRing(rng, 3+rng.IntN(5), cx, cy, 2, 10+rng.Float64()*20, integral))
		}
	case 5: // rings sharing points
		base := starRing(rng, 4+rng.IntN(30), 0, 0, 50, 100, true)
		p = Polygon{base}
		for h := 1 + rng.IntN(3); h > 0; h-- {
			hole := starRing(rng, 3+rng.IntN(6), 0, 0, 10, 120, true)
			hole[0] = base[rng.IntN(len(base))]
			p = append(p, hole)
		}
	default: // self-crossing ring with holes
		p = Polygon{randomRing(rng, 3+rng.IntN(30), 1000, integral)}
		for h := 1 + rng.IntN(5); h > 0; h-- {
			p = append(p, randomRing(rng, 3+rng.IntN(8), 1000, integral))
		}
	}
	// scales: exact in-circle, 64-bit areas, 128-bit areas, wide, floats of any size
	switch rng.IntN(8) {
	case 0:
		return p
	case 1:
		return transform(p, 1<<14/1000.0, 0, 0, integral)
	case 2:
		return transform(p, 1<<22/1000.0, -1e5, 3e5, integral)
	case 3:
		return transform(p, 1<<30/1000.0, 0, 0, integral)
	case 4: // span > 2^31: the wide predicates
		return transform(p, 2.1e9/1000.0, 0, 0, true)
	case 5:
		return transform(p, 1e-6, 13.4, 52.5, false)
	case 6:
		return transform(p, 1e12, -3e14, 0, rng.IntN(2) == 0)
	default:
		return transform(p, 0.37, 0.5, -0.25, false)
	}
}

// ---------- differential test against the C++ ----------

const cdtOracleSource = `
#include <algorithm>
#include <array>
#include <cassert>
#include <cmath>
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <limits>
#include <tuple>
#include <type_traits>
#include <utility>
#include <vector>
#define private public
#include "earstrip.hpp"
#undef private

using Poly = std::vector<std::vector<std::array<double, 2>>>;

static bool readAll(void *p, size_t n) { return fread(p, 1, n, stdin) == n; }
template <class T> static void put(T v) { fwrite(&v, sizeof v, 1, stdout); }

template <class Index> static int run()
{
    uint32_t count;
    if (!readAll(&count, 4))
        return 1;
    earstrip::detail::Constrained<Index> c;
    Poly poly;
    std::vector<double> extra;
    for (uint32_t k = 0; k < count; ++k) {
        uint32_t rings;
        if (!readAll(&rings, 4))
            return 1;
        poly.resize(rings);
        for (auto &ring : poly) {
            uint32_t n;
            if (!readAll(&n, 4))
                return 1;
            ring.resize(n);
            if (n && !readAll(ring.data(), 16 * (size_t)n))
                return 1;
        }
        const bool loaded = c.load(earstrip::detail::NestedRings<Poly>{poly});
        put<uint8_t>(loaded);
        if (!loaded)
            continue;
        put<uint8_t>(c.build());
        put<uint32_t>((uint32_t)c.strips().size());
        for (Index i : c.strips())
            put<uint32_t>((uint32_t)i);
        put<uint32_t>((uint32_t)c.stripEnds().size());
        for (uint32_t e : c.stripEnds())
            put<uint32_t>(e);
        for (uint8_t cw : c.stripClockwise())
            put<uint8_t>(cw);
        put<uint32_t>(c.extraCount());
        c.extraPoints(extra);
        put<uint32_t>((uint32_t)extra.size());
        for (double v : extra)
            put<double>(v);
        put<double>(c.inputRingsArea());
        put<double>(c.covered(c.strips(), c.stripEnds()));
    }
    fflush(stdout);
    return 0;
}

int main(int argc, char **argv)
{
    if (argc > 1 && strcmp(argv[1], "16") == 0)
        return run<uint16_t>();
    return run<uint32_t>();
}
`

type cdtResult struct {
	loaded, built bool
	strips        []uint32
	ends          []uint32
	cw            []uint8
	extraCount    uint32
	extra         []uint64 // bits
	area, covered uint64   // bits
}

func (r *cdtResult) String() string {
	return fmt.Sprintf("loaded %v built %v strips %v ends %v cw %v extra %d %v area %v covered %v", r.loaded, r.built,
		r.strips, r.ends, r.cw, r.extraCount, r.extra, math.Float64frombits(r.area), math.Float64frombits(r.covered))
}

func (r *cdtResult) equal(o *cdtResult) bool {
	eq32 := func(a, b []uint32) bool { return bytes.Equal(u32bytes(a), u32bytes(b)) }
	if r.loaded != o.loaded || r.built != o.built || !eq32(r.strips, o.strips) || !eq32(r.ends, o.ends) ||
		!bytes.Equal(r.cw, o.cw) || r.extraCount != o.extraCount || r.area != o.area || r.covered != o.covered ||
		len(r.extra) != len(o.extra) {
		return false
	}
	for i := range r.extra {
		if r.extra[i] != o.extra[i] {
			return false
		}
	}
	return true
}

func u32bytes(v []uint32) []byte {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], x)
	}
	return b
}

func goResult[I Index](c *constrained[I], p Polygon, extra *[]float64) cdtResult {
	var r cdtResult
	r.loaded = c.load(p)
	if !r.loaded {
		return r
	}
	r.built = c.build()
	for _, i := range c.strips {
		r.strips = append(r.strips, uint32(i))
	}
	r.ends = append(r.ends, c.stripEnds...)
	r.cw = append(r.cw, c.stripCW...)
	r.extraCount = c.extraCount()
	*extra = c.extraPoints(*extra)
	for _, v := range *extra {
		r.extra = append(r.extra, math.Float64bits(v))
	}
	r.area = math.Float64bits(c.inputRingsArea())
	r.covered = math.Float64bits(c.covered(c.strips, c.stripEnds))
	return r
}

func readCResult(rd *bufio.Reader) (cdtResult, error) {
	var r cdtResult
	var b [8]byte
	u8 := func() uint8 {
		io.ReadFull(rd, b[:1])
		return b[0]
	}
	u32 := func() uint32 {
		io.ReadFull(rd, b[:4])
		return binary.LittleEndian.Uint32(b[:4])
	}
	u64 := func() uint64 {
		io.ReadFull(rd, b[:8])
		return binary.LittleEndian.Uint64(b[:8])
	}
	if _, err := rd.Peek(1); err != nil {
		return r, err
	}
	r.loaded = u8() != 0
	if !r.loaded {
		return r, nil
	}
	r.built = u8() != 0
	r.strips = make([]uint32, u32())
	for i := range r.strips {
		r.strips[i] = u32()
	}
	r.ends = make([]uint32, u32())
	for i := range r.ends {
		r.ends[i] = u32()
	}
	r.cw = make([]uint8, len(r.ends))
	for i := range r.cw {
		r.cw[i] = u8()
	}
	r.extraCount = u32()
	r.extra = make([]uint64, u32())
	for i := range r.extra {
		r.extra[i] = u64()
	}
	r.area = u64()
	_, err := rd.Peek(8)
	r.covered = u64()
	return r, err
}

func writePolygons(w io.Writer, polys []Polygon) {
	bw := bufio.NewWriter(w)
	var b [8]byte
	u32 := func(v uint32) {
		binary.LittleEndian.PutUint32(b[:4], v)
		bw.Write(b[:4])
	}
	u32(uint32(len(polys)))
	for _, p := range polys {
		u32(uint32(len(p)))
		for _, ring := range p {
			u32(uint32(len(ring)))
			for _, q := range ring {
				binary.LittleEndian.PutUint64(b[:], math.Float64bits(q[0]))
				bw.Write(b[:])
				binary.LittleEndian.PutUint64(b[:], math.Float64bits(q[1]))
				bw.Write(b[:])
			}
		}
	}
	bw.Flush()
}

// buildOracle compiles the C++ oracle; the test is skipped unless EARSTRIP_CDT_ORACLE is set.
func buildOracle(t *testing.T) string {
	t.Helper()
	if os.Getenv("EARSTRIP_CDT_ORACLE") == "" {
		t.Skip("set EARSTRIP_CDT_ORACLE=1 to compare with the C++ (needs a C++17 compiler)")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "oracle.cpp")
	if err := os.WriteFile(src, []byte(cdtOracleSource), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "oracle")
	include, _ := filepath.Abs(filepath.Join("third_party", "earstrip"))
	args := []string{"-std=c++17", "-O2", "-ffp-contract=off", "-I", include, "-o", bin, src}
	if os.Getenv("EARSTRIP_CDT_ASSERT") == "" {
		args = append([]string{"-DNDEBUG"}, args...)
	}
	cxx := os.Getenv("CXX")
	if cxx == "" {
		cxx = "c++"
	}
	if out, err := exec.Command(cxx, args...).CombinedOutput(); err != nil {
		t.Fatalf("compiling the oracle: %v\n%s", err, out)
	}
	return bin
}

// compareWithOracle runs the polygons through the C++ and the Go and reports the differing ones.
func compareWithOracle[I Index](t *testing.T, bin, name string, polys []Polygon) {
	t.Helper()
	mode := "32"
	if ^I(0) == I(math.MaxUint16) {
		mode = "16"
	}
	var in bytes.Buffer
	writePolygons(&in, polys)
	cmd := exec.Command(bin, mode)
	cmd.Stdin = &in
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: oracle: %v\n%s", name, err, stderr.Bytes())
	}
	rd := bufio.NewReader(bytes.NewReader(out))
	var c constrained[I]
	var extra []float64
	diffs, failed, withExtra, notLoaded, indices := 0, 0, 0, 0, 0
	for k, p := range polys {
		want, err := readCResult(rd)
		if err != nil && err != io.EOF {
			t.Fatalf("%s: reading polygon %d: %v", name, k, err)
		}
		got := goResult(&c, p, &extra)
		switch {
		case !got.loaded:
			notLoaded++
		case !got.built:
			failed++
		case got.extraCount > 0:
			withExtra++
		}
		indices += len(got.strips)
		if !got.equal(&want) {
			diffs++
			if diffs <= 3 {
				msg := fmt.Sprintf("%s: polygon %d (%d rings) differs", name, k, len(p))
				if total := pointCount(p); total <= 40 {
					msg += fmt.Sprintf("\npolygon %v\n   go %v\n  c++ %v", p, &got, &want)
				} else {
					msg += fmt.Sprintf(" (%d points)\n   go: built %v %d indices %d extra\n  c++: built %v %d indices %d extra",
						total, got.built, len(got.strips), got.extraCount, want.built, len(want.strips), want.extraCount)
				}
				t.Error(msg)
			}
		}
	}
	t.Logf("%s: %d polygons, %d differ (%d not loaded, %d failed, %d with added points, %d indices)", name,
		len(polys), diffs, notLoaded, failed, withExtra, indices)
}

func pointCount(p Polygon) int {
	n := 0
	for _, r := range p {
		n += len(r)
	}
	return n
}

func TestConstrainedOracle(t *testing.T) {
	bin := buildOracle(t)
	hand := []Polygon{
		{square(0, 0, 10, 10)},
		{square(0, 0, 10, 10), square(2, 2, 4, 4)},
		{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}},
		{{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}, {10, 0}, {10, 10}, {0, 10}}},
		{square(0, 0, 10, 10), square(20, 0, 24, 4)},
		{{{0, 0}, {5, 5}, {10, 0}, {10, 10}, {5, 5}, {0, 10}}},
		{square(0, 0, 10, 10), {{0, 0}, {5, 2}, {5, 5}}},
		{square(0, 0, 10, 10), square(5, 5, 15, 15)},
		{{{21.0, 52.2}, {21.01, 52.21}, {21.01, 52.2}, {21.0, 52.21}}},
		{square(0.1, 0.2, 0.7, 0.9), square(0.3, 0.3, 0.4, 0.5)},
		{{{0, 0}, {1, 1}, {2, 2}, {3, 3}}},
		{{{0, 0}, {math.Inf(1), 1}, {2, math.NaN()}, {3, 0}}},
		{{{-2147483648, -2147483648}, {2147483647, 2147483647}, {2147483647, -2147483648}, {-2147483648, 2147483647}}},
		{{{-2147483648, 0}, {2147483647, 1}, {0, 2147483647}, {1, -2147483648}, {-5, 3}}},
	}
	rng := rand.New(rand.NewPCG(7, 8))
	// a ring with many crossings runs into the work limit
	hand = append(hand, Polygon{randomRing(rng, 1200, 1e6, true)})
	compareWithOracle[uint32](t, bin, "hand-made", hand)

	var random []Polygon
	n := 5000
	if testing.Short() {
		n = 500
	}
	if v := os.Getenv("EARSTRIP_CDT_RANDOM"); v != "" { // more random polygons, another seed
		var seed uint64
		fmt.Sscan(v, &n, &seed)
		rng = rand.New(rand.NewPCG(seed, 8))
	}
	for range n {
		random = append(random, randomPolygon(rng))
	}
	compareWithOracle[uint32](t, bin, "random", random)
	compareWithOracle[uint16](t, bin, "random uint16", random)

	// the 16-bit index range: crossings that run out of indices
	big := Polygon{{}}
	for k := 0; k < 65530; k++ {
		a := 2 * math.Pi * float64(k) / 65530
		big[0] = append(big[0], [2]float64{math.Round(1e6 * math.Cos(a)), math.Round(1e6 * math.Sin(a))})
	}
	big = append(big, [][2]float64{{-2e6, -10}, {2e6, -10}, {2e6, 10}, {-2e6, 10}})
	compareWithOracle[uint16](t, bin, "uint16 range", []Polygon{big, {big[0][:65533], big[1][:1]}})

	dataDir := os.Getenv("EARSTRIP_DATA")
	if dataDir == "" {
		dataDir = "data"
	}
	for _, name := range []string{"world", "gom4old", "z9", "z13"} {
		polys, err := readPLY(filepath.Join(dataDir, name+".bin"))
		if err != nil {
			t.Logf("%s: %v", name, err)
			continue
		}
		step := 1
		if limit := os.Getenv("EARSTRIP_CDT_SAMPLE"); limit != "" {
			var m int
			fmt.Sscan(limit, &m)
			if m > 0 && len(polys) > m {
				step = len(polys) / m
			}
		}
		var sample []Polygon
		for i := 0; i < len(polys); i += step {
			sample = append(sample, polys[i])
		}
		compareWithOracle[uint32](t, bin, name, sample)
		if name == "world" || name == "gom4old" {
			// as floating point coordinates, rounded to the grid
			var floats []Polygon
			for i := 0; i < len(sample); i += 4 {
				floats = append(floats, transform(sample[i], 1e-5, 13.37, 52.51, false))
			}
			compareWithOracle[uint32](t, bin, name+" as floats", floats)
		}
	}
}

// readPLY reads the benchmark polygons: u32 magic "PLY1", polygons, rings, points; u32 polyRingStart[polygons+1];
// u32 ringPointStart[rings+1]; i16 xy[2*points].
func readPLY(path string) ([]Polygon, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 16 || string(data[:4]) != "PLY1" {
		return nil, fmt.Errorf("%s: not PLY1", path)
	}
	u32 := func(i int) uint32 { return binary.LittleEndian.Uint32(data[4*i:]) }
	polygons, rings, points := int(u32(1)), int(u32(2)), int(u32(3))
	polyStart := 4
	ringStart := polyStart + polygons + 1
	xy := 4 * (ringStart + rings + 1)
	if len(data) < xy+4*points {
		return nil, fmt.Errorf("%s: truncated", path)
	}
	coord := func(i int) float64 { return float64(int16(binary.LittleEndian.Uint16(data[xy+2*i:]))) }
	out := make([]Polygon, polygons)
	for p := range out {
		r0, r1 := int(u32(polyStart+p)), int(u32(polyStart+p+1))
		poly := make(Polygon, r1-r0)
		for r := r0; r < r1; r++ {
			s0, s1 := int(u32(ringStart+r)), int(u32(ringStart+r+1))
			ring := make([][2]float64, s1-s0)
			for i := range ring {
				ring[i] = [2]float64{coord(2 * (s0 + i)), coord(2*(s0+i) + 1)}
			}
			poly[r-r0] = ring
		}
		out[p] = poly
	}
	return out, nil
}

func TestConstrainedLimitsAndCloser(t *testing.T) {
	if got := (&constrained[uint16]{}).maxPoints(); got != 65534 {
		t.Errorf("uint16 maxPoints %d", got)
	}
	if got := (&constrained[uint32]{}).maxPoints(); got != math.MaxUint32-2 {
		t.Errorf("uint32 maxPoints %d", got)
	}
	if got := (&constrained[uint64]{}).maxPoints(); got != math.MaxUint32-2 {
		t.Errorf("uint64 maxPoints %d", got)
	}
	var c constrained[uint64]
	runCDT(t, &c, Polygon{square(0, 0, 10, 10), square(5, 5, 15, 15)})
	area := c.inputRingsArea() // 200 - 200: the hole is counted whole
	if area != 0 {
		t.Fatalf("inputRingsArea %v", area)
	}
	// the constrained strips cover 150; nothing covers 0, exactly the ring area
	if !c.closer(nil, nil, area) {
		t.Error("empty strips not closer to an area of 0")
	}
	if c.closer(nil, nil, 150) {
		t.Error("empty strips closer to an area of 150")
	}
}
