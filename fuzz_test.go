package earstrip_test

import (
	"testing"

	"github.com/cwbudde/go-earstrip"
	"github.com/cwbudde/go-earstrip/internal/cases"
	"github.com/cwbudde/go-earstrip/internal/verify"
)

// FuzzInvariants decodes the fuzz bytes into a polygon with holes and small integer coordinates (cases.Decode)
// and runs it with the default settings and with the ears alone (constrained triangulation and forced cuts off).
// Neither may panic, and every result must satisfy verify.Invariants: strip ends ascending, indices below the
// number of points plus the added ones, no flat triangle or flipping strip in an ear result, the exact area for
// Exact. internal/oracle's TestInvariantsOnOracle shows that the C++ satisfies the same invariants.
//
//	go test -run '^$' -fuzz FuzzInvariants -fuzztime 2m .
func FuzzInvariants(f *testing.F) {
	seeds := []cases.Rings{
		{cases.Square},
		{cases.Square, {{3, 3}, {7, 3}, {7, 7}, {3, 7}}},
		{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}},                            // bow tie
		{cases.Square, {{10, 10}, {4, 7}, {7, 4}}},                        // hole touching the outer ring
		{cases.Square, {{20, 20}, {30, 20}, {30, 30}}},                    // hole outside
		{{{0, 0}, {10, 0}, {10, 10}, {5, 10}, {5, 15}, {5, 10}, {0, 10}}}, // zero width spike
		{{{0, 0}, {10, 0}, {10, 10}, {0, 10}, {0, 0}, {10, 0}, {10, 10}}}, // walked twice
		{cases.Square, {{0, 2}, {5, 2}, {5, 8}, {0, 8}}, {{6, 6}}},        // hole along an edge, point hole
		{{{0, 0}, {5, 0}, {10, 0}}},                                       // all collinear
	}
	for _, s := range seeds {
		f.Add(cases.Encode(s, 1))
		big := make(cases.Rings, len(s)) // the same shape at the largest scale
		for r, ring := range s {
			big[r] = cases.Scaled(ring, 256, 0, 0)
		}
		f.Add(cases.Encode(big, 256))
	}
	var def, ears earstrip.Triangulator[uint32] // reused across inputs, as a caller would
	ears.SetConstrained(false)
	ears.SetForcedCuts(false)
	f.Fuzz(func(t *testing.T, data []byte) {
		rings := cases.Decode(data)
		poly := verify.Float(rings)
		for _, run := range []struct {
			name   string
			t      *earstrip.Triangulator[uint32]
			forced bool
		}{{"default", &def, true}, {"ears only", &ears, false}} {
			s := run.t.Run(poly)
			err := verify.Invariants(rings, int(s), run.forced, run.t.Strips(), run.t.StripEnds(), run.t.StripClockwise(),
				run.t.ExtraPoints())
			if err != nil {
				t.Fatalf("%s: status %v: %v\nrings %v\nstrips %v ends %v", run.name, s, err, rings, run.t.Strips(), run.t.StripEnds())
			}
		}
	})
}
