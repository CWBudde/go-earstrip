// Package cases holds the polygons of earstrip's test/test.cpp (the named cases of main(), its fixtures and its
// random generators) and further generators for the differential tests, shared by the package tests and the
// oracle tests.
package cases

import (
	"bufio"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
)

// Rings is a polygon with integer coordinates, ring 0 the outer ring (test/test.cpp's Rings of int32 points).
type Rings = [][][2]int64

// Ring is one ring of points.
type Ring = [][2]int64

// Case is one call of check() in test/test.cpp: the polygon, whether the ears (forced cuts and the constrained
// triangulation off) accept it, whether they drop a hole, and whether either result is fine (degenerate input
// outside the guarantees; an accepted one must still cover exactly).
type Case struct {
	Name    string
	Rings   Rings
	OK      bool
	Dropped bool
	Either  bool
}

// Square is test.cpp's square.
var Square = Ring{{0, 0}, {10, 0}, {10, 10}, {0, 10}}

// NearSpike is a ring of the gom4 data whose corners all turn one way but that crosses itself near a spike.
var NearSpike = Ring{{68, 1684}, {129, 1657}, {176, 1626}, {196, 1608}, {212, 1585}, {223, 1559}, {228, 1531},
	{227, 1248}, {227, 1249}, {230, 526}, {0, 527}, {0, 1698}}

// Reversed returns r in the opposite direction.
func Reversed(r Ring) Ring {
	out := slices.Clone(r)
	slices.Reverse(out)
	return out
}

// Scaled returns r scaled by s and moved by dx, dy.
func Scaled(r Ring, s, dx, dy int64) Ring {
	out := make(Ring, len(r))
	for i, p := range r {
		out[i] = [2]int64{p[0]*s + dx, p[1]*s + dy}
	}
	return out
}

