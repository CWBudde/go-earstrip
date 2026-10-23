package earstrip

import (
	"cmp"
	"math"
	"math/bits"
	"slices"
)

// Port of detail::Constrained<Index> from earstrip.hpp. The mesh, the fill and the strips are in constrained_mesh.go.
// Names follow the C++ (without the leading underscore); the comments are the C++ ones.
//
// Floating point products that are added or subtracted are wrapped in explicit float64 conversions: Go fuses
// a*b+c into FMA instructions on some platforms, an explicit conversion keeps the rounding of each product, as the
// C++ compiled without contraction.

// Floating point input is rounded to a grid of this many units over its bounding box
const gridSize = 1 << 30

// Up to this coordinate span the in-circle test is exact in 64 bits
const smallSpan = 1 << 14

type cdtVertex struct {
	x, y int64
}

type cdtPoint struct {
	x, y float64
}

// int128 is a signed 128-bit integer for the exact predicates.
type int128 struct {
	lo uint64
	hi int64
}

func b2u64(b bool) uint64 {
	if b {
		return 1
	}
	return 0
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func mul128(a, b int64) int128 {
	ua, ub := uint64(a), uint64(b)
	if a < 0 {
		ua = 0 - ua
	}
	if b < 0 {
		ub = 0 - ub
	}
	hi, lo := bits.Mul64(ua, ub)
	r := int128{lo: lo, hi: int64(hi)}
	if (a < 0) != (b < 0) {
		return r.neg()
	}
	return r
}

// times returns the product with b; it must fit 127 bits.
func (x int128) times(b int64) int128 {
	m := x
	if x.hi < 0 {
		m = x.neg()
	}
	y := uint64(b)
	if b < 0 {
		y = 0 - y
	}
	carry, lo := bits.Mul64(m.lo, y)
	r := int128{lo: lo, hi: int64(carry + uint64(m.hi)*y)}
	if (x.hi < 0) != (b < 0) {
		return r.neg()
	}
	return r
}

func (x int128) neg() int128 {
	return int128{lo: 0 - x.lo, hi: int64(^uint64(x.hi) + b2u64(x.lo == 0))}
}

func (x int128) add(o int128) int128 {
	lo := x.lo + o.lo
	return int128{lo: lo, hi: int64(uint64(x.hi) + uint64(o.hi) + b2u64(lo < x.lo))}
}

func (x int128) sub(o int128) int128 { return x.add(o.neg()) }

func (x int128) less(o int128) bool {
	if x.hi != o.hi {
		return x.hi < o.hi
	}
	return x.lo < o.lo
}

func (x int128) lessEq(o int128) bool { return !o.less(x) }

func (x int128) sign() int {
	if x.hi < 0 {
		return -1
	}
	if x.hi > 0 || x.lo != 0 {
		return 1
	}
	return 0
}

func (x int128) abs() int128 {
	if x.hi < 0 {
		return x.neg()
	}
	return x
}

func (x int128) toDouble() float64 {
	if x.hi < 0 {
		return -x.neg().toDouble()
	}
	return float64(float64(x.hi)*18446744073709551616.0) + float64(x.lo)
}

// floorDiv returns the floor of n / d for d > 0 whose quotient fits 62 bits and the remainder in [0, d).
func floorDiv(n, d int128) (int64, int128) {
	q := int64(math.Floor(n.toDouble() / d.toDouble()))
	r := n.sub(d.times(q))
	for r.sign() < 0 {
		q--
		r = r.add(d)
	}
	for d.lessEq(r) {
		q++
		r = r.sub(d)
	}
	return q, r
}

// Net number of times ring 0 and the holes run along a half-edge, in its direction
type wind struct {
	outer, holes int32
}

func (w wind) add(o wind) wind { return wind{w.outer + o.outer, w.holes + o.holes} }
func (w wind) neg() wind       { return wind{-w.outer, -w.holes} }

type segment struct {
	key  uint64 // low vertex << 32 | high vertex
	wind wind   // from low to high
}

type pending struct {
	a, b uint32
	wind wind
}

type sortItem struct {
	x, y  int64
	index uint32
}

type edgePair struct {
	u, v uint32
}

type walkEnd uint8

const (
	walkTarget walkEnd = iota
	walkVertex
	walkCrossing
	walkFailed
)

type location uint8

const (
	locVertex location = iota
	locEdge
	locTriangle
	locOutside
)

// Triangle states
const (
	triUnseen uint8 = iota
	triSeen
	triDropped
	triFree
	triUsed
)

// constrained is the constrained Delaunay triangulation of the ring edges into strips. The rings' crossings are
// rerouted through a grid point next to them, which becomes an added point. Kept: what lies inside ring 0 (non-zero
// winding) and outside every hole. Integer input of 32 bits is used as it is; other input is rounded to a 2^30 grid
// over its bounding box.
type constrained[I Index] struct {
	input      []cdtPoint // ring points as given
	ringEnds   []uint32
	sorted     []sortItem
	vertexOf   []uint32    // vertex of each input point
	vertices   []cdtVertex // distinct input points in sweep order, then added points
	outIndex   []I         // index of each vertex in the result
	vertexEdge []uint32    // a half-edge leaving each vertex
	hullNext   []uint32
	hullPrev   []uint32
	hullEdge   []uint32
	// Half-edge e of triangle e / 3 runs from org[e] to org[next(e)] with the triangle on its left
	org           []uint32
	twin          []uint32
	wind          []wind
	segments      []segment
	radix         []radixPair // buffers of radixSort, Go only
	radixTmp      []radixPair
	pending       []pending
	crossed       []edgePair
	stack         []uint32
	chain         []uint32
	triangleWind  []wind
	state         []uint8
	degree        []uint8
	candidates    []uint32
	run           []uint32 // vertices of a strip run
	taken         []uint32 // triangles of the last strip
	stripVertices []uint32 // vertices of the last strip
	strips        []I
	stripEnds     []uint32
	stripCW       []uint8 // first triangle of the strip runs clockwise
	pointCount    uint32  // input points
	inputVertices uint32
	vertexCount   uint32
	triangleCount uint32
	numExtra      uint32 // C++ _extraCount
	work          uint64
	workLimit     uint64
	originX       float64
	originY       float64
	scale         float64
	wide          bool // coordinate differences may exceed 32 bits
	exactCircle   bool // in-circle fits 64 bits
	smallArea     bool // ring areas fit 64 bits
	integral      bool // every coordinate is an integer of 32 bits: no rounding to the grid
	failed        bool // a limit was hit; the strips are empty
}

// maxPoints is the number of input and added points the strips can index.
func (c *constrained[I]) maxPoints() uint32 {
	indexCount := uint64(^I(0)) + 1 // wraps to 0 for 64-bit indices, as in C++
	return uint32(min(indexCount-2, math.MaxUint32-2))
}

// cdtResize sets the length of s to n, keeping its contents (std::vector::resize).
func cdtResize[T any](s []T, n int) []T {
	if n <= len(s) {
		return s[:n]
	}
	if n <= cap(s) {
		s = s[:n]
		return s
	}
	return append(s, make([]T, n-len(s))...)
}

// ---------- fill rule ----------

// Inside ring 0 and outside the holes (inside a hole its turned winding is negative)
func keep(w wind) bool { return w.outer != 0 && w.holes >= 0 }

// An edge the rule can see: it is kept, split at crossings and never flipped.
func significant(w wind) bool { return w.outer != 0 || w.holes != 0 }

// C++ sameForRule (equal as far as the rule can tell) is only used by assertions, which are left as comments.

// ---------- the steps ----------

// load copies the rings; nothing reads them afterwards. False when they have more points than I can count.
func (c *constrained[I]) load(rings Rings) bool {
	ringCount := uint32(rings.Count())
	total := uint64(0)
	for r := uint32(0); r < ringCount; r++ {
		total += uint64(rings.Size(int(r)))
	}
	c.pointCount = 0
	c.ringEnds = c.ringEnds[:0]
	if total > uint64(c.maxPoints()) {
		return false
	}
	if uint64(len(c.input)) < total {
		c.input = cdtResize(c.input, int(total))
	}
	c.ringEnds = cdtResize(c.ringEnds, int(ringCount))
	used := uint32(0)
	c.integral = true
	for r := uint32(0); r < ringCount; r++ {
		size := uint32(rings.Size(int(r)))
		for i := uint32(0); i < size; i++ {
			x, y := rings.X(int(r), int(i)), rings.Y(int(r), int(i))
			c.integral = c.integral && integer(x) && integer(y)
			c.input[used] = cdtPoint{x, y}
			used++
		}
		c.ringEnds[r] = used
	}
	c.pointCount = used
	return true
}

// build triangulates the loaded rings; false when a limit was hit.
func (c *constrained[I]) build() bool {
	c.strips = c.strips[:0]
	c.stripEnds = c.stripEnds[:0]
	c.stripCW = c.stripCW[:0]
	c.numExtra = 0
	c.vertexCount, c.inputVertices = 0, 0
	c.triangleCount = 0
	c.failed = false
	if c.pointCount < 3 {
		return true
	}
	c.toGrid()
	c.sortVertices()
	if c.vertexCount < 3 || !c.collectSegments() || !c.sweep() {
		return true
	}
	c.work = 0
	c.workLimit = 16*(uint64(len(c.segments))+uint64(c.vertexCount)) + (1 << 16)
	for i := range c.segments {
		s := c.segments[i]
		c.insertSegment(uint32(s.key>>32), uint32(s.key), s.wind)
		if c.failed {
			c.numExtra = 0
			return false
		}
	}
	c.fill()
	c.buildStrips()
	return true
}

// extraPoints writes the added points as x, y pairs into out[:0]; strip index pointCount + k is added point k.
func (c *constrained[I]) extraPoints(out []float64) []float64 {
	out = out[:0]
	for k := uint32(0); k < c.numExtra; k++ {
		p := c.extraPoint(k)
		out = append(out, p.x, p.y)
	}
	return out
}

func (c *constrained[I]) extraCount() uint32 { return c.numExtra }

// inputRingsArea returns the doubled area of ring 0 minus the holes, as the rings are given.
func (c *constrained[I]) inputRingsArea() float64 {
	rv := 0.0
	begin := uint32(0)
	for r := range c.ringEnds {
		end := c.ringEnds[r]
		a := 0.0
		for i := begin; i < end; i++ {
			j := begin
			if i+1 < end {
				j = i + 1
			}
			p, q := c.input[i], c.input[j]
			a += float64(p.x*q.y) - float64(q.x*p.y)
		}
		if r == 0 {
			rv += math.Abs(a)
		} else {
			rv += -math.Abs(a)
		}
		begin = end
	}
	return rv
}

// closer reports whether the given strips (over the input points) cover an area strictly closer to the ring area
// than the constrained ones.
func (c *constrained[I]) closer(strips []I, ends []uint32, ringsArea float64) bool {
	return math.Abs(c.covered(strips, ends)-ringsArea) < math.Abs(c.covered(c.strips, c.stripEnds)-ringsArea)
}

func integer(v float64) bool { return v == math.Floor(v) && v >= -2147483648.0 && v <= 2147483647.0 }

func crossPoints(a, b, cc cdtPoint) float64 {
	return float64((b.x-a.x)*(cc.y-a.y)) - float64((b.y-a.y)*(cc.x-a.x))
}

func (c *constrained[I]) covered(strips []I, ends []uint32) float64 {
	sum := 0.0
	begin := uint32(0)
	for _, end := range ends {
		for k := begin; k+2 < end; k++ {
			sum += math.Abs(crossPoints(c.point(uint32(strips[k])), c.point(uint32(strips[k+1])), c.point(uint32(strips[k+2]))))
		}
		begin = end
	}
	return sum
}

func (c *constrained[I]) point(index uint32) cdtPoint {
	if index < c.pointCount {
		return c.input[index]
	}
	return c.extraPoint(index - c.pointCount)
}

func (c *constrained[I]) extraPoint(k uint32) cdtPoint {
	v := c.vertices[c.inputVertices+k]
	if c.integral {
		return cdtPoint{float64(v.x), float64(v.y)}
	}
	return cdtPoint{c.originX + float64(v.x)/c.scale, c.originY + float64(v.y)/c.scale}
}

// ---------- input ----------

// std::min / std::max / std::clamp on doubles: the first argument wins unless the comparison says otherwise.
func stdMin(a, b float64) float64 {
	if b < a {
		return b
	}
	return a
}

func stdMax(a, b float64) float64 {
	if a < b {
		return b
	}
	return a
}

func gridCoord(v float64) int64 {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		v = 0
	} else if gridSize < v {
		v = gridSize
	}
	return int64(math.Round(v)) // llround: half away from zero
}

