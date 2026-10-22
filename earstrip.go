// Package earstrip triangulates polygons with holes into triangle strips.
//
// It is a Go port of earstrip (https://github.com/GLMap/earstrip) by Evgen Bodunov: ear clipping that cuts ears
// along zig-zag strips, about 1.7 indices per triangle instead of 3, with a constrained Delaunay triangulation for
// polygons whose rings cross or touch. The ideas are told step by step in
// https://globus.software/blog/2026/10/16/cutting-map-polygons/.
package earstrip

// Index is the type of the vertex indices in the strips; a polygon may have at most as many vertices as it can count.
type Index interface {
	~uint16 | ~uint32 | ~uint64
}

// Status tells how a polygon was covered.
type Status uint8

const (
	// Exact: the triangles cover the polygon exactly.
	Exact Status = iota
	// HolesLeftOut: a hole could not be joined to the boundary (it lies outside or crosses it) and is left out.
	HolesLeftOut
	// Approximate: the boundary ran out of ears (the polygon is not simple); the rest was cut by forced cuts.
	Approximate
	// Refused: as Approximate, with forced cuts switched off: the strips cover only what the ears did.
	Refused
	// Constrained: covered by the constrained triangulation: the rings cross, a hole could not be joined or there
	// are more than 128 rings; what lies inside ring 0 by non-zero winding and outside every hole.
	Constrained
)

func (s Status) String() string {
	switch s {
	case Exact:
		return "Exact"
	case HolesLeftOut:
		return "HolesLeftOut"
	case Approximate:
		return "Approximate"
	case Refused:
		return "Refused"
	case Constrained:
		return "Constrained"
	}
	return "Status(?)"
}

// Rings is a polygon as rings of points: ring 0 is the outer ring, the others are holes. Either orientation is
// fine, a repeated closing point too. Indices in the result count the vertices of all rings in order.
type Rings interface {
	Count() int         // number of rings
	Size(r int) int     // number of points of ring r
	X(r, i int) float64 // x of point i of ring r
	Y(r, i int) float64 // y of point i of ring r
}

// Polygon is a polygon as a slice of rings, each a slice of points; ring 0 is the outer ring.
type Polygon [][][2]float64

func (p Polygon) Count() int         { return len(p) }
func (p Polygon) Size(r int) int     { return len(p[r]) }
func (p Polygon) X(r, i int) float64 { return p[r][i][0] }
func (p Polygon) Y(r, i int) float64 { return p[r][i][1] }

// FlatRings is a polygon as flat coordinates, Dim numbers per vertex (x and y first, Dim 0 means 2), and the vertex
// index where each hole starts, as earcut takes it.
type FlatRings struct {
	Coords     []float64
	Dim        int
	HoleStarts []int
}

func (f FlatRings) dim() int {
	if f.Dim < 2 {
		return 2
	}
	return f.Dim
}

func (f FlatRings) vertices() int { return len(f.Coords) / f.dim() }

func (f FlatRings) start(r int) int {
	if r == 0 {
		return 0
	}
	return f.HoleStarts[r-1]
}

func (f FlatRings) end(r int) int {
	if r < len(f.HoleStarts) {
		return f.HoleStarts[r]
	}
	return f.vertices()
}

func (f FlatRings) Count() int {
	if f.vertices() == 0 {
		return 0
	}
	return len(f.HoleStarts) + 1
}

func (f FlatRings) Size(r int) int {
	if e, s := f.end(r), f.start(r); e > s {
		return e - s
	}
	return 0
}

func (f FlatRings) X(r, i int) float64 { return f.Coords[(f.start(r)+i)*f.dim()] }
func (f FlatRings) Y(r, i int) float64 { return f.Coords[(f.start(r)+i)*f.dim()+1] }

// Run triangulates a polygon given as rings of points, ring 0 the outer ring.
func (t *Triangulator[I]) Run(polygon [][][2]float64) Status { return t.RunRings(Polygon(polygon)) }

// RunFlat triangulates a polygon given as flat coordinates, dim numbers per vertex (x and y first), and the vertex
// index where each hole starts.
func (t *Triangulator[I]) RunFlat(coords []float64, dim int, holeStarts []int) Status {
	return t.RunRings(FlatRings{Coords: coords, Dim: dim, HoleStarts: holeStarts})
}

// RunRings triangulates a polygon given through the Rings interface.
func (t *Triangulator[I]) RunRings(rings Rings) Status { return t.runRings(rings) }