// Named returns the cases main() of test/test.cpp checks one by one, in its order, up to the random polygons;
// the variants, stars and combs are in their own functions.
func Named() []Case {
	sq := Square
	r := func(rings ...Ring) Rings { return rings }
	cs := []Case{
		{Name: "triangle", Rings: r(Ring{{0, 0}, {5, 0}, {0, 7}}), OK: true},
		{Name: "triangle cw", Rings: r(Ring{{0, 0}, {0, 7}, {5, 0}}), OK: true},
		{Name: "convex quad", Rings: r(sq), OK: true},
		{Name: "convex quad cw closed", Rings: r(Ring{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}}), OK: true},
		{Name: "concave quad", Rings: r(Ring{{0, 0}, {10, 0}, {3, 3}, {0, 10}}), OK: true},
		{Name: "concave quad cw", Rings: r(Reversed(Ring{{0, 0}, {10, 0}, {3, 3}, {0, 10}})), OK: true},
		{Name: "convex hexagon", Rings: r(Ring{{0, 0}, {4, -2}, {8, 0}, {8, 4}, {4, 6}, {0, 4}}), OK: true},
		{Name: "L shape", Rings: r(Ring{{0, 0}, {10, 0}, {10, 4}, {4, 4}, {4, 10}, {0, 10}}), OK: true},
		{Name: "square with hole", Rings: r(sq, Ring{{3, 3}, {7, 3}, {7, 7}, {3, 7}}), OK: true},
		{Name: "square with cw hole", Rings: r(sq, Reversed(Ring{{3, 3}, {7, 3}, {7, 7}, {3, 7}})), OK: true},
		{Name: "two holes", Rings: r(Scaled(sq, 2, 0, 0), Ring{{2, 2}, {8, 2}, {8, 8}, {2, 8}}, Ring{{12, 12}, {18, 12}, {15, 18}}), OK: true},
		{Name: "two holes same rightmost x", Rings: r(Scaled(sq, 2, 0, 0), Ring{{2, 2}, {8, 2}, {8, 8}, {2, 8}}, Ring{{4, 12}, {8, 12}, {8, 18}}), OK: true},
		{Name: "hole touching outer vertex", Rings: r(sq, Ring{{10, 10}, {4, 7}, {7, 4}}), OK: true},
		{Name: "hole touching outer, not rightmost", Rings: r(Ring{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 5}}, Ring{{0, 5}, {5, 3}, {5, 7}}), OK: true},
		{Name: "hole vertex on vertical outer edge", Rings: r(sq, Ring{{10, 5}, {4, 8}, {4, 2}}), OK: true},
		{Name: "hole vertex on horizontal outer edge", Rings: r(sq, Ring{{6, 10}, {3, 8}, {5, 6}}), OK: true},
		{Name: "hole touching other hole", Rings: r(Scaled(sq, 2, 0, 0), Ring{{2, 2}, {8, 2}, {8, 8}, {2, 8}}, Ring{{8, 8}, {14, 9}, {9, 14}}), OK: true},
		{Name: "holes touching outer and each other", Rings: r(Ring{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 5}}, Ring{{0, 5}, {5, 3}, {5, 7}}, Ring{{5, 7}, {8, 9}, {9, 4}}), OK: true},
		{Name: "duplicates and collinear", Rings: r(Ring{{0, 0}, {0, 0}, {5, 0}, {10, 0}, {10, 0}, {10, 5}, {10, 10}, {5, 10}, {5, 10}, {0, 10}, {0, 5}, {0, 0}}), OK: true},
		{Name: "collinear concave", Rings: r(Ring{{0, 0}, {4, 0}, {8, 0}, {8, 4}, {6, 4}, {4, 4}, {4, 8}, {2, 8}, {0, 8}, {0, 4}}), OK: true},
		// a spike makes the ring non-simple: its ears may be cut before the cleanup sees it, so refusing is allowed
		{Name: "zero width spike", Rings: r(Ring{{0, 0}, {10, 0}, {10, 10}, {5, 10}, {5, 15}, {5, 10}, {0, 10}}), OK: true, Either: true},
		{Name: "zero width notch", Rings: r(Ring{{0, 0}, {10, 0}, {10, 10}, {5, 10}, {5, 5}, {5, 10}, {0, 10}}), OK: true, Either: true},
		{Name: "all collinear", Rings: r(Ring{{0, 0}, {5, 0}, {10, 0}}), OK: true},
		{Name: "two distinct points", Rings: r(Ring{{0, 0}, {5, 0}, {0, 0}, {5, 0}}), OK: true},
		{Name: "empty", Rings: r(Ring{}), OK: true},
		// Rings whose corners all turn one way but that wind around more than once are not convex: no fast path for
		// them. Refusing them is allowed; an accepted one must still be covered exactly by triangles that all turn
		// one way.
		{Name: "near spike crossing itself (gom4)", Rings: r(NearSpike), OK: true, Either: true},
	}
	cs = append(cs, Combs()...)
	// World scale coordinates (~600 million).
	const big = 600000000
	cs = append(cs,
		Case{Name: "world-scale square with hole", Rings: r(Scaled(sq, big/10, -big/2, -big/2),
			Scaled(Ring{{3, 3}, {7, 3}, {7, 7}, {3, 7}}, big/10, -big/2, -big/2)), OK: true},
		Case{Name: "world-scale L", Rings: r(Scaled(Ring{{0, 0}, {10, 0}, {10, 4}, {4, 4}, {4, 10}, {0, 10}}, big/10, 0, 0)), OK: true},
		// Invalid input.
		Case{Name: "bow tie", Rings: r(Ring{{0, 0}, {10, 10}, {10, 0}, {0, 10}})},
		Case{Name: "bow tie asymmetric", Rings: r(Ring{{0, 0}, {20, 20}, {20, 0}, {0, 10}})},
		Case{Name: "hole outside", Rings: r(sq, Ring{{20, 20}, {30, 20}, {30, 30}}), OK: true, Dropped: true},
		Case{Name: "hole left of outer", Rings: r(sq, Ring{{-30, 2}, {-20, 2}, {-25, 8}}), OK: true, Dropped: true},
		Case{Name: "hole outside, other hole kept", Rings: r(Scaled(sq, 2, 0, 0), Ring{{2, 2}, {8, 2}, {5, 8}}, Ring{{30, 30}, {40, 30}, {35, 40}}), OK: true, Dropped: true},
		// Degenerate and invalid inputs: a refusal hands them to the constrained triangulation, an acceptance must
		// still cover exactly
		Case{Name: "single-point holes", Rings: r(sq, Ring{{3, 3}}, Ring{{6, 6}}, Ring{{5, 2}}), OK: true, Either: true},
		Case{Name: "ring touching itself at a vertex", Rings: r(Ring{{0, 0}, {5, 0}, {5, 5}, {10, 5}, {10, 10}, {5, 10}, {5, 5}, {0, 5}}), OK: true, Either: true},
		Case{Name: "hole equal to the outer ring", Rings: r(sq, sq), OK: true, Either: true},
		Case{Name: "hole along an outer edge", Rings: r(sq, Ring{{0, 2}, {5, 2}, {5, 8}, {0, 8}}), OK: true, Either: true},
		Case{Name: "touching holes closing a loop", Rings: r(Scaled(sq, 3, 0, 0),
			Ring{{6, 3}, {12, 6}, {6, 9}, {3, 6}},
			Ring{{12, 6}, {18, 3}, {24, 6}, {18, 9}},
			Ring{{12, 6}, {18, 9}, {12, 12}, {9, 9}},
			Ring{{12, 6}, {9, 9}, {6, 9}, {9, 3}}), OK: true, Either: true},
		// Valid inputs that tempt a wrong diagonal
		Case{Name: "collinear tips", Rings: r(Ring{{0, 0}, {10, 0}, {10, 4}, {8, 2}, {6, 4}, {4, 2}, {2, 4}, {0, 2}}), OK: true},
		Case{Name: "collinear valleys", Rings: r(Ring{{0, 0}, {12, 0}, {12, 6}, {10, 2}, {8, 6}, {6, 2}, {4, 6}, {2, 2}, {0, 6}}), OK: true},
		Case{Name: "holes on one row", Rings: r(Scaled(sq, 4, 0, 0), Ring{{4, 10}, {8, 10}, {6, 14}}, Ring{{14, 10}, {18, 10}, {16, 14}}, Ring{{24, 10}, {28, 10}, {26, 14}}), OK: true},
		Case{Name: "hole right of a reflex vertex", Rings: r(Ring{{0, 0}, {20, 0}, {20, 20}, {0, 20}, {0, 12}, {10, 10}, {0, 8}}, Ring{{12, 9}, {16, 9}, {14, 11}}), OK: true},
	)
	for _, p := range Parse(SnapRounded) {
		cs = append(cs, Case{Name: "snap rounded, " + p.Name, Rings: p.Rings, OK: true})
	}
	for _, p := range Parse(EdgeTouching) {
		cs = append(cs, Case{Name: p.Name, Rings: p.Rings, OK: true, Either: strings.Contains(p.Name, "invalid")})
	}
	return cs
}

