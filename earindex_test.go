package earstrip_test

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/cwbudde/go-earstrip"
	"github.com/cwbudde/go-earstrip/internal/cases"
	"github.com/cwbudde/go-earstrip/internal/demo"
)

// With CheckEarIndex on, every ear test that may use the kd tree also scans the bucket slab and panics when the two
// disagree, so these runs only have to reach many such tests.
func TestEarIndexMatchesSlab(t *testing.T) {
	checked := earstrip.CheckEarIndex(t)
	var tr earstrip.Triangulator[uint32]
	runAll := func(t *testing.T, polygon [][][2]float64) {
		t.Helper()
		for _, careful := range []bool{true, false} {
			tr.SetForcedCuts(careful)
			tr.SetConstrained(!careful)
			tr.Run(polygon)
		}
		tr.SetForcedCuts(true)
		tr.SetConstrained(true)
	}

	t.Run("workloads", func(t *testing.T) {
		sizes := []int{1000, 10_000}
		if testing.Short() {
			sizes = sizes[:1]
		}
		for _, w := range demo.Workloads() {
			for _, n := range sizes {
				f, err := demo.Workload(w.Name, n)
				if err != nil {
					t.Fatal(err)
				}
				runAll(t, unflat(f))
			}
		}
	})

	t.Run("wild", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(7, 8))
		for _, kind := range cases.Kinds() {
			for range 300 {
				runAll(t, cases.Wild(rng, kind))
			}
		}
	})

	t.Run("near degenerate", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(9, 10))
		for _, w := range []string{"smooth ring", "spiky ring", "comb", "square with 49 holes", "ring with kinks"} {
			f, err := demo.Workload(w, 3000)
			if err != nil {
				t.Fatal(err)
			}
			base := unflat(f)
			for _, m := range nearDegenerate {
				runAll(t, m.apply(rng, base))
			}
		}
		for _, m := range nearDegenerate {
			for i := range 20 {
				rings := cases.Wild(rng, cases.KindDegenerate)
				t.Run(fmt.Sprintf("%s/%d", m.name, i), func(t *testing.T) { runAll(t, m.apply(rng, rings)) })
			}
		}
	})

	if checked() == 0 {
		t.Fatal("no ear test used the index")
	}
	t.Logf("%d ear tests compared", checked())
}

func unflat(f demo.Flat) [][][2]float64 {
	starts := append(append([]int{0}, f.HoleStarts...), len(f.Coords)/2)
	out := make([][][2]float64, 0, len(starts)-1)
	for r := 0; r+1 < len(starts); r++ {
		ring := make([][2]float64, 0, starts[r+1]-starts[r])
		for i := starts[r]; i < starts[r+1]; i++ {
			ring = append(ring, [2]float64{f.Coords[2*i], f.Coords[2*i+1]})
		}
		out = append(out, ring)
	}
	return out
}

// Changes of a polygon that leave few exact predicates: rounding in every cross, collinear runs, repeated points,
// coordinates near the bounds of the index, and non-finite ones.
var nearDegenerate = []struct {
	name  string
	apply func(rng *rand.Rand, rings [][][2]float64) [][][2]float64
}{
	{"thirds far out", mapPoints(func(_ *rand.Rand, p [2]float64) [2]float64 {
		return [2]float64{1e7 + p[0]/3, -3e6 + p[1]/3}
	})},
	{"irrational", mapPoints(func(_ *rand.Rand, p [2]float64) [2]float64 {
		return [2]float64{p[0] * math.Pi, p[1] * math.Sqrt2}
	})},
	{"jitter", mapPoints(func(rng *rand.Rand, p [2]float64) [2]float64 {
		return [2]float64{p[0] + (rng.Float64()-0.5)*1e-9, p[1] + (rng.Float64()-0.5)*1e-9}
	})},
	{"tiny", mapPoints(func(_ *rand.Rand, p [2]float64) [2]float64 {
		return [2]float64{p[0] * 1e-310, p[1] * 1e-310}
	})},
	{"near bound", mapPoints(func(_ *rand.Rand, p [2]float64) [2]float64 {
		return [2]float64{p[0] * 0x1p488, p[1] * 0x1p488}
	})},
	{"beyond bound", mapPoints(func(_ *rand.Rand, p [2]float64) [2]float64 {
		return [2]float64{p[0] * 0x1p500, p[1] * 0x1p500}
	})},
	{"collinear runs", func(rng *rand.Rand, rings [][][2]float64) [][][2]float64 {
		out := make([][][2]float64, len(rings))
		for r, ring := range rings {
			for i, p := range ring {
				out[r] = append(out[r], p)
				if rng.IntN(4) == 0 {
					q := ring[(i+1)%len(ring)]
					for k := 1.0; k < 4; k++ {
						out[r] = append(out[r], [2]float64{p[0] + (q[0]-p[0])*k/4, p[1] + (q[1]-p[1])*k/4})
					}
				}
			}
		}
		return out
	}},
	{"repeated points", func(rng *rand.Rand, rings [][][2]float64) [][][2]float64 {
		out := make([][][2]float64, len(rings))
		for r, ring := range rings {
			for _, p := range ring {
				out[r] = append(out[r], p)
				if rng.IntN(5) == 0 {
					out[r] = append(out[r], p, p)
				}
			}
			if len(ring) > 10 && rng.IntN(2) == 0 { // a point of the ring visited again further on
				out[r] = append(out[r], ring[rng.IntN(len(ring))])
			}
		}
		return out
	}},
	// No infinity: in a hole one can make dropBlocker drop an item twice, with the index or without.
	{"one NaN", func(rng *rand.Rand, rings [][][2]float64) [][][2]float64 {
		return setOne(rng, rings, math.NaN())
	}},
}

func mapPoints(f func(rng *rand.Rand, p [2]float64) [2]float64) func(*rand.Rand, [][][2]float64) [][][2]float64 {
	return func(rng *rand.Rand, rings [][][2]float64) [][][2]float64 {
		out := make([][][2]float64, len(rings))
		for r, ring := range rings {
			out[r] = make([][2]float64, len(ring))
			for i, p := range ring {
				out[r][i] = f(rng, p)
			}
		}
		return out
	}
}

func setOne(rng *rand.Rand, rings [][][2]float64, v float64) [][][2]float64 {
	out := mapPoints(func(_ *rand.Rand, p [2]float64) [2]float64 { return p })(rng, rings)
	if len(out) > 0 && len(out[0]) > 0 {
		out[0][rng.IntN(len(out[0]))][rng.IntN(2)] = v
	}
	return out
}
