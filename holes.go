package earstrip

import (
	"math"
	"math/bits"
)

// sortOrder sorts the holes by decreasing rightmost x, then increasing y. Holes with the same rightmost point are
// tied, and which of them is bridged first changes the strips; std::sort leaves their order to the standard
// library, so this is libc++'s std::sort step by step: the same order as the C++ built with clang's libc++.
func (t *Triangulator[I]) sortOrder() {
	holes := t.holes
	cxxSort(t.order, func(a, b uint32) bool {
		ha, hb := &holes[a], &holes[b]
		if ha.x != hb.x {
			return ha.x > hb.x
		}
		return ha.y < hb.y
	})
}

// cxxSort is libc++'s std::sort (LLVM 17 and later, __introsort) for a comparator that is not std::less: the
// branching sort3/4/5 and the partition that keeps equal elements right of the pivot, heap sort past the depth
// limit. Equal elements end up in the same order as there.
func cxxSort(a []uint32, less func(x, y uint32) bool) {
	if len(a) == 0 {
		return
	}
	s := cxxSorter{a, less}
	s.introsort(0, len(a), 2*(bits.Len(uint(len(a)))-1), true)
}

type cxxSorter struct {
	a    []uint32
	less func(x, y uint32) bool
}

func (s cxxSorter) swap(i, j int) { s.a[i], s.a[j] = s.a[j], s.a[i] }

// stable, 2-3 compares, 0-2 swaps
func (s cxxSorter) sort3(x, y, z int) bool {
	a, less := s.a, s.less
	if !less(a[y], a[x]) { // if x <= y
		if !less(a[z], a[y]) { // if y <= z
			return false // x <= y && y <= z
		}
		s.swap(y, z) // x <= z && y < z
		if less(a[y], a[x]) {
			s.swap(x, y) // x < y && y <= z
		}
		return true
	}
	if less(a[z], a[y]) { // x > y, if y > z
		s.swap(x, z) // x < y && y < z
		return true
	}
	s.swap(x, y) // x > y && y <= z
	if less(a[z], a[y]) {
		s.swap(y, z) // x <= y && y < z
	}
	return true
}

// stable, 3-6 compares, 0-5 swaps
func (s cxxSorter) sort4(x1, x2, x3, x4 int) {
	a, less := s.a, s.less
	s.sort3(x1, x2, x3)
	if less(a[x4], a[x3]) {
		s.swap(x3, x4)
		if less(a[x3], a[x2]) {
			s.swap(x2, x3)
			if less(a[x2], a[x1]) {
				s.swap(x1, x2)
			}
		}
	}
}

// stable, 4-10 compares, 0-9 swaps
func (s cxxSorter) sort5(x1, x2, x3, x4, x5 int) {
	a, less := s.a, s.less
	s.sort4(x1, x2, x3, x4)
	if less(a[x5], a[x4]) {
		s.swap(x4, x5)
		if less(a[x4], a[x3]) {
			s.swap(x3, x4)
			if less(a[x3], a[x2]) {
				s.swap(x2, x3)
				if less(a[x2], a[x1]) {
					s.swap(x1, x2)
				}
			}
		}
	}
}

func (s cxxSorter) insertionSort(first, last int) {
	a, less := s.a, s.less
	if first == last {
		return
	}
	for i := first + 1; i != last; i++ {
		j := i - 1
		if less(a[i], a[j]) {
			t := a[i]
			k := j
			j = i
			for {
				a[j] = a[k]
				j = k
				if j == first {
					break
				}
				k--
				if !less(t, a[k]) {
					break
				}
			}
			a[j] = t
		}
	}
}

// Assumes that the element before first is not greater than any element in [first, last).
func (s cxxSorter) insertionSortUnguarded(first, last int) {
	a, less := s.a, s.less
	if first == last {
		return
	}
	for i := first + 1; i != last; i++ {
		j := i - 1
		if less(a[i], a[j]) {
			t := a[i]
			k := j
			j = i
			for {
				a[j] = a[k]
				j = k
				k--
				if !less(t, a[k]) {
					break
				}
			}
			a[j] = t
		}
	}
}

