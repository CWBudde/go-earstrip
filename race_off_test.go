//go:build !race

package earstrip_test

// raceEnabled is true under the race detector, which slows the code down too much for timing checks.
const raceEnabled = false
