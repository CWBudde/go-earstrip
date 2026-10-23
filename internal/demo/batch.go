package demo

import (
	"fmt"
	"math"
	"time"

	"github.com/cwbudde/go-earstrip"
)

// Batch is many polygons in one buffer, as a map tile holds them: the rings of all polygons one after another.
type Batch struct {
	// Coords are x, y per vertex of all rings of all polygons, in order.
	Coords []float64
	// RingStarts is the vertex where each ring starts, ascending, the first 0. A ring ends where the next starts.
	RingStarts []int
	// PolygonStarts is the ring where each polygon starts, ascending, the first 0: its first ring is the outer ring,
	// the others are holes.
	PolygonStarts []int
}

// BatchResult is what RunMany gives: all polygons as one mesh, for one draw call. The slices alias the Runner's
// buffers until its next run.
type BatchResult struct {
	// Vertices are x, y of the input vertices in order, then the extra points of each polygon, polygon by polygon.
	Vertices []float32
	// Restart holds the strips of all polygons, RestartIndex between strips and between polygons, indices into
	// Vertices: Triangulator.Restart of each polygon, offset to where its vertices are.
	Restart []uint32
	// List holds the same triangles as a list, three indices each into Vertices: Triangulator.Triangles of each
	// polygon, offset the same way.
	List []uint32
	// StripRestartEnds is, per strip, where it ends in Restart; StripListEnds where its triangles end in List.
	StripRestartEnds []uint32
	StripListEnds    []uint32
	// PolygonStrips is the first strip of each polygon and, last, the number of strips.
	PolygonStrips []uint32
	// PolygonStatus is the status of each polygon.
	PolygonStatus []earstrip.Status
	// Statuses counts the polygons per status.
	Statuses [earstrip.Constrained + 1]int
	// ExtraPoints is the number of points the constrained triangulation added, at the end of Vertices.
	ExtraPoints int
	// Elapsed is how long the triangulation of all polygons took, with the copying into the shared buffers.
	Elapsed time.Duration
}

// checkBatch returns an error for a batch whose starts are out of order or out of range, or that RunFlat would not
// take.
func checkBatch(b Batch) error {
	if err := check(Flat{Coords: b.Coords}); err != nil {
		return err
	}
	n := len(b.Coords) / 2
	if n > math.MaxUint32/2 {
		return fmt.Errorf("%d vertices: too many for 32-bit indices", n)
	}
	if n > 0 && (len(b.RingStarts) == 0 || b.RingStarts[0] != 0) {
		return fmt.Errorf("ring starts %v: want the first 0", b.RingStarts)
	}
	for i, s := range b.RingStarts {
		if s > n || (i > 0 && s < b.RingStarts[i-1]) {
			return fmt.Errorf("ring start %d is %d: want them ascending, at most %d", i, s, n)
		}
	}
	if len(b.RingStarts) > 0 && (len(b.PolygonStarts) == 0 || b.PolygonStarts[0] != 0) {
		return fmt.Errorf("polygon starts %v: want the first 0", b.PolygonStarts)
	}
	for i, s := range b.PolygonStarts {
		if s > len(b.RingStarts) || (i > 0 && s < b.PolygonStarts[i-1]) {
			return fmt.Errorf("polygon start %d is %d: want them ascending, at most %d", i, s, len(b.RingStarts))
		}
	}
	return nil
}

// batchBuffers are the Runner's buffers for RunMany, reused from run to run.
type batchBuffers struct {
	holes              []int
	restart, triangles []uint32
	res                BatchResult
}

// RunMany triangulates every polygon of the batch on its own, as Run would, and joins the results into one mesh.
// Options.TriangleList is ignored: the result holds both the strips and the triangle list.
func (r *Runner) RunMany(b Batch, o Options) (BatchResult, error) {
	if err := checkBatch(b); err != nil {
		return BatchResult{}, err
	}
	o.TriangleList = false
	r.set(o)

	m := &r.batch
	res := &m.res
	n := len(b.Coords) / 2
	res.Vertices = res.Vertices[:0]
	for _, c := range b.Coords {
		res.Vertices = append(res.Vertices, float32(c))
	}
	res.Restart = res.Restart[:0]
	res.List = res.List[:0]
	res.StripRestartEnds = res.StripRestartEnds[:0]
	res.StripListEnds = res.StripListEnds[:0]
	res.PolygonStrips = res.PolygonStrips[:0]
	res.PolygonStatus = res.PolygonStatus[:0]
	res.Statuses = [earstrip.Constrained + 1]int{}
	res.ExtraPoints = 0

	start := time.Now()
	for p, ringFirst := range b.PolygonStarts {
		ringEnd := len(b.RingStarts)
		if p+1 < len(b.PolygonStarts) {
			ringEnd = b.PolygonStarts[p+1]
		}
		first, end := n, n
		if ringFirst < ringEnd {
			first = b.RingStarts[ringFirst]
		}
		if ringEnd < len(b.RingStarts) {
			end = b.RingStarts[ringEnd]
		}
		m.holes = m.holes[:0]
		for _, s := range b.RingStarts[min(ringFirst+1, ringEnd):ringEnd] {
			m.holes = append(m.holes, s-first)
		}
		status := r.t.RunFlat(b.Coords[2*first:2*end], 2, m.holes)
		res.PolygonStrips = append(res.PolygonStrips, uint32(len(res.StripRestartEnds)))
		res.PolygonStatus = append(res.PolygonStatus, status)
		res.Statuses[status]++

		// Index k of the polygon is vertex first + k below count, else extra point k - count, which goes after
		// the vertices already in the buffer.
		count := uint32(end - first)
		base := uint32(first)
		extraBase := uint32(len(res.Vertices) / 2)
		extra := r.t.ExtraPoints()
		for _, c := range extra {
			res.Vertices = append(res.Vertices, float32(c))
		}
		res.ExtraPoints += len(extra) / 2
		at := func(k uint32) uint32 {
			if k < count {
				return base + k
			}
			return extraBase + k - count
		}

		m.restart = r.t.Restart(m.restart[:0], RestartIndex)
		if len(m.restart) > 0 && len(res.Restart) > 0 {
			res.Restart = append(res.Restart, RestartIndex)
		}
		for _, k := range m.restart {
			if k == RestartIndex {
				res.StripRestartEnds = append(res.StripRestartEnds, uint32(len(res.Restart)))
				res.Restart = append(res.Restart, RestartIndex)
				continue
			}
			res.Restart = append(res.Restart, at(k))
		}
		if len(m.restart) > 0 {
			res.StripRestartEnds = append(res.StripRestartEnds, uint32(len(res.Restart)))
		}

		m.triangles = r.t.Triangles(m.triangles[:0])
		for _, k := range m.triangles {
			res.List = append(res.List, at(k))
		}
		begin := uint32(0)
		listEnd := uint32(len(res.List) - len(m.triangles))
		for _, e := range r.t.StripEnds() {
			// A strip of k indices has k - 2 triangles; Triangles skips one shorter than 3.
			if e-begin > 2 {
				listEnd += 3 * (e - begin - 2)
			}
			res.StripListEnds = append(res.StripListEnds, listEnd)
			begin = e
		}
	}
	res.Elapsed = time.Since(start)
	res.PolygonStrips = append(res.PolygonStrips, uint32(len(res.StripRestartEnds)))
	return *res, nil
}
