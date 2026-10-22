package earstrip

import "math"

// Links a single ring stored in array order, flags its corners and lists the reflex ones as blockers;
// true when the ring is convex: every corner strictly convex and the edges turn around once.
func (t *Triangulator[I]) classifyRing(count uint32, reversed bool) bool {
	n := t.nodes[:count]
	blockers := t.blockers
	items := t.items
	slot := t.slot
	sign := 1.0
	if reversed {
		sign = -1.0
	}
	found, turns := uint32(0), uint32(0)
	firstDir, lastDir := 0, 0
	ex, ey := n[0].x-n[count-1].x, n[0].y-n[count-1].y
	for i := uint32(0); i < count; i++ {
		prev, next := i-1, i+1
		if i == 0 {
			prev = count - 1
		}
		if next == count {
			next = 0
		}
		node := &n[i]
		fx, fy := n[next].x-node.x, n[next].y-node.y
		reflex := (float64(ex*fy)-float64(ey*fx))*sign <= 0
		node.reflex = b2u(reflex)
		node.flag = node.reflex
		if reversed {
			node.prev, node.next = next, prev
		} else {
			node.prev, node.next = prev, next
		}
		// written for every node, kept only for reflex ones
		blockers[found] = i
		items[found] = item{node.x, node.y, i}
		slot[i] = found
		found += uint32(node.reflex)
		// a ring that turns around once switches the x direction of its edges twice; dir is -1, 0 or 1, so
		// dir*lastDir < 0 is "both set and different". Branch-free: the direction flips are hard to predict.
		dir := int(b2u(fx > 0)) - int(b2u(fx < 0))
		turns += uint32(b2u(dir*lastDir < 0))
		if firstDir == 0 {
			firstDir = dir
		}
		if dir != 0 {
			lastDir = dir
		}
		ex = fx
		ey = fy
	}
	if firstDir != lastDir {
		turns++
	}
	t.blockerCount = found
	return found == 0 && turns == 2
}

// Flags the corners of the linked boundary and collects blockers; returns the vertex count.
func (t *Triangulator[I]) classifyCycle(start uint32) uint32 {
	n := t.nodes
	blockers := t.blockers
	items := t.items
	slot := t.slot
	count, found, v := uint32(0), uint32(0), start
	for {
		node := &n[v]
		node.reflex = b2u(crossN(&n[node.prev], node, &n[node.next]) <= 0)
		node.flag = node.reflex
		blockers[found] = v
		items[found] = item{node.x, node.y, v}
		slot[v] = found
		found += uint32(node.reflex)
		count++
		v = node.next
		if v == start {
			break
		}
	}
	t.blockerCount = found
	return count
}

// True when all four corners of a single quad turn the way of its area.
func (t *Triangulator[I]) convexQuad(area float64) bool {
	n := t.nodes[:4]
	sign := 1.0
	if area < 0 {
		sign = -1.0
	}
	return minf(minf(crossN(&n[3], &n[0], &n[1])*sign, crossN(&n[0], &n[1], &n[2])*sign),
		minf(crossN(&n[1], &n[2], &n[3])*sign, crossN(&n[2], &n[3], &n[0])*sign)) > 0
}

// A convex ring is one zig-zag strip, wound the way the ring runs.
func (t *Triangulator[I]) emitConvex(count uint32, clockwise bool) {
	n := t.nodes
	lo, hi := uint32(1), count-1
	t.put(n[0].index)
	for lo <= hi {
		t.put(n[lo].index)
		lo++
		if lo <= hi {
			t.put(n[hi].index)
			hi--
		}
	}
	t.endStrip(clockwise)
}

// Adds an index to the strip being written.
func (t *Triangulator[I]) put(i I) { t.strips = append(t.strips, i) }

// Ends the strip being written; clockwise: its first triangle runs clockwise.
func (t *Triangulator[I]) endStrip(clockwise bool) {
	t.stripEnds = append(t.stripEnds, uint32(len(t.strips)))
	t.stripCW = append(t.stripCW, b2u(clockwise))
}