func (s cxxSorter) insertionSortIncomplete(first, last int) bool {
	a, less := s.a, s.less
	switch last - first {
	case 0, 1:
		return true
	case 2:
		if less(a[last-1], a[first]) {
			s.swap(first, last-1)
		}
		return true
	case 3:
		s.sort3(first, first+1, last-1)
		return true
	case 4:
		s.sort4(first, first+1, first+2, last-1)
		return true
	case 5:
		s.sort5(first, first+1, first+2, first+3, last-1)
		return true
	}
	j := first + 2
	s.sort3(first, first+1, j)
	const limit = 8
	count := 0
	for i := j + 1; i != last; i++ {
		if less(a[i], a[j]) {
			t := a[i]
			k := j
			j = i
			for {
				a[j] = a[k]
				j = k
				if j == first {
					break
				}
				k--
				if !less(t, a[k]) {
					break
				}
			}
			a[j] = t
			count++
			if count == limit {
				return i+1 == last
			}
		}
		j = i
	}
	return true
}

// Partitions [first, last) around the pivot a[first], equal elements right of it; returns the pivot's position and
// whether the range was already partitioned.
func (s cxxSorter) partitionWithEqualsOnRight(first, last int) (int, bool) {
	a, less := s.a, s.less
	begin := first
	pivot := a[first]
	for {
		first++
		if !less(a[first], pivot) {
			break
		}
	}
	if begin == first-1 {
		for first < last {
			last--
			if less(a[last], pivot) {
				break
			}
		}
	} else {
		for {
			last--
			if less(a[last], pivot) {
				break
			}
		}
	}
	already := first >= last
	for first < last {
		s.swap(first, last)
		for {
			first++
			if !less(a[first], pivot) {
				break
			}
		}
		for {
			last--
			if less(a[last], pivot) {
				break
			}
		}
	}
	pivotPos := first - 1
	if begin != pivotPos {
		a[begin] = a[pivotPos]
	}
	a[pivotPos] = pivot
	return pivotPos, already
}

// As above with equal elements left of the pivot; returns the position after the pivot.
func (s cxxSorter) partitionWithEqualsOnLeft(first, last int) int {
	a, less := s.a, s.less
	begin := first
	pivot := a[first]
	if less(pivot, a[last-1]) {
		for {
			first++
			if less(pivot, a[first]) {
				break
			}
		}
	} else {
		// while (++first < last && !comp(pivot, *first))
		for {
			first++
			if first >= last || less(pivot, a[first]) {
				break
			}
		}
	}
	if first < last {
		for {
			last--
			if !less(pivot, a[last]) {
				break
			}
		}
	}
	for first < last {
		s.swap(first, last)
		for {
			first++
			if less(pivot, a[first]) {
				break
			}
		}
		for {
			last--
			if !less(pivot, a[last]) {
				break
			}
		}
	}
	pivotPos := first - 1
	if begin != pivotPos {
		a[begin] = a[pivotPos]
	}
	a[pivotPos] = pivot
	return first
}

func (s cxxSorter) introsort(first, last, depth int, leftmost bool) {
	a, less := s.a, s.less
	const limit, nintherThreshold = 24, 128
	for {
		n := last - first
		switch n {
		case 0, 1:
			return
		case 2:
			if less(a[last-1], a[first]) {
				s.swap(first, last-1)
			}
			return
		case 3:
			s.sort3(first, first+1, last-1)
			return
		case 4:
			s.sort4(first, first+1, first+2, last-1)
			return
		case 5:
			s.sort5(first, first+1, first+2, first+3, last-1)
			return
		}
		if n < limit {
			if leftmost {
				s.insertionSort(first, last)
			} else {
				s.insertionSortUnguarded(first, last)
			}
			return
		}
		if depth == 0 {
			s.heapSort(first, last)
			return
		}
		depth--
		half := n / 2
		if n > nintherThreshold {
			s.sort3(first, first+half, last-1)
			s.sort3(first+1, first+half-1, last-2)
			s.sort3(first+2, first+half+1, last-3)
			s.sort3(first+half-1, first+half, first+half+1)
			s.swap(first, first+half)
		} else {
			s.sort3(first+half, first, last-1)
		}
		if !leftmost && !less(a[first-1], a[first]) {
			first = s.partitionWithEqualsOnLeft(first, last)
			continue
		}
		i, already := s.partitionWithEqualsOnRight(first, last)
		if already {
			fs := s.insertionSortIncomplete(first, i)
			if s.insertionSortIncomplete(i+1, last) {
				if fs {
					return
				}
				last = i
				continue
			} else if fs {
				first = i + 1
				continue
			}
		}
		s.introsort(first, i, depth, leftmost)
		leftmost = false
		first = i + 1
	}
}

