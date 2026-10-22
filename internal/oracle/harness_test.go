//go:build earstrip_cgo

package oracle_test

// The checks of the package tests run on the C++ results: they show that the port of test/test.cpp's harness, its
// fixtures and the fuzz invariants are right, independent of the Go port.

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"regexp"
	"slices"
	"testing"

	"github.com/cwbudde/go-earstrip/internal/cases"
	"github.com/cwbudde/go-earstrip/internal/oracle"
	"github.com/cwbudde/go-earstrip/internal/verify"
)

var earsOnly = oracle.Options{NoConstrained: true, NoForcedCuts: true}

// checkOracle is check() of test.cpp over the C++ ears.
func checkOracle(o *oracle.Oracle, cs cases.Case) error {
	r := o.Run(verify.Float(cs.Rings), earsOnly)
	ok := r.Status != oracle.Refused
	if (ok != cs.OK && !cs.Either) || r.HolesLeftOut != cs.Dropped {
		return fmt.Errorf("result %v (status %d) dropped %v, expected %v dropped %v", ok, r.Status, r.HolesLeftOut, cs.OK, cs.Dropped)
	}
	if !ok {
		return nil
	}
	_, err := verify.Exact(cs.Rings, r.Strips, r.Ends, cs.Dropped)
	return err
}

func TestHarnessOnOracle(t *testing.T) {
	o := oracle.New()
	defer o.Close()
	all := cases.Named()
	all = append(all, cases.NearSpikeVariants()...)
	all = append(all, cases.Stars()...)
	rng := rand.New(rand.NewPCG(777, 0))
	for i := range 2000 {
		all = append(all, cases.Case{Name: fmt.Sprintf("grid holes %d", i), Rings: cases.GridHoles(rng), OK: true})
	}
	rng = rand.New(rand.NewPCG(12345, 0))
	for i := range 3000 {
		scale := int64(1)
		if i%3 == 2 {
			scale = 40000
		}
		all = append(all, cases.Case{Name: fmt.Sprintf("random %d", i), Rings: cases.RandomPolygon(rng, scale), OK: true})
	}
	failed := 0
	for _, cs := range all {
		if err := checkOracle(o, cs); err != nil {
			failed++
			if failed <= 20 {
				t.Errorf("%s: %v", cs.Name, err)
			}
		}
	}
	t.Logf("%d cases, %d failed", len(all), failed)
}

// TestInvariantsOnOracle runs the fuzz invariants on the C++ results for random fuzz inputs and random wild
// polygons: an invariant the C++ breaks would be a false alarm in FuzzInvariants.
func TestInvariantsOnOracle(t *testing.T) {
	o := oracle.New()
	defer o.Close()
	settings := []struct {
		name   string
		opts   oracle.Options
		forced bool
	}{
		{"default", oracle.Options{}, true},
		{"ears only", earsOnly, false},
		{"no constrained", oracle.Options{NoConstrained: true}, true},
	}
	rng := rand.New(rand.NewPCG(1, 2))
	n := 200_000
	if testing.Short() {
		n = 20_000
	}
	failed := 0
	kinds := map[string]int{}
	digits := regexp.MustCompile(`-?\d+`)
	report := func(what string, rings cases.Rings, err error) {
		failed++
		kinds[digits.ReplaceAllString(err.Error(), "N")]++
		if failed <= 20 {
			t.Errorf("%s: %v\n%v", what, err, rings)
		}
	}
	buf := make([]byte, 64)
	for i := range n {
		buf = buf[:8+rng.IntN(56)]
		for k := range buf {
			buf[k] = byte(rng.Uint32())
		}
		rings := cases.Decode(buf)
		for _, s := range settings {
			r := o.Run(verify.Float(rings), s.opts)
			if err := verify.Invariants(rings, r.Status, s.forced, r.Strips, r.Ends, r.CW, r.Extra); err != nil {
				report(fmt.Sprintf("fuzz input %d, %s", i, s.name), rings, err)
			}
		}
	}
	perKind := 2000
	if testing.Short() {
		perKind = 200
	}
	for _, kind := range cases.Kinds() {
		if kind == cases.KindFloat {
			continue // Invariants takes integer rings
		}
		for i := range perKind {
			poly := cases.Wild(rng, kind)
			rings := toInt(poly)
			for _, s := range settings {
				r := o.Run(poly, s.opts)
				if err := verify.Invariants(rings, r.Status, s.forced, r.Strips, r.Ends, r.CW, r.Extra); err != nil {
					report(fmt.Sprintf("%v %d, %s", kind, i, s.name), rings, err)
				}
			}
		}
	}
	for k, c := range kinds {
		t.Logf("%6d × %s", c, k)
	}
	t.Logf("%d fuzz inputs, %d failures", n, failed)
}

// toInt converts rings with integral coordinates.
func toInt(poly [][][2]float64) cases.Rings {
	out := make(cases.Rings, len(poly))
	for r, ring := range poly {
		out[r] = make(cases.Ring, len(ring))
		for i, p := range ring {
			out[r][i] = [2]int64{int64(p[0]), int64(p[1])}
		}
	}
	return out
}

// TestOracleConsistency checks assumptions of the differential tests on the C++ alone: a reused triangulator gives
// what a fresh one gives, triangle list mode gives triangles() of the strips, and the flat interface gives what the
// nested rings give. List mode runs on its own triangulator: after a list mode run, a strip mode run of a polygon
// with more than 128 rings leaves the stale triangleList() of the C++ in place (that path skips runOnce, which
// clears it), so the differential tests never switch one triangulator between the modes.
func TestOracleConsistency(t *testing.T) {
	reused, listed := oracle.New(), oracle.New()
	defer reused.Close()
	defer listed.Close()
	rng := rand.New(rand.NewPCG(4, 4))
	for i := range 3000 {
		kind := cases.Kinds()[i%len(cases.Kinds())]
		poly := cases.Wild(rng, kind)
		for _, opts := range []oracle.Options{{}, earsOnly} {
			a, b := reused.Run(poly, opts), oracle.Run(poly, opts)
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("%v %d: reused %+v, fresh %+v", kind, i, a, b)
			}
			lopts := opts
			lopts.TriangleList = true
			l := listed.Run(poly, lopts)
			if !slices.Equal(l.List, a.Triangles) || len(l.Strips) != 0 || l.Status != a.Status {
				t.Fatalf("%v %d: list mode %v, triangles %v", kind, i, l.List, a.Triangles)
			}
			empty := false
			var coords []float64
			var holes []int
			for r, ring := range poly {
				empty = empty || len(ring) == 0
				if r > 0 {
					holes = append(holes, len(coords)/2)
				}
				for _, q := range ring {
					coords = append(coords, q[0], q[1])
				}
			}
			if empty {
				continue // the flat interface has no empty rings
			}
			if f := reused.RunFlat(coords, 2, holes, opts); !reflect.DeepEqual(f, a) {
				t.Fatalf("%v %d: flat %+v, nested %+v", kind, i, f, a)
			}
		}
	}
}