// Sorts the blockers into buckets along the longer axis of their bounds, so a query is one contiguous range;
// few blockers stay in one bucket.
func (t *Triangulator[I]) buildIndex() {
	n := t.nodes
	count := t.blockerCount
	blockers := t.blockers
	items := t.items
	slot := t.slot
	t.live, t.built = count, count
	t.buckets = 1
	// a short list stays as the classification wrote it
	if count <= smallBlockers {
		return
	}
	minX, minY := n[blockers[0]].x, n[blockers[0]].y
	maxX, maxY := minX, minY
	for i := uint32(0); i < count; i++ {
		b := blockers[i]
		minX = minf(minX, n[b].x)
		maxX = maxf(maxX, n[b].x)
		minY = minf(minY, n[b].y)
		maxY = maxf(maxY, n[b].y)
	}
	t.axisY = maxY-minY > maxX-minX
	extent := maxX - minX
	t.bucketOrigin = minX
	if t.axisY {
		extent = maxY - minY
		t.bucketOrigin = minY
	}
	buckets := uint32(maxf(1.0, float64(count)/blockersPerBucket))
	t.buckets = buckets
	t.bucketScale = 0
	if extent > 0 {
		t.bucketScale = float64(buckets) / extent
	}
	t.bucketStart = assign(t.bucketStart, int(buckets+1), 0)
	start := t.bucketStart
	for i := uint32(0); i < count; i++ {
		start[t.bucketOf(&n[blockers[i]])+1]++
	}
	for c := uint32(0); c < buckets; c++ {
		start[c+1] += start[c]
	}
	// fill from each bucket's start, then shift the starts back
	for k := uint32(0); k < count; k++ {
		b := blockers[k]
		c := t.bucketOf(&n[b])
		i := start[c]
		start[c]++
		items[i] = item{n[b].x, n[b].y, b}
		slot[b] = i
	}
	for c := buckets; c > 0; c-- {
		start[c] = start[c-1]
	}
	start[0] = 0
}

func (t *Triangulator[I]) bucket(u float64) uint32 {
	v := int64((u - t.bucketOrigin) * t.bucketScale)
	if hi := int64(t.buckets) - 1; v > hi {
		v = hi
	}
	if v < 0 {
		v = 0
	}
	return uint32(v)
}

func (t *Triangulator[I]) bucketOf(v *node[I]) uint32 {
	if t.axisY {
		return t.bucket(v.y)
	}
	return t.bucket(v.x)
}

// True when no blocker in items [begin, end) lies in the closed triangle a, b, c (counter-clockwise) away from its
// corners; the neighbors ia and ic are corners themselves.
func emptyTriangle[I Index](items []item, begin, end uint32, a, b, c *node[I], ia, ic uint32) bool {
	ax, ay, bx, by, cx, cy := a.x, a.y, b.x, b.y, c.x, c.y
	abx, aby, bcx, bcy, cax, cay := bx-ax, by-ay, cx-bx, cy-by, ax-cx, ay-cy
	// branch-free pass marking blockers in the closed triangle; the rare hits are checked against the corners after
	items = items[begin:end]
	hits := uint32(0)
	for i := range items {
		p := &items[i]
		s1 := float64(abx*(p.y-ay)) - float64(aby*(p.x-ax))
		s2 := float64(bcx*(p.y-by)) - float64(bcy*(p.x-bx))
		s3 := float64(cax*(p.y-cy)) - float64(cay*(p.x-cx))
		// std::min(std::min(s1, s2), s3) >= 0, which passes over a NaN in s2 or s3 as std::min does; i < 32
		// (smallBlockers), the mask spares the shift its range check
		hits |= uint32(b2u(s1 >= 0)&b2u(!(s2 < 0))&b2u(!(s3 < 0))&b2u(p.node != ia)&b2u(p.node != ic)) << (uint(i) & 31)
	}
	for hits != 0 {
		p := &items[lowestBit(uint64(hits))]
		hits &= hits - 1
		if cross(a.x, a.y, b.x, b.y, p.x, p.y) < 0 || cross(b.x, b.y, c.x, c.y, p.x, p.y) < 0 || cross(c.x, c.y, a.x, a.y, p.x, p.y) < 0 {
			continue
		}
		if (p.x != a.x || p.y != a.y) && (p.x != b.x || p.y != b.y) && (p.x != c.x || p.y != c.y) {
			return false
		}
	}
	return true
}

