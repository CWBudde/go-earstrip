package earstrip

import "math/bits"

// The mesh of detail::Constrained<Index>: the Delaunay triangulation of the vertices, the inserted ring segments,
// the added points, the fill and the strips.

// ---------- mesh ----------

// multipleOf3 is 1 when x % 3 == 0, else 0, without a branch: the branch of a test of e % 3 goes either way at random
// and is often mispredicted. x * 0xAAAAAAAAAAAAAAAB (the inverse of 3 modulo 2^64) is at most 0x5555555555555555
// exactly for the multiples of 3, which the borrow of subtracting 0x5555555555555556 tells.
func multipleOf3(x uint64) uint64 {
	_, borrow := bits.Sub64(x*0xAAAAAAAAAAAAAAAB, 0x5555555555555556, 0)
	return borrow
}

// nextEdge and prevEdge as e % 3 == 2 ? e - 2 : e + 1 and e % 3 == 0 ? e + 2 : e - 1, for every e: e % 3 == 2 is
// (e + 1) % 3 == 0, taken in 64 bits so that e + 1 does not wrap.
func nextEdge(e uint32) uint32 {
	n := uint64(e) + 1
	return uint32(n - 3*multipleOf3(n))
}

func prevEdge(e uint32) uint32 {
	return e - 1 + 3*uint32(multipleOf3(uint64(e)))
}

func (c *constrained[I]) newTriangle(a, b, cc uint32) uint32 {
	e := 3 * c.triangleCount
	c.triangleCount++
	c.org[e] = a
	c.org[e+1] = b
	c.org[e+2] = cc
	c.twin[e], c.twin[e+1], c.twin[e+2] = none, none, none
	c.wind[e], c.wind[e+1], c.wind[e+2] = wind{}, wind{}, wind{}
	return e
}

func (c *constrained[I]) link(e, f uint32) {
	c.twin[e] = f
	if f != none {
		c.twin[f] = e
	}
}

func (c *constrained[I]) addWind(e, from uint32, w wind) {
	if c.org[e] != from {
		w = w.neg()
	}
	c.wind[e] = c.wind[e].add(w)
	if c.twin[e] != none {
		c.wind[c.twin[e]] = c.wind[e].neg()
	}
}

// findOut calls f with the half-edges leaving v until it returns true; returns that half-edge or none.
func (c *constrained[I]) findOut(v uint32, f func(e uint32) bool) uint32 {
	e0 := c.vertexEdge[v]
	e := e0
	for {
		if f(e) {
			return e
		}
		t := c.twin[prevEdge(e)]
		if t == none {
			break
		}
		if t == e0 {
			return none
		}
		e = t
	}
	// on the boundary: the other way round
	e = e0
	for c.twin[e] != none {
		e = nextEdge(c.twin[e])
		if f(e) {
			return e
		}
	}
	return none
}

// findEdge returns half-edge a -> b, or b -> a when that one has no twin, or none.
func (c *constrained[I]) findEdge(a, b uint32) uint32 {
	rv := uint32(none)
	c.findOut(a, func(e uint32) bool {
		if c.org[nextEdge(e)] == b {
			rv = e
			return true
		}
		if p := prevEdge(e); c.org[p] == b {
			if c.twin[p] != none {
				rv = c.twin[p]
			} else {
				rv = p
			}
			return true
		}
		return false
	})
	return rv
}

// boundaryOut returns the boundary half-edge leaving v, or none inside.
func (c *constrained[I]) boundaryOut(v uint32) uint32 {
	e0 := c.vertexEdge[v]
	e := e0
	for c.twin[e] != none {
		e = nextEdge(c.twin[e])
		if e == e0 {
			return none
		}
	}
	return e
}

// nextBoundary and prevBoundary return the boundary half-edges after and before boundary half-edge e.
func (c *constrained[I]) nextBoundary(e uint32) uint32 {
	g := nextEdge(e)
	for c.twin[g] != none {
		g = nextEdge(c.twin[g])
	}
	return g
}

func (c *constrained[I]) prevBoundary(e uint32) uint32 {
	g := prevEdge(e)
	for c.twin[g] != none {
		g = prevEdge(c.twin[g])
	}
	return g
}

