// Package plydata reads the PLY1 polygon files of earstrip's benchmark data (bench-data-1: z13.bin, z9.bin,
// world.bin, gom4old.bin) and computes the result hash that bench/bench.cpp writes into them.
//
// Format, little-endian:
//
//	u32 magic "PLY1", u32 polygons, u32 rings, u32 points
//	u32 polyRingStart[polygons + 1]   first ring of each polygon; the first ring is the outer one
//	u32 ringPointStart[rings + 1]     first point of each ring
//	i16 xy[2 * points]
//	u32 hashEars[polygons]            optional (z13, z9): FNV-1a of the ears' strips, see Hash
//	u32 hashEarcut[polygons]          optional (z13, z9): FNV-1a of earcut.hpp 3.2.4's indices
//
// The files are OpenStreetMap data under the ODbL; they are downloaded by scripts/fetch-data.sh and never committed.
package plydata

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Magic is "PLY1" read as a little-endian u32.
const Magic = 0x31594c50

// File is a loaded PLY1 file.
type File struct {
	PolyRingStart  []uint32 // first ring of each polygon, Len()+1 entries
	RingPointStart []uint32 // first point of each ring, offsets into XY/2
	XY             []int16  // x, y of every point
	HashEars       []uint32 // expected hash of the ears' strips per polygon; nil when the file has none
	HashEarcut     []uint32 // hash of earcut.hpp's indices per polygon; nil when the file has none
}

// Load reads a PLY1 file.
func Load(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return f, nil
}

