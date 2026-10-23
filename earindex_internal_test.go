package earstrip

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

// CheckEarIndex turns on checkEarIndex for the test and returns a function that reports how many answers were
// compared since; earindex_test.go uses it from outside the package.
func CheckEarIndex(t testing.TB) func() uint64 {
	checkEarIndex = true
	start := earIndexChecks
	t.Cleanup(func() { checkEarIndex = false })
	return func() uint64 { return earIndexChecks - start }
}

// The edge pruning rests on the float cross being largest over a box at the corner earIndex.empty picks; random
// boxes of float points, from tiny to near earIndexBound, never have a point above it.
func TestEarCrossMonotone(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	coord := func(scale float64) float64 { return (rng.Float64()*2 - 1) * scale }
	for range 200_000 {
		scale := math.Ldexp(1, rng.IntN(1000)-490)
		if scale > earIndexBound {
			scale = earIndexBound
		}
		px, py, qx, qy := coord(scale), coord(scale), coord(scale), coord(scale)
		x0, x1 := coord(scale), coord(scale)
		y0, y1 := coord(scale), coord(scale)
		if rng.IntN(4) == 0 { // a box around the edge's own line
			x0, y0 = px+(qx-px)*rng.Float64(), py+(qy-py)*rng.Float64()
			x1, y1 = x0+math.Abs(coord(scale*1e-12)), y0+math.Abs(coord(scale*1e-12))
		}
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		top := cross(px, py, qx, qy, pick(qy-py >= 0, x0, x1), pick(qx-px >= 0, y1, y0))
		for range 8 {
			rx, ry := x0+(x1-x0)*rng.Float64(), y0+(y1-y0)*rng.Float64()
			rx, ry = min(max(rx, x0), x1), min(max(ry, y0), y1)
			if got := cross(px, py, qx, qy, rx, ry); got > top {
				t.Fatalf("edge (%v, %v)-(%v, %v), box [%v, %v]x[%v, %v]: cross at (%v, %v) = %v above the corner's %v",
					px, py, qx, qy, x0, x1, y0, y1, rx, ry, got, top)
			}
		}
	}
}

func TestSelectEarItems(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for _, n := range []int{1, 2, 17, 18, 100, 1000, 5000} {
		for _, pattern := range []string{"random", "sorted", "reversed", "equal", "few", "organ"} {
			p := make([]earItem, n)
			for i := range p {
				var v float64
				switch pattern {
				case "random":
					v = rng.Float64()
				case "sorted":
					v = float64(i)
				case "reversed":
					v = float64(n - i)
				case "equal":
					v = 1
				case "few":
					v = float64(rng.IntN(3))
				case "organ":
					v = float64(min(i, n-i))
				}
				p[i] = earItem{p: [2]float64{rng.Float64(), v}, node: uint32(i)}
			}
			sorted := make([]float64, n)
			for i := range p {
				sorted[i] = p[i].p[1]
			}
			slices.Sort(sorted)
			for _, k := range []int{0, n / 3, n / 2, n - 1} {
				q := slices.Clone(p)
				selectEarItems(q, k, 1)
				if q[k].p[1] != sorted[k] {
					t.Fatalf("%s %d, k %d: %v, want %v", pattern, n, k, q[k].p[1], sorted[k])
				}
				for i := range q {
					if (i < k && q[i].p[1] > q[k].p[1]) || (i > k && q[i].p[1] < q[k].p[1]) {
						t.Fatalf("%s %d, k %d: item %d out of place", pattern, n, k, i)
					}
				}
				seen := make([]bool, n)
				for i := range q {
					seen[q[i].node] = true
				}
				if slices.Contains(seen, false) {
					t.Fatalf("%s %d, k %d: items lost", pattern, n, k)
				}
			}
		}
	}
}
