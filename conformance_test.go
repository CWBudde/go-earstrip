package earstrip_test

import (
	"testing"

	"github.com/cwbudde/go-earstrip"
	"github.com/cwbudde/go-earstrip/internal/plydata"
)

// conformanceShort is how many polygons of a file -short checks.
const conformanceShort = 100_000

// TestConformance compares the ears (constrained triangulation and forced cuts off) with the C++ results stored in
// z13.bin and z9.bin, hash for hash, as `bench --check` does. The files come from `just fetch-data`; the test skips
// without them (EARSTRIP_DATA points elsewhere).
func TestConformance(t *testing.T) {
	for _, name := range []string{"z13", "z9"} {
		t.Run(name, func(t *testing.T) {
			f := loadData(t, name+".bin")
			if f.HashEars == nil {
				t.Fatalf("%s.bin has no stored hashes", name)
			}
			n := f.Len()
			if testing.Short() && n > conformanceShort {
				n = conformanceShort
			}
			var tr earstrip.Triangulator[uint32]
			tr.SetConstrained(false)
			tr.SetForcedCuts(false)
			var mismatches []int
			var p plydata.Poly
			var panicked error
			panics := 0
			for i := range n {
				p = f.Polygon(i)
				var s earstrip.Status
				if err := protect(func() { s = tr.RunRings(&p) }); err != nil {
					if panicked == nil {
						panicked = err
					}
					mismatches = append(mismatches, i)
					panics++
					if panics == 100 {
						t.Errorf("stopped after 100 panics, at polygon %d", i)
						break
					}
					continue
				}
				if plydata.Hash(s, tr.HolesLeftOut(), tr.Strips(), tr.StripEnds()) != f.HashEars[i] {
					mismatches = append(mismatches, i)
				}
			}
			for k, i := range mismatches {
				if k == 20 {
					break
				}
				q := f.Polygon(i)
				t.Errorf("polygon %d: %d rings, %d points: hash differs", i, q.Count(), q.Points())
			}
			if panicked != nil {
				t.Errorf("first panic: %v", panicked)
			}
			t.Logf("%s: %d polygons, %d mismatches", name, n, len(mismatches))
			if len(mismatches) > 0 {
				t.Fatalf("%d of %d polygons differ from the C++ results", len(mismatches), n)
			}
		})
	}
}

// loadData loads a benchmark file or skips when it is missing.
func loadData(tb testing.TB, name string) *plydata.File {
	tb.Helper()
	path, ok := plydata.Path(name)
	if !ok {
		tb.Skipf("%s not found (run `just fetch-data` or set EARSTRIP_DATA)", name)
	}
	f, err := plydata.Load(path)
	if err != nil {
		tb.Fatal(err)
	}
	return f
}