// flip replaces the diagonal e of a convex quad by the other one. With e = a -> b, c its apex and d the apex of its
// twin, e's triangle becomes a, d, c and the twin's becomes b, c, d; the edges c -> a and d -> b stay in place.
func (c *constrained[I]) flip(e uint32) {
	// Local copies of the slices: Go would load them from c again after every store. link is written out for the
	// same reason.
	org, twin, wnd := c.org, c.twin, c.wind
	f := twin[e]
	e1, e2, f1, f2 := nextEdge(e), prevEdge(e), nextEdge(f), prevEdge(f)
	a, b, cc, d := org[e], org[f], org[e2], org[f2]
	te, tf := twin[e1], twin[f1]
	we, wf := wnd[e1], wnd[f1]
	org[e1] = d
	org[f1] = cc
	// link(e, tf)
	twin[e] = tf
	if tf != none {
		twin[tf] = e
	}
	wnd[e] = wf
	// link(f, te)
	twin[f] = te
	if te != none {
		twin[te] = f
	}
	wnd[f] = we
	// link(e1, f1)
	twin[e1] = f1
	twin[f1] = e1
	wnd[e1], wnd[f1] = wind{}, wind{}
	if tf == none {
		c.hullEdge[a] = e
	}
	if te == none {
		c.hullEdge[b] = f
	}
	vertexEdge := c.vertexEdge
	vertexEdge[a] = e
	vertexEdge[b] = f
	vertexEdge[cc] = e2
	vertexEdge[d] = f2
}

// legalize flips the edges on the stack until the triangles beside them are locally Delaunay; each edge on the stack
// has the new point as the apex of its triangle. Edges the fill rule sees stay.
func (c *constrained[I]) legalize() {
	// Local copies of the slices, as in flip; flip does not change them, nor the triangle count.
	org, twin, wnd, stack := c.org, c.twin, c.wind, c.stack
	limit := 64*c.triangleCount + 1024
	// flips only improve, so this ends; the guard keeps a bad float predicate from looping
	for flips := uint32(0); len(stack) > 0 && flips <= limit; {
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		f := twin[e]
		if f == none || significant(wnd[e]) || !c.inCircle(org[e], org[nextEdge(e)], org[prevEdge(e)], org[prevEdge(f)]) {
			continue
		}
		c.flip(e)
		flips++
		stack = append(stack, e, prevEdge(f))
	}
	c.stack = stack[:0]
}

// ---------- Delaunay triangulation of the vertices ----------

// sweep adds the vertices in sorted order, each outside the hull of the previous ones; false when all are collinear.
func (c *constrained[I]) sweep() bool {
	n := c.vertexCount
	k := uint32(2)
	for k < n && c.orient(0, 1, k) == 0 {
		k++
	}
	if k == n {
		return false
	}
	// fan from vertex k over the collinear run 0 .. k - 1, the run on the hull
	left := c.orient(0, 1, k) > 0
	for i := uint32(0); i+1 < k; i++ {
		if left {
			e := c.newTriangle(i, i+1, k)
			if i > 0 {
				c.link(e+2, e-2)
			}
			c.vertexEdge[i] = e
		} else {
			e := c.newTriangle(i+1, i, k)
			if i > 0 {
				c.link(e+1, e-1)
			}
			c.vertexEdge[i] = e + 1
		}
	}
	last := 3 * (k - 2)
	if left {
		for i := uint32(0); i+1 < k; i++ {
			c.setHull(i, i+1, 3*i)
		}
		c.setHull(k-1, k, last+1)
		c.setHull(k, 0, 2)
		c.vertexEdge[k-1] = last + 1
		c.vertexEdge[k] = 2
	} else {
		for i := uint32(0); i+1 < k; i++ {
			c.setHull(i+1, i, 3*i)
		}
		c.setHull(0, k, 1)
		c.setHull(k, k-1, last+2)
		c.vertexEdge[k-1] = last
		c.vertexEdge[k] = last + 2
	}
	c.stack = c.stack[:0]
	for q := k + 1; q < n; q++ {
		c.addRight(q, q-1)
	}
	return true
}

func (c *constrained[I]) setHull(from, to, e uint32) {
	c.hullNext[from] = to
	c.hullPrev[to] = from
	c.hullEdge[from] = e
}

