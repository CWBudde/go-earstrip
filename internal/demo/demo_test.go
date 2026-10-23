package demo

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/cwbudde/go-earstrip"
)

var defaults = Options{ForcedCuts: true, Constrained: true, ExtraPoints: true}

func TestPresets(t *testing.T) {
	var r Runner
	seen := map[[2]string]bool{}
	for _, p := range Presets() {
		key := [2]string{p.Group, p.Name}
		if seen[key] {
			t.Errorf("%s / %s: listed twice", p.Group, p.Name)
		}
		seen[key] = true
		res, err := r.Run(FlatInts(p.Rings), defaults)
		if err != nil {
			t.Errorf("%s / %s: %v", p.Group, p.Name, err)
			continue
		}
		if len(res.Triangles)%3 != 0 {
			t.Errorf("%s / %s: %d triangle indices", p.Group, p.Name, len(res.Triangles))
		}
	}
}

func TestGenerate(t *testing.T) {
	var r Runner
	for _, kind := range Kinds() {
		for seed := range uint64(5) {
			f, err := Generate(kind, seed)
			if err != nil {
				t.Fatalf("%s: %v", kind, err)
			}
			again, _ := Generate(kind, seed)
			if !slices.Equal(f.Coords, again.Coords) {
				t.Errorf("%s, seed %d: not deterministic", kind, seed)
			}
			if _, err := r.Run(f, defaults); err != nil {
				t.Errorf("%s, seed %d: %v", kind, seed, err)
			}
		}
	}
	if _, err := Generate("nope", 1); err == nil {
		t.Error("unknown kind: no error")
	}
}

func TestRunMatchesTriangulator(t *testing.T) {
	polygon := [][][2]float64{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}, {{3, 3}, {7, 3}, {7, 7}, {3, 7}}}
	var tr earstrip.Triangulator[uint32]
	status := tr.Run(polygon)

	var r Runner
	f := FlatFloats(polygon)
	if !slices.Equal(f.HoleStarts, []int{4}) {
		t.Fatalf("hole starts %v, want [4]", f.HoleStarts)
	}
	res, err := r.Run(f, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != status || !slices.Equal(res.Strips, tr.Strips()) || !slices.Equal(res.StripEnds, tr.StripEnds()) ||
		!slices.Equal(res.Triangles, tr.Triangles(nil)) {
		t.Errorf("got %v %v %v, want %v %v %v", res.Status, res.Strips, res.StripEnds, status, tr.Strips(), tr.StripEnds())
	}

	list, err := r.Run(f, Options{ForcedCuts: true, Constrained: true, ExtraPoints: true, TriangleList: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Strips) != 0 || !slices.Equal(list.Triangles, tr.Triangles(nil)) {
		t.Errorf("triangle list mode: strips %v, triangles %v, want none and %v", list.Strips, list.Triangles, tr.Triangles(nil))
	}
}

// TestRestart decodes Result.Restart as WebGL 2 draws a TRIANGLE_STRIP with primitive restart and checks that it
// gives the triangles of Result.Triangles, in the same order and orientation.
func TestRestart(t *testing.T) {
	var r Runner
	for _, p := range Presets() {
		res, err := r.Run(FlatInts(p.Rings), defaults)
		if err != nil {
			t.Fatalf("%s / %s: %v", p.Group, p.Name, err)
		}
		var got []uint32
		strip := 0
		for i := range res.Restart {
			if res.Restart[i] == RestartIndex {
				strip = 0
				continue
			}
			strip++
			if strip < 3 {
				continue
			}
			a, b, c := res.Restart[i-2], res.Restart[i-1], res.Restart[i]
			if strip%2 == 0 {
				a, b = b, a
			}
			if a != b && b != c && a != c {
				got = append(got, a, b, c)
			}
		}
		if !slices.Equal(got, res.Triangles) {
			t.Errorf("%s / %s: restart buffer %v gives %v, want %v", p.Group, p.Name, res.Restart, got, res.Triangles)
		}
	}
	list, err := r.Run(FlatInts(Presets()[0].Rings), Options{TriangleList: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Restart) != 0 {
		t.Errorf("triangle list mode: restart buffer %v, want none", list.Restart)
	}
}

func TestExtraPoints(t *testing.T) {
	var r Runner
	bowTie := Flat{Coords: []float64{0, 0, 10, 10, 10, 0, 0, 10}}
	res, err := r.Run(bowTie, defaults)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != earstrip.Constrained || len(res.Extra) == 0 {
		t.Errorf("bow tie: %v with %d extra coordinates, want Constrained with a crossing point", res.Status, len(res.Extra))
	}
}

func TestBadInput(t *testing.T) {
	var r Runner
	for name, f := range map[string]Flat{
		"odd":            {Coords: []float64{0, 0, 1}},
		"not finite":     {Coords: []float64{0, 0, 1, 0, 0, math.Inf(1)}},
		"holes reversed": {Coords: make([]float64, 20), HoleStarts: []int{6, 3}},
		"hole past end":  {Coords: make([]float64, 20), HoleStarts: []int{11}},
	} {
		if _, err := r.Run(f, defaults); err == nil {
			t.Errorf("%s: no error", name)
		}
		if _, err := r.Time(f, defaults, time.Millisecond); err == nil {
			t.Errorf("%s: Time gave no error", name)
		}
	}
}

func TestTime(t *testing.T) {
	var r Runner
	tm, err := r.Time(FlatInts(Presets()[0].Rings), defaults, 2*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if tm.Runs < 1 || tm.PerRun <= 0 {
		t.Errorf("%d runs at %v", tm.Runs, tm.PerRun)
	}
}
