package verify

import (
	"errors"
	"fmt"
)

// Status values, the same numbers as earstrip.Status and earstrip::Status.
const (
	StatusExact = iota
	StatusHolesLeftOut
	StatusApproximate
	StatusRefused
	StatusConstrained
)

// Invariants checks what any result over integer rings must satisfy, whatever the input: a known status; never
// Refused with forced cuts on; the structure of the strips (see Structure); added points only with Constrained;
// for an ear result (Exact, HolesLeftOut) no flat triangle and no strip that flips orientation, and for Exact the
// exact area of the outer ring minus the holes (when the outer ring has an area); for Constrained over 32-bit
// coordinates every triangle positive in its strip's winding.
func Invariants[I Index](rings IntRings, status int, forcedCuts bool, strips []I, ends []uint32, cw []uint8, extra []float64) error {
	if status < StatusExact || status > StatusConstrained {
		return fmt.Errorf("unknown status %d", status)
	}
	if forcedCuts && status == StatusRefused {
		return errors.New("refused with forced cuts on")
	}
	n := 0
	for _, r := range rings {
		n += len(r)
	}
	if err := Structure(n, strips, ends, cw, extra); err != nil {
		return err
	}
	if len(extra) > 0 && status != StatusConstrained {
		return fmt.Errorf("%d added points with status %d", len(extra)/2, status)
	}
	switch status {
	case StatusExact, StatusHolesLeftOut:
		// an outer ring without area covers nothing, whatever the holes: the area formula only holds with one
		dropped := status == StatusHolesLeftOut || len(rings) == 0 || RingArea2(rings[0]).Sign() == 0
		if _, err := Exact(rings, strips, ends, dropped); err != nil {
			return fmt.Errorf("status %d: %w", status, err)
		}
	case StatusConstrained:
		// input beyond 32 bits is rounded to a grid: its triangles are positive there, not always in the input
		if !fits32(rings) {
			break
		}
		if _, err := Cover(strips, ends, cw, Points(Float(rings)), extra); err != nil {
			return fmt.Errorf("constrained: %w", err)
		}
	}
	return nil
}

func fits32(rings IntRings) bool {
	for _, r := range rings {
		for _, p := range r {
			if p[0] != int64(int32(p[0])) || p[1] != int64(int32(p[1])) {
				return false
			}
		}
	}
	return true
}
