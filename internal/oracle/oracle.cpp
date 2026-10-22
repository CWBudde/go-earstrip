//go:build earstrip_cgo

// C API over earstrip::Triangulator<uint32_t> for the Go differential tests. A handle keeps one triangulator, reused
// across runs as a caller of the library would; the accessors expose its last result until the next run.

// No fused multiply-add: the Go port computes every product and sum rounded on its own, and cgo does not let
// -ffp-contract=off through #cgo CXXFLAGS, so the pragmas switch contraction off for everything below.
#if defined(__clang__)
#    pragma clang fp contract(off)
#elif defined(__GNUC__)
#    pragma GCC optimize("fp-contract=off")
#endif

#include "earstrip.hpp"

#include <cstddef>
#include <cstdint>
#include <vector>

namespace {

struct Pt {
    double x, y;
};

struct Ring {
    const Pt *p;
    size_t n;
    size_t size() const { return n; }
    const Pt &operator[](size_t i) const { return p[i]; }
};

// A polygon as nested rings over flat coordinates, so that the C++ sees exactly the rings the Go side passes to
// Triangulator.Run (an empty ring stays a ring, unlike in the flat interface).
struct Polygon {
    const Pt *base;
    const uint32_t *sizes;
    size_t count;
    std::vector<size_t> starts;
    size_t size() const { return count; }
    Ring operator[](size_t r) const { return {base + starts[r], sizes[r]}; }
};

struct Oracle {
    earstrip::Triangulator<uint32_t> t;
    std::vector<uint32_t> tris;
    Polygon poly{};
};

enum Flags {
    NoConstrained = 1,
    NoForcedCuts = 2,
    NoExtraPoints = 4,
    TriangleList = 8,
};

void configure(Oracle *o, int flags)
{
    o->t.setConstrained(!(flags & NoConstrained));
    o->t.setForcedCuts(!(flags & NoForcedCuts));
    o->t.setExtraPoints(!(flags & NoExtraPoints));
    o->t.setTriangleList((flags & TriangleList) != 0);
}

int finish(Oracle *o, earstrip::Status s)
{
    o->t.triangles(o->tris);
    return (int)s;
}

} // namespace

extern "C" {

void *oracle_new() { return new Oracle(); }

void oracle_free(void *h) { delete (Oracle *)h; }

void oracle_shrink(void *h) { ((Oracle *)h)->t.shrink(); }

// Runs rings given as flat x, y coordinates and the number of points of each ring.
int oracle_run(void *h, const double *coords, const uint32_t *sizes, size_t ringCount, int flags)
{
    Oracle *o = (Oracle *)h;
    configure(o, flags);
    o->poly.base = (const Pt *)coords;
    o->poly.sizes = sizes;
    o->poly.count = ringCount;
    o->poly.starts.resize(ringCount);
    size_t start = 0;
    for (size_t r = 0; r < ringCount; ++r) {
        o->poly.starts[r] = start;
        start += sizes[r];
    }
    return finish(o, o->t.run(o->poly));
}

// Runs the flat interface: dim numbers per vertex, the vertex index where each hole starts.
int oracle_run_flat(void *h, const double *coords, size_t vertexCount, size_t dim, const uint32_t *holeStarts, size_t holeCount, int flags)
{
    Oracle *o = (Oracle *)h;
    configure(o, flags);
    return finish(o, o->t.run(coords, vertexCount, dim, holeStarts, holeCount));
}

int oracle_holes_left_out(void *h) { return ((Oracle *)h)->t.holesLeftOut(); }

const uint32_t *oracle_strips(void *h, size_t *n)
{
    const auto &v = ((Oracle *)h)->t.strips();
    *n = v.size();
    return v.data();
}

const uint32_t *oracle_strip_ends(void *h, size_t *n)
{
    const auto &v = ((Oracle *)h)->t.stripEnds();
    *n = v.size();
    return v.data();
}

const uint8_t *oracle_strip_cw(void *h, size_t *n)
{
    const auto &v = ((Oracle *)h)->t.stripClockwise();
    *n = v.size();
    return v.data();
}

const double *oracle_extra(void *h, size_t *n)
{
    const auto &v = ((Oracle *)h)->t.extraPoints();
    *n = v.size();
    return v.data();
}

const uint32_t *oracle_list(void *h, size_t *n)
{
    const auto &v = ((Oracle *)h)->t.triangleList();
    *n = v.size();
    return v.data();
}

const uint32_t *oracle_triangles(void *h, size_t *n)
{
    const auto &v = ((Oracle *)h)->tris;
    *n = v.size();
    return v.data();
}

} // extern "C"
