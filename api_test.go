package earstrip_test

import (
	"math"
	"strings"
	"testing"

	earstrip "github.com/cwbudde/go-earstrip"
)

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Fatalf("no panic, want one containing %q", want)
		}
		if s, _ := r.(string); !strings.Contains(s, want) {
			t.Fatalf("panic %v, want one containing %q", r, want)
		}
	}()
	f()
}

func circle(n int) [][2]float64 {
	ring := make([][2]float64, n)
	for i := range ring {
		a := 2 * math.Pi * float64(i) / float64(n)
		ring[i] = [2]float64{math.Round(1e6 * math.Cos(a)), math.Round(1e6 * math.Sin(a))}
	}
	return ring
}

func TestIndexTypeTooSmall(t *testing.T) {
	var small earstrip.Triangulator[uint16]
	if s := small.Run([][][2]float64{circle(1 << 16)}); s == earstrip.Refused {
		t.Fatalf("65536 vertices fit uint16 indices, got %v", s)
	}
	mustPanic(t, "do not fit the index type", func() { small.Run([][][2]float64{circle(1<<16 + 1)}) })
}

func TestRunFlatHoleStarts(t *testing.T) {
	sq := []float64{0, 0, 10, 0, 10, 10, 0, 10, 3, 3, 7, 3, 7, 7, 3, 7}
	var tr earstrip.Triangulator[uint32]
	for _, hs := range [][]int{{100}, {-1}, {6, 4}, {9}} {
		mustPanic(t, "hole start", func() { tr.RunFlat(sq, 2, hs) })
	}
	for _, hs := range [][]int{nil, {4}, {4, 4}, {8}} {
		if s := tr.RunFlat(sq, 2, hs); s != earstrip.Exact {
			t.Errorf("hole starts %v: status %v, want Exact", hs, s)
		}
	}
}

// A strip run after a list run must not keep the old list, also when many rings take the constrained path first.
func TestTriangleListClearedAfterModeSwitch(t *testing.T) {
	var tr earstrip.Triangulator[uint32]
	tr.SetTriangleList(true)
	tr.Run([][][2]float64{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}})
	if len(tr.TriangleList()) == 0 {
		t.Fatal("list mode wrote no triangles")
	}
	tr.SetTriangleList(false)
	poly := [][][2]float64{{{0, 0}, {1000, 0}, {1000, 1000}, {0, 1000}}}
	for i := range 200 {
		x, y := float64(10+(i%14)*70), float64(10+(i/14)*70)
		poly = append(poly, [][2]float64{{x, y}, {x + 20, y}, {x + 10, y + 20}})
	}
	if s := tr.Run(poly); s != earstrip.Constrained {
		t.Fatalf("status %v, want Constrained", s)
	}
	if n := len(tr.TriangleList()); n != 0 {
		t.Fatalf("stale triangle list of %d indices", n)
	}
}