// toGrid sets the integer coordinates of the input points and the exactness modes of the predicates.
func (c *constrained[I]) toGrid() {
	n := c.pointCount
	if uint32(len(c.sorted)) < n {
		c.sorted = cdtResize(c.sorted, int(n))
	}
	items := c.sorted
	if !c.integral {
		minX, minY := c.input[0].x, c.input[0].y
		maxX, maxY := minX, minY
		for i := uint32(1); i < n; i++ {
			minX = stdMin(minX, c.input[i].x)
			maxX = stdMax(maxX, c.input[i].x)
			minY = stdMin(minY, c.input[i].y)
			maxY = stdMax(maxY, c.input[i].y)
		}
		span := stdMax(maxX-minX, maxY-minY)
		if span > 0 && !math.IsInf(span, 0) && !math.IsNaN(span) {
			c.scale = gridSize / span
		} else {
			c.scale = 1
		}
		c.originX = minX
		c.originY = minY
		for i := uint32(0); i < n; i++ {
			items[i] = sortItem{gridCoord((c.input[i].x - minX) * c.scale), gridCoord((c.input[i].y - minY) * c.scale), i}
		}
		c.wide = false
		c.exactCircle = false
		c.smallArea = false
	} else {
		minX, minY := int64(c.input[0].x), int64(c.input[0].y)
		maxX, maxY := minX, minY
		for i := uint32(0); i < n; i++ {
			x, y := int64(c.input[i].x), int64(c.input[i].y)
			items[i] = sortItem{x, y, i}
			minX = min(minX, x)
			maxX = max(maxX, x)
			minY = min(minY, y)
			maxY = max(maxY, y)
		}
		span := max(maxX-minX, maxY-minY)
		c.wide = span > math.MaxInt32
		c.exactCircle = span <= smallSpan
		c.smallArea = span < (1 << 23)
	}
}