// Combs are the comb and hole cases: hundreds of reflex corners go through the bucket index.
func Combs() []Case {
	const teeth = 120
	var spikes Ring
	for i := range int64(teeth) {
		spikes = append(spikes, [2]int64{i * 10, 0}, [2]int64{i*10 + 4, 0}, [2]int64{i*10 + 5, 60 + (i*37)%30}, [2]int64{i*10 + 6, 0})
	}
	spikes = append(spikes, [2]int64{teeth * 10, 0}, [2]int64{teeth * 10, 100}, [2]int64{0, 100})
	var slots Ring
	for i := range int64(teeth) {
		slots = append(slots, [2]int64{i * 10, 0}, [2]int64{i*10 + 5, 0}, [2]int64{i*10 + 5, 90}, [2]int64{i*10 + 8, 90}, [2]int64{i*10 + 8, 0})
	}
	slots = append(slots, [2]int64{teeth * 10, 0}, [2]int64{teeth * 10, 100}, [2]int64{0, 100})
	holes := Rings{Scaled(Square, 100, 0, 0)}
	for i := range int64(9) {
		for j := range int64(9) {
			holes = append(holes, Ring{{i*100 + 20, j*100 + 20}, {i*100 + 80, j*100 + 30}, {i*100 + 70, j*100 + 80}, {i*100 + 50, j*100 + 50}})
		}
	}
	return []Case{
		{Name: "comb, 483 points, notches", Rings: Rings{spikes}, OK: true},
		{Name: "comb, 603 points, slots", Rings: Rings{slots}, OK: true},
		{Name: "81 holes", Rings: holes, OK: true},
	}
}

