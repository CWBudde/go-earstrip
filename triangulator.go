package earstrip

import "unsafe"

// Triangulator triangulates polygons into triangle strips. An instance is meant to be reused: its buffers only
// grow. The zero value is ready to use, with forced cuts, the constrained triangulation and extra points on.
type Triangulator[I Index] struct {
	nodes        []node[I]
	strips       []I
	stripEnds    []uint32
	stripCW      []uint8 // first triangle of the strip runs clockwise
	list         []I     // triangle list mode output
	listMode     bool
	blockers     []uint32
	holes        []hole
	merged       []uint8  // ring is part of the boundary
	order        []uint32 // holes by decreasing rightmost x
	queue        []uint32 // merged node ranges to look for touching holes
	samePos      []uint32 // cyclic lists of nodes at one position
	cycle        []uint32 // nodes of a boundary in walking order
	loops        []uint32 // start node and length of loops still to clip
	hash         []uint32
	items        []item   // blockers by bucket; a single bucket stays compact, otherwise gone ones get x = inf
	bucketStart  []uint32 // first item of each bucket
	slot         []uint32 // item of each indexed node
	bucketOrigin float64
	bucketScale  float64
	buckets      uint32 // 0 is read as 1, see ears.go
	axisY        bool
	live         uint32 // blockers still reflex
	built        uint32 // items in the index when it was built
	blockerCount uint32 // blockers found by the last classification
	droppedHoles bool
	outerCount   uint32
	forced       bool // a forced cut was made or the boundary untangled
	careful      bool // second run of a polygon the ears could not finish
	// settings, stored negated so that the zero value has them on
	noForcedCuts  bool
	noConstrained bool
	noExtra       bool
	cdt           constrained[I]
	extra         []float64 // points the constrained triangulation added
	// the input of Run and RunFlat during the run, passed on by pointer so that it is not allocated
	runPolygon Polygon
	runFlat    FlatRings
}

const (
	// Joining a hole to the boundary scans the boundary: with more rings the constrained triangulation is faster
	maxEarRings = 128
	// Up to this many blockers an ear test scans them all; more are sorted into buckets of about blockersPerBucket.
	smallBlockers     = 32
	blockersPerBucket = 2.0
	// Largest boundary the second run splits at its crossings; larger ones go straight to forced cuts.
	splitLimit = 4096
)

type node[I Index] struct {
	x, y       float64
	prev, next uint32
	index      I
	reflex     uint8 // corner is not strictly convex
	// C++ union: "shared" while joining rings (another ring has a vertex at this position), then "indexed" while
	// clipping (listed in the blocker index).
	flag uint8
	ring uint16 // 0 outer, holes from 1
}

type item struct {
	x, y float64
	node uint32
}

type hole struct {
	x, y         float64
	node         uint32 // rightmost vertex
	first, count uint32
	ring         uint16
}

// cross returns twice the signed area of a, b, c. The explicit float64 conversions keep the compiler from fusing
// the products into FMA instructions, so the result is the same as in C++ on every platform.
func cross(ax, ay, bx, by, cx, cy float64) float64 {
	return float64((bx-ax)*(cy-ay)) - float64((by-ay)*(cx-ax))
}

// crossN is cross of three nodes.
func crossN[I Index](a, b, c *node[I]) float64 { return cross(a.x, a.y, b.x, b.y, c.x, c.y) }

func samePos[I Index](a, b *node[I]) bool { return a.x == b.x && a.y == b.y }