func sortItemLess(a, b *sortItem) bool {
	if a.x != b.x {
		return a.x < b.x
	}
	if a.y != b.y {
		return a.y < b.y
	}
	return a.index < b.index
}

func sortItemCompare(a, b sortItem) int {
	if a.x != b.x {
		return cmp.Compare(a.x, b.x)
	}
	if a.y != b.y {
		return cmp.Compare(a.y, b.y)
	}
	return cmp.Compare(a.index, b.index)
}

// From this many items on, sortVertices and collectSegments use radixSort. Any correct sort gives the same result,
// so this is a matter of speed only.
const radixMin = 256

// radixPair is a key with a payload for radixSort.
type radixPair struct {
	key, val uint64
}

// radixSort sorts a by key, stably, moving the pairs between a and tmp (of the same length); it returns the one that
// holds the result. It sorts by the bytes of the key from the lowest and skips a byte that all keys share.
func radixSort(a, tmp []radixPair) []radixPair {
	if len(a) == 0 {
		return a
	}
	tmp = tmp[:len(a)]
	var counts [8][256]uint32
	for i := range a {
		k := a[i].key
		for d := range counts {
			counts[d][byte(k>>(8*d))]++
		}
	}
	n := uint32(len(a))
	for d := range counts {
		count := &counts[d]
		shift := 8 * uint(d)
		if count[byte(a[0].key>>shift)] == n {
			continue
		}
		sum := uint32(0)
		for b, k := range count {
			count[b] = sum
			sum += k
		}
		for _, p := range a {
			b := byte(p.key >> shift)
			tmp[count[b]] = p
			count[b]++
		}
		a, tmp = tmp, a
	}
	return a
}

