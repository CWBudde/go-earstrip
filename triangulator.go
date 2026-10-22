package earstrip

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

// runRings is the port of Triangulator::runRings; see the remaining methods below (agent-owned).
func (t *Triangulator[I]) runRings(rings Rings) Status {
	panic("earstrip: runRings not implemented")
}