// minf and maxf are std::min and std::max: the first argument unless the second compares smaller (larger), which
// differs from the builtins for NaN and signed zeros.
func minf(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

func maxf(a, b float64) float64 {
	if a < b {
		return b
	}
	return a
}

func b2u(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// grow resizes s to n elements when it is shorter (std::vector::resize when growing): the old elements are kept,
// the new ones are zero.
func grow[T any](s []T, n int) []T {
	if len(s) < n {
		s = append(s, make([]T, n-len(s))...)
	}
	return s
}

// assign sets s to n copies of v (std::vector::assign), reusing its storage.
func assign[T any](s []T, n int, v T) []T {
	if cap(s) < n {
		s = make([]T, n)
	} else {
		s = s[:n]
	}
	for i := range s {
		s[i] = v
	}
	return s
}

// The ears are only right on a simple boundary. When they run out, the polygon is cut again from the start with
// its crossings untangled first, so that no ear cut before the stall spans the wrong area.
func (t *Triangulator[I]) runRings(rings Rings) Status {
	s := t.runStrips(rings)
	if t.listMode {
		t.list = t.Triangles(t.list[:0])
		t.strips = t.strips[:0]
		t.stripEnds = t.stripEnds[:0]
		t.stripCW = t.stripCW[:0]
	}
	return s
}

func (t *Triangulator[I]) runStrips(rings Rings) Status {
	t.extra = t.extra[:0]
	t.careful = false
	// With many rings the constrained triangulation goes first; when it fails, it would fail again after the ears
	manyRings := !t.noConstrained && rings.Count() > maxEarRings
	if manyRings && t.cover(rings, false) {
		return Constrained
	}
	s := t.runOnce(rings)
	if !t.noConstrained && !manyRings && (s == Refused || s == HolesLeftOut) && t.cover(rings, s == HolesLeftOut) {
		return Constrained
	}
	if s != Refused || t.noForcedCuts {
		return s
	}
	t.careful = true
	t.runOnce(rings)
	return Approximate
}

// Replaces the result by the constrained triangulation; false keeps what the ears did: when it hits a limit, adds
// points that are not allowed or, with compare, covers an area no closer to the rings' than the ears.
func (t *Triangulator[I]) cover(rings Rings, compare bool) bool {
	if !t.cdt.load(rings) || !t.cdt.build() || (t.noExtra && t.cdt.extraCount() > 0) {
		return false
	}
	if compare {
		area := t.cdt.inputRingsArea()
		if t.cdt.closer(t.strips, t.stripEnds, area) {
			return false
		}
	}
	t.strips, t.cdt.strips = t.cdt.strips, t.strips
	t.stripEnds, t.cdt.stripEnds = t.cdt.stripEnds, t.stripEnds
	t.stripCW, t.cdt.stripCW = t.cdt.stripCW, t.stripCW
	t.extra = t.cdt.extraPoints(t.extra)
	t.droppedHoles = false
	return true
}

func (t *Triangulator[I]) runOnce(rings Rings) Status {
	t.strips = t.strips[:0]
	t.stripEnds = t.stripEnds[:0]
	t.stripCW = t.stripCW[:0]
	t.list = t.list[:0]
	t.droppedHoles = false
	t.forced = false
	ringCount := uint32(rings.Count())
	if ringCount == 0 {
		return Exact
	}
	total := uint32(0)
	for r := uint32(0); r < ringCount; r++ {
		total += uint32(rings.Size(int(r)))
	}
	if uint32(len(t.nodes)) < total+2*ringCount {
		capacity := int(total + 2*ringCount)
		t.nodes = grow(t.nodes, capacity)
		t.samePos = grow(t.samePos, capacity)
		t.slot = grow(t.slot, capacity)
		t.blockers = grow(t.blockers, capacity)
		t.items = grow(t.items, capacity)
	}

	count, area, _ := t.loadRing(rings, 0, 0, 0)
	if count < 3 {
		return Exact
	}
	outerCount := count
	t.outerCount = count
	t.holes = t.holes[:0]
	if ringCount == 1 {
		if count == 3 {
			// a triangle needs no tests, a flat one has nothing to cover
			if area != 0 {
				t.emitConvex(3, area < 0)
			}
			return Exact
		}
		if count == 4 && t.convexQuad(area) {
			t.emitConvex(4, area < 0)
			return Exact
		}
		if t.classifyRing(count, area < 0) {
			t.emitConvex(count, area < 0)
			return Exact
		}
		if t.careful {
			return t.finish(t.clipLoops(0, count))
		}
		return t.finish(t.clip(0, count))
	}

	t.linkRing(0, count, area < 0)
	t.setRing(0, count, 0)
	used := count
	base := uint32(rings.Size(0))
	t.holes = t.holes[:0]
	for r := uint32(1); r < ringCount; r++ {
		holeCount, holeArea, rightmost := t.loadRing(rings, r, used, base)
		base += uint32(rings.Size(int(r)))
		if holeCount < 3 || holeArea == 0 {
			continue
		}
		t.linkRing(used, holeCount, holeArea > 0)
		id := uint16(len(t.holes) + 1)
		t.setRing(used, holeCount, id)
		t.holes = append(t.holes, hole{x: t.nodes[rightmost].x, y: t.nodes[rightmost].y, node: rightmost, first: used, count: holeCount, ring: id})
		used += holeCount
	}
	t.findTouches(used)
	t.merged = assign(t.merged, len(t.holes)+1, 0)
	t.merged[0] = 1
	t.order = t.order[:0]
	for h := uint32(0); h < uint32(len(t.holes)); h++ {
		t.order = append(t.order, h)
	}
	t.sortOrder()
	// Rings touching the boundary join at the shared vertex; the rest get a bridge, rightmost first.
	t.joinTouchingRings(0, outerCount)
	for _, h := range t.order {
		hole := &t.holes[h]
		if t.merged[hole.ring] != 0 {
			continue
		}
		if !t.bridge(hole.node, &used) {
			t.droppedHoles = true
			continue
		}
		t.merged[hole.ring] = 1
		t.joinTouchingRings(hole.first, hole.count)
	}
	boundary := t.classifyCycle(0)
	if t.careful {
		return t.finish(t.clipLoops(0, boundary))
	}
	return t.finish(t.clip(0, boundary))
}

func (t *Triangulator[I]) finish(clipped bool) Status {
	if !clipped {
		return Refused
	}
	if t.forced {
		return Approximate
	}
	if t.droppedHoles {
		return HolesLeftOut
	}
	return Exact
}

// Copies ring r into nodes from first on, skipping repeated points; returns the kept count, twice the signed area
// and the rightmost node.
func (t *Triangulator[I]) loadRing(rings Rings, r, first, indexBase uint32) (count uint32, area float64, rightmost uint32) {
	// the package's own ring types skip the interface for every point; the result is the same
	switch p := rings.(type) {
	case *Polygon:
		return loadStrided(t.nodes, flatten((*p)[r]), 2, first, indexBase)
	case Polygon:
		return loadStrided(t.nodes, flatten(p[r]), 2, first, indexBase)
	case int16Rings:
		return loadStrided(t.nodes, p.RingXY16(int(r)), 2, first, indexBase)
	case *FlatRings:
		return loadStrided(t.nodes, flatRing(p, int(r)), p.dim(), first, indexBase)
	case FlatRings:
		return loadStrided(t.nodes, flatRing(&p, int(r)), p.dim(), first, indexBase)
	}
	n := t.nodes[first:]
	size := uint32(rings.Size(int(r)))
	if size < 3 {
		return 0, 0, 0
	}
	ri := int(r)
	lastX, lastY := rings.X(ri, 0), rings.Y(ri, 0)
	sum, rightX := 0.0, lastX
	n[0].x = lastX
	n[0].y = lastY
	n[0].index = I(indexBase)
	count = 1
	right := uint32(0)
	for i := uint32(1); i < size; i++ {
		x, y := rings.X(ri, int(i)), rings.Y(ri, int(i))
		node := &n[count]
		node.x = x
		node.y = y
		node.index = I(indexBase + i)
		// a repeated point adds nothing to the area and is overwritten by the next one
		sum += float64((lastX - x) * (lastY + y))
		if x > rightX {
			right = count
			rightX = x
		}
		if x != lastX || y != lastY {
			count++
		}
		lastX = x
		lastY = y
	}
	sum += float64((lastX - n[0].x) * (lastY + n[0].y))
	for count > 1 && samePos(&n[count-1], &n[0]) {
		count--
	}
	area = sum
	if right < count {
		rightmost = first + right
	} else {
		rightmost = first
	}
	return count, area, rightmost
}

// int16Rings is implemented by rings stored as int16 x, y pairs (the benchmark files): they are read in place,
// as the C++ benchmark reads its own int16 points, instead of one interface call per coordinate.
type int16Rings interface {
	Rings
	RingXY16(r int) []int16
}

// loadStrided is loadRing for a ring given as coordinates, x and y first in every stride numbers.
func loadStrided[I Index, C float64 | int16](nodes []node[I], coords []C, stride int, first, indexBase uint32) (count uint32, area float64, rightmost uint32) {
	n := nodes[first:]
	size := uint32(len(coords) / stride)
	if size < 3 {
		return 0, 0, 0
	}
	n = n[:size]
	lastX, lastY := float64(coords[0]), float64(coords[1])
	sum, rightX := 0.0, lastX
	n[0].x = lastX
	n[0].y = lastY
	n[0].index = I(indexBase)
	count = 1
	right := uint32(0)
	for i, k := uint32(1), stride; i < size; i, k = i+1, k+stride {
		x, y := float64(coords[k]), float64(coords[k+1])
		node := &n[count]
		node.x = x
		node.y = y
		node.index = I(indexBase + i)
		// a repeated point adds nothing to the area and is overwritten by the next one
		sum += float64((lastX - x) * (lastY + y))
		if x > rightX {
			right = count
			rightX = x
		}
		if x != lastX || y != lastY {
			count++
		}
		lastX = x
		lastY = y
	}
	sum += float64((lastX - n[0].x) * (lastY + n[0].y))
	for count > 1 && samePos(&n[count-1], &n[0]) {
		count--
	}
	area = sum
	if right < count {
		rightmost = first + right
	} else {
		rightmost = first
	}
	return count, area, rightmost
}

// flatten views the points of a ring as x, y pairs.
func flatten(ring [][2]float64) []float64 {
	if len(ring) == 0 {
		return nil
	}
	return unsafe.Slice(&ring[0][0], 2*len(ring))
}

// flatRing returns the coordinates of ring r of f.
func flatRing(f *FlatRings, r int) []float64 {
	size := f.Size(r)
	if size == 0 {
		return nil
	}
	dim := f.dim()
	start := f.start(r) * dim
	return f.Coords[start : start+size*dim : start+size*dim]
}

// Links nodes [first, first + count) into a cycle, backwards when reversed.
func (t *Triangulator[I]) linkRing(first, count uint32, reversed bool) {
	n := t.nodes
	last := first + count - 1
	if !reversed {
		for i := first; i < last; i++ {
			n[i].next = i + 1
			n[i+1].prev = i
		}
		n[last].next = first
		n[first].prev = last
	} else {
		for i := first; i < last; i++ {
			n[i].prev = i + 1
			n[i+1].next = i
		}
		n[last].prev = first
		n[first].next = last
	}
}

func (t *Triangulator[I]) setRing(first, count uint32, ring uint16) {
	n := t.nodes
	for i := first; i < first+count; i++ {
		n[i].ring = ring
	}
}
