package earstrip

// constrained is the constrained Delaunay triangulation of the ring edges into strips (detail::Constrained).
// Stub: the interface the Triangulator uses; implemented in constrained.go / constrained_mesh.go.
type constrained[I Index] struct {
	strips    []I
	stripEnds []uint32
	stripCW   []uint8
}

// load copies the rings; nothing reads them afterwards. False when they have more points than I can count.
func (c *constrained[I]) load(rings Rings) bool { panic("earstrip: constrained.load not implemented") }

// build triangulates the loaded rings; false when a limit was hit.
func (c *constrained[I]) build() bool { panic("earstrip: constrained.build not implemented") }

// extraPoints writes the added points as x, y pairs into out[:0]; strip index pointCount + k is added point k.
func (c *constrained[I]) extraPoints(out []float64) []float64 {
	panic("earstrip: constrained.extraPoints not implemented")
}

func (c *constrained[I]) extraCount() uint32 {
	panic("earstrip: constrained.extraCount not implemented")
}

// inputRingsArea returns the doubled area of ring 0 minus the holes, as the rings are given.
func (c *constrained[I]) inputRingsArea() float64 {
	panic("earstrip: constrained.inputRingsArea not implemented")
}

// closer reports whether the given strips (over the input points) cover an area strictly closer to the ring area
// than the constrained ones.
func (c *constrained[I]) closer(strips []I, ends []uint32, ringsArea float64) bool {
	panic("earstrip: constrained.closer not implemented")
}
