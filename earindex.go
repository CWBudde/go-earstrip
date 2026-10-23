package earstrip

import (
	"math"
	"math/bits"
	"slices"
)

// The bucket slab of an ear test spans a whole range of one axis, and the zig-zag strips of a large ring make long
// thin ears whose slab holds a large part of the blockers. earIndex answers the same question with a kd tree over
// the blockers. It is not in the C++: it only changes how fast the answer comes, not the answer, which is a bool.
//
// Why the answers are the same: emptyTriangleBox reports whether an item of the slab passes the item test (the box
// test, not ia or ic, the three cross >= 0, not a corner). An item passing it passes the box test, so its coordinate
// on the bucket axis lies in [lo, hi]. Where bucket is monotone on [lo, hi] (see indexable), the item is in a
// bucket of [bucket(lo), bucket(hi)], so in the slab. Hence "an item of the slab passes" is "an item anywhere
// passes", and the tree tests every item it does not prove to fail, with the same item test.

const (
	// Most items per leaf of the tree; at most 64, one mask word.
	earLeafItems = 16
	// A slab of at most this many items is scanned; a larger one may use the tree.
	earIndexSlab = 64
	// The tree is built once the large slabs since the last classification have visited this many times the items.
	earIndexWork = 2
	// Coordinates up to this size cannot overflow cross (differences below 2^511, products below 2^1022, and their
	// difference below 2^1023), so it never gives an infinity or a NaN.
	earIndexBound = 0x1p510
	// bucket rounds v = (u - origin) * scale toward zero; up to this size the conversion to int64 is exact.
	earIndexBucketBound = 0x1p62

	noEarPos = ^uint32(0)
)

var (
	// checkEarIndex makes every ear test that may use the tree compare its answer with the slab scan's; for tests.
	checkEarIndex  bool
	earIndexChecks uint64 // answers compared while checkEarIndex is on
)

type earItem struct {
	p    [2]float64 // x, y; x = inf once dropped, as in the buckets
	node uint32
}

type earNode struct {
	minX, maxX, minY, maxY float64 // bounds of the items when built
	live                   uint32  // items not dropped since
}

// earIndex is a kd tree over the blockers in heap order: node 1 is the root, the children of k are 2k and 2k + 1,
// and the leaves are nodes leaves to 2 leaves - 1. Leaf j holds items [j n / leaves, (j + 1) n / leaves).
type earIndex struct {
	items  []earItem
	pos    []uint32 // tree position of each node indexed when the tree was built, noEarPos when left out
	nodes  []earNode
	leaves uint32
	ready  bool
	work   uint64 // items scanned by large slabs since the last classification
}

// reset drops the tree. A classification lists new blockers and calls it; rebuildIndex keeps the live ones, which
// the tree goes on to hold, with the gone ones dropped.
func (e *earIndex) reset() {
	e.ready = false
	e.work = 0
}

// build makes the tree from the live items. Items with a coordinate beyond earIndexBound (dropped ones, infinities
// and NaNs among them) are left out: they fail the box test of every triangle with corners within the bound, the
// only ones the tree is asked about.
func (e *earIndex) build(items []item, nodes int) {
	e.pos = grow(e.pos, nodes)
	e.items = e.items[:0]
	for i := range items {
		p := &items[i]
		if !(math.Abs(p.x) <= earIndexBound && math.Abs(p.y) <= earIndexBound) {
			e.pos[p.node] = noEarPos
			continue
		}
		e.items = append(e.items, earItem{[2]float64{p.x, p.y}, p.node})
	}
	n := uint64(len(e.items))
	leaves := uint64(1)
	for leaves*earLeafItems < n {
		leaves *= 2
	}
	e.leaves = uint32(leaves)
	e.nodes = grow(e.nodes, int(2*leaves))
	tree := e.nodes
	all := e.items
	// Top down, in heap order, so that a node's range is final when it is split: the bounds, then a median split
	// along the longer side.
	for k := uint64(1); k < 2*leaves; k++ {
		level := uint(bits.Len64(k) - 1)
		span := leaves >> level
		l0 := (k - 1<<level) * span
		lo, hi := l0*n/leaves, (l0+span)*n/leaves
		p := all[lo:hi]
		nd := &tree[k]
		nd.live = uint32(len(p))
		if len(p) == 0 {
			nd.minX, nd.maxX, nd.minY, nd.maxY = math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
			continue
		}
		// the builtins: there is no NaN, and the bounds are only compared, so the sign of a zero does not matter
		minX, maxX, minY, maxY := p[0].p[0], p[0].p[0], p[0].p[1], p[0].p[1]
		for i := range p {
			x, y := p[i].p[0], p[i].p[1]
			minX, maxX = min(minX, x), max(maxX, x)
			minY, maxY = min(minY, y), max(maxY, y)
		}
		nd.minX, nd.maxX, nd.minY, nd.maxY = minX, maxX, minY, maxY
		if k < leaves {
			axis := 0
			if maxY-minY > maxX-minX {
				axis = 1
			}
			selectEarItems(p, int((l0+span/2)*n/leaves-lo), axis)
		}
	}
	for i := range all {
		e.pos[all[i].node] = uint32(i)
	}
	e.ready = true
}