// radixBuffers returns the two buffers of radixSort with n pairs each.
func (c *constrained[I]) radixBuffers(n uint32) ([]radixPair, []radixPair) {
	if uint32(len(c.radix)) < n {
		c.radix = cdtResize(c.radix, int(n))
		c.radixTmp = cdtResize(c.radixTmp, int(n))
	}
	return c.radix[:n], c.radixTmp[:n]
}

// sortVertices sorts the points by x, then y, and merges equal ones into vertices; a vertex keeps the last index of
// its point.
func (c *constrained[I]) sortVertices() {
	n := c.pointCount
	items := c.sorted[:n]
	switch {
	case n <= 32:
		for i := uint32(1); i < n; i++ {
			item := items[i]
			j := i
			for ; j > 0 && sortItemLess(&item, &items[j-1]); j-- {
				items[j] = items[j-1]
			}
			items[j] = item
		}
	case n < radixMin:
		slices.SortFunc(items, sortItemCompare)
	default:
		c.radixSortItems(items)
	}
	c.reserveVertices(n)
	if uint32(len(c.vertexOf)) < n {
		c.vertexOf = cdtResize(c.vertexOf, int(n))
	}
	count := uint32(0)
	for i := uint32(0); i < n; {
		j := i
		for j+1 < n && items[j+1].x == items[i].x && items[j+1].y == items[i].y {
			j++
		}
		c.vertices[count] = cdtVertex{items[i].x, items[i].y}
		c.outIndex[count] = I(items[j].index)
		for k := i; k <= j; k++ {
			c.vertexOf[items[k].index] = count
		}
		count++
		i = j + 1
	}
	c.vertexCount, c.inputVertices = count, count
}

