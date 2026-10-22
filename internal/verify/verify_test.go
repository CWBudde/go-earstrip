package verify

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
)

func TestInt128(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	vals := []int64{0, 1, -1, 2, math.MaxInt64, math.MinInt64, math.MaxInt32, math.MinInt32}
	for range 200 {
		vals = append(vals, int64(rng.Uint64()), int64(rng.Uint32())-1<<31)
	}
	for _, a := range vals {
		for _, b := range vals[:40] {
			p := Mul64(a, b)
			want := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
			if p.Big().Cmp(want) != 0 {
				t.Fatalf("%d * %d = %v, want %v", a, b, p, want)
			}
			s := p.Add(I128(b))
			if s.Big().Cmp(new(big.Int).Add(want, big.NewInt(b))) != 0 {
				t.Fatalf("%v + %d = %v", p, b, s)
			}
			if p.Neg().Big().Cmp(new(big.Int).Neg(want)) != 0 || p.Sign() != want.Sign() || p.Abs().Big().Cmp(new(big.Int).Abs(want)) != 0 {
				t.Fatalf("neg/sign/abs of %v", p)
			}
		}
	}
}

func TestExact(t *testing.T) {
	sq := IntRings{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}, {{3, 3}, {7, 3}, {7, 7}, {3, 7}}}
	strips := []uint32{1, 5, 0, 4, 3, 7, 2, 6, 1, 5} // the C++ result
	if n, err := Exact(sq, strips, []uint32{10}, false); err != nil || n != 8 {
		t.Fatalf("%d triangles, %v", n, err)
	}
	if _, err := Exact(sq, strips[:8], []uint32{8}, false); err == nil {
		t.Error("missing area not found")
	}
	if _, err := Exact(sq, []uint32{0, 1, 0}, []uint32{3}, true); err == nil {
		t.Error("flat triangle not found")
	}
	if _, err := Exact(sq, []uint32{0, 1, 9}, []uint32{3}, true); err == nil {
		t.Error("index out of range not found")
	}
	if _, err := Exact(sq, []uint32{0, 1, 2, 0}, []uint32{4}, true); err == nil {
		t.Error("orientation flip not found")
	}
	if got := TrianglesFromStrips(strips, []uint32{10}, []uint8{0}); len(got) != 24 || got[0] != 1 || got[3] != 0 {
		t.Errorf("TrianglesFromStrips %v", got)
	}
	tris := TrianglesFromStrips([]uint32{0, 1, 2, 3}, []uint32{4}, []uint8{1})
	dec := DecodeRestart([]uint32{0, 0, 1, 2, 3}, math.MaxUint32)
	if len(tris) != 6 || len(dec) != 6 || tris[0] != dec[0] || tris[1] != dec[1] || tris[2] != dec[2] {
		t.Errorf("clockwise strip: %v, restart decoded %v", tris, dec)
	}
}
