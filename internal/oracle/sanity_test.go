//go:build earstrip_cgo

package oracle_test

import (
	"testing"

	"github.com/cwbudde/go-earstrip/internal/oracle"
	"github.com/cwbudde/go-earstrip/internal/plydata"
)

// TestOracleHashes checks the oracle, the PLY1 loader and plydata.Hash together against the hashes stored in the
// data files: the C++ ears (constrained triangulation and forced cuts off) must give the stored HashEars for every
// polygon. Independent of the Go port.
func TestOracleHashes(t *testing.T) {
	for _, name := range []string{"z9.bin", "z13.bin"} {
		t.Run(name, func(t *testing.T) {
			f := loadData(t, name)
			if f.HashEars == nil {
				t.Fatalf("%s has no stored hashes", name)
			}
			o := oracle.New()
			defer o.Close()
			opts := oracle.Options{NoConstrained: true, NoForcedCuts: true}
			n := limit(f.Len(), name == "z13.bin")
			var coords []float64
			var sizes []uint32
			mismatches := 0
			for i := range n {
				coords, sizes = f.Polygon(i).Flat(coords, sizes)
				r := o.RunSized(coords, sizes, opts)
				if plydata.Hash(r.Status, r.HolesLeftOut, r.Strips, r.Ends) != f.HashEars[i] {
					if mismatches < 20 {
						p := f.Polygon(i)
						t.Errorf("polygon %d (%d rings, %d points): status %d, hash differs", i, p.Count(), p.Points(), r.Status)
					}
					mismatches++
				}
			}
			t.Logf("%s: %d polygons, %d hash mismatches", name, n, mismatches)
			if mismatches > 0 {
				t.Fatalf("%d of %d polygons differ from the stored hashes", mismatches, n)
			}
		})
	}
}
