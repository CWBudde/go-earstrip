package earstrip

// Finds two boundary edges through v that cross properly, a -> b and c -> d in walking order. Quadratic in the
// boundary, which is why only small boundaries are split.
func (t *Triangulator[I]) findCrossing(v, count uint32) (ia, ib, ic, id uint32, ok bool) {
	if count > splitLimit {
		return 0, 0, 0, 0, false
	}
	n := t.nodes
	t.cycle = t.cycle[:0]
	w := v
	for i := uint32(0); i < count; i, w = i+1, n[w].next {
		t.cycle = append(t.cycle, w)
	}
	cycle := t.cycle
	for i := uint32(0); i+2 < count; i++ {
		a, b := &n[cycle[i]], &n[cycle[i+1]]
		minX, maxX, minY, maxY := minf(a.x, b.x), maxf(a.x, b.x), minf(a.y, b.y), maxf(a.y, b.y)
		for j := i + 2; j < count; j++ {
			if i == 0 && j+1 == count {
				continue // the edge into a
			}
			dj := j + 1
			if dj == count {
				dj = 0
			}
			c, d := &n[cycle[j]], &n[cycle[dj]]
			if maxf(c.x, d.x) < minX || minf(c.x, d.x) > maxX || maxf(c.y, d.y) < minY || minf(c.y, d.y) > maxY {
				continue
			}
			if crossN(a, b, c)*crossN(a, b, d) < 0 && crossN(c, d, a)*crossN(c, d, b) < 0 {
				return cycle[i], cycle[i+1], cycle[j], cycle[dj], true
			}
		}
	}
	return 0, 0, 0, 0, false
}

func (t *Triangulator[I]) cycleLength(v uint32) uint32 {
	n := t.nodes
	count, w := uint32(0), v
	for {
		count++
		w = n[w].next
		if w == v {
			break
		}
	}
	return count
}

// Turns the cycle through v counter-clockwise; false when it has no area.
func (t *Triangulator[I]) orient(v uint32) bool {
	n := t.nodes
	area := 0.0
	w := v
	for {
		p := &n[w]
		q := &n[p.next]
		area += float64(p.x*q.y) - float64(q.x*p.y)
		w = p.next
		if w == v {
			break
		}
	}
	if area < 0 {
		w = v
		for {
			p := &n[w]
			p.prev, p.next = p.next, p.prev
			w = p.prev
			if w == v {
				break
			}
		}
	}
	return area != 0
}

// Even-odd test of point p against the rings as loaded, before any bridge or cut.
func (t *Triangulator[I]) insideRings(px, py float64) bool {
	n := t.nodes
	inside := false
	ring := func(first, count uint32) {
		for i, j := first, first+count-1; i < first+count; j, i = i, i+1 {
			a, b := &n[j], &n[i]
			if (a.y > py) != (b.y > py) && px < (b.x-a.x)*(py-a.y)/(b.y-a.y)+a.x {
				inside = !inside
			}
		}
	}
	ring(0, t.outerCount)
	for k := range t.holes {
		ring(t.holes[k].first, t.holes[k].count)
	}
	return inside
}

// A point just inside the counter-clockwise loop through v, left of the middle of its longest edge.
func (t *Triangulator[I]) pointInside(v uint32) (px, py float64) {
	n := t.nodes
	best, w := v, v
	bestLen := -1.0
	for {
		p := &n[w]
		q := &n[p.next]
		length := float64((q.x-p.x)*(q.x-p.x)) + float64((q.y-p.y)*(q.y-p.y))
		if length > bestLen {
			bestLen = length
			best = w
		}
		w = p.next
		if w == v {
			break
		}
	}
	p := &n[best]
	q := &n[p.next]
	const e = 1e-4
	px = (p.x+q.x)/2 - float64((q.y-p.y)*e)
	py = (p.y+q.y)/2 + float64((q.x-p.x)*e)
	return px, py
}