// Bucket variant: a branch-free bounding box pass, then exact tests on the few hits.
func emptyTriangleBox[I Index](items []item, begin, end uint32, a, b, c *node[I], ia, ic uint32) bool {
	minX, maxX := minf(a.x, minf(b.x, c.x)), maxf(a.x, maxf(b.x, c.x))
	minY, maxY := minf(a.y, minf(b.y, c.y)), maxf(a.y, maxf(b.y, c.y))
	for base := begin; base < end; base += 64 {
		stop := min(end, base+64)
		mask := uint64(0)
		for i := base; i < stop; i++ {
			p := &items[i]
			in := uint64(b2u(p.x >= minX) & b2u(p.x <= maxX) & b2u(p.y >= minY) & b2u(p.y <= maxY) &
				b2u(p.node != ia) & b2u(p.node != ic))
			mask |= in << (i - base)
		}
		for mask != 0 {
			p := &items[base+lowestBit(mask)]
			mask &= mask - 1
			if cross(a.x, a.y, b.x, b.y, p.x, p.y) < 0 || cross(b.x, b.y, c.x, c.y, p.x, p.y) < 0 || cross(c.x, c.y, a.x, a.y, p.x, p.y) < 0 {
				continue
			}
			if (p.x != a.x || p.y != a.y) && (p.x != b.x || p.y != b.y) && (p.x != c.x || p.y != c.y) {
				return false
			}
		}
	}
	return true
}

// isEar is split in two so that its quick answers inline: a reflex corner, no blockers left.
func (t *Triangulator[I]) isEar(v uint32) bool {
	b := &t.nodes[v]
	if b.reflex != 0 {
		return false
	}
	return t.live == 0 || t.emptyEar(b)
}

func (t *Triangulator[I]) emptyEar(b *node[I]) bool {
	n := t.nodes
	ia, ic := b.prev, b.next
	a, c := &n[ia], &n[ic]
	if t.buckets <= 1 {
		return emptyTriangle(t.items, 0, t.live, a, b, c, ia, ic)
	}
	var lo, hi float64
	if t.axisY {
		lo, hi = minf(a.y, minf(b.y, c.y)), maxf(a.y, maxf(b.y, c.y))
	} else {
		lo, hi = minf(a.x, minf(b.x, c.x)), maxf(a.x, maxf(b.x, c.x))
	}
	start := t.bucketStart
	return emptyTriangleBox(t.items, start[t.bucket(lo)], start[t.bucket(hi)+1], a, b, c, ia, ic)
}

// Updates the corner flag of node v, dropping it from the blockers once it turns convex.
func (t *Triangulator[I]) refreshCorner(v, prev, next *node[I], id uint32) {
	reflex := crossN(prev, v, next) <= 0
	if v.flag != 0 && !reflex {
		t.dropBlocker(v, id)
	}
	v.reflex = b2u(reflex)
}

// Removes node id from the blocker index: swapped out of a single bucket, marked gone in several.
func (t *Triangulator[I]) dropBlocker(v *node[I], id uint32) {
	v.flag = 0
	items := t.items
	i := t.slot[id]
	t.live--
	if t.buckets <= 1 {
		items[i] = items[t.live]
		t.slot[items[i].node] = i
	} else {
		items[i].x = math.Inf(1)
		if t.live*2 < t.built {
			t.rebuildIndex()
		}
	}
}

// Rebuilds the buckets from the live items once half of them are gone.
func (t *Triangulator[I]) rebuildIndex() {
	blockers := t.blockers
	found := uint32(0)
	items := t.items
	slot := t.slot
	inf := math.Inf(1)
	for i := uint32(0); i < t.built; i++ {
		if items[i].x != inf {
			blockers[found] = items[i].node
			items[found] = items[i]
			slot[items[i].node] = found
			found++
		}
	}
	t.blockerCount = found
	t.buildIndex()
}