// heapSort is std::__partial_sort(first, last, last): make_heap (assuming both children, as for arithmetic
// types), then sort_heap with Floyd's sift down.
func (s cxxSorter) heapSort(first, last int) {
	n := last - first
	siftDownN := n - 1
	if n&1 != 0 {
		siftDownN = n
	}
	if n > 1 {
		for start := (siftDownN - 2) / 2; start >= 0; start-- {
			s.siftDown(first, siftDownN, start)
		}
		s.siftUp(first, last, n)
	}
	for n := last - first; n > 1; last, n = last-1, n-1 {
		s.popHeap(first, last, n)
	}
}

func (s cxxSorter) siftDown(first, length, start int) {
	a, less := s.a, s.less
	child := start
	if length < 2 || (length-2)/2 < child {
		return
	}
	child = 2*child + 1
	if less(a[first+child], a[first+child+1]) {
		child++
	}
	if less(a[first+child], a[first+start]) {
		return
	}
	top := a[first+start]
	for {
		a[first+start] = a[first+child]
		start = child
		if (length-2)/2 < child {
			break
		}
		child = 2*child + 1
		if less(a[first+child], a[first+child+1]) {
			child++
		}
		if less(a[first+child], top) {
			break
		}
	}
	a[first+start] = top
}

func (s cxxSorter) siftUp(first, last, length int) {
	a, less := s.a, s.less
	if length > 1 {
		length = (length - 2) / 2
		ptr := first + length
		last--
		if less(a[ptr], a[last]) {
			t := a[last]
			for {
				a[last] = a[ptr]
				last = ptr
				if length == 0 {
					break
				}
				length = (length - 1) / 2
				ptr = first + length
				if !less(a[ptr], t) {
					break
				}
			}
			a[last] = t
		}
	}
}

func (s cxxSorter) popHeap(first, last, length int) {
	a := s.a
	if length > 1 {
		top := a[first]
		hole := s.floydSiftDown(first, length)
		last--
		if hole == last {
			a[hole] = top
		} else {
			a[hole] = a[last]
			hole++
			a[last] = top
			s.siftUp(first, hole, hole-first)
		}
	}
}

func (s cxxSorter) floydSiftDown(first, length int) int {
	a, less := s.a, s.less
	hole, childI, child := first, first, 0
	for {
		childI += child + 1
		child = 2*child + 1
		if child+1 < length && less(a[childI], a[childI+1]) {
			childI++
			child++
		}
		a[hole] = a[childI]
		hole = childI
		if child > (length-2)/2 {
			return hole
		}
	}
}

// Chains nodes at equal positions and flags positions that several rings share.
func (t *Triangulator[I]) findTouches(used uint32) {
	n := t.nodes
	bitCount := bitWidth(used*2 - 1)
	mask := uint32(1)<<bitCount - 1
	t.hash = assign(t.hash, int(mask+1), uint32(none))
	hash := t.hash
	same := t.samePos
	for i := uint32(0); i < used; i++ {
		a := &n[i]
		a.flag = 0
		same[i] = i
		key := bitsOf(a.x)*0x9E3779B97F4A7C15 ^ bitsOf(a.y)*0xC2B2AE3D27D4EB4F
		key ^= key >> 31
		key *= 0xBF58476D1CE4E5B9
		h := uint32(key>>40) & mask
		for {
			j := hash[h]
			if j == none {
				hash[h] = i
				break
			}
			if samePos(&n[j], a) {
				same[i] = same[j]
				same[j] = i
				if n[j].ring != a.ring {
					a.flag = 1
					n[j].flag = 1
				}
				break
			}
			h = (h + 1) & mask
		}
	}
}

