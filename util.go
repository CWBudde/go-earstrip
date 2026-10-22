package earstrip

import (
	"math"
	"math/bits"
)

// none marks a missing node, vertex or edge.
const none = math.MaxUint32

func bitWidth(v uint32) uint32 { return uint32(bits.Len32(v)) }

func lowestBit(v uint64) uint32 { return uint32(bits.TrailingZeros64(v)) }

// bitsOf returns the bits of v with -0 and 0 the same position.
func bitsOf(v float64) uint64 { return math.Float64bits(v + 0.0) }
