package verify

import (
	"errors"
	"fmt"
	"math"
)

// Index is the type of strip indices (earstrip.Index without importing the package under test).
type Index interface {
	~uint16 | ~uint32 | ~uint64
}

// IntRings is a polygon with integer coordinates, ring 0 the outer ring; test/test.cpp's Rings.
type IntRings = [][][2]int64

// Float converts integer rings to the rings Triangulator.Run takes.
func Float(rings IntRings) [][][2]float64 {
	out := make([][][2]float64, len(rings))
	for r, ring := range rings {
		out[r] = make([][2]float64, len(ring))
		for i, p := range ring {
			out[r][i] = [2]float64{float64(p[0]), float64(p[1])}
		}
	}
	return out
}

// Points returns the points of all rings in order, as the strip indices count them.
func Points[T any](rings [][][2]T) [][2]T {
	var all [][2]T
	for _, r := range rings {
		all = append(all, r...)
	}
	return all
}

// RingArea2 is the doubled signed area of a ring (ringArea2 of test/test.cpp).
func RingArea2(r [][2]int64) Int128 {
	var a Int128
	for i, p := range r {
		q := r[(i+1)%len(r)]
		a = a.Add(Mul64(p[0], q[1])).Sub(Mul64(q[0], p[1]))
	}
	return a
}

// ExpectedArea2 is the doubled area of the outer ring minus the holes, each taken by absolute value.
func ExpectedArea2(rings IntRings) Int128 {
	var expected Int128
	for r, ring := range rings {
		a := RingArea2(ring).Abs()
		if r == 0 {
			expected = expected.Add(a)
		} else {
			expected = expected.Sub(a)
		}
	}
	return expected
}

// Exact is the result check of check() in test/test.cpp for an accepted result: every strip at least a triangle,
// indices in range, no flat triangle, the orientation never flips within a strip, and, unless holes were dropped,
// the triangles' doubled area equals the outer ring's minus the holes' exactly. It returns the number of
// triangles and the first problem found.
func Exact[I Index](rings IntRings, strips []I, ends []uint32, expectDropped bool) (int, error) {
	all := Points(rings)
	var sum Int128
	triangles := 0
	var why error
	fail := func(err error) {
		if why == nil {
			why = err
		}
	}
	start := uint32(0)
	for _, end := range ends {
		sign := 0
		if end < start+3 {
			fail(errors.New("strip shorter than a triangle"))
		}
		if int(end) > len(strips) {
			return triangles, fmt.Errorf("strip end %d past %d indices", end, len(strips))
		}
		for k := start; k+2 < end; k++ {
			triangles++
			ia, ib, ic := uint64(strips[k]), uint64(strips[k+1]), uint64(strips[k+2])
			if ia >= uint64(len(all)) || ib >= uint64(len(all)) || ic >= uint64(len(all)) {
				fail(fmt.Errorf("index out of range in triangle %d", k))
				continue
			}
			a, b, c := all[ia], all[ib], all[ic]
			cr := Mul64(b[0]-a[0], c[1]-a[1]).Sub(Mul64(b[1]-a[1], c[0]-a[0]))
			if (k-start)&1 != 0 {
				cr = cr.Neg()
			}
			sg := cr.Sign()
			if sg == 0 {
				fail(fmt.Errorf("flat triangle %d: %d %d %d", k, ia, ib, ic))
			}
			if sg != 0 && sign != 0 && sg != sign {
				fail(fmt.Errorf("strip orientation flips at %d", k))
			}
			if sg != 0 {
				sign = sg
			}
			sum = sum.Add(cr.Abs())
		}
		start = end
	}
	if why != nil {
		return triangles, why
	}
	// with a dropped hole the triangles cover the outer ring minus the other holes, which this check does not know
	if expected := ExpectedArea2(rings); !expectDropped && sum != expected {
		return triangles, fmt.Errorf("area %v expected %v", sum, expected)
	}
	return triangles, nil
}