// radixSortItems sorts the items in the order of sortItemLess. The key x - minX, y - minY orders as x, then y (both
// spans fit 32 bits: the coordinates are integers of 32 bits or on the grid), and toGrid wrote the items in the
// order of their indices, which the stable sort keeps for equal points.
func (c *constrained[I]) radixSortItems(items []sortItem) {
	minX, minY := items[0].x, items[0].y
	maxX, maxY := minX, minY
	for i := range items {
		minX = min(minX, items[i].x)
		maxX = max(maxX, items[i].x)
		minY = min(minY, items[i].y)
		maxY = max(maxY, items[i].y)
	}
	shift := uint(bits.Len64(uint64(maxY - minY)))
	mask := uint64(1)<<shift - 1
	pairs, tmp := c.radixBuffers(uint32(len(items)))
	for i := range items {
		pairs[i] = radixPair{uint64(items[i].x-minX)<<shift | uint64(items[i].y-minY), uint64(items[i].index)}
	}
	pairs = radixSort(pairs, tmp)
	for i := range items {
		p := pairs[i]
		items[i] = sortItem{minX + int64(p.key>>shift), minY + int64(p.key&mask), uint32(p.val)}
	}
}

// reserveVertices grows the vertex arrays to count vertices and the half-edges to 2 * count triangles.
func (c *constrained[I]) reserveVertices(count uint32) {
	if uint32(len(c.vertices)) < count {
		size := max(int(count), 2*len(c.vertices), 16)
		c.vertices = cdtResize(c.vertices, size)
		c.outIndex = cdtResize(c.outIndex, size)
		c.vertexEdge = cdtResize(c.vertexEdge, size)
		c.hullNext = cdtResize(c.hullNext, size)
		c.hullPrev = cdtResize(c.hullPrev, size)
		c.hullEdge = cdtResize(c.hullEdge, size)
	}
	if len(c.org) < 6*int(count) {
		size := max(6*int(count), 2*len(c.org))
		c.org = cdtResize(c.org, size)
		c.twin = cdtResize(c.twin, size)
		c.wind = cdtResize(c.wind, size)
	}
}

func segmentCompare(a, b segment) int { return cmp.Compare(a.key, b.key) }

