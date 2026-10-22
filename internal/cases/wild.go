package cases

import (
	"math"
	"math/rand/v2"
)

// Kind is a family of generated polygons for the differential tests.
type Kind int

// The kinds of generated polygons.
const (
	KindSimple     Kind = iota // RandomPolygon: valid, the ears cover it exactly
	KindGrid                   // GridHoles: valid, many holes sharing rows
	KindCrossing               // random points: the ring crosses itself
	KindTouching               // holes sharing vertices with the outer ring and each other
	KindLooseHoles             // holes inside, outside and across the outer ring
	KindDegenerate             // duplicate points, collinear runs, spikes, tiny and empty rings
	KindFloat                  // lon/lat-like float coordinates
	KindHuge                   // integer coordinates beyond 2^31
	KindManyRings              // more than 128 rings
	KindSmall                  // a few points on a tiny grid: many coincidences
	kinds
)

// Kinds lists every Kind.
func Kinds() []Kind {
	out := make([]Kind, kinds)
	for i := range out {
		out[i] = Kind(i)
	}
	return out
}

func (k Kind) String() string {
	return [...]string{"simple", "grid", "crossing", "touching", "loose holes", "degenerate", "float", "huge",
		"many rings", "small"}[k]
}

// Wild returns a random polygon of the given kind.
func Wild(rng *rand.Rand, kind Kind) [][][2]float64 {
	switch kind {
	case KindSimple:
		scale := int64(1)
		if rng.IntN(3) == 0 {
			scale = 40000
		}
		return toFloat(RandomPolygon(rng, scale))
	case KindGrid:
		return toFloat(GridHoles(rng))
	case KindCrossing:
		rings := Rings{randomPoints(rng, 3+rng.IntN(40), 1000)}
		for range rng.IntN(3) {
			rings = append(rings, randomPoints(rng, 3+rng.IntN(8), 1000))
		}
		return toFloat(rings)
	case KindTouching:
		return toFloat(touching(rng))
	case KindLooseHoles:
		outer := starRing(rng, 24+rng.IntN(40), 1000)
		rings := Rings{outer}
		for range 1 + rng.IntN(8) {
			cx, cy := int64(rng.IntN(3000)-1500), int64(rng.IntN(3000)-1500)
			rings = append(rings, Scaled(randomConvex(rng, 3+rng.IntN(5), 30+int64(rng.IntN(400))), 1, cx, cy))
		}
		return toFloat(rings)
	case KindDegenerate:
		return toFloat(degenerate(rng))
	case KindFloat:
		return lonLat(rng)
	case KindHuge:
		base := RandomPolygon(rng, 1)
		if rng.IntN(2) == 0 {
			base = touching(rng)
		}
		s := int64(1) << (20 + rng.IntN(16)) // up to 2^35 * 10^4 ~ 2^48: still exact in float64
		off := int64(rng.IntN(1<<20)) << 30
		out := make(Rings, len(base))
		for r, ring := range base {
			out[r] = Scaled(ring, s, off, -off)
		}
		return toFloat(out)
	case KindManyRings:
		return toFloat(manyRings(rng))
	case KindSmall:
		rings := Rings{randomPoints(rng, 3+rng.IntN(9), 6)}
		for range rng.IntN(4) {
			rings = append(rings, randomPoints(rng, 1+rng.IntN(5), 6))
		}
		return toFloat(rings)
	}
	panic("cases: unknown kind")
}

func toFloat(rings Rings) [][][2]float64 {
	out := make([][][2]float64, len(rings))
	for r, ring := range rings {
		out[r] = make([][2]float64, len(ring))
		for i, p := range ring {
			out[r][i] = [2]float64{float64(p[0]), float64(p[1])}
		}
	}
	return out
}

func randomPoints(rng *rand.Rand, n int, size int) Ring {
	ring := make(Ring, n)
	for i := range ring {
		ring[i] = [2]int64{int64(rng.IntN(size + 1)), int64(rng.IntN(size + 1))}
	}
	return ring
}

// starRing is a star-shaped ring around the origin.
func starRing(rng *rand.Rand, n int, radius float64) Ring {
	ring := make(Ring, n)
	for i := range ring {
		a := 2 * math.Pi * (float64(i) + 0.4*rng.Float64()) / float64(n)
		r := radius * (0.4 + 0.6*rng.Float64())
		ring[i] = [2]int64{lround(r * math.Cos(a)), lround(r * math.Sin(a))}
	}
	return ring
}

// randomConvex is a convex-ish ring of n points within radius of the origin.
func randomConvex(rng *rand.Rand, n int, radius int64) Ring {
	ring := make(Ring, n)
	for i := range ring {
		a := 2 * math.Pi * (float64(i) + 0.5*rng.Float64()) / float64(n)
		r := float64(radius) * (0.5 + 0.5*rng.Float64())
		ring[i] = [2]int64{lround(r * math.Cos(a)), lround(r * math.Sin(a))}
	}
	if rng.IntN(2) == 0 {
		return Reversed(ring)
	}
	return ring
}