// addRight joins vertex q to the hull edges it sees, starting next to the previous vertex p.
func (c *constrained[I]) addRight(q, p uint32) {
	lo, hi := p, p
	for c.orient(c.hullPrev[lo], lo, q) < 0 {
		lo = c.hullPrev[lo]
	}
	for c.orient(hi, c.hullNext[hi], q) < 0 {
		hi = c.hullNext[hi]
	}
	prev := uint32(none)
	for u := lo; u != hi; {
		w := c.hullNext[u]
		e := c.newTriangle(w, u, q)
		c.link(e, c.hullEdge[u])
		c.link(e+1, prev)
		if prev == none {
			c.setHull(lo, q, e+1)
		}
		prev = e + 2
		c.stack = append(c.stack, e)
		u = w
	}
	c.setHull(q, hi, prev)
	c.vertexEdge[q] = prev
	c.legalize()
}

// ---------- constraints ----------

// insertSegment makes the segment a -> b a chain of edges and adds its winding; a crossing with an edge the rule
// sees adds a vertex at the rounded crossing point and reroutes both through it.
func (c *constrained[I]) insertSegment(a, b uint32, w wind) {
	c.pending = c.pending[:0]
	c.pending = append(c.pending, pending{a, b, w})
	for len(c.pending) > 0 {
		s := c.pending[len(c.pending)-1]
		c.pending = c.pending[:len(c.pending)-1]
		if s.a == s.b {
			continue
		}
		h := c.findEdge(s.a, s.b)
		if h != none {
			c.addWind(h, s.a, s.wind)
			continue
		}
		hit := uint32(none)
		switch c.walk(s.a, s.b, &hit) {
		case walkTarget:
			c.makeEdge(s.a, s.b)
		case walkVertex:
			c.makeEdge(s.a, hit)
			c.pending = append(c.pending, pending{hit, s.b, s.wind})
		case walkCrossing:
			c.resolve(s, hit)
			continue
		case walkFailed:
			// assert(false)
			continue
		}
		to := hit
		if hit == none {
			to = s.b
		}
		h = c.findEdge(s.a, to)
		// assert(h != none)
		if h != none {
			c.addWind(h, s.a, s.wind)
		}
	}
}

// walk walks from vertex a towards vertex b listing the crossed edges; stops at b, at a vertex on the way (hit) or
// at a crossed edge the rule sees (hit = that half-edge).
func (c *constrained[I]) walk(a, b uint32, hit *uint32) walkEnd {
	c.crossed = c.crossed[:0]
	va, vb := c.vertices[a], c.vertices[b]
	h := uint32(none)
	c.findOut(a, func(e uint32) bool {
		x, y := c.org[nextEdge(e)], c.org[prevEdge(e)]
		vx, vy := c.vertices[x], c.vertices[y]
		ox := c.orientV(va, vx, vb)
		if ox == 0 && sameDirection(va, vx, vb) {
			*hit = x
			return true
		}
		oy := c.orientV(va, vy, vb)
		if oy == 0 && sameDirection(va, vy, vb) {
			*hit = y
			return true
		}
		if ox > 0 && oy < 0 {
			h = nextEdge(e)
			return true
		}
		return false
	})
	if *hit != none {
		return walkVertex
	}
	if h == none {
		return walkFailed
	}
	// h runs from the right of a -> b to its left; a straight walk crosses each triangle once
	for step := uint32(0); step <= c.triangleCount; step++ {
		if significant(c.wind[h]) {
			*hit = h
			return walkCrossing
		}
		c.crossed = append(c.crossed, edgePair{c.org[h], c.org[nextEdge(h)]})
		t := c.twin[h]
		if t == none {
			return walkFailed
		}
		z := c.org[prevEdge(t)]
		if z == b {
			return walkTarget
		}
		o := c.orientV(va, vb, c.vertices[z])
		if o == 0 {
			*hit = z
			return walkVertex
		}
		if o > 0 {
			h = nextEdge(t)
		} else {
			h = prevEdge(t)
		}
	}
	// assert(false)
	return walkFailed
}

