package demo

import (
	"math"
	"slices"
	"testing"

	"github.com/cwbudde/go-earstrip"
)

func TestWorkloads(t *testing.T) {
	var r Runner
	for _, w := range Workloads() {
		for _, n := range []int{3, 100, 1000, 10_000, 100_000} {
			f, err := Workload(w.Name, n)
			if err != nil {
				t.Fatalf("%s, %d: %v", w.Name, n, err)
			}
			again, _ := Workload(w.Name, n)
			if !slices.Equal(f.Coords, again.Coords) || !slices.Equal(f.HoleStarts, again.HoleStarts) {
				t.Errorf("%s, %d: not deterministic", w.Name, n)
			}
			got := len(f.Coords) / 2
			if n >= 100 && math.Abs(float64(got-n)) > 0.05*float64(n) {
				t.Errorf("%s, %d: %d vertices", w.Name, n, got)
			}
			for _, v := range f.Coords {
				if v != math.Trunc(v) || math.Abs(v) > 1<<25 {
					t.Errorf("%s, %d: coordinate %v, want an integer within 2^25", w.Name, n, v)
					break
				}
			}
			res, err := r.Run(f, defaults)
			if err != nil {
				t.Fatalf("%s, %d: %v", w.Name, n, err)
			}
			if n < 100 {
				continue
			}
			want := earstrip.Exact
			if w.Name == "ring with kinks" || len(f.HoleStarts) >= 128 {
				want = earstrip.Constrained
			}
			if res.Status != want {
				t.Errorf("%s, %d: %v, want %v", w.Name, n, res.Status, want)
			}
		}
	}
}

func TestWorkloadBadInput(t *testing.T) {
	for _, n := range []int{2, MaxWorkloadVertices + 1} {
		if _, err := Workload(Workloads()[0].Name, n); err == nil {
			t.Errorf("%d vertices: no error", n)
		}
	}
	if _, err := Workload("nope", 100); err == nil {
		t.Error("unknown workload: no error")
	}
}
