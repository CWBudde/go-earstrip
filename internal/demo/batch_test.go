package demo

import (
	"os"
	"slices"
	"testing"
)

// batchOf joins the polygons into one batch.
func batchOf(polygons []Flat) Batch {
	var b Batch
	for _, f := range polygons {
		b.PolygonStarts = append(b.PolygonStarts, len(b.RingStarts))
		base := len(b.Coords) / 2
		b.RingStarts = append(b.RingStarts, base)
		for _, s := range f.HoleStarts {
			b.RingStarts = append(b.RingStarts, base+s)
		}
		b.Coords = append(b.Coords, f.Coords...)
	}
	return b
}

// checkBatchResult checks that RunMany's result is, polygon by polygon, what Run gives for the polygon on its own.
func checkBatchResult(t *testing.T, polygons []Flat, o Options) BatchResult {
	t.Helper()
	b := batchOf(polygons)
	var many Runner
	res, err := many.RunMany(b, o)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.PolygonStrips) != len(polygons)+1 || len(res.PolygonStatus) != len(polygons) {
		t.Fatalf("%d polygon strips, %d statuses for %d polygons", len(res.PolygonStrips), len(res.PolygonStatus),
			len(polygons))
	}
	if len(res.StripRestartEnds) != len(res.StripListEnds) || int(res.PolygonStrips[len(polygons)]) != len(res.StripListEnds) {
		t.Fatalf("%d strip ends in Restart, %d in List, %d strips", len(res.StripRestartEnds), len(res.StripListEnds),
			res.PolygonStrips[len(polygons)])
	}
	if len(res.List) == 0 || res.StripListEnds[len(res.StripListEnds)-1] != uint32(len(res.List)) ||
		res.StripRestartEnds[len(res.StripRestartEnds)-1] != uint32(len(res.Restart)) {
		t.Fatalf("the last strip ends at %d of %d in Restart, %d of %d in List",
			res.StripRestartEnds[len(res.StripRestartEnds)-1], len(res.Restart),
			res.StripListEnds[len(res.StripListEnds)-1], len(res.List))
	}

	var one Runner
	var statuses [len(res.Statuses)]int
	n := len(b.Coords) / 2
	extraAt := n
	for p, f := range polygons {
		own, err := one.Run(f, o)
		if err != nil {
			t.Fatal(err)
		}
		statuses[own.Status]++
		if res.PolygonStatus[p] != own.Status {
			t.Errorf("polygon %d: status %v, alone %v", p, res.PolygonStatus[p], own.Status)
		}
		// The polygon's indices, mapped back to its own.
		base := b.RingStarts[b.PolygonStarts[p]]
		count := len(f.Coords) / 2
		extras := len(own.Extra) / 2
		back := func(k uint32) uint32 {
			switch {
			case k == RestartIndex:
				return k
			case int(k) >= base && int(k) < base+count:
				return k - uint32(base)
			case int(k) >= extraAt && int(k) < extraAt+extras:
				return uint32(count + int(k) - extraAt)
			}
			t.Fatalf("polygon %d: index %d is not one of its vertices", p, k)
			return 0
		}
		for k := range extras {
			if res.Vertices[2*(extraAt+k)] != float32(own.Extra[2*k]) ||
				res.Vertices[2*(extraAt+k)+1] != float32(own.Extra[2*k+1]) {
				t.Errorf("polygon %d: extra point %d differs", p, k)
			}
		}

		s0, s1 := res.PolygonStrips[p], res.PolygonStrips[p+1]
		if int(s1-s0) != len(own.StripEnds) {
			t.Errorf("polygon %d: %d strips, alone %d", p, s1-s0, len(own.StripEnds))
			continue
		}
		var restart, list []uint32
		if s1 > s0 {
			from := uint32(0)
			if s0 > 0 {
				from = res.StripRestartEnds[s0-1] + 1 // after the restart index
			}
			for _, k := range res.Restart[from:res.StripRestartEnds[s1-1]] {
				restart = append(restart, back(k))
			}
			from = 0
			if s0 > 0 {
				from = res.StripListEnds[s0-1]
			}
			for _, k := range res.List[from:res.StripListEnds[s1-1]] {
				list = append(list, back(k))
			}
		}
		if !slices.Equal(restart, own.Restart) {
			t.Errorf("polygon %d: restart buffer %v, alone %v", p, restart, own.Restart)
		}
		if !slices.Equal(list, own.Triangles) {
			t.Errorf("polygon %d: triangles %v, alone %v", p, list, own.Triangles)
		}
		extraAt += extras
	}
	if extraAt != n+res.ExtraPoints || len(res.Vertices) != 2*extraAt {
		t.Errorf("%d vertices with %d extra points, want %d", len(res.Vertices)/2, res.ExtraPoints, extraAt)
	}
	if res.Statuses != statuses {
		t.Errorf("statuses %v, alone %v", res.Statuses, statuses)
	}
	return res
}