// makeEdge flips the crossed edges until a -> b is an edge: a convex quad gets the other diagonal, which is queued
// again while it still crosses; a concave one waits.
func (c *constrained[I]) makeEdge(a, b uint32) {
	va, vb := c.vertices[a], c.vertices[b]
	head := 0
	limit := 64 + 8*len(c.crossed)*len(c.crossed)
	for step := 0; head < len(c.crossed) && step < limit; step++ {
		p := c.crossed[head]
		head++
		u, v := p.u, p.v
		g := c.findEdge(u, v)
		if g == none || c.twin[g] == none {
			// assert(false)
			continue
		}
		cc, d := c.org[prevEdge(g)], c.org[prevEdge(c.twin[g])]
		if c.orient(cc, d, u)*c.orient(cc, d, v) < 0 {
			c.flip(g)
			if c.orientV(va, vb, c.vertices[cc])*c.orientV(va, vb, c.vertices[d]) < 0 {
				c.crossed = append(c.crossed, edgePair{cc, d})
			}
		} else {
			c.crossed = append(c.crossed, edgePair{u, v})
		}
	}
	// assert(head == len(c.crossed))
}

// resolve handles segment s crossing edge h: both go through a vertex at the rounded crossing point.
func (c *constrained[I]) resolve(s pending, h uint32) {
	c.work++
	if c.work > c.workLimit {
		c.fail()
		return
	}
	r, l := c.org[h], c.org[nextEdge(h)]
	w := c.wind[h]
	c.wind[h] = wind{}
	if c.twin[h] != none {
		c.wind[c.twin[h]] = wind{}
	}
	x := c.insertPoint(c.crossing(s.a, s.b, r, l), r)
	if x == none {
		c.fail()
		return
	}
	c.pending = append(c.pending, pending{x, l, w}, pending{r, x, w}, pending{x, s.b, s.wind}, pending{s.a, x, s.wind})
}

func (c *constrained[I]) fail() {
	c.failed = true
	c.pending = c.pending[:0]
}

// ---------- added points ----------

func (c *constrained[I]) insertPoint(p cdtVertex, from uint32) uint32 {
	where := uint32(none)
	loc := c.locate(p, from, &where)
	if loc == locVertex {
		return where
	}
	if c.pointCount+c.numExtra >= c.maxPoints() {
		return none
	}
	x := c.vertexCount
	c.vertexCount++
	c.reserveVertices(c.vertexCount)
	c.vertices[x] = p
	c.outIndex[x] = I(c.pointCount + c.numExtra)
	c.numExtra++
	c.stack = c.stack[:0]
	switch loc {
	case locTriangle:
		c.splitTriangle(where, x)
	case locEdge:
		c.splitEdge(where, x)
	default:
		c.extendHull(where, x)
	}
	c.legalize()
	return x
}

