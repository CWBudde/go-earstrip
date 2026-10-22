//go:build earstrip_cgo

package oracle

/*
#cgo CXXFLAGS: -std=c++17 -O2 -I${SRCDIR}/../../third_party/earstrip
#include <stddef.h>
#include <stdint.h>

void *oracle_new(void);
void oracle_free(void *h);
void oracle_shrink(void *h);
int oracle_run(void *h, const double *coords, const uint32_t *sizes, size_t ringCount, int flags);
int oracle_run_flat(void *h, const double *coords, size_t vertexCount, size_t dim, const uint32_t *holeStarts, size_t holeCount, int flags);
int oracle_holes_left_out(void *h);
const uint32_t *oracle_strips(void *h, size_t *n);
const uint32_t *oracle_strip_ends(void *h, size_t *n);
const uint8_t *oracle_strip_cw(void *h, size_t *n);
const double *oracle_extra(void *h, size_t *n);
const uint32_t *oracle_list(void *h, size_t *n);
const uint32_t *oracle_triangles(void *h, size_t *n);
*/
import "C" //nolint:gocritic // cgo pseudo-package, not a duplicate of unsafe

import (
	"runtime"
	"unsafe" //nolint:gocritic // see import "C"
)

// Status values of earstrip::Status, the same numbers as earstrip.Status.
const (
	Exact = iota
	HolesLeftOut
	Approximate
	Refused
	Constrained
)

// Options are the settings of a run; the zero value is the library default (constrained triangulation, forced
// cuts and extra points on, strips).
type Options struct {
	NoConstrained bool // setConstrained(false)
	NoForcedCuts  bool // setForcedCuts(false)
	NoExtraPoints bool // setExtraPoints(false)
	TriangleList  bool // setTriangleList(true)
}

func (o Options) flags() C.int {
	var f C.int
	if o.NoConstrained {
		f |= 1
	}
	if o.NoForcedCuts {
		f |= 2
	}
	if o.NoExtraPoints {
		f |= 4
	}
	if o.TriangleList {
		f |= 8
	}
	return f
}

// Result is the C++ triangulator's result of one run, copied into Go memory.
type Result struct {
	Status       int
	HolesLeftOut bool
	Strips, Ends []uint32
	CW           []uint8
	Extra        []float64
	List         []uint32 // triangleList(), filled in triangle list mode
	Triangles    []uint32 // triangles() of the strips (empty in triangle list mode, where the strips are empty)
}

// Oracle is a reused earstrip::Triangulator<uint32_t>. It is not safe for concurrent use.
type Oracle struct {
	h      unsafe.Pointer
	coords []float64
	sizes  []uint32
}

// New returns an oracle; Close frees it (a finalizer does too).
func New() *Oracle {
	o := &Oracle{h: C.oracle_new()}
	runtime.SetFinalizer(o, (*Oracle).Close)
	return o
}

// Close frees the C++ triangulator.
func (o *Oracle) Close() {
	if o.h != nil {
		C.oracle_free(o.h)
		o.h = nil
	}
}

// Shrink calls shrink() on the triangulator.
func (o *Oracle) Shrink() { C.oracle_shrink(o.h) }

// Run triangulates rings of points, ring 0 the outer ring, as Triangulator.Run does.
func (o *Oracle) Run(rings [][][2]float64, opts Options) Result {
	o.coords, o.sizes = o.coords[:0], o.sizes[:0]
	for _, r := range rings {
		for _, p := range r {
			o.coords = append(o.coords, p[0], p[1])
		}
		o.sizes = append(o.sizes, uint32(len(r)))
	}
	return o.RunSized(o.coords, o.sizes, opts)
}

// RunSized triangulates rings given as flat x, y coordinates of all rings and the number of points of each ring;
// the C++ sees them as nested rings, like Run.
func (o *Oracle) RunSized(coords []float64, sizes []uint32, opts Options) Result {
	s := C.oracle_run(o.h, ptr[C.double](coords), ptr[C.uint32_t](sizes), C.size_t(len(sizes)), opts.flags())
	runtime.KeepAlive(coords)
	runtime.KeepAlive(sizes)
	return o.result(int(s))
}

// RunFlat triangulates through the C++ flat interface, as Triangulator.RunFlat does: dim numbers per vertex (0
// means 2), the vertex index where each hole starts.
func (o *Oracle) RunFlat(coords []float64, dim int, holeStarts []int, opts Options) Result {
	if dim < 2 {
		dim = 2
	}
	holes := make([]uint32, len(holeStarts))
	for i, h := range holeStarts {
		holes[i] = uint32(h)
	}
	s := C.oracle_run_flat(o.h, ptr[C.double](coords), C.size_t(len(coords)/dim), C.size_t(dim), ptr[C.uint32_t](holes),
		C.size_t(len(holes)), opts.flags())
	runtime.KeepAlive(coords)
	runtime.KeepAlive(holes)
	return o.result(int(s))
}

func (o *Oracle) result(status int) Result {
	var n C.size_t
	r := Result{Status: status, HolesLeftOut: C.oracle_holes_left_out(o.h) != 0}
	r.Strips = copyOut[uint32](unsafe.Pointer(C.oracle_strips(o.h, &n)), n)
	r.Ends = copyOut[uint32](unsafe.Pointer(C.oracle_strip_ends(o.h, &n)), n)
	r.CW = copyOut[uint8](unsafe.Pointer(C.oracle_strip_cw(o.h, &n)), n)
	r.Extra = copyOut[float64](unsafe.Pointer(C.oracle_extra(o.h, &n)), n)
	r.List = copyOut[uint32](unsafe.Pointer(C.oracle_list(o.h, &n)), n)
	r.Triangles = copyOut[uint32](unsafe.Pointer(C.oracle_triangles(o.h, &n)), n)
	return r
}

// Run triangulates rings with a fresh C++ triangulator.
func Run(rings [][][2]float64, opts Options) Result {
	o := New()
	defer o.Close()
	return o.Run(rings, opts)
}

func ptr[T, E any](s []E) *T {
	if len(s) == 0 {
		return nil
	}
	return (*T)(unsafe.Pointer(&s[0]))
}

func copyOut[T any](p unsafe.Pointer, n C.size_t) []T {
	if n == 0 {
		return nil
	}
	return append([]T(nil), unsafe.Slice((*T)(p), int(n))...)
}