// Cover is cover() of test/test.cpp: the doubled area the strips cover over the input points and the added ones;
// an error on a bad index, a flat triangle or a triangle against its strip's winding (StripClockwise).
func Cover[I Index](strips []I, ends []uint32, cw []uint8, points [][2]float64, extra []float64) (float64, error) {
	all := append([][2]float64(nil), points...)
	for k := 0; k+1 < len(extra); k += 2 {
		all = append(all, [2]float64{extra[k], extra[k+1]})
	}
	if len(cw) != len(ends) {
		return 0, fmt.Errorf("%d strip ends, %d clockwise flags", len(ends), len(cw))
	}
	area := 0.0
	start := uint32(0)
	for s, end := range ends {
		if end < start+3 {
			return 0, errors.New("strip shorter than a triangle")
		}
		if int(end) > len(strips) {
			return 0, fmt.Errorf("strip end %d past %d indices", end, len(strips))
		}
		for k := start; k+2 < end; k++ {
			ia, ib, ic := uint64(strips[k]), uint64(strips[k+1]), uint64(strips[k+2])
			if ia >= uint64(len(all)) || ib >= uint64(len(all)) || ic >= uint64(len(all)) {
				return 0, errors.New("index out of range")
			}
			a, b, c := all[ia], all[ib], all[ic]
			cr := float64((b[0]-a[0])*(c[1]-a[1])) - float64((b[1]-a[1])*(c[0]-a[0]))
			if ((k-start)&1 != 0) != (cw[s] != 0) {
				cr = -cr
			}
			if cr <= 0 {
				if cr == 0 {
					return 0, fmt.Errorf("flat triangle %d", k)
				}
				return 0, fmt.Errorf("triangle %d against the strip's winding", k)
			}
			area += cr
		}
		start = end
	}
	return area, nil
}

// Structure checks what holds for every result: strip ends ascending and ending at len(strips), one clockwise flag
// per strip, every strip at least a triangle, and every index below n plus the number of added points.
func Structure[I Index](n int, strips []I, ends []uint32, cw []uint8, extra []float64) error {
	if len(extra)%2 != 0 {
		return fmt.Errorf("%d extra coordinates, not pairs", len(extra))
	}
	if len(cw) != len(ends) {
		return fmt.Errorf("%d strip ends, %d clockwise flags", len(ends), len(cw))
	}
	prev := uint32(0)
	for s, e := range ends {
		if e < prev+3 {
			return fmt.Errorf("strip %d: end %d after %d, shorter than a triangle", s, e, prev)
		}
		prev = e
	}
	if int(prev) != len(strips) {
		return fmt.Errorf("last strip ends at %d, %d indices", prev, len(strips))
	}
	limit := uint64(n + len(extra)/2)
	for k, i := range strips {
		if uint64(i) >= limit {
			return fmt.Errorf("index %d at %d out of range (%d points, %d added)", i, k, n, len(extra)/2)
		}
	}
	for k, v := range extra {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("extra coordinate %d is %v", k, v)
		}
	}
	return nil
}

// TrianglesFromStrips is Triangulator.Triangles written out independently: three indices per triangle, the first
// of each strip's pairs swapped as the clockwise flag and the position say.
func TrianglesFromStrips[I Index](strips []I, ends []uint32, cw []uint8) []I {
	var out []I
	begin := uint32(0)
	for s, end := range ends {
		for k := begin; k+2 < end; k++ {
			a, b := strips[k], strips[k+1]
			if ((k-begin)&1 != 0) != (cw[s] != 0) {
				a, b = b, a
			}
			out = append(out, a, b, strips[k+2])
		}
		begin = end
	}
	return out
}

// DecodeRestart splits a restart index buffer into strips and returns their triangles, each strip starting
// counter-clockwise (Restart duplicates the first index of a strip that would start clockwise).
func DecodeRestart[I Index](buf []I, restart I) []I {
	var out []I
	start := 0
	for i := 0; i <= len(buf); i++ {
		if i < len(buf) && buf[i] != restart {
			continue
		}
		strip := buf[start:i]
		for k := 0; k+2 < len(strip); k++ {
			a, b := strip[k], strip[k+1]
			if k&1 != 0 {
				a, b = b, a
			}
			if a == b || b == strip[k+2] || a == strip[k+2] {
				continue // the degenerate triangle of a duplicated first index
			}
			out = append(out, a, b, strip[k+2])
		}
		start = i + 1
	}
	return out
}
