package earstrip_test

// The outputs below are those of the C++ reference (earstrip.hpp through internal/oracle) for the same input.

import (
	"fmt"

	"github.com/cwbudde/go-earstrip"
)

// A square with a square hole becomes one strip of ten indices: eight triangles.
func ExampleTriangulator_Run() {
	var t earstrip.Triangulator[uint32]
	status := t.Run([][][2]float64{
		{{0, 0}, {10, 0}, {10, 10}, {0, 10}}, // outer ring
		{{3, 3}, {7, 3}, {7, 7}, {3, 7}},     // hole
	})
	fmt.Println(status)
	fmt.Printf("strips: %v ends: %v clockwise: %v\n", t.Strips(), t.StripEnds(), t.StripClockwise())
	fmt.Println("triangles:", t.Triangles(nil))
	// Output:
	// Exact
	// strips: [1 5 0 4 3 7 2 6 1 5] ends: [10] clockwise: [0]
	// triangles: [1 5 0 0 5 4 0 4 3 3 4 7 3 7 2 2 7 6 2 6 1 1 6 5]
}

// Rings that cross go to the constrained triangulation, which adds the crossing as a point: index 4 here.
func ExampleTriangulator_ExtraPoints() {
	var t earstrip.Triangulator[uint32]
	status := t.Run([][][2]float64{{{0, 0}, {10, 10}, {10, 0}, {0, 10}}}) // a bow tie
	fmt.Println(status, "added:", t.ExtraPoints())
	fmt.Println("strips:", t.Strips(), "ends:", t.StripEnds())
	// Output:
	// Constrained added: [5 5]
	// strips: [1 4 2 3 0 4] ends: [3 6]
}

func ExampleTriangulate() {
	l := [][][2]float64{{{0, 0}, {10, 0}, {10, 4}, {4, 4}, {4, 10}, {0, 10}}}
	fmt.Println(earstrip.Triangulate(l))
	// Output:
	// [1 2 0 0 2 3 0 3 5 5 3 4]
}

// Earcut takes earcut's arguments: flat coordinates, the vertex index where each hole starts, numbers per vertex.
func ExampleEarcut() {
	coords := []float64{0, 0, 10, 0, 10, 10, 0, 10, 3, 3, 7, 3, 7, 7, 3, 7}
	fmt.Println(earstrip.Earcut(coords, []int{4}, 2))
	// Output:
	// [1 5 0 0 5 4 0 4 3 3 4 7 3 7 2 2 7 6 2 6 1 1 6 5]
}
