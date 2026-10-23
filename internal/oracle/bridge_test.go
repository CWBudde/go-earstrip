//go:build earstrip_cgo

package oracle_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
)

// bridgeTies is a square with a grid of small holes on integer coordinates, made for the ties of the hole bridging:
// rightmost points that share x or y, rays along horizontal edges and through vertices, holes that touch each other
// or the outer ring, both orientations, holes given twice. Up to 27 × 27 cells, so that the merged nodes span many
// of bridge's blocks and groups.
func bridgeTies(rng *rand.Rand) [][][2]float64 {
	k := 2 + rng.IntN(26)
	const cell = 4
	w := float64(k * cell)
	rings := [][][2]float64{{{0, 0}, {w, 0}, {w, w}, {0, w}}}
	for cx := range k {
		for cy := range k {
			if rng.IntN(5) == 0 {
				continue
			}
			x, y := float64(cx*cell), float64(cy*cell)
			var ring [][2]float64
			switch rng.IntN(6) {
			case 0: // fills the cell: shares edges and corners with its neighbors and the outer ring
				ring = [][2]float64{{x, y}, {x + cell, y}, {x + cell, y + cell}, {x, y + cell}}
			case 1: // square inside the cell, corners on the grid lines of the others
				ring = [][2]float64{{x + 1, y + 1}, {x + 3, y + 1}, {x + 3, y + 3}, {x + 1, y + 3}}
			case 2: // diamond: rightmost point at the middle height
				ring = [][2]float64{{x + 2, y}, {x + 4, y + 2}, {x + 2, y + 4}, {x, y + 2}}
			case 3: // triangle with a vertical right edge
				ring = [][2]float64{{x + 1, y + 2}, {x + 3, y + 1}, {x + 3, y + 3}}
			case 4: // spike touching the right neighbor's cell corner
				ring = [][2]float64{{x + 1, y + 1}, {x + 4, y}, {x + 2, y + 2}}
			default: // random small hull on the grid points of the cell
				ring = [][2]float64{{x + float64(rng.IntN(2)), y + float64(rng.IntN(2))},
					{x + 2 + float64(rng.IntN(3)), y + float64(rng.IntN(2))},
					{x + 2 + float64(rng.IntN(3)), y + 2 + float64(rng.IntN(3))},
					{x + float64(rng.IntN(2)), y + 2 + float64(rng.IntN(3))}}
			}
			if rng.IntN(2) == 0 {
				slices.Reverse(ring)
			}
			rings = append(rings, ring)
			if rng.IntN(20) == 0 {
				rings = append(rings, slices.Clone(ring))
			}
		}
	}
	return rings
}

// TestDifferentialBridgeTies compares hole grids full of bridging ties; with more than 128 rings only the settings
// without the constrained triangulation take the ears.
func TestDifferentialBridgeTies(t *testing.T) {
	count := 600
	if testing.Short() {
		count = 100
	}
	forSettings(t, func(t *testing.T, p *pair) {
		m := mismatches{t: t}
		rng := rand.New(rand.NewPCG(2130, 2207))
		for i := range count {
			poly := bridgeTies(rng)
			if err := p.compareRings(poly); err != nil && !m.add(fmt.Sprintf("bridge ties %d", i), poly, err) {
				break
			}
		}
		m.report(count)
	})
}