// NearSpikeVariants are the 192 variants of NearSpike: rotations by quarter turns, mirrors, every start point,
// tile and world scale. Either result is fine.
func NearSpikeVariants() []Case {
	var cs []Case
	for sym := range 8 {
		for start := range NearSpike {
			for _, scale := range []int64{1, 100000} {
				ring := make(Ring, 0, len(NearSpike))
				for i := range NearSpike {
					p := NearSpike[(start+i)%len(NearSpike)]
					if sym&1 != 0 {
						p = [2]int64{-p[0], p[1]}
					}
					for range sym / 2 {
						p = [2]int64{-p[1], p[0]}
					}
					ring = append(ring, [2]int64{p[0] * scale, p[1] * scale})
				}
				cs = append(cs, Case{Name: "near spike variant " + strconv.Itoa(len(cs)), Rings: Rings{ring}, OK: true, Either: true})
			}
		}
	}
	return cs
}

// Stars are the star polygons {points/step} (every corner turns the same way, the ring winds step times), each
// also reversed, and a square walked twice. Either result is fine.
func Stars() []Case {
	var cs []Case
	for points := 5; points <= 13; points++ {
		for step := 2; 2*step < points; step++ {
			if gcd(points, step) != 1 {
				continue
			}
			var star Ring
			for i := range points {
				angle := 2 * math.Pi * float64(step) * float64(i) / float64(points)
				star = append(star, [2]int64{lround(1000 * math.Cos(angle)), lround(1000 * math.Sin(angle))})
			}
			name := "star " + strconv.Itoa(points) + "/" + strconv.Itoa(step)
			cs = append(cs,
				Case{Name: name, Rings: Rings{star}, OK: true, Either: true},
				Case{Name: name + " reversed", Rings: Rings{Reversed(star)}, OK: true, Either: true})
		}
	}
	cs = append(cs, Case{Name: "square twice", Rings: Rings{{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}, {10, 0}, {10, 10}, {0, 10}}}, OK: true, Either: true})
	return cs
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// lround rounds half away from zero, as std::lround.
func lround(v float64) int64 { return int64(math.Round(v)) }

// NamedRings is a named polygon of a fixture.
type NamedRings struct {
	Name  string
	Rings Rings
}

// Parse reads polygons in the fixture format of test/test.cpp: "# name", one line of "x,y" points per ring (outer
// first), blank lines between polygons.
func Parse(text string) []NamedRings {
	var out []NamedRings
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if line[0] == '#' {
			out = append(out, NamedRings{Name: line[2:]})
			continue
		}
		last := &out[len(out)-1]
		var ring Ring
		for _, tok := range strings.Fields(line) {
			xs, ys, _ := strings.Cut(tok, ",")
			x, err1 := strconv.ParseInt(xs, 10, 64)
			y, err2 := strconv.ParseInt(ys, 10, 64)
			if err1 != nil || err2 != nil {
				panic("cases: bad point " + tok)
			}
			ring = append(ring, [2]int64{x, y})
		}
		last.Rings = append(last.Rings, ring)
	}
	return out
}