// Covers the lobe a loop of two vertices u -> w stands for: the lobe runs from u to w through the crossing x, which
// lies on the edge s0 -> s1 at ts and on t0 -> t1 at tt; of the two triangles of real vertices around it, the one with
// the smaller extra wedge is cut.
func (t *Triangulator[I]) lobe(u, w, extraA uint32, wedgeA float64, extraB uint32, wedgeB float64) {
	n := t.nodes
	third := extraB
	if wedgeA <= wedgeB {
		third = extraA
	}
	ccw := crossN(&n[u], &n[w], &n[third]) > 0
	t.put(n[u].index)
	if ccw {
		t.put(n[w].index)
		t.put(n[third].index)
	} else {
		t.put(n[third].index)
		t.put(n[w].index)
	}
	t.endStrip(false)
}

// Second run of a polygon the ears could not finish: the boundary is cut at every crossing into loops, a -> b
// crossing c -> d giving a -> d ... and b ... c -> b. Each loop the rings fill (even-odd, so both lobes of a figure
// eight and not a hole) is turned counter-clockwise and clipped on its own; a loop of two vertices becomes the
// triangle that covers its lobe best.
func (t *Triangulator[I]) clipLoops(v, count uint32) bool {
	n := t.nodes
	t.loops = append(t.loops[:0], v, count)
	for len(t.loops) > 0 {
		k := len(t.loops)
		c, s := t.loops[k-1], t.loops[k-2]
		t.loops = t.loops[:k-2]
		if c < 3 {
			continue
		}
		if ia, ib, ic, id, ok := t.findCrossing(s, c); ok {
			// the crossing lies on a -> b at t and on c -> d at u
			ca, cb := crossN(&n[ic], &n[id], &n[ia]), crossN(&n[ic], &n[id], &n[ib])
			cc, cd := crossN(&n[ia], &n[ib], &n[ic]), crossN(&n[ia], &n[ib], &n[id])
			tt, u := ca/(ca-cb), cc/(cc-cd)
			xx, xy := n[ia].x+float64(tt*(n[ib].x-n[ia].x)), n[ia].y+float64(tt*(n[ib].y-n[ia].y))
			n[ia].next = id
			n[id].prev = ia
			n[ic].next = ib
			n[ib].prev = ic
			t.forced = true
			la, lb := t.cycleLength(ia), t.cycleLength(ib)
			if la == 2 && t.insideLobe(ia, id, xx, xy) {
				t.lobe(id, ia, ic, u, ib, 1-tt)
			}
			if lb == 2 && t.insideLobe(ib, ic, xx, xy) {
				t.lobe(ib, ic, ia, tt, id, 1-u)
			}
			t.loops = append(t.loops, ia, la, ib, lb)
			continue
		}
		if !t.orient(s) {
			continue
		}
		px, py := t.pointInside(s)
		if !t.insideRings(px, py) {
			continue
		}
		t.clip(s, t.classifyCycle(s))
	}
	return true
}

// True when the lobe u, w and the crossing x is filled by the rings, tested at its centroid.
func (t *Triangulator[I]) insideLobe(u, w uint32, xx, xy float64) bool {
	n := t.nodes
	return t.insideRings((n[u].x+n[w].x+xx)/3, (n[u].y+n[w].y+xy)/3)
}

// No ear is left: cuts the convex corner with the fewest blockers in its triangle (the smallest one on a tie),
// or the corner at v when none is convex, and continues after it.
func (t *Triangulator[I]) forcedCut(v, count uint32) uint32 {
	n := t.nodes
	best, bestHits, w := v, uint32(none), v
	bestArea := 0.0
	for i := uint32(0); i < count; i, w = i+1, n[w].next {
		b := &n[w]
		a, c := &n[b.prev], &n[b.next]
		area := crossN(a, b, c)
		if area <= 0 {
			continue
		}
		hits := t.blockersInside(a, b, c, b.prev, b.next)
		if hits < bestHits || (hits == bestHits && area < bestArea) {
			best = w
			bestHits = hits
			bestArea = area
		}
	}
	a, b := n[best].prev, n[best].next
	t.unlink(best)
	t.put(n[best].index)
	t.put(n[b].index)
	t.put(n[a].index)
	t.endStrip(false)
	t.forced = true
	return b
}