// Joins every unmerged hole that shares a vertex with the merged nodes [first, first + count), transitively.
func (t *Triangulator[I]) joinTouchingRings(first, count uint32) {
	n := t.nodes
	same := t.samePos
	t.queue = append(t.queue[:0], first, count)
	for len(t.queue) > 0 {
		q := len(t.queue)
		end := t.queue[q-1] + t.queue[q-2]
		begin := t.queue[q-2]
		t.queue = t.queue[:q-2]
		for s := begin; s < end; s++ {
			if n[s].flag == 0 {
				continue
			}
			for v := same[s]; v != s; v = same[v] {
				ring := n[v].ring
				if t.merged[ring] != 0 || !t.joinTouching(&t.holes[ring-1]) {
					continue
				}
				t.merged[ring] = 1
				t.markRing(t.holes[ring-1].first, t.holes[ring-1].count)
				t.queue = append(t.queue, t.holes[ring-1].first, t.holes[ring-1].count)
			}
		}
	}
}

// Splices a hole into the boundary at a vertex they share; false when it touches nothing merged.
func (t *Triangulator[I]) joinTouching(h *hole) bool {
	n := t.nodes
	same := t.samePos
	for s := h.first; s < h.first+h.count; s++ {
		if n[s].flag == 0 {
			continue
		}
		hs := &n[s]
		hp, hn := &n[hs.prev], &n[hs.next]
		for v := same[s]; v != s; v = same[v] {
			bv := &n[v]
			if t.merged[bv.ring] == 0 {
				continue
			}
			bp, bn := &n[bv.prev], &n[bv.next]
			if !t.inSector(v, hp.x, hp.y, false) || !t.inSector(v, hn.x, hn.y, false) || !t.inSector(s, bp.x, bp.y, false) ||
				!t.inSector(s, bn.x, bn.y, false) {
				continue
			}
			// v -> hole after s ... s -> boundary after v
			vn, sn := bv.next, hs.next
			n[v].next = sn
			n[sn].prev = v
			n[s].next = vn
			n[vn].prev = s
			t.markEdge(v)
			t.markEdge(s)
			return true
		}
	}
	return false
}

// True when point p lies in the sector of node t's corner.
func (t *Triangulator[I]) inSector(tn uint32, px, py float64, strict bool) bool {
	n := t.nodes
	v := &n[tn]
	a, c := &n[v.prev], &n[v.next]
	c1, c2 := cross(a.x, a.y, v.x, v.y, px, py), cross(v.x, v.y, c.x, c.y, px, py)
	if crossN(a, v, c) > 0 {
		if strict {
			return c1 > 0 && c2 > 0
		}
		return c1 >= 0 && c2 >= 0
	}
	if strict {
		return c1 > 0 || c2 > 0
	}
	return c1 >= 0 || c2 >= 0
}

// Among the boundary nodes at target's position, the one whose sector contains p, or none.
func (t *Triangulator[I]) pickCopy(target uint32, px, py float64) uint32 {
	if t.inSector(target, px, py, true) {
		return target
	}
	n := t.nodes
	same := t.samePos
	loose := uint32(none)
	if t.inSector(target, px, py, false) {
		loose = target
	}
	for v := same[target]; v != target; v = same[v] {
		if t.merged[n[v].ring] == 0 {
			continue
		}
		if t.inSector(v, px, py, true) {
			return v
		}
		if loose == none && t.inSector(v, px, py, false) {
			loose = v
		}
	}
	return loose
}

