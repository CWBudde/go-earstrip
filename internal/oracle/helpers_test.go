//go:build earstrip_cgo

package oracle_test

import (
	"sync"
	"testing"

	"github.com/cwbudde/go-earstrip/internal/plydata"
)

// shortPolygons is how many polygons of a large file -short runs.
const shortPolygons = 100_000

var (
	filesMu sync.Mutex
	files   = map[string]*plydata.File{} // loaded once per test binary; read-only afterwards
)

// loadData loads a benchmark file or skips the test when it is missing.
func loadData(t testing.TB, name string) *plydata.File {
	t.Helper()
	filesMu.Lock()
	defer filesMu.Unlock()
	if f, ok := files[name]; ok {
		return f
	}
	path, ok := plydata.Path(name)
	if !ok {
		t.Skipf("%s not found (run `just fetch-data` or set EARSTRIP_DATA)", name)
	}
	f, err := plydata.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	files[name] = f
	return f
}

// limit returns how many of n polygons to run: all, or shortPolygons under -short when sampled.
func limit(n int, sampled bool) int {
	if sampled && testing.Short() && n > shortPolygons {
		return shortPolygons
	}
	return n
}
