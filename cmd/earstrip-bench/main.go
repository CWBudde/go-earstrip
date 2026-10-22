// Command earstrip-bench times go-earstrip over a PLY1 file of polygons, as earstrip's bench/bench.cpp does.
//
//	earstrip-bench <file.bin> [-r repeats] [--check] [--no-constrained]
//
// --check compares the ears' strips (constrained triangulation and forced cuts off) with the hashes stored in the
// file (z13.bin, z9.bin) and exits with 1 on a difference. The timing is the best of the repeats (default 5) of a
// pass over every polygon with one reused Triangulator[uint16], the C++ index type. Then it counts the statuses
// and, on the polygons the ears cannot cut exactly, the share of pixels on a 128 x 128 grid where the triangles
// differ from the even-odd fill of the rings.
package main

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/cwbudde/go-earstrip"
	"github.com/cwbudde/go-earstrip/internal/plydata"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func usage() int {
	fmt.Fprintln(os.Stderr, "usage: earstrip-bench <file.bin> [-r repeats] [--check] [--no-constrained]")
	return 2
}

func run(args []string) int {
	if len(args) < 1 {
		return usage()
	}
	repeats, check, constrained := 5, false, true
	for i := 1; i < len(args); i++ {
		switch {
		case args[i] == "-r" && i+1 < len(args):
			i++
			r, err := strconv.Atoi(args[i])
			if err != nil || r < 1 {
				return usage()
			}
			repeats = r
		case args[i] == "--check":
			check = true
		case args[i] == "--no-constrained":
			constrained = false
		default:
			return usage()
		}
	}
	f, err := plydata.Load(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	fmt.Printf("%d polygons, %d rings, %d points\n", f.Len(), f.Rings(), f.Points())

	var strip earstrip.Triangulator[uint16]
	var p plydata.Poly
	if check {
		if f.HashEars == nil {
			fmt.Fprintln(os.Stderr, "--check: the file has no result hashes")
			return 2
		}
		// the hashes are of the ears alone
		strip.SetForcedCuts(false)
		strip.SetConstrained(false)
		mismatches := 0
		for i := range f.Len() {
			p = f.Polygon(i)
			s := strip.RunRings(&p)
			if plydata.Hash(s, strip.HolesLeftOut(), strip.Strips(), strip.StripEnds()) != f.HashEars[i] {
				mismatches++
			}
		}
		fmt.Printf("check: %d of %d polygons differ from the expected strips\n", mismatches, f.Len())
		strip.SetForcedCuts(true)
		strip.SetConstrained(true)
		if mismatches > 0 {
			return 1
		}
	}

	strip.SetConstrained(constrained)
	best := time.Duration(math.MaxInt64)
	for range repeats {
		start := time.Now()
		for i := range f.Len() {
			p = f.Polygon(i)
			strip.RunRings(&p)
		}
		best = min(best, time.Since(start))
	}

	// what the triangulator makes of the polygons the ears cannot cut exactly
	var exact, holes, approximate, covered, added, triangles, indices, strips int
	var dev, worst float64
	var tris []uint16
	for i := range f.Len() {
		p = f.Polygon(i)
		s := strip.RunRings(&p)
		switch s {
		case earstrip.Exact:
			exact++
		case earstrip.HolesLeftOut:
			holes++
		}
		indices += len(strip.Strips())
		strips += len(strip.StripEnds())
		tris = strip.Triangles(tris[:0])
		triangles += len(tris) / 3
		if s != earstrip.Approximate && s != earstrip.Constrained {
			continue
		}
		if s == earstrip.Approximate {
			approximate++
		} else {
			covered++
		}
		added += len(strip.ExtraPoints()) / 2
		d := pixelError(p, tris, strip.ExtraPoints())
		dev += d
		worst = max(worst, d)
	}
	fmt.Printf("earstrip:   best of %d %.1f ms; %d triangles, %.3f indices per triangle in %d strips\n", repeats,
		float64(best.Microseconds())/1000, triangles, float64(indices)/float64(max(triangles, 1)), strips)
	hard := approximate + covered
	fmt.Printf("            exact %d, holes left out %d, constrained %d with %d added points, approximate %d\n",
		exact, holes, covered, added, approximate)
	mean := 0.0
	if hard > 0 {
		mean = dev / float64(hard)
	}
	fmt.Printf("            on those %d, wrong pixels against the even-odd fill: mean %.4f, worst %.4f\n", hard, mean, worst)
	return 0
}

type pt struct{ x, y int16 }

// pixelError is pixelError of bench.cpp: on a 128 x 128 grid over the polygon, the pixels where the union of the
// triangles differs from the even-odd fill of the rings, relative to the pixels the rings fill. Added points are
// truncated to the int16 grid as the C++ does.
func pixelError(poly plydata.Poly, tris []uint16, extra []float64) float64 {
	var all []pt
	minX, minY, maxX, maxY := 1e30, 1e30, -1e30, -1e30
	for r := range poly.Count() {
		for i := range poly.Size(r) {
			q := pt{int16(poly.X(r, i)), int16(poly.Y(r, i))}
			all = append(all, q)
			minX, maxX = min(minX, float64(q.x)), max(maxX, float64(q.x))
			minY, maxY = min(minY, float64(q.y)), max(maxY, float64(q.y))
		}
	}
	for k := 0; k+1 < len(extra); k += 2 {
		all = append(all, pt{int16(extra[k]), int16(extra[k+1])})
	}
	const grid = 128
	sx, sy := (maxX-minX)/grid, (maxY-minY)/grid
	filled, wrong := 0, 0
	for gy := range grid {
		y := minY + (float64(gy)+0.5)*sy
		for gx := range grid {
			x := minX + (float64(gx)+0.5)*sx
			inRings := false
			for r := range poly.Count() {
				n := poly.Size(r)
				for i, j := 0, n-1; i < n; j, i = i, i+1 {
					ax, ay, bx, by := poly.X(r, j), poly.Y(r, j), poly.X(r, i), poly.Y(r, i)
					if (ay > y) != (by > y) && x < (bx-ax)*(y-ay)/(by-ay)+ax {
						inRings = !inRings
					}
				}
			}
			inTriangles := false
			for k := 0; k+2 < len(tris) && !inTriangles; k += 3 {
				a, b, c := all[tris[k]], all[tris[k+1]], all[tris[k+2]]
				d1 := sub(b.x, a.x)*(y-float64(a.y)) - sub(b.y, a.y)*(x-float64(a.x))
				d2 := sub(c.x, b.x)*(y-float64(b.y)) - sub(c.y, b.y)*(x-float64(b.x))
				d3 := sub(a.x, c.x)*(y-float64(c.y)) - sub(a.y, c.y)*(x-float64(c.x))
				inTriangles = (d1 >= 0 && d2 >= 0 && d3 >= 0) || (d1 <= 0 && d2 <= 0 && d3 <= 0)
			}
			if inRings {
				filled++
			}
			if inRings != inTriangles {
				wrong++
			}
		}
	}
	if filled > 0 {
		return float64(wrong) / float64(filled)
	}
	if wrong > 0 {
		return 1
	}
	return 0
}

// sub is u - v with the C++ promotion to int: no int16 overflow.
func sub(u, v int16) float64 { return float64(int(u) - int(v)) }