// RandomPolygon is randomPolygon of test/test.cpp: a random star-shaped outer ring with small holes inside its
// kernel, some touching each other, with repeated and collinear points sprinkled in and random ring orientations.
// The ears must cover it exactly. (The random stream differs from the C++ mt19937.)
func RandomPolygon(rng *rand.Rand, scale int64) Rings {
	unit := rng.Float64
	const radius = 10000.0
	points := 24 + int(unit()*300) // enough points to keep the inner disk inside
	rings := Rings{nil}
	for i := range points {
		angle := 2 * math.Pi * (float64(i) + 0.4*unit()) / float64(points)
		r := radius * (0.4 + 0.6*unit())
		rings[0] = append(rings[0], [2]int64{lround(r * math.Cos(angle)), lround(r * math.Sin(angle))})
	}
	// holes on a grid of cells inside the disk of radius 0.35 * radius, which the star always contains
	cells := int(unit() * 7)
	cell := 0.7 * radius / float64(max(cells, 1)) / math.Sqrt2
	grid := func(k int) int64 { return int64(math.Floor(-0.35*radius/math.Sqrt2 + float64(k)*cell)) }
	for cx := range cells {
		for cy := range cells {
			if unit() < 0.3 {
				continue
			}
			x0, y0 := float64(grid(cx)), float64(grid(cy))
			var hole Ring
			if unit() < 0.3 {
				// full cell square: squares on the diagonals touch at corners in an X that encloses nothing
				if cx != cy && (cx+cy != cells-1 || cells%2 == 0) {
					continue
				}
				hole = Ring{{grid(cx), grid(cy)}, {grid(cx + 1), grid(cy)}, {grid(cx + 1), grid(cy + 1)}, {grid(cx), grid(cy + 1)}}
			} else {
				n := 3 + int(unit()*6)
				for i := range n {
					angle := 2 * math.Pi * (float64(i) + 0.5*unit()) / float64(n)
					r := cell * (0.15 + 0.3*unit())
					hole = append(hole, [2]int64{lround(x0 + cell/2 + r*math.Cos(angle)), lround(y0 + cell/2 + r*math.Sin(angle))})
				}
			}
			rings = append(rings, hole)
		}
	}
	for k, ring := range rings {
		// repeated points and collinear midpoints
		var out Ring
		for i, p := range ring {
			out = append(out, p)
			q := ring[(i+1)%len(ring)]
			if unit() < 0.05 {
				out = append(out, p)
			}
			if unit() < 0.05 && (q[0]-p[0])%2 == 0 && (q[1]-p[1])%2 == 0 {
				out = append(out, [2]int64{(p[0] + q[0]) / 2, (p[1] + q[1]) / 2})
			}
		}
		if unit() < 0.5 {
			out = Reversed(out)
		}
		for i := range out {
			out[i] = [2]int64{out[i][0] * scale, out[i][1] * scale}
		}
		rings[k] = out
	}
	return rings
}

// GridHoles is gridHoles of test/test.cpp: tiny convex holes with corners on a coarse integer grid in separate
// cells, so that many vertices of different holes share a row with the rightmost vertex of another hole.
func GridHoles(rng *rand.Rand) Rings {
	coord := func() int64 { return int64(rng.IntN(7)) }
	count := func() int { return 3 + rng.IntN(5) }
	cells := int64(3 + rng.IntN(10))
	const cell = 8
	rings := Rings{{{-cell, -cell}, {cells*cell + cell, -cell}, {cells*cell + cell, cells*cell + cell}, {-cell, cells*cell + cell}}}
	for cx := range cells {
		for cy := range cells {
			if rng.IntN(4) == 0 {
				continue
			}
			// convex hull of a few random grid points
			var pts Ring
			for i := count(); i > 0; i-- {
				pts = append(pts, [2]int64{cx*cell + coord(), cy*cell + coord()})
			}
			slices.SortFunc(pts, func(a, b [2]int64) int {
				if a[0] != b[0] {
					return int(a[0] - b[0])
				}
				return int(a[1] - b[1])
			})
			var hull Ring
			for range 2 {
				base := len(hull)
				for _, p := range pts {
					for len(hull) >= base+2 {
						a, b := hull[len(hull)-2], hull[len(hull)-1]
						if (b[0]-a[0])*(p[1]-a[1])-(b[1]-a[1])*(p[0]-a[0]) > 0 {
							break
						}
						hull = hull[:len(hull)-1]
					}
					hull = append(hull, p)
				}
				hull = hull[:len(hull)-1]
				slices.Reverse(pts)
			}
			if len(hull) >= 3 {
				if rng.IntN(2) == 1 {
					rings = append(rings, hull)
				} else {
					rings = append(rings, Reversed(hull))
				}
			}
		}
	}
	return rings
}