// Strips returns the vertex indices of all strips, one after another. Within a strip the triangles alternate as a
// GPU expects; StripClockwise tells, per strip, whether its first triangle runs clockwise.
func (t *Triangulator[I]) Strips() []I { return t.strips }

// StripEnds returns the end of each strip in Strips.
func (t *Triangulator[I]) StripEnds() []uint32 { return t.stripEnds }

// StripClockwise returns, per strip, 1 when its first triangle runs clockwise.
func (t *Triangulator[I]) StripClockwise() []uint8 { return t.stripCW }

// Triangles appends the same triangles as a list to dst, three indices each, every one counter-clockwise (positive
// area with y up).
func (t *Triangulator[I]) Triangles(dst []I) []I {
	begin := uint32(0)
	for s, end := range t.stripEnds {
		cw := uint32(t.stripCW[s] & 1)
		for k := begin; k+2 < end; k++ {
			flip := ((k - begin) & 1) ^ cw
			dst = append(dst, t.strips[k+flip], t.strips[k+1-flip], t.strips[k+2])
		}
		begin = end
	}
	return dst
}

// Restart appends all strips to dst as one index buffer for a triangle strip draw call, restart between strips, as
// WebGL 2 and WebGPU take it. Every triangle is counter-clockwise: a strip that would start clockwise begins with
// its first index twice.
func (t *Triangulator[I]) Restart(dst []I, restart I) []I {
	begin := uint32(0)
	for s, end := range t.stripEnds {
		if s > 0 {
			dst = append(dst, restart)
		}
		if t.stripCW[s] != 0 {
			dst = append(dst, t.strips[begin])
		}
		dst = append(dst, t.strips[begin:end]...)
		begin = end
	}
	return dst
}

// SetTriangleList makes Run write a triangle list instead of strips: Run then fills TriangleList (three indices per
// triangle, each counter-clockwise, the same as Triangles) and leaves Strips empty.
func (t *Triangulator[I]) SetTriangleList(on bool) { t.listMode = on }

// TriangleList returns the triangle list of the last run in triangle list mode.
func (t *Triangulator[I]) TriangleList() []I { return t.list }

// HolesLeftOut reports whether the last run left out a hole that could not be joined to the boundary.
func (t *Triangulator[I]) HolesLeftOut() bool { return t.droppedHoles }

// SetForcedCuts sets whether, when the boundary runs out of ears, the corners that do the least damage are cut
// instead of giving up (default on). Only used when the constrained triangulation is off or fails.
func (t *Triangulator[I]) SetForcedCuts(on bool) { t.noForcedCuts = !on }

// SetConstrained sets whether polygons the ears cannot finish, those with a hole the ears leave out (when that
// covers the rings' area better) and those with more than 128 rings go to the constrained triangulation (default
// on): Status Constrained.
func (t *Triangulator[I]) SetConstrained(on bool) { t.noConstrained = !on }

// SetExtraPoints lets the constrained triangulation add points where rings cross (default on); off, such polygons
// take the ears' way instead, so every index is an input vertex.
func (t *Triangulator[I]) SetExtraPoints(on bool) { t.noExtra = !on }

// ExtraPoints returns the points added by the last run as x, y pairs: index n + k in the strips is point k, n being
// the number of input vertices of all rings.
func (t *Triangulator[I]) ExtraPoints() []float64 { return t.extra }

// Shrink frees the buffers, which otherwise keep the size of the largest polygon seen, and the last result; the
// settings stay. The next run allocates again.
func (t *Triangulator[I]) Shrink() {
	listMode, noForcedCuts, noConstrained, noExtra := t.listMode, t.noForcedCuts, t.noConstrained, t.noExtra
	*t = Triangulator[I]{}
	t.listMode, t.noForcedCuts, t.noConstrained, t.noExtra = listMode, noForcedCuts, noConstrained, noExtra
}

// Triangulate triangulates a polygon (rings of points, outer ring first) into a triangle list, three indices per
// counter-clockwise triangle.
func Triangulate(polygon [][][2]float64) []uint32 {
	var t Triangulator[uint32]
	t.Run(polygon)
	return t.Triangles(nil)
}

// Earcut takes earcut's arguments, flat coordinates, the vertex index where each hole starts and the numbers per
// vertex (0 means 2), and returns the same kind of result: three indices per counter-clockwise triangle, every
// index an input vertex.
func Earcut(data []float64, holeIndices []int, dim int) []uint32 {
	var t Triangulator[uint32]
	t.SetExtraPoints(false)
	t.RunFlat(data, dim, holeIndices)
	return t.Triangles(nil)
}