// locate finds point p walking straight from vertex s: a vertex, an edge or a triangle holding it (where), or
// outside beyond a boundary edge it sees.
func (c *constrained[I]) locate(p cdtVertex, s uint32, where *uint32) location {
	for guard := uint32(0); guard <= c.vertexCount; guard++ {
		vs := c.vertices[s]
		if vs.x == p.x && vs.y == p.y {
			*where = s
			return locVertex
		}
		h, next := uint32(none), uint32(none)
		done := false
		found := locVertex
		onRay := func(v, edge uint32) {
			switch vv := c.vertices[v]; {
			case vv.x == p.x && vv.y == p.y:
				*where = v
				found = locVertex
				done = true
			case nearer(vs, p, vv):
				*where = edge
				found = locEdge
				done = true
			default:
				next = v
			}
		}
		c.findOut(s, func(e uint32) bool {
			x, y := c.org[nextEdge(e)], c.org[prevEdge(e)]
			vx, vy := c.vertices[x], c.vertices[y]
			ox := c.orientV(vs, vx, p)
			if ox == 0 && sameDirection(vs, vx, p) {
				onRay(x, e)
				return true
			}
			oy := c.orientV(vs, vy, p)
			if oy == 0 && sameDirection(vs, vy, p) {
				onRay(y, prevEdge(e))
				return true
			}
			if ox > 0 && oy < 0 {
				switch o := c.orientV(vx, vy, p); {
				case o > 0:
					*where = e / 3
					found = locTriangle
					done = true
				case o == 0:
					*where = nextEdge(e)
					found = locEdge
					done = true
				default:
					h = nextEdge(e)
				}
				return true
			}
			return false
		})
		if done {
			return found
		}
		if next != none {
			s = next
			continue
		}
		if h == none {
			// outside, in the outer angle of hull vertex s
			out := c.boundaryOut(s)
			if out != none {
				if c.orientV(vs, c.vertices[c.org[nextEdge(out)]], p) < 0 {
					*where = out
					return locOutside
				}
				in := c.prevBoundary(out)
				if c.orientV(c.vertices[c.org[in]], vs, p) < 0 {
					*where = in
					return locOutside
				}
			}
			// assert(false)
			*where = s
			return locVertex
		}
		// h runs from the right of s -> p to its left
		for step := uint32(0); ; step++ {
			if step > c.triangleCount {
				// assert(false)
				*where = s
				return locVertex
			}
			t := c.twin[h]
			if t == none {
				*where = h
				return locOutside
			}
			l, r, z := c.org[t], c.org[nextEdge(t)], c.org[prevEdge(t)]
			vz := c.vertices[z]
			o1, o2 := c.orientV(c.vertices[r], vz, p), c.orientV(vz, c.vertices[l], p)
			if o1 >= 0 && o2 >= 0 {
				if o1 > 0 && o2 > 0 {
					*where = t / 3
					return locTriangle
				}
				if o1 == 0 && o2 == 0 {
					*where = z
					return locVertex
				}
				if o1 == 0 {
					*where = nextEdge(t)
				} else {
					*where = prevEdge(t)
				}
				return locEdge
			}
			oz := c.orientV(vs, p, vz)
			if oz == 0 {
				s = z
				break
			}
			if oz > 0 {
				h = nextEdge(t)
			} else {
				h = prevEdge(t)
			}
		}
	}
	// assert(false)
	*where = s
	return locVertex
}

// splitTriangle splits triangle t into three around the new vertex x.
func (c *constrained[I]) splitTriangle(t, x uint32) {
	e0 := 3 * t
	e1, e2 := e0+1, e0+2
	a, b, cc := c.org[e0], c.org[e1], c.org[e2]
	t1, t2 := c.twin[e1], c.twin[e2]
	w1, w2 := c.wind[e1], c.wind[e2]
	g := c.newTriangle(b, cc, x)
	k := c.newTriangle(cc, a, x)
	c.org[e2] = x
	c.twin[e1], c.twin[e2] = none, none
	c.wind[e1], c.wind[e2] = wind{}, wind{}
	// t: a -> b -> x, g: b -> c -> x, k: c -> a -> x
	c.link(g, t1)
	c.wind[g] = w1
	c.link(k, t2)
	c.wind[k] = w2
	c.link(e1, g+2)
	c.link(g+1, k+2)
	c.link(k+1, e2)
	c.vertexEdge[a] = e0
	c.vertexEdge[b] = g
	c.vertexEdge[cc] = k
	c.vertexEdge[x] = e2
	c.stack = append(c.stack, e0, g, k)
}

// splitEdge splits the edge of half-edge h and the triangles beside it at the new vertex x; both halves keep its
// winding.
func (c *constrained[I]) splitEdge(h, x uint32) {
	h1, h2, f := nextEdge(h), prevEdge(h), c.twin[h]
	a, b, cc := c.org[h], c.org[h1], c.org[h2]
	th1 := c.twin[h1]
	wh1, w := c.wind[h1], c.wind[h]
	// h's triangle becomes a -> x -> c, a new one x -> b -> c
	g := c.newTriangle(x, b, cc)
	c.org[h1] = x
	c.link(g+1, th1)
	c.wind[g+1] = wh1
	c.link(h1, g+2)
	c.wind[h1] = wind{}
	c.wind[g] = w
	c.vertexEdge[a] = h
	c.vertexEdge[b] = g + 1
	c.vertexEdge[cc] = h2
	c.vertexEdge[x] = h1
	c.stack = append(c.stack, h2, g+1)
	if f == none {
		c.twin[h] = none
		c.twin[g] = none
		return
	}
	// f's triangle b -> a -> d becomes b -> x -> d, a new one x -> a -> d
	f1, f2 := nextEdge(f), prevEdge(f)
	d := c.org[f2]
	tf1 := c.twin[f1]
	wf1 := c.wind[f1]
	k := c.newTriangle(x, a, d)
	c.org[f1] = x
	c.link(k+1, tf1)
	c.wind[k+1] = wf1
	c.link(f1, k+2)
	c.wind[f1] = wind{}
	c.wind[k] = w.neg()
	c.link(h, k)
	c.link(g, f)
	c.vertexEdge[d] = f2
	c.stack = append(c.stack, f2, k+1)
}