// Unlinks node v and refreshes the corners of its neighbors.
func (t *Triangulator[I]) unlink(v uint32) {
	n := t.nodes
	a, c := n[v].prev, n[v].next
	na, nc := &n[a], &n[c]
	na.next = c
	nc.prev = a
	t.refreshCorner(na, &n[na.prev], nc, a)
	t.refreshCorner(nc, na, &n[nc.next], c)
}

// Drops repeated points and zero-area corners from the boundary through v; returns the vertex count.
func (t *Triangulator[I]) cleanup(v, count uint32) (uint32, uint32) {
	n := t.nodes
	since := uint32(0)
	for count >= 3 && since < count {
		node := &n[v]
		if crossN(&n[node.prev], node, &n[node.next]) == 0 {
			prev := node.prev
			t.unlink(v)
			count--
			v = prev
			since = 0
		} else {
			v = node.next
			since++
		}
	}
	if count >= 3 {
		t.classifyCycle(v)
		t.buildIndex()
	}
	return v, count
}

// Blockers in the closed triangle a, b, c away from its corners, counted over the whole index.
func (t *Triangulator[I]) blockersInside(a, b, c *node[I], ia, ic uint32) uint32 {
	end := t.built
	if t.buckets <= 1 {
		end = t.live
	}
	items := t.items[:end]
	hits := uint32(0)
	for i := range items {
		p := &items[i]
		if p.node == ia || p.node == ic || cross(a.x, a.y, b.x, b.y, p.x, p.y) < 0 || cross(b.x, b.y, c.x, c.y, p.x, p.y) < 0 ||
			cross(c.x, c.y, a.x, a.y, p.x, p.y) < 0 {
			continue
		}
		if (p.x != a.x || p.y != a.y) && (p.x != b.x || p.y != b.y) && (p.x != c.x || p.y != c.y) {
			hits++
		}
	}
	return hits
}

// Cuts ears from the boundary through v into zig-zag strips; false when no ear is left even after the cleanup and
// forced cuts are off.
func (t *Triangulator[I]) clip(v, count uint32) bool {
	if count < 3 {
		return true
	}
	t.buildIndex()
	n := t.nodes
	stall := uint32(0)
	cleaned := false
	for count >= 3 {
		if !t.isEar(v) {
			v = n[v].next
			stall++
			if stall < count {
				continue
			}
			if cleaned {
				if !t.careful {
					return false
				}
				v = t.forcedCut(v, count)
				count--
				stall = 0
				cleaned = false
				continue
			}
			v, count = t.cleanup(v, count)
			cleaned = true
			stall = 0
			continue
		}
		stall = 0
		cleaned = false
		a, b := n[v].prev, n[v].next
		t.unlink(v)
		count--
		t.put(n[v].index)
		var sideA, firstA bool
		switch {
		case count >= 3 && t.isEar(a):
			t.put(n[a].index)
			t.put(n[b].index)
			sideA, firstA = true, true
		case count >= 3 && t.isEar(b):
			t.put(n[b].index)
			t.put(n[a].index)
			sideA, firstA = false, false
		default:
			t.put(n[b].index)
			t.put(n[a].index)
			t.endStrip(false)
			v = n[b].next
			continue
		}
		// Zig-zag: the next ear alternates between the two ends of the last cut.
		for {
			if sideA {
				z := n[a].prev
				t.put(n[z].index)
				t.unlink(a)
				a = z
			} else {
				z := n[b].next
				t.put(n[z].index)
				t.unlink(b)
				b = z
			}
			count--
			sideA = !sideA
			next := b
			if sideA {
				next = a
			}
			if count < 3 || !t.isEar(next) {
				break
			}
		}
		// v, a, b runs clockwise when the strip went on at a first
		t.endStrip(firstA)
		if sideA {
			v = b
		} else {
			v = a
		}
	}
	return true
}