// selectEarItems reorders p so that p[k] holds the item that sorting by coordinate axis would put there, with no
// larger coordinate before it and no smaller one after it (std::nth_element). Coordinates are not NaN.
func selectEarItems(p []earItem, k, axis int) {
	axis &= 1
	lo, hi := 0, len(p)-1
	// Hoare partitions around a median of three; a run of bad pivots falls back to sorting.
	for budget := 2 * bits.Len(uint(len(p))); hi-lo > 16; budget-- {
		if budget == 0 {
			if axis == 0 {
				slices.SortFunc(p[lo:hi+1], cmpEarX)
			} else {
				slices.SortFunc(p[lo:hi+1], cmpEarY)
			}
			return
		}
		mid := lo + (hi-lo)/2
		if p[mid].p[axis] < p[lo].p[axis] {
			p[mid], p[lo] = p[lo], p[mid]
		}
		if p[hi].p[axis] < p[lo].p[axis] {
			p[hi], p[lo] = p[lo], p[hi]
		}
		if p[hi].p[axis] < p[mid].p[axis] {
			p[hi], p[mid] = p[mid], p[hi]
		}
		pivot := p[mid].p[axis]
		i, j := lo, hi
		for i <= j {
			for p[i].p[axis] < pivot {
				i++
			}
			for p[j].p[axis] > pivot {
				j--
			}
			if i <= j {
				p[i], p[j] = p[j], p[i]
				i++
				j--
			}
		}
		// now p[lo..j] <= pivot <= p[i..hi], and p[j+1..i-1] equal the pivot
		switch {
		case k <= j:
			hi = j
		case k >= i:
			lo = i
		default:
			return
		}
	}
	for i := lo + 1; i <= hi; i++ {
		for j := i; j > lo && p[j].p[axis] < p[j-1].p[axis]; j-- {
			p[j], p[j-1] = p[j-1], p[j]
		}
	}
}

func cmpEarX(a, b earItem) int { return cmpFloat(a.p[0], b.p[0]) }
func cmpEarY(a, b earItem) int { return cmpFloat(a.p[1], b.p[1]) }

func cmpFloat(a, b float64) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// drop marks the item of node id gone, as dropBlocker does in the buckets. Only nodes indexed since the build are
// dropped, and those all got their pos then.
func (e *earIndex) drop(id uint32) {
	j := e.pos[id]
	if j == noEarPos {
		return
	}
	e.items[j].p[0] = math.Inf(1)
	n, leaves := uint64(len(e.items)), uint64(e.leaves)
	// the leaf holding j: the last l with l n / leaves <= j
	k := uint32((uint64(j+1)*leaves-1)/n) + e.leaves
	nodes := e.nodes
	for ; k >= 1; k >>= 1 {
		nodes[k].live--
	}
}