// collectSegments sets the ring edges as segments between vertices with their net winding: ring 0 turned
// counterclockwise, holes clockwise, a flat ring as given. Equal segments are merged; false when none is left.
func (c *constrained[I]) collectSegments() bool {
	c.segments = c.segments[:0]
	begin := uint32(0)
	for r := range c.ringEnds {
		end := c.ringEnds[r]
		if end-begin >= 2 {
			areaSign := c.ringAreaSign(begin, end)
			sign := int32(-1)
			if areaSign == 0 || (areaSign > 0) == (r == 0) {
				sign = 1
			}
			for i := begin; i < end; i++ {
				j := begin
				if i+1 < end {
					j = i + 1
				}
				a, b := c.vertexOf[i], c.vertexOf[j]
				if a == b {
					continue
				}
				s := sign
				key := uint64(a)<<32 | uint64(b)
				if a > b {
					s = -sign
					key = uint64(b)<<32 | uint64(a)
				}
				w := wind{0, s}
				if r == 0 {
					w = wind{s, 0}
				}
				c.segments = append(c.segments, segment{key, w})
			}
		}
		begin = end
	}
	s := c.segments
	n := uint32(len(s))
	switch {
	case n <= 32:
		for i := uint32(1); i < n; i++ {
			item := s[i]
			j := i
			for ; j > 0 && item.key < s[j-1].key; j-- {
				s[j] = s[j-1]
			}
			s[j] = item
		}
	case n < radixMin:
		slices.SortFunc(s, segmentCompare)
	default:
		// The order of equal keys may differ from std::sort's, but the merge below only adds their windings.
		pairs, tmp := c.radixBuffers(n)
		for i := range s {
			pairs[i] = radixPair{s[i].key, uint64(uint32(s[i].wind.outer))<<32 | uint64(uint32(s[i].wind.holes))}
		}
		pairs = radixSort(pairs, tmp)
		for i := range s {
			s[i] = segment{pairs[i].key, wind{int32(pairs[i].val >> 32), int32(uint32(pairs[i].val))}}
		}
	}
	kept := uint32(0)
	for i := uint32(0); i < n; {
		merged := s[i]
		j := i + 1
		for ; j < n && s[j].key == merged.key; j++ {
			merged.wind = merged.wind.add(s[j].wind)
		}
		if significant(merged.wind) {
			s[kept] = merged
			kept++
		}
		i = j
	}
	c.segments = s[:kept]
	return kept > 0
}

// ringAreaSign returns the sign of the area of the ring of input points [begin, end) on the vertices. Exact.
func (c *constrained[I]) ringAreaSign(begin, end uint32) int {
	o := c.vertices[c.vertexOf[begin]]
	if c.smallArea {
		area := int64(0)
		for i := begin; i < end; i++ {
			j := begin
			if i+1 < end {
				j = i + 1
			}
			p, q := c.vertices[c.vertexOf[i]], c.vertices[c.vertexOf[j]]
			area += (p.x-o.x)*(q.y-o.y) - (q.x-o.x)*(p.y-o.y)
		}
		return int(b2i(area > 0) - b2i(area < 0))
	}
	var area int128
	for i := begin; i < end; i++ {
		j := begin
		if i+1 < end {
			j = i + 1
		}
		p, q := c.vertices[c.vertexOf[i]], c.vertices[c.vertexOf[j]]
		area = area.add(mul128(p.x-o.x, q.y-o.y)).sub(mul128(q.x-o.x, p.y-o.y))
	}
	return area.sign()
}

// ---------- predicates ----------

// orientV returns the sign of the turn a -> b -> c: 1 counterclockwise, -1 clockwise, 0 collinear. Exact.
func (c *constrained[I]) orientV(a, b, cc cdtVertex) int {
	abx, aby, acx, acy := b.x-a.x, b.y-a.y, cc.x-a.x, cc.y-a.y
	if !c.wide {
		d := abx*acy - aby*acx
		return int(b2i(d > 0) - b2i(d < 0))
	}
	return mul128(abx, acy).sub(mul128(aby, acx)).sign()
}

func (c *constrained[I]) orient(a, b, cc uint32) int {
	return c.orientV(c.vertices[a], c.vertices[b], c.vertices[cc])
}