// Parse decodes the contents of a PLY1 file.
func Parse(data []byte) (*File, error) {
	if len(data) < 16 || binary.LittleEndian.Uint32(data) != Magic {
		return nil, errors.New("not a PLY1 file")
	}
	polys := uint64(binary.LittleEndian.Uint32(data[4:]))
	rings := uint64(binary.LittleEndian.Uint32(data[8:]))
	points := uint64(binary.LittleEndian.Uint32(data[12:]))
	need := 16 + 4*(polys+1) + 4*(rings+1) + 4*points
	if uint64(len(data)) < need {
		return nil, fmt.Errorf("truncated: %d bytes, the header needs %d", len(data), need)
	}
	rest := data[16:]
	f := &File{}
	f.PolyRingStart, rest = u32s(rest, polys+1)
	f.RingPointStart, rest = u32s(rest, rings+1)
	f.XY, rest = i16s(rest, 2*points)
	if uint64(len(rest)) >= 4*polys {
		f.HashEars, rest = u32s(rest, polys)
	}
	if uint64(len(rest)) >= 4*polys {
		f.HashEarcut, _ = u32s(rest, polys)
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	return f, nil
}

func u32s(b []byte, n uint64) ([]uint32, []byte) {
	out := make([]uint32, n)
	for i := range out {
		out[i] = binary.LittleEndian.Uint32(b[4*i:])
	}
	return out, b[4*n:]
}

func i16s(b []byte, n uint64) ([]int16, []byte) {
	out := make([]int16, n)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return out, b[2*n:]
}

func (f *File) validate() error {
	rings, points := uint32(len(f.RingPointStart)-1), uint32(len(f.XY)/2)
	for i := 1; i < len(f.PolyRingStart); i++ {
		if f.PolyRingStart[i] < f.PolyRingStart[i-1] || f.PolyRingStart[i] > rings {
			return fmt.Errorf("polygon %d: bad ring start %d", i-1, f.PolyRingStart[i])
		}
	}
	for i := 1; i < len(f.RingPointStart); i++ {
		if f.RingPointStart[i] < f.RingPointStart[i-1] || f.RingPointStart[i] > points {
			return fmt.Errorf("ring %d: bad point start %d", i-1, f.RingPointStart[i])
		}
	}
	return nil
}

// Len returns the number of polygons.
func (f *File) Len() int { return len(f.PolyRingStart) - 1 }

// Rings returns the total number of rings.
func (f *File) Rings() int { return len(f.RingPointStart) - 1 }

// Points returns the total number of points.
func (f *File) Points() int { return len(f.XY) / 2 }

// Polygon returns polygon i as a view into the file. Pass a pointer to a reused Poly variable to an interface
// parameter (t.RunRings(&p)) to avoid an allocation per polygon.
func (f *File) Polygon(i int) Poly {
	first := f.PolyRingStart[i]
	return Poly{f: f, first: first, count: f.PolyRingStart[i+1] - first}
}

// Poly is one polygon of a File. It implements earstrip.Rings (Count, Size, X, Y) without importing it: indices count
// from the polygon's first point, as bench/bench.cpp does.
type Poly struct {
	f            *File
	first, count uint32 // first ring in the file, number of rings
}

// Count returns the number of rings.
func (p Poly) Count() int { return int(p.count) }

// Size returns the number of points of ring r.
func (p Poly) Size(r int) int {
	s := p.f.RingPointStart[p.first+uint32(r):]
	return int(s[1] - s[0])
}

// X returns x of point i of ring r.
func (p Poly) X(r, i int) float64 {
	return float64(p.f.XY[2*(int(p.f.RingPointStart[p.first+uint32(r)])+i)])
}

// Y returns y of point i of ring r.
func (p Poly) Y(r, i int) float64 {
	return float64(p.f.XY[2*(int(p.f.RingPointStart[p.first+uint32(r)])+i)+1])
}

// RingXY16 returns the x, y pairs of ring r; earstrip reads them in place instead of calling X and Y per point.
func (p Poly) RingXY16(r int) []int16 {
	s := p.f.RingPointStart[p.first+uint32(r):]
	return p.f.XY[2*s[0] : 2*s[1]]
}

// Points returns the number of points of all rings.
func (p Poly) Points() int {
	if p.count == 0 {
		return 0
	}
	return int(p.f.RingPointStart[p.first+p.count] - p.f.RingPointStart[p.first])
}

// Nested returns the polygon as rings of points, ring 0 the outer ring (the layout of earstrip.Polygon).
func (p Poly) Nested() [][][2]float64 {
	out := make([][][2]float64, p.count)
	for r := range out {
		ring := make([][2]float64, p.Size(r))
		for i := range ring {
			ring[i] = [2]float64{p.X(r, i), p.Y(r, i)}
		}
		out[r] = ring
	}
	return out
}

// Flat returns the polygon as flat x, y coordinates and the size of each ring.
func (p Poly) Flat(coords []float64, sizes []uint32) ([]float64, []uint32) {
	coords, sizes = coords[:0], sizes[:0]
	if p.count == 0 {
		return coords, sizes
	}
	begin, end := p.f.RingPointStart[p.first], p.f.RingPointStart[p.first+p.count]
	for _, v := range p.f.XY[2*begin : 2*end] {
		coords = append(coords, float64(v))
	}
	for r := range int(p.count) {
		sizes = append(sizes, uint32(p.Size(r)))
	}
	return coords, sizes
}

// Files are the four files of the bench-data-1 release.
var Files = []string{"z13.bin", "z9.bin", "world.bin", "gom4old.bin"}

// DataDir returns the directory holding the benchmark files: $EARSTRIP_DATA when set, otherwise the nearest data/
// directory, from the working directory upwards, that holds one of Files (a git worktree below the main checkout
// finds the main checkout's data/ this way). Empty when there is none.
func DataDir() string {
	if dir := os.Getenv("EARSTRIP_DATA"); dir != "" {
		return dir
	}
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		cand := filepath.Join(dir, "data")
		for _, name := range Files {
			if _, err := os.Stat(filepath.Join(cand, name)); err == nil {
				return cand
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// Path returns the path of a benchmark file in DataDir and whether it exists.
func Path(name string) (string, bool) {
	dir := DataDir()
	if dir == "" {
		return "", false
	}
	p := filepath.Join(dir, name)
	_, err := os.Stat(p)
	return p, err == nil
}

// Refused is the value of earstrip.Status Refused (and of earstrip::Status::Refused).
const Refused = 3

// Hash is hashOf of bench/bench.cpp: FNV-1a over 32-bit little-endian words, first 1 for a refused polygon, else 2
// when holes were left out or 0, then (not refused) every strip index and every strip end. Indices wider than 32
// bits are truncated as the C++ conversion to uint32_t does.
func Hash[S ~uint8 | ~int, I ~uint16 | ~uint32 | ~uint64](status S, holesLeftOut bool, strips []I, ends []uint32) uint32 {
	ok := int(status) != Refused
	var first uint32 = 1
	if ok {
		first = 0
		if holesLeftOut {
			first = 2
		}
	}
	h := fnvWord(2166136261, first)
	if ok {
		for _, i := range strips {
			h = fnvWord(h, uint32(i))
		}
		for _, e := range ends {
			h = fnvWord(h, e)
		}
	}
	return h
}

func fnvWord(h, v uint32) uint32 {
	for i := range 4 {
		h ^= (v >> (8 * i)) & 0xff
		h *= 16777619
	}
	return h
}
