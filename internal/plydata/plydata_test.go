package plydata

import (
	"encoding/binary"
	"hash/fnv"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// build writes a PLY1 file: polygons as rings of points, optionally with both hash arrays.
func build(polys [][][][2]int16, hashes bool) []byte {
	var ringStart, pointStart []uint32
	var xy []int16
	rings := uint32(0)
	for _, p := range polys {
		ringStart = append(ringStart, rings)
		for _, r := range p {
			pointStart = append(pointStart, uint32(len(xy)/2))
			for _, q := range r {
				xy = append(xy, q[0], q[1])
			}
			rings++
		}
	}
	ringStart = append(ringStart, rings)
	pointStart = append(pointStart, uint32(len(xy)/2))
	b := binary.LittleEndian.AppendUint32(nil, Magic)
	for _, v := range []uint32{uint32(len(polys)), rings, uint32(len(xy) / 2)} {
		b = binary.LittleEndian.AppendUint32(b, v)
	}
	for _, v := range append(ringStart, pointStart...) {
		b = binary.LittleEndian.AppendUint32(b, v)
	}
	for _, v := range xy {
		b = binary.LittleEndian.AppendUint16(b, uint16(v))
	}
	if hashes {
		for i := range 2 * len(polys) {
			b = binary.LittleEndian.AppendUint32(b, uint32(1000+i))
		}
	}
	return b
}

func TestParse(t *testing.T) {
	polys := [][][][2]int16{
		{{{0, 0}, {10, 0}, {10, 10}, {0, 10}}, {{3, 3}, {7, 3}, {7, 7}}},
		{{{-5, -32768}, {32767, 1}, {2, 2}}},
		{},
	}
	for _, hashes := range []bool{false, true} {
		f, err := Parse(build(polys, hashes))
		if err != nil {
			t.Fatal(err)
		}
		if f.Len() != 3 || f.Rings() != 3 || f.Points() != 10 {
			t.Fatalf("%d polygons, %d rings, %d points", f.Len(), f.Rings(), f.Points())
		}
		if hashes != (f.HashEars != nil) || hashes != (f.HashEarcut != nil) {
			t.Fatalf("hashes %v: ears %v earcut %v", hashes, f.HashEars, f.HashEarcut)
		}
		if hashes && (f.HashEars[2] != 1002 || f.HashEarcut[0] != 1003) {
			t.Errorf("hashes %v %v", f.HashEars, f.HashEarcut)
		}
		for i, want := range polys {
			p := f.Polygon(i)
			if p.Count() != len(want) {
				t.Fatalf("polygon %d: %d rings, want %d", i, p.Count(), len(want))
			}
			points := 0
			for r, ring := range want {
				points += len(ring)
				if p.Size(r) != len(ring) {
					t.Fatalf("polygon %d ring %d: %d points, want %d", i, r, p.Size(r), len(ring))
				}
				for k, q := range ring {
					if p.X(r, k) != float64(q[0]) || p.Y(r, k) != float64(q[1]) {
						t.Errorf("polygon %d ring %d point %d: (%v, %v), want %v", i, r, k, p.X(r, k), p.Y(r, k), q)
					}
				}
			}
			if p.Points() != points {
				t.Errorf("polygon %d: Points %d, want %d", i, p.Points(), points)
			}
			coords, sizes := p.Flat(nil, nil)
			if len(coords) != 2*points || len(sizes) != len(want) {
				t.Errorf("polygon %d: Flat %v %v", i, coords, sizes)
			}
			if n := p.Nested(); len(n) != len(want) {
				t.Errorf("polygon %d: Nested %v", i, n)
			}
		}
	}
}

func TestParseErrors(t *testing.T) {
	good := build([][][][2]int16{{{{0, 0}, {1, 0}, {0, 1}}}}, false)
	bad := slices.Clone(good)
	binary.LittleEndian.PutUint32(bad[16+8+4:], 99) // a ring point start past the points
	for name, data := range map[string][]byte{
		"empty":     nil,
		"magic":     append([]byte("PLY2"), good[4:]...),
		"truncated": good[:len(good)-1],
		"bad start": bad,
	} {
		if _, err := Parse(data); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

// TestHash checks Hash against hash/fnv's FNV-1a over the little-endian bytes of the words hashOf feeds it.
func TestHash(t *testing.T) {
	ref := func(words ...uint32) uint32 {
		h := fnv.New32a()
		for _, w := range words {
			h.Write(binary.LittleEndian.AppendUint32(nil, w))
		}
		return h.Sum32()
	}
	strips := []uint32{1, 5, 0, 4, 3, 7, 2, 6, 1, 5}
	ends := []uint32{10}
	words := append(append([]uint32{0}, strips...), ends...)
	if got, want := Hash(uint8(0), false, strips, ends), ref(words...); got != want {
		t.Errorf("exact: %#x, want %#x", got, want)
	}
	words[0] = 2
	if got, want := Hash(1, true, strips, ends), ref(words...); got != want {
		t.Errorf("holes left out: %#x, want %#x", got, want)
	}
	// refused: only the 1, whatever the strips
	if got, want := Hash(Refused, true, strips, ends), ref(1); got != want {
		t.Errorf("refused: %#x, want %#x", got, want)
	}
	s16 := []uint16{1, 5, 0, 4, 3, 7, 2, 6, 1, 5}
	if Hash(uint8(0), false, s16, ends) != Hash(uint8(0), false, strips, ends) {
		t.Error("uint16 and uint32 indices hash differently")
	}
}

func TestDataDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("EARSTRIP_DATA", dir)
	if DataDir() != dir {
		t.Fatalf("DataDir %q, want %q", DataDir(), dir)
	}
	if _, ok := Path("z9.bin"); ok {
		t.Fatal("z9.bin found in an empty directory")
	}
	if err := os.WriteFile(filepath.Join(dir, "z9.bin"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if p, ok := Path("z9.bin"); !ok || p != filepath.Join(dir, "z9.bin") {
		t.Fatalf("Path %q %v", p, ok)
	}
	// without the variable: the nearest data/ upwards with a benchmark file
	t.Setenv("EARSTRIP_DATA", "")
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(filepath.Join(root, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "a", "data"), 0o755); err != nil { // no benchmark file: skipped
		t.Fatal(err)
	}
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data", "world.bin"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(deep)
	want, _ := filepath.EvalSymlinks(filepath.Join(root, "data"))
	got, _ := filepath.EvalSymlinks(DataDir())
	if got != want {
		t.Errorf("DataDir %q, want %q", got, want)
	}
}
