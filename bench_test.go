package earstrip_test

import (
	"math"
	"testing"

	"github.com/cwbudde/go-earstrip"
	"github.com/cwbudde/go-earstrip/internal/plydata"
)

// benchFile triangulates every polygon of a benchmark file per op with one reused Triangulator, default settings
// (as bench/bench.cpp times it, with uint16 indices there). Skips without the file.
func benchFile(b *testing.B, name string) {
	f := loadData(b, name)
	var tr earstrip.Triangulator[uint32]
	var p plydata.Poly
	indices := 0
	b.ReportAllocs()
	for b.Loop() {
		indices = 0
		for i := range f.Len() {
			p = f.Polygon(i)
			tr.RunRings(&p)
			indices += len(tr.Strips())
		}
	}
	b.ReportMetric(float64(f.Len()), "polygons/op")
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N)/float64(f.Len()), "ns/polygon")
	b.ReportMetric(float64(indices), "indices/op")
}

// BenchmarkZ13: every polygon of zoom level 13 of the Masovian Voivodeship, 3.7 million polygons.
func BenchmarkZ13(b *testing.B) { benchFile(b, "z13.bin") }

// BenchmarkZ9: the same map at zoom level 9, 684 thousand polygons.
func BenchmarkZ9(b *testing.B) { benchFile(b, "z9.bin") }

// BenchmarkWorld: the polygons of GLMap's world map tiles, up to 11,675 holes in one.
func BenchmarkWorld(b *testing.B) { benchFile(b, "world.bin") }

// BenchmarkGom4old: an older build of the Masovian map with rings that cross or touch.
func BenchmarkGom4old(b *testing.B) { benchFile(b, "gom4old.bin") }

// BenchmarkSmall triangulates hand-made polygons; the Triangulator is reused, so steady state allocates nothing.
func BenchmarkSmall(b *testing.B) {
	circle := func(n int, r, cx, cy float64, reverse bool) [][2]float64 {
		ring := make([][2]float64, n)
		for i := range ring {
			a := 2 * math.Pi * float64(i) / float64(n)
			if reverse {
				a = -a
			}
			ring[i] = [2]float64{cx + r*math.Cos(a), cy + r*math.Sin(a)}
		}
		return ring
	}
	comb := make([][2]float64, 0, 483)
	for i := range 120 {
		x := float64(i * 10)
		comb = append(comb, [2]float64{x, 0}, [2]float64{x + 4, 0}, [2]float64{x + 5, float64(60 + (i*37)%30)}, [2]float64{x + 6, 0})
	}
	comb = append(comb, [2]float64{1200, 0}, [2]float64{1200, 100}, [2]float64{0, 100})
	holes := [][][2]float64{{{0, 0}, {1000, 0}, {1000, 1000}, {0, 1000}}}
	for i := range 9 {
		for j := range 9 {
			x, y := float64(i*100), float64(j*100)
			holes = append(holes, [][2]float64{{x + 20, y + 20}, {x + 80, y + 30}, {x + 70, y + 80}, {x + 50, y + 50}})
		}
	}
	polys := []struct {
		name string
		poly [][][2]float64
	}{
		{"square", [][][2]float64{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}}},
		{"square with hole", [][][2]float64{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}, {{3, 3}, {7, 3}, {7, 7}, {3, 7}}}},
		{"circle 64", [][][2]float64{circle(64, 100, 0, 0, false)}},
		{"circle 1000 with hole", [][][2]float64{circle(1000, 100, 0, 0, false), circle(200, 30, 10, 0, true)}},
		{"comb 483", [][][2]float64{comb}},
		{"81 holes", holes},
		{"bow tie", [][][2]float64{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}}},
	}
	for _, p := range polys {
		b.Run(p.name, func(b *testing.B) {
			var tr earstrip.Triangulator[uint32]
			b.ReportAllocs()
			for b.Loop() {
				tr.Run(p.poly)
			}
		})
	}
}
