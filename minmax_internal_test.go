package earstrip

import (
	"math"
	"testing"
)

// min3, max3 and min4 must agree with the nested std::min and std::max wherever it matters, NaNs included.
func TestMinMaxNested(t *testing.T) {
	nan := math.NaN()
	vals := []float64{nan, math.Inf(-1), -2, 0, 1, 1, 7, math.Inf(1)}
	same := func(a, b float64) bool { return a == b || (a != a && b != b) }
	for _, a := range vals {
		for _, b := range vals {
			for _, c := range vals {
				if got, want := min3(a, b, c), minf(a, minf(b, c)); !same(got, want) {
					t.Errorf("min3(%v, %v, %v) = %v, want %v", a, b, c, got, want)
				}
				if got, want := max3(a, b, c), maxf(a, maxf(b, c)); !same(got, want) {
					t.Errorf("max3(%v, %v, %v) = %v, want %v", a, b, c, got, want)
				}
				for _, d := range vals {
					if got, want := min4(a, b, c, d), minf(minf(a, b), minf(c, d)); !same(got, want) {
						t.Errorf("min4(%v, %v, %v, %v) = %v, want %v", a, b, c, d, got, want)
					}
				}
			}
		}
	}
}