// extendHull joins the new vertex x, outside the hull, to the boundary edges it sees around boundary half-edge h.
func (c *constrained[I]) extendHull(h, x uint32) {
	vx := c.vertices[x]
	sees := func(e uint32) bool {
		return c.orientV(c.vertices[c.org[e]], c.vertices[c.org[nextEdge(e)]], vx) < 0
	}
	first := h
	for e := c.prevBoundary(first); e != h && sees(e); e = c.prevBoundary(e) {
		first = e
	}
	c.chain = c.chain[:0]
	e := first
	for {
		c.chain = append(c.chain, e)
		e = c.nextBoundary(e)
		if e == first || !sees(e) {
			break
		}
	}
	prev := uint32(none)
	for _, edge := range c.chain {
		u, w := c.org[edge], c.org[nextEdge(edge)]
		g := c.newTriangle(w, u, x)
		c.link(g, edge)
		c.wind[g] = c.wind[edge].neg()
		c.link(g+1, prev)
		prev = g + 2
		c.vertexEdge[u] = g + 1
		c.stack = append(c.stack, g)
	}
	c.vertexEdge[x] = prev
}

// ---------- fill ----------

// fill sets the winding numbers of the triangles, from the outside in, and the kept ones.
func (c *constrained[I]) fill() {
	triangles := c.triangleCount
	if uint32(len(c.triangleWind)) < triangles {
		c.triangleWind = cdtResize(c.triangleWind, max(int(triangles), 2*len(c.triangleWind)))
	}
	c.state = cdtResize(c.state, int(triangles))
	clear(c.state)
	start := uint32(none)
	for e := uint32(0); e < 3*triangles; e++ {
		if c.twin[e] == none {
			start = e
			break
		}
	}
	if start == none {
		return
	}
	c.triangleWind[start/3] = c.wind[start]
	c.state[start/3] = triSeen
	c.stack = c.stack[:0]
	c.stack = append(c.stack, start/3)
	for len(c.stack) > 0 {
		t := c.stack[len(c.stack)-1]
		c.stack = c.stack[:len(c.stack)-1]
		w := c.triangleWind[t]
		for e := 3 * t; e < 3*t+3; e++ {
			f := c.twin[e]
			if f == none {
				// assert(sameForRule(w, c.wind[e])): equal outer and holes
				continue
			}
			n := f / 3
			nw := w.add(c.wind[f])
			if c.state[n] == triUnseen {
				c.state[n] = triSeen
				c.triangleWind[n] = nw
				c.stack = append(c.stack, n)
			}
			// else assert(sameForRule(c.triangleWind[n], nw))
		}
	}
	for t := uint32(0); t < triangles; t++ {
		if keep(c.triangleWind[t]) {
			c.state[t] = triFree
		} else {
			c.state[t] = triDropped
		}
	}
}

// ---------- strips ----------