// touching makes holes that share vertices with the outer ring or with each other.
func touching(rng *rand.Rand) Rings {
	const size = 60
	outer := Ring{{0, 0}, {size, 0}, {size, size}, {0, size}}
	// put some vertices on the outer edges
	var o Ring
	for i, p := range outer {
		o = append(o, p)
		q := outer[(i+1)%len(outer)]
		if rng.IntN(2) == 0 {
			t := int64(1 + rng.IntN(size-1))
			o = append(o, [2]int64{p[0] + (q[0]-p[0])*t/size, p[1] + (q[1]-p[1])*t/size})
		}
	}
	rings := Rings{o}
	var verts Ring
	verts = append(verts, o...)
	for range 1 + rng.IntN(6) {
		// a triangle with one corner at an existing vertex (or edge point) and two inside
		anchor := verts[rng.IntN(len(verts))]
		h := Ring{anchor}
		for range 2 + rng.IntN(2) {
			h = append(h, [2]int64{int64(5 + rng.IntN(size-10)), int64(5 + rng.IntN(size-10))})
		}
		if rng.IntN(2) == 0 {
			h = Reversed(h)
		}
		rings = append(rings, h)
		verts = append(verts, h...)
	}
	return rings
}

func degenerate(rng *rand.Rand) Rings {
	rings := RandomPolygon(rng, 1)
	if rng.IntN(2) == 0 {
		rings = GridHoles(rng)
	}
	for r, ring := range rings {
		var out Ring
		for i, p := range ring {
			out = append(out, p)
			q := ring[(i+1)%len(ring)]
			switch rng.IntN(12) {
			case 0: // repeated point, several times
				for range 1 + rng.IntN(3) {
					out = append(out, p)
				}
			case 1: // collinear run
				for k := int64(1); k < 4; k++ {
					out = append(out, [2]int64{p[0] + (q[0]-p[0])*k/4, p[1] + (q[1]-p[1])*k/4})
				}
			case 2: // zero-width spike
				out = append(out, [2]int64{p[0] + int64(rng.IntN(21)-10), p[1] + int64(rng.IntN(21)-10)}, p)
			}
		}
		if rng.IntN(8) == 0 && r > 0 {
			out = out[:rng.IntN(min(3, len(out)))] // empty, single-point or two-point hole
		}
		if rng.IntN(10) == 0 {
			out = append(out, out...) // walked twice
		}
		rings[r] = out
	}
	if rng.IntN(6) == 0 {
		rings = append(rings, Ring{}) // empty ring
	}
	return rings
}

func lonLat(rng *rand.Rand) [][][2]float64 {
	cx, cy := 21.0122+rng.Float64(), 52.2297+rng.Float64()
	n := 8 + rng.IntN(200)
	out := [][][2]float64{make([][2]float64, n)}
	for i := range n {
		a := 2 * math.Pi * (float64(i) + 0.4*rng.Float64()) / float64(n)
		r := 0.01 * (0.5 + 0.5*rng.Float64())
		out[0][i] = [2]float64{cx + r*math.Cos(a), cy + 0.6*r*math.Sin(a)}
	}
	for range rng.IntN(5) {
		m := 3 + rng.IntN(6)
		hx, hy := cx+0.004*(rng.Float64()-0.5), cy+0.003*(rng.Float64()-0.5)
		h := make([][2]float64, m)
		for i := range m {
			a := -2 * math.Pi * float64(i) / float64(m)
			h[i] = [2]float64{hx + 0.0008*math.Cos(a), hy + 0.0005*math.Sin(a)}
		}
		out = append(out, h)
	}
	if rng.IntN(4) == 0 { // a crossing: swap two points
		i, j := rng.IntN(n), rng.IntN(n)
		out[0][i], out[0][j] = out[0][j], out[0][i]
	}
	return out
}

func manyRings(rng *rand.Rand) Rings {
	nx, ny := 12+rng.IntN(10), 11+rng.IntN(10) // 132 to 420 holes
	rings := Rings{{{0, 0}, {int64(20*nx + 10), 0}, {int64(20*nx + 10), int64(20*ny + 10)}, {0, int64(20*ny + 10)}}}
	for i := range nx {
		for j := range ny {
			x, y := int64(10+20*i), int64(10+20*j)
			switch rng.IntN(10) {
			case 0: // touching the neighbour
				rings = append(rings, Ring{{x, y}, {x, y + 10}, {x + 20, y + 10}, {x + 10, y}})
			case 1: // crossing the outer ring
				rings = append(rings, Ring{{x, -5}, {x + 5, -5}, {x + 5, y}})
			default:
				rings = append(rings, Ring{{x, y}, {x, y + 10}, {x + 10, y + 10}, {x + 10, y}})
			}
		}
	}
	if rng.IntN(3) == 0 { // a crossing outer ring
		rings[0] = append(rings[0], [2]int64{int64(10*nx + 5), int64(-20)})
	}
	return rings
}
