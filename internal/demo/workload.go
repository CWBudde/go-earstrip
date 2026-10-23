package demo

import (
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/cwbudde/go-earstrip/internal/cases"
)

// The benchmark of the demo compares triangulators on polygons that come in any size, so that it can show how the
// time grows with the number of vertices. Coordinates are integers within ±2²⁵, where the predicates of the ears are
// exact.

// MaxWorkloadVertices is the largest size Workload makes.
const MaxWorkloadVertices = 250_000

// WorkloadInfo names a polygon family of the benchmark.
type WorkloadInfo struct {
	Name  string
	About string
}

var workloads = []struct {
	WorkloadInfo
	make func(rng *rand.Rand, vertices int) cases.Rings
}{
	{WorkloadInfo{"smooth ring", "One ring around a center like a coastline: wide bays and capes, and a " +
		"little noise at the scale of its edges."}, func(rng *rand.Rand, n int) cases.Rings {
		return cases.Rings{starRing(rng, n, 0, 0, radius, false)}
	}},
	{WorkloadInfo{"spiky ring", "One ring around a center, every corner at a random distance from half the " +
		"radius to all of it: a needle per corner, hard for ear clipping."}, func(rng *rand.Rand, n int) cases.Rings {
		return cases.Rings{starRing(rng, n, 0, 0, radius, true)}
	}},
	{WorkloadInfo{"comb", "One ring of thin teeth: almost every other corner is reflex."}, comb},
	{WorkloadInfo{"square with 49 holes", "Seven by seven smooth holes, each with a share of the vertices."},
		func(rng *rand.Rand, n int) cases.Rings { return holes(rng, n, 7) }},
	{WorkloadInfo{"square with small holes", "Hexagonal holes on a grid, one per six vertices. With more than " +
		"128 rings earstrip starts with the constrained triangulation."},
		func(rng *rand.Rand, n int) cases.Rings { return holes(rng, n, math.MaxInt) }},
	{WorkloadInfo{"ring with kinks", "The smooth ring with every 25th pair of corners swapped, so that it " +
		"crosses itself there, as rounding to a tile grid makes map polygons do."}, kinks},
}

// Workloads lists the polygon families Workload makes, in their order.
func Workloads() []WorkloadInfo {
	out := make([]WorkloadInfo, len(workloads))
	for i, w := range workloads {
		out[i] = w.WorkloadInfo
	}
	return out
}

// Workload returns the polygon of the named family (see Workloads) with about the given number of vertices. The
// same name and size always give the same polygon.
func Workload(name string, vertices int) (Flat, error) {
	if vertices < 3 || vertices > MaxWorkloadVertices {
		return Flat{}, fmt.Errorf("%d vertices: want 3 to %d", vertices, MaxWorkloadVertices)
	}
	for i, w := range workloads {
		if w.Name == name {
			return FlatInts(w.make(rand.New(rand.NewPCG(uint64(vertices), uint64(i))), vertices)), nil
		}
	}
	return Flat{}, fmt.Errorf("unknown workload %q", name)
}

// radius is the size of the rings around a center. Their corners are more than 10 units apart up to
// MaxWorkloadVertices, so rounding to integers keeps them simple.
const radius = 1 << 20

// starRing returns n corners in angular order around (cx, cy), so that the ring is simple, at distances from
// 0.5 r to r: random ones if spiky, else a smooth outline with noise of about the length of an edge.
func starRing(rng *rand.Rand, n int, cx, cy, r float64, spiky bool) cases.Ring {
	return polar(starCorners(rng, n, r, spiky), cx, cy)
}

// starCorners returns the angles and distances of starRing's corners.
func starCorners(rng *rand.Rand, n int, r float64, spiky bool) [][2]float64 {
	// The outline: 0.7 r and four waves of up to 0.05 r each.
	var phase [4]float64
	for k := range phase {
		phase[k] = 2 * math.Pi * rng.Float64()
	}
	edge := 2 * math.Pi * 0.5 * r / float64(n)
	corners := make([][2]float64, n)
	for i := range corners {
		angle := 2 * math.Pi * (float64(i) + 0.8*rng.Float64()) / float64(n)
		var d float64
		if spiky {
			d = r * (0.5 + 0.5*rng.Float64())
		} else {
			d = 0.7 * r
			for k, p := range phase {
				d += 0.05 * r * math.Sin(float64(k+2)*angle+p)
			}
			d += edge * (2*rng.Float64() - 1)
		}
		corners[i] = [2]float64{angle, d}
	}
	return corners
}

// polar converts angles and distances around (cx, cy) to integer points.
func polar(corners [][2]float64, cx, cy float64) cases.Ring {
	ring := make(cases.Ring, len(corners))
	for i, c := range corners {
		ring[i] = [2]int64{int64(math.Round(cx + c[1]*math.Cos(c[0]))), int64(math.Round(cy + c[1]*math.Sin(c[0])))}
	}
	return ring
}

// comb has teeth of four corners along the bottom edge, as in cases.Combs, and three more corners.
func comb(_ *rand.Rand, n int) cases.Rings {
	teeth := max(1, (n-3)/4)
	ring := make(cases.Ring, 0, 4*teeth+3)
	for i := range int64(teeth) {
		ring = append(ring, [2]int64{i * 10, 0}, [2]int64{i*10 + 4, 0}, [2]int64{i*10 + 5, 60 + (i*37)%30},
			[2]int64{i*10 + 6, 0})
	}
	w := int64(teeth) * 10
	return cases.Rings{append(ring, [2]int64{w, 0}, [2]int64{w, 100}, [2]int64{0, 100})}
}

// holes is a square with up to maxCells × maxCells smooth holes, one per cell of a grid, which share the
// vertices: about 6 each while the grid may grow, at least 3.
func holes(rng *rand.Rand, n, maxCells int) cases.Rings {
	k := min(max(1, int(math.Round(math.Sqrt(float64(n-4)/6)))), maxCells)
	per := max(3, int(math.Round(float64(n-4)/float64(k*k))))
	// The grid stays within 2²⁵ up to MaxWorkloadVertices: 204 cells of 2¹⁷.
	const cell = 1 << 17
	w := int64(k) * cell
	rings := cases.Rings{{{0, 0}, {w, 0}, {w, w}, {0, w}}}
	for cx := range k {
		for cy := range k {
			rings = append(rings, starRing(rng, per, (float64(cx)+0.5)*cell, (float64(cy)+0.5)*cell, 0.4*cell, false))
		}
	}
	return rings
}

// kinks swaps corners 25 k + 1 and 25 k + 2 of a smooth ring. It first moves them and their neighbors to the
// same distance: edges between four points on a circle in the order a, c, b, d cross.
func kinks(rng *rand.Rand, n int) cases.Rings {
	corners := starCorners(rng, n, radius, false)
	for i := 1; i+2 < len(corners); i += 25 {
		for j := i - 1; j <= i+2; j++ {
			corners[j][1] = corners[i][1]
		}
		corners[i], corners[i+1] = corners[i+1], corners[i]
	}
	return cases.Rings{polar(corners, 0, 0)}
}