// Joins the hole through its rightmost node m to the boundary (Eberly); false when no boundary vertex is visible.
func (t *Triangulator[I]) bridge(m uint32, used *uint32) bool {
	n := t.nodes
	merged := t.merged
	mx, my := n[m].x, n[m].y
	hp, hn := &n[n[m].prev], &n[n[m].next]
	bestX := math.Inf(1)
	rayX := bestX
	edge, onEdge, onRay := uint32(none), uint32(none), uint32(none)
	up := false
	// the boundary is every node of a merged ring, each with its outgoing edge
	//
	// Only a node whose edge has an end at y == my or ends on both sides of my can change a result below (an edge with
	// a NaN end only through its other end at my), and no such node is in a block whose bounds leave out my. The
	// other blocks are scanned in the same increasing order as the C++ loop over [0, used), so the first and last
	// winners of the ties are the same.
	for ranges, k := t.blocksAtY(my, *used), 0; k < len(ranges); k += 2 {
		for v := ranges[k]; v < ranges[k+1]; v++ {
			a := &n[v]
			if merged[a.ring] == 0 {
				continue
			}
			b := &n[a.next]
			// nearest vertex lying on the ray itself, which the crossing test passes over
			if a.y == my && a.x > mx && a.x < rayX {
				rayX = a.x
				onRay = v
			}
			through := false
			if (a.y <= my) != (b.y <= my) {
				// computed from the lower end so that coincident bridge edges give the same x
				lo, hi := a, b
				if !(a.y < b.y) {
					lo, hi = b, a
				}
				x := lo.x
				if lo.y != my {
					x = lo.x + (my-lo.y)*(hi.x-lo.x)/(hi.y-lo.y)
				}
				if x >= mx {
					u := b.y > a.y
					if x < bestX || (x == bestX && u && !up) {
						bestX = x
						edge = v
						up = u
					}
					through = x == mx && lo.y != my
				}
			} else if a.y == my && b.y == my {
				through = (a.x < mx && mx < b.x) || (b.x < mx && mx < a.x)
			}
			// m inside this edge with the hole on its inner side
			if through && crossN(a, b, hp) >= 0 && crossN(a, b, hn) >= 0 {
				onEdge = v
			}
		}
	}

	if onEdge != none {
		// m lies on the edge: insert the hole into it.
		m2, mp, edgeEnd := *used, n[m].prev, n[onEdge].next
		*used++
		n[m2] = n[m]
		n[onEdge].next = m
		n[m].prev = onEdge
		n[mp].next = m2
		n[m2].prev = mp
		n[m2].next = edgeEnd
		n[edgeEnd].prev = m2
		t.samePos[m2] = t.samePos[m]
		t.samePos[m] = m2
		t.markEdge(onEdge)
		t.markEdge(m2)
		t.markEdge(mp)
		return true
	}
	if onRay != none && rayX < bestX {
		// The ray touches a vertex before any crossing: nothing lies between it and m.
		if tn := t.pickCopy(onRay, mx, my); tn != none {
			t.splice(tn, m, used)
			return true
		}
	}
	if edge == none {
		return false
	}
	edgeEnd := n[edge].next
	ea, eb := &n[edge], &n[edgeEnd]
	hit := uint32(none)
	if ea.y == my && ea.x == bestX {
		hit = edge
	} else if eb.y == my && eb.x == bestX {
		hit = edgeEnd
	}
	if hit != none {
		// The ray hits a vertex: visible when m lies in the sector of one of its copies.
		tn := t.pickCopy(hit, mx, my)
		if tn == none {
			return false
		}
		t.splice(tn, m, used)
		return true
	}
	if !up {
		return false
	}
	// Visible candidate: the edge end with larger x unless a blocker in triangle m, hit point, p is closer in angle.
	pi := edgeEnd
	if ea.x >= eb.x {
		pi = edge
	}
	p := &n[pi]
	ix, px, py := bestX, p.x, p.y
	above := py > my
	y0, y1 := py, my
	if above {
		y0, y1 = my, py
	}
	best := uint32(none)
	bdx, bdy := 0.0, 0.0
	// only nodes inside the box count, and a block's bounds hold all of its merged nodes; same order as the C++
	for ranges, k := t.blocksInBox(mx, px, y0, y1, *used), 0; k < len(ranges); k += 2 {
		for v := ranges[k]; v < ranges[k+1]; v++ {
			r := &n[v]
			box := r.x >= mx && r.x <= px && r.y >= y0 && r.y <= y1
			if box && !(r.x == px && r.y == py) && merged[r.ring] != 0 {
				var inside bool
				if above {
					inside = cross(mx, my, ix, my, r.x, r.y) >= 0 && cross(ix, my, px, py, r.x, r.y) >= 0 && cross(px, py, mx, my, r.x, r.y) >= 0
				} else {
					inside = cross(mx, my, px, py, r.x, r.y) >= 0 && cross(px, py, ix, my, r.x, r.y) >= 0 && cross(ix, my, mx, my, r.x, r.y) >= 0
				}
				if inside && crossN(&n[r.prev], r, &n[r.next]) <= 0 {
					dx, dy := r.x-mx, math.Abs(r.y-my)
					lhs, rhs := dy*bdx, bdy*dx
					if best == none || lhs < rhs || (lhs == rhs && dx < bdx) {
						best = v
						bdx = dx
						bdy = dy
					}
				}
			}
		}
	}
	target := pi
	if best != none {
		target = best
	}
	if tn := t.pickCopy(target, mx, my); tn != none {
		t.splice(tn, m, used)
	} else {
		t.splice(target, m, used)
	}
	return true
}