// empty is emptyTriangleBox over the whole tree, for a triangle with corners within earIndexBound.
//
// A node is skipped when no item in it can pass the item test: none are live, its bounds miss the triangle's box,
// or the cross of one triangle edge is < 0 at all of its points. For the last, take the edge from p to q and
// cross(p, q, r) = fl(fl(A fl(ry - py)) - fl(B fl(rx - px))) with A = fl(qx - px), B = fl(qy - py), as the item
// test evaluates it. Rounding to nearest is monotone, so fl(ry - py) grows with ry, fl(A d) follows d when A >= 0
// and goes against it when A <= 0, and fl(s - t) grows with s and falls with t. So cross grows with ry when A >= 0,
// else falls, and falls with rx when B >= 0, else grows. With no overflow (earIndexBound) the largest float cross
// over the node's bounds is therefore the one at the corner (B >= 0 ? minX : maxX, A >= 0 ? maxY : minY), and when
// that is < 0, so is the cross of every item in the node.
func (e *earIndex) empty(a, b, c *[2]float64, ia, ic uint32) bool {
	ax, ay, bx, by, cx, cy := a[0], a[1], b[0], b[1], c[0], c[1]
	minX, maxX := min3(ax, bx, cx), max3(ax, bx, cx)
	minY, maxY := min3(ay, by, cy), max3(ay, by, cy)
	// the corner of a node where each edge's cross is largest: low x, high y
	abLowX, abHighY := by-ay >= 0, bx-ax >= 0
	bcLowX, bcHighY := cy-by >= 0, cx-bx >= 0
	caLowX, caHighY := ay-cy >= 0, ax-cx >= 0
	nodes := e.nodes
	leaves := e.leaves
	n, nl := uint64(len(e.items)), uint64(leaves)
	var stack [64]uint32 // depth first: a node's children go on top, so it holds at most one per level and one more
	stack[0] = 1
	sp := 1
	for sp > 0 {
		sp--
		k := stack[sp]
		nd := &nodes[k]
		x0, x1, y0, y1 := nd.minX, nd.maxX, nd.minY, nd.maxY
		if nd.live == 0 || x1 < minX || x0 > maxX || y1 < minY || y0 > maxY ||
			cross(ax, ay, bx, by, pick(abLowX, x0, x1), pick(abHighY, y1, y0)) < 0 ||
			cross(bx, by, cx, cy, pick(bcLowX, x0, x1), pick(bcHighY, y1, y0)) < 0 ||
			cross(cx, cy, ax, ay, pick(caLowX, x0, x1), pick(caHighY, y1, y0)) < 0 {
			continue
		}
		if k < leaves {
			stack[sp] = 2 * k
			stack[sp+1] = 2*k + 1
			sp += 2
			continue
		}
		l := uint64(k - leaves)
		p := e.items[l*n/nl : (l+1)*n/nl]
		// the branch-free pass of emptyTriangleBox, then the exact tests on the hits
		mask := uint64(0)
		for i := range p {
			q := &p[i]
			in := uint64(b2u(q.p[0] >= minX) & b2u(q.p[0] <= maxX) & b2u(q.p[1] >= minY) & b2u(q.p[1] <= maxY) &
				b2u(q.node != ia) & b2u(q.node != ic))
			mask |= in << (uint(i) & 63)
		}
		for mask != 0 {
			q := &p[lowestBit(mask)]
			mask &= mask - 1
			px, py := q.p[0], q.p[1]
			if cross(ax, ay, bx, by, px, py) < 0 || cross(bx, by, cx, cy, px, py) < 0 || cross(cx, cy, ax, ay, px, py) < 0 {
				continue
			}
			if (px != ax || py != ay) && (px != bx || py != by) && (px != cx || py != cy) {
				return false
			}
		}
	}
	return true
}

func pick(first bool, a, b float64) float64 {
	if first {
		return a
	}
	return b
}

// emptyEarIndexed is the multi-bucket ear test when the slab [begin, end) for the bounds lo, hi on the bucket
// axis is large: the tree where it gives the same answer, else the slab scan.
func (t *Triangulator[I]) emptyEarIndexed(a, b, c *node[I], ia, ic uint32, lo, hi float64, begin, end uint32) bool {
	if !t.indexable(a, b, c, lo, hi) {
		return emptyTriangleBox(t.items, begin, end, a, b, c, ia, ic)
	}
	e := &t.ears
	if !e.ready {
		if !checkEarIndex {
			e.work += uint64(end - begin)
			if e.work < earIndexWork*uint64(t.built) {
				return emptyTriangleBox(t.items, begin, end, a, b, c, ia, ic)
			}
		}
		e.build(t.items[:t.built], len(t.slot))
	}
	empty := e.empty(&[2]float64{a.x, a.y}, &[2]float64{b.x, b.y}, &[2]float64{c.x, c.y}, ia, ic)
	if checkEarIndex {
		earIndexChecks++
		if empty != emptyTriangleBox(t.items, begin, end, a, b, c, ia, ic) {
			panic("earstrip: the ear index and the bucket slab disagree")
		}
	}
	return empty
}

// indexable is true when the tree gives the answer of the slab scan for the triangle a, b, c with bounds lo, hi on
// the bucket axis: its corners lie within earIndexBound, and v = (u - origin) * scale of bucket stays within
// earIndexBucketBound at lo and hi. Then, for u in [lo, hi], fl(u - origin) lies between its values at lo and hi
// (rounding is monotone), both finite, and so does v (scale is finite and >= 0: an infinite scale makes v infinite
// or NaN at lo or hi, a zero one makes v zero). Within the bound the conversion to int64 truncates exactly, so
// bucket(u) is in [bucket(lo), bucket(hi)].
func (t *Triangulator[I]) indexable(a, b, c *node[I], lo, hi float64) bool {
	vlo := (lo - t.bucketOrigin) * t.bucketScale
	vhi := (hi - t.bucketOrigin) * t.bucketScale
	return vlo >= -earIndexBucketBound && vhi <= earIndexBucketBound &&
		math.Abs(a.x) <= earIndexBound && math.Abs(a.y) <= earIndexBound &&
		math.Abs(b.x) <= earIndexBound && math.Abs(b.y) <= earIndexBound &&
		math.Abs(c.x) <= earIndexBound && math.Abs(c.y) <= earIndexBound
}