// inCircle reports whether d lies inside the circle through the counterclockwise triangle a, b, c; inexact results
// count as outside.
func (c *constrained[I]) inCircle(ia, ib, ic, id uint32) bool {
	a, b, cc, d := c.vertices[ia], c.vertices[ib], c.vertices[ic], c.vertices[id]
	if c.exactCircle {
		adx, ady, bdx, bdy, cdx, cdy := a.x-d.x, a.y-d.y, b.x-d.x, b.y-d.y, cc.x-d.x, cc.y-d.y
		det := (adx*adx+ady*ady)*(bdx*cdy-bdy*cdx) + (bdx*bdx+bdy*bdy)*(cdx*ady-cdy*adx) +
			(cdx*cdx+cdy*cdy)*(adx*bdy-ady*bdx)
		return det > 0
	}
	adx, ady, bdx, bdy := float64(a.x-d.x), float64(a.y-d.y), float64(b.x-d.x), float64(b.y-d.y)
	cdx, cdy := float64(cc.x-d.x), float64(cc.y-d.y)
	al := float64(adx*adx) + float64(ady*ady)
	bl := float64(bdx*bdx) + float64(bdy*bdy)
	cl := float64(cdx*cdx) + float64(cdy*cdy)
	det := float64(al*(float64(bdx*cdy)-float64(bdy*cdx))) + float64(bl*(float64(cdx*ady)-float64(cdy*adx))) +
		float64(cl*(float64(adx*bdy)-float64(ady*bdx)))
	// The bound is a sum of products of non-negative finite numbers, so bound * 1e-14 >= 0: without a positive det
	// the test below fails anyway, and the bound need not be computed.
	if !(det > 0) {
		return false
	}
	bound := float64(al*(math.Abs(float64(bdx*cdy))+math.Abs(float64(bdy*cdx)))) +
		float64(bl*(math.Abs(float64(cdx*ady))+math.Abs(float64(cdy*adx)))) +
		float64(cl*(math.Abs(float64(adx*bdy))+math.Abs(float64(ady*bdx))))
	return det > bound*1e-14
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// sameDirection reports for collinear a, p, q other than a: p and q lie on the same side of a.
func sameDirection(a, p, q cdtVertex) bool {
	if p.x != a.x {
		return (p.x > a.x) == (q.x > a.x)
	}
	return (p.y > a.y) == (q.y > a.y)
}

// nearer reports for collinear a, p, q with p and q on the same side of a: p is nearer to a than q.
func nearer(a, p, q cdtVertex) bool {
	if q.x != a.x {
		return abs64(p.x-a.x) < abs64(q.x-a.x)
	}
	return abs64(p.y-a.y) < abs64(q.y-a.y)
}

// crossing returns the grid point for the crossing of the properly crossing segments ab and cd: of the corners of
// the grid cell holding the crossing and the segment ends, the one that moves both segments over the least area, a
// corner on a tie. A corner lies in the bounding box of both segments.
func (c *constrained[I]) crossing(ia, ib, ic, id uint32) cdtVertex {
	a, b, cv, d := c.vertices[ia], c.vertices[ib], c.vertices[ic], c.vertices[id]
	ux, uy, vx, vy := b.x-a.x, b.y-a.y, d.x-cv.x, d.y-cv.y
	den := mul128(ux, vy).sub(mul128(uy, vx))
	num := mul128(cv.x-a.x, vy).sub(mul128(cv.y-a.y, vx))
	if den.sign() < 0 {
		den = den.neg()
		num = num.neg()
	}
	// crossing = a + (num * u) / den, num / den in (0, 1)
	qx, rx := floorDiv(num.times(ux), den) // remainders in [0, den)
	qy, ry := floorDiv(num.times(uy), den)
	fx, fy := a.x+qx, a.y+qy
	px, py := float64(fx)+rx.toDouble()/den.toDouble(), float64(fy)+ry.toDouble()/den.toDouble()
	cost := func(v cdtVertex) int128 {
		return mul128(ux, v.y-a.y).sub(mul128(uy, v.x-a.x)).abs().add(mul128(vx, v.y-cv.y).sub(mul128(vy, v.x-cv.x)).abs())
	}
	distance := func(v cdtVertex) float64 {
		return float64((float64(v.x)-px)*(float64(v.x)-px)) + float64((float64(v.y)-py)*(float64(v.y)-py))
	}
	// the cell corners, a tie going to the nearer one
	corner := cdtVertex{fx + b2i(den.lessEq(rx.add(rx))), fy + b2i(den.lessEq(ry.add(ry)))}
	cornerCost := cost(corner)
	cornerDistance := distance(corner)
	for k := 0; k < 4; k++ {
		v := cdtVertex{fx + int64(k&1), fy + int64(k>>1)}
		if (v.x != fx && rx.sign() == 0) || (v.y != fy && ry.sign() == 0) {
			continue
		}
		vc := cost(v)
		vd := distance(v)
		if vc.less(cornerCost) || (vc == cornerCost && vd < cornerDistance) {
			corner = v
			cornerCost = vc
			cornerDistance = vd
		}
	}
	// a segment end adds no vertex, but must move less area than the corner
	end := a
	endCost := cost(a)
	endDistance := distance(a)
	for _, v := range [3]cdtVertex{b, cv, d} {
		vc := cost(v)
		vd := distance(v)
		if vc.less(endCost) || (vc == endCost && vd < endDistance) {
			end = v
			endCost = vc
			endDistance = vd
		}
	}
	if endCost.less(cornerCost) {
		return end
	}
	return corner
}