func TestRunManyMatchesRun(t *testing.T) {
	var polygons []Flat
	for _, p := range Presets() {
		polygons = append(polygons, FlatInts(p.Rings))
	}
	for _, kind := range Kinds() {
		for seed := range uint64(3) {
			f, err := Generate(kind, seed)
			if err != nil {
				t.Fatal(err)
			}
			polygons = append(polygons, f)
		}
	}
	// An empty polygon in the middle gives no strips and no restart index.
	polygons = slices.Insert(polygons, 3, Flat{})
	res := checkBatchResult(t, polygons, defaults)
	if res.ExtraPoints == 0 {
		t.Error("no extra points: the batch should hold polygons whose rings cross")
	}
	if slices.Contains(res.Restart[:1], RestartIndex) || res.Restart[len(res.Restart)-1] == RestartIndex {
		t.Error("a restart index at either end")
	}

	// Run again on the same Runner: the reused buffers give the same result.
	var r Runner
	first, _ := r.RunMany(batchOf(polygons), defaults)
	restart := slices.Clone(first.Restart)
	again, _ := r.RunMany(batchOf(polygons), defaults)
	if !slices.Equal(restart, again.Restart) {
		t.Error("a second run differs")
	}
}

func TestRunManyChecks(t *testing.T) {
	var r Runner
	for name, b := range map[string]Batch{
		"odd coordinates":   {Coords: []float64{0, 0, 1}, RingStarts: []int{0}, PolygonStarts: []int{0}},
		"no rings":          {Coords: []float64{0, 0, 1, 0, 1, 1}},
		"ring past the end": {Coords: []float64{0, 0, 1, 0, 1, 1}, RingStarts: []int{0, 4}, PolygonStarts: []int{0}},
		"rings descending":  {Coords: []float64{0, 0, 1, 0, 1, 1}, RingStarts: []int{0, 2, 1}, PolygonStarts: []int{0}},
		"no polygons":       {Coords: []float64{0, 0, 1, 0, 1, 1}, RingStarts: []int{0}},
		"polygon past rings": {
			Coords: []float64{0, 0, 1, 0, 1, 1}, RingStarts: []int{0}, PolygonStarts: []int{0, 2},
		},
	} {
		if _, err := r.RunMany(b, defaults); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if res, err := r.RunMany(Batch{}, defaults); err != nil || len(res.Restart) != 0 {
		t.Errorf("empty batch: %v, %d indices", err, len(res.Restart))
	}
}

// worldFile reads a level of the world map that cmd/naturalearth wrote.
func worldFile(t testing.TB, level string) World {
	t.Helper()
	data, err := os.ReadFile("../../web/public/world-" + level + ".bin")
	if err != nil {
		t.Fatal(err)
	}
	w, err := DecodeWorld(data)
	if err != nil {
		t.Fatalf("%s: %v", level, err)
	}
	return w
}

func TestWorld(t *testing.T) {
	for _, level := range []string{"110m", "50m"} {
		t.Run(level, func(t *testing.T) {
			w := worldFile(t, level)
			if len(w.Countries) < 150 || len(w.Polygons) < len(w.Countries) {
				t.Errorf("%d countries, %d polygons", len(w.Countries), len(w.Polygons))
			}
			again, err := DecodeWorld(w.Encode())
			if err != nil || !slices.EqualFunc(again.Polygons, w.Polygons, func(a, b WorldPolygon) bool {
				return a.Country == b.Country && slices.EqualFunc(a.Rings, b.Rings, slices.Equal)
			}) || !slices.Equal(again.Countries, w.Countries) {
				t.Errorf("encode and decode: %v, or the world differs", err)
			}

			b := w.Batch()
			var polygons []Flat
			for p, first := range b.PolygonStarts {
				end := len(b.RingStarts)
				if p+1 < len(b.PolygonStarts) {
					end = b.PolygonStarts[p+1]
				}
				var rings [][][2]float64
				for _, r := range w.Polygons[p].Rings {
					var ring [][2]float64
					for _, q := range r {
						ring = append(ring, [2]float64{float64(q[0]), float64(q[1])})
					}
					rings = append(rings, ring)
				}
				if len(rings) != end-first {
					t.Fatalf("polygon %d: %d rings in the batch, %d in the world", p, end-first, len(rings))
				}
				polygons = append(polygons, FlatFloats(rings))
			}
			res := checkBatchResult(t, polygons, defaults)
			t.Logf("%d polygons, %d vertices, %d extra points, %d strips, %d strip indices, %d list indices, "+
				"statuses %v, %v", len(polygons), len(b.Coords)/2, res.ExtraPoints, len(res.StripListEnds),
				len(res.Restart), len(res.List), res.Statuses, res.Elapsed)
		})
	}
}

func TestDecodeWorldBroken(t *testing.T) {
	w := worldFile(t, "110m")
	data := w.Encode()
	for name, b := range map[string][]byte{
		"empty":     nil,
		"magic":     append([]byte("XXXX"), data[4:]...),
		"version":   append(append([]byte{}, data[:4]...), append([]byte{9}, data[5:]...)...),
		"truncated": data[:len(data)/2],
		"trailing":  append(slices.Clone(data), 0),
	} {
		if _, err := DecodeWorld(b); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func BenchmarkRunManyWorld(b *testing.B) {
	for _, level := range []string{"110m", "50m"} {
		b.Run(level, func(b *testing.B) {
			w := worldFile(b, level)
			batch := w.Batch()
			var r Runner
			for b.Loop() {
				if _, err := r.RunMany(batch, defaults); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
