package main

import (
	"fmt"
	"runtime"
	"syscall/js"
	"time"

	"github.com/cwbudde/go-earstrip/internal/demo"
)

// runner is the one Triangulator of the page, reused by every call as an application would.
var runner demo.Runner

var presets = demo.Presets()

// main publishes globalThis.earstrip, sets globalThis.earstripReady, calls globalThis.onEarstripReady if the loader
// set it, and then keeps the Go program alive for the callbacks.
func main() {
	api := js.Global().Get("Object").New()
	api.Set("goVersion", runtime.Version())
	for name, fn := range map[string]Func{
		"presets":     listPresets,
		"preset":      preset,
		"kinds":       kinds,
		"generate":    generate,
		"triangulate": triangulate,
		"time":        timeRuns,
	} {
		api.Set(name, export(name, fn))
	}

	js.Global().Set("earstrip", api)
	js.Global().Set("earstripReady", true)
	if ready := js.Global().Get("onEarstripReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}

// listPresets returns [{group, name}] of the presets, in their order.
func listPresets([]js.Value) (any, error) {
	out := make([]any, len(presets))
	for i, p := range presets {
		out[i] = map[string]any{"group": p.Group, "name": p.Name}
	}
	return out, nil
}

// preset(index) returns preset index as {coords: Float64Array, holeStarts: number[]}.
func preset(args []js.Value) (any, error) {
	i, err := argInt(args, 0, "index")
	if err != nil {
		return nil, err
	}
	if i < 0 || i >= len(presets) {
		return nil, fmt.Errorf("index %d: want 0 to %d", i, len(presets)-1)
	}
	return flatToJS(demo.FlatInts(presets[i].Rings)), nil
}

// kinds returns the names of the generated polygon families.
func kinds([]js.Value) (any, error) {
	return stringsToJS(demo.Kinds()), nil
}

// generate(kind, seed) returns a random polygon as {coords, holeStarts}.
func generate(args []js.Value) (any, error) {
	kind, err := argString(args, 0, "kind")
	if err != nil {
		return nil, err
	}
	seed, err := argInt(args, 1, "seed")
	if err != nil {
		return nil, err
	}
	f, err := demo.Generate(kind, uint64(seed))
	if err != nil {
		return nil, err
	}
	return flatToJS(f), nil
}

// triangulate(coords, holeStarts, options) runs the triangulator and returns {status, holesLeftOut, strips,
// stripEnds, stripClockwise, triangles, extra} as typed arrays.
func triangulate(args []js.Value) (any, error) {
	f, o, err := argPolygon(args)
	if err != nil {
		return nil, err
	}
	res, err := runner.Run(f, o)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"status":         res.Status.String(),
		"holesLeftOut":   res.HolesLeftOut,
		"strips":         uint32sToJS(res.Strips),
		"stripEnds":      uint32sToJS(res.StripEnds),
		"stripClockwise": bytesToJS(res.StripClockwise),
		"triangles":      uint32sToJS(res.Triangles),
		"extra":          float64sToJS(res.Extra),
	}, nil
}

// timeRuns(coords, holeStarts, options, budgetMs) times the triangulation and returns {runs, nsPerRun}.
func timeRuns(args []js.Value) (any, error) {
	f, o, err := argPolygon(args)
	if err != nil {
		return nil, err
	}
	ms, err := argFloat(args, 3, "budgetMs")
	if err != nil {
		return nil, err
	}
	if ms <= 0 || ms > 10000 {
		return nil, fmt.Errorf("budget %v ms: want more than 0, at most 10000", ms)
	}
	tm, err := runner.Time(f, o, time.Duration(ms*float64(time.Millisecond)))
	if err != nil {
		return nil, err
	}
	return map[string]any{"runs": tm.Runs, "nsPerRun": float64(tm.PerRun.Nanoseconds())}, nil
}