// Bridge between boundary node t and hole node m: t -> m ... hole ... m' -> t'.
func (t *Triangulator[I]) splice(tn, m uint32, used *uint32) {
	n := t.nodes
	t2, m2 := *used, *used+1
	*used += 2
	tnext, mp := n[tn].next, n[m].prev
	n[t2] = n[tn]
	n[m2] = n[m]
	n[tn].next = m
	n[m].prev = tn
	n[mp].next = m2
	n[m2].prev = mp
	n[m2].next = t2
	n[t2].prev = m2
	n[t2].next = tnext
	n[tnext].prev = t2
	t.samePos[t2] = t.samePos[tn]
	t.samePos[tn] = t2
	t.samePos[m2] = t.samePos[m]
	t.samePos[m] = m2
	t.markEdge(tn)
	t.markEdge(t2)
	t.markEdge(m2)
	t.markEdge(mp)
}

// Nodes [k·bridgeBlockSize, (k+1)·bridgeBlockSize) share bridge block k, and bridgeGroupNodes nodes share a group,
// whose bounds hold those of its blocks.
const (
	bridgeBlockShift = 3
	bridgeBlockSize  = 1 << bridgeBlockShift
	bridgeGroupShift = bridgeBlockShift + 4
	bridgeGroupNodes = 1 << bridgeGroupShift
)

// bridgeBlock lets bridge's two scans pass over runs of nodes that cannot change their result. yLo and yHi enclose
// the y of both ends of the outgoing edge of every merged node in the run, xLo and xHi the x of these nodes. The
// bounds only grow: a node is added again whenever its edge changes, so they may be loose but always hold every
// merged node's current edge. A NaN coordinate is left out: a scan only takes a NaN end when the other end is at the
// ray's y, which is in the bounds.
type bridgeBlock struct{ yLo, yHi, xLo, xHi float64 }

// add widens the bounds by a node at x, ay whose edge ends at y = by. Plain comparisons, so that a NaN changes
// nothing.
func (b *bridgeBlock) add(x, ay, by float64) {
	if ay < b.yLo {
		b.yLo = ay
	}
	if ay > b.yHi {
		b.yHi = ay
	}
	if by < b.yLo {
		b.yLo = by
	}
	if by > b.yHi {
		b.yHi = by
	}
	if x < b.xLo {
		b.xLo = x
	}
	if x > b.xHi {
		b.xHi = x
	}
}

// merge widens the bounds by those of o.
func (b *bridgeBlock) merge(o *bridgeBlock) {
	if o.yLo < b.yLo {
		b.yLo = o.yLo
	}
	if o.yHi > b.yHi {
		b.yHi = o.yHi
	}
	if o.xLo < b.xLo {
		b.xLo = o.xLo
	}
	if o.xHi > b.xHi {
		b.xHi = o.xHi
	}
}

// holdsY is false when no edge of the run has an end at y or ends on both sides of it.
func (b *bridgeBlock) holdsY(y float64) bool { return b.yLo <= y && y <= b.yHi }

// meetsBox is false when no node of the run lies in [x0, x1] × [y0, y1].
func (b *bridgeBlock) meetsBox(x0, x1, y0, y1 float64) bool {
	return b.xHi >= x0 && b.xLo <= x1 && b.yHi >= y0 && b.yLo <= y1
}

// Below this many nodes, or with a single hole, bridge scans every node as the C++ does: the blocks would cost more
// than they save.
const bridgeIndexMin = 64