// buildStrips covers the kept triangles with strips. A strip starts at a triangle with the fewest free neighbors,
// takes the longest of its zig-zag runs and grows both ways.
func (c *constrained[I]) buildStrips() {
	triangles := c.triangleCount
	if uint32(len(c.degree)) < triangles {
		c.degree = cdtResize(c.degree, max(int(triangles), 2*len(c.degree)))
	}
	c.candidates = c.candidates[:0]
	for t := uint32(0); t < triangles; t++ {
		if c.state[t] != triFree {
			continue
		}
		d := uint8(0)
		for e := 3 * t; e < 3*t+3; e++ {
			if c.twin[e] != none && c.state[c.twin[e]/3] == triFree {
				d++
			}
		}
		c.degree[t] = d
		if d <= 1 {
			c.candidates = append(c.candidates, t)
		}
	}
	for i, j := 0, len(c.candidates)-1; i < j; i, j = i+1, j-1 {
		c.candidates[i], c.candidates[j] = c.candidates[j], c.candidates[i]
	}
	scan := uint32(0)
	for {
		t := uint32(none)
		for len(c.candidates) > 0 {
			cand := c.candidates[len(c.candidates)-1]
			c.candidates = c.candidates[:len(c.candidates)-1]
			if c.state[cand] == triFree {
				t = cand
				break
			}
		}
		if t == none {
			for scan < triangles && c.state[scan] != triFree {
				scan++
			}
			if scan == triangles {
				break
			}
			t = scan
		}
		c.emitStrip(t)
	}
}

// runLength counts the free triangles in a row from the triangle of exit half-edge g; the newest strip vertex is
// g's origin when atStart.
func (c *constrained[I]) runLength(g uint32, atStart bool) uint32 {
	count := uint32(0)
	for {
		u := c.twin[g]
		if u == none || c.state[u/3] != triFree {
			return count
		}
		count++
		if atStart {
			g = nextEdge(u)
		} else {
			g = prevEdge(u)
		}
		atStart = !atStart
	}
}

// takeRun takes the run into run (new vertices) and marks its triangles used.
func (c *constrained[I]) takeRun(g uint32, atStart bool) {
	for {
		u := c.twin[g]
		if u == none || c.state[u/3] != triFree {
			return
		}
		c.state[u/3] = triUsed
		c.taken = append(c.taken, u/3)
		c.run = append(c.run, c.org[prevEdge(u)])
		if atStart {
			g = nextEdge(u)
		} else {
			g = prevEdge(u)
		}
		atStart = !atStart
	}
}

func (c *constrained[I]) emitStrip(t uint32) {
	c.state[t] = triUsed
	e := 3 * t
	// option o: rotation o / 2, counterclockwise order when o is even
	best, bestLength := uint32(0), uint32(0)
	for o := uint32(0); o < 6; o++ {
		r := o / 2
		h0, h1, h2 := e+r, e+(r+1)%3, e+(r+2)%3
		var length uint32
		if o&1 == 0 {
			length = c.runLength(h1, false) + c.runLength(h0, true)
		} else {
			length = c.runLength(h1, true) + c.runLength(h2, false)
		}
		if length > bestLength || o == 0 {
			best = o
			bestLength = length
		}
	}
	r := best / 2
	h0, h1, h2 := e+r, e+(r+1)%3, e+(r+2)%3
	ccw := best&1 == 0
	c.taken = c.taken[:0]
	c.taken = append(c.taken, t)
	c.run = c.run[:0]
	if ccw {
		c.takeRun(h0, true)
	} else {
		c.takeRun(h2, false)
	}
	c.stripVertices = c.stripVertices[:0]
	for i := len(c.run); i > 0; i-- {
		c.stripVertices = append(c.stripVertices, c.run[i-1])
	}
	a, b, cc := c.org[h0], c.org[h1], c.org[h2]
	if ccw {
		c.stripVertices = append(c.stripVertices, a, b, cc)
	} else {
		c.stripVertices = append(c.stripVertices, a, cc, b)
	}
	c.run = c.run[:0]
	c.takeRun(h1, !ccw)
	c.stripVertices = append(c.stripVertices, c.run...)
	for _, v := range c.stripVertices {
		c.strips = append(c.strips, c.outIndex[v])
	}
	c.stripEnds = append(c.stripEnds, uint32(len(c.strips)))
	cw := uint8(0)
	if c.orient(c.stripVertices[0], c.stripVertices[1], c.stripVertices[2]) < 0 {
		cw = 1
	}
	c.stripCW = append(c.stripCW, cw)
	// neighbors of the used triangles lose a free neighbor
	for _, used := range c.taken {
		for g := 3 * used; g < 3*used+3; g++ {
			if c.twin[g] == none {
				continue
			}
			n := c.twin[g] / 3
			if c.state[n] == triFree {
				c.degree[n]--
				if c.degree[n] <= 1 {
					c.candidates = append(c.candidates, n)
				}
			}
		}
	}
}