// Empties the bridge blocks and groups for nodes [0, count), or turns them off for a small polygon.
func (t *Triangulator[I]) resetBridgeBlocks(count uint32) {
	t.bridgeIndexed = len(t.holes) > 1 && count >= bridgeIndexMin
	if !t.bridgeIndexed {
		return
	}
	empty := bridgeBlock{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
	t.bridgeBlocks = assign(t.bridgeBlocks, int((count+bridgeBlockSize-1)>>bridgeBlockShift), empty)
	t.bridgeGroups = assign(t.bridgeGroups, int((count+bridgeGroupNodes-1)>>bridgeGroupShift), empty)
}

// Adds node v and its outgoing edge to v's block and group; called for every merged node and again when its edge
// changes.
func (t *Triangulator[I]) markEdge(v uint32) {
	if !t.bridgeIndexed {
		return
	}
	n := t.nodes
	a := &n[v]
	x, ay, by := a.x, a.y, n[a.next].y
	t.bridgeBlocks[v>>bridgeBlockShift].add(x, ay, by)
	t.bridgeGroups[v>>bridgeGroupShift].add(x, ay, by)
}

// Adds the nodes [first, first + count) of a ring that has just been merged, a block at a time.
func (t *Triangulator[I]) markRing(first, count uint32) {
	if !t.bridgeIndexed {
		return
	}
	n := t.nodes
	for v, end := first, first+count; v < end; {
		k := v >> bridgeBlockShift
		run := bridgeBlock{math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)}
		for stop := min((k+1)<<bridgeBlockShift, end); v < stop; v++ {
			a := &n[v]
			run.add(a.x, a.y, n[a.next].y)
		}
		t.bridgeBlocks[k].merge(&run)
		t.bridgeGroups[k>>(bridgeGroupShift-bridgeBlockShift)].merge(&run)
	}
}

// Node ranges [start, end), as pairs in increasing order, that cover every block of nodes [0, used) whose bounds
// hold y.
func (t *Triangulator[I]) blocksAtY(y float64, used uint32) []uint32 {
	hits := t.blockHits[:0]
	if !t.bridgeIndexed {
		hits = append(hits, 0, used)
		t.blockHits = hits
		return hits
	}
	blocks, groups := t.bridgeBlocks, t.bridgeGroups
	for g := uint32(0); g < used; g += bridgeGroupNodes {
		if !groups[g>>bridgeGroupShift].holdsY(y) {
			continue
		}
		for first, end := g, min(g+bridgeGroupNodes, used); first < end; first += bridgeBlockSize {
			if blocks[first>>bridgeBlockShift].holdsY(y) {
				hits = addRange(hits, first, min(first+bridgeBlockSize, used))
			}
		}
	}
	t.blockHits = hits
	return hits
}

// Node ranges [start, end), as pairs in increasing order, that cover every block of nodes [0, used) whose bounds
// meet [x0, x1] × [y0, y1].
func (t *Triangulator[I]) blocksInBox(x0, x1, y0, y1 float64, used uint32) []uint32 {
	hits := t.blockHits[:0]
	if !t.bridgeIndexed {
		hits = append(hits, 0, used)
		t.blockHits = hits
		return hits
	}
	blocks, groups := t.bridgeBlocks, t.bridgeGroups
	for g := uint32(0); g < used; g += bridgeGroupNodes {
		if !groups[g>>bridgeGroupShift].meetsBox(x0, x1, y0, y1) {
			continue
		}
		for first, end := g, min(g+bridgeGroupNodes, used); first < end; first += bridgeBlockSize {
			if blocks[first>>bridgeBlockShift].meetsBox(x0, x1, y0, y1) {
				hits = addRange(hits, first, min(first+bridgeBlockSize, used))
			}
		}
	}
	t.blockHits = hits
	return hits
}

// addRange appends [start, end) to the range pairs, joined to the last one when it ends at start.
func addRange(ranges []uint32, start, end uint32) []uint32 {
	if k := len(ranges); k > 0 && ranges[k-1] == start {
		ranges[k-1] = end
		return ranges
	}
	return append(ranges, start, end)
}
