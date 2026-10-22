package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"syscall/js"

	"github.com/cwbudde/go-earstrip/internal/demo"
)

// Errors reach JavaScript as a returned object {error: "<message>"}, never as a thrown exception (syscall/js cannot
// throw): every function either returns its result or that object. web/src/wasm/earstrip.ts passes each result
// through unwrap, which throws an Error with the message. A panic inside a function is recovered and returned the
// same way, so a bad call does not end the Go program.

// Func is one function of globalThis.earstrip. It gets the JavaScript arguments and returns a value js.ValueOf
// accepts (a js.Value, a number, a string, a bool, nil, []any or map[string]any) or an error.
type Func func(args []js.Value) (any, error)

// export wraps fn as a JavaScript function; name prefixes its error messages.
func export(name string, fn Func) js.Func {
	return js.FuncOf(func(_ js.Value, args []js.Value) (result any) {
		defer func() {
			if r := recover(); r != nil {
				result = errorValue(fmt.Errorf("%s: panic: %v", name, r))
			}
		}()
		v, err := fn(args)
		if err != nil {
			return errorValue(fmt.Errorf("%s: %w", name, err))
		}
		// Convert here, inside the recover: js.ValueOf panics on types it does not know.
		return js.ValueOf(v)
	})
}

// errorValue is the {error: "<message>"} object a failing function returns.
func errorValue(err error) js.Value {
	return js.ValueOf(map[string]any{"error": err.Error()})
}

var errMissing = errors.New("missing")

// arg returns argument i, named name in error messages.
func arg(args []js.Value, i int, name string) (js.Value, error) {
	if i >= len(args) || args[i].IsUndefined() {
		return js.Undefined(), fmt.Errorf("argument %d (%s): %w", i, name, errMissing)
	}
	return args[i], nil
}

// argFloat returns argument i as a number.
func argFloat(args []js.Value, i int, name string) (float64, error) {
	v, err := arg(args, i, name)
	if err != nil {
		return 0, err
	}
	if v.Type() != js.TypeNumber {
		return 0, fmt.Errorf("argument %d (%s): want a number, got %s", i, name, v.Type())
	}
	return v.Float(), nil
}

// argInt returns argument i as an int; it must be a whole number.
func argInt(args []js.Value, i int, name string) (int, error) {
	f, err := argFloat(args, i, name)
	if err != nil {
		return 0, err
	}
	if f != math.Trunc(f) || math.Abs(f) > 1<<53 {
		return 0, fmt.Errorf("argument %d (%s): want an integer, got %v", i, name, f)
	}
	return int(f), nil
}

// argString returns argument i as a string.
func argString(args []js.Value, i int, name string) (string, error) {
	v, err := arg(args, i, name)
	if err != nil {
		return "", err
	}
	if v.Type() != js.TypeString {
		return "", fmt.Errorf("argument %d (%s): want a string, got %s", i, name, v.Type())
	}
	return v.String(), nil
}

// argPolygon reads the arguments coords (a Float64Array of x, y pairs), holeStarts (an array of vertex indices) and
// options ({forcedCuts, constrained, extraPoints, triangleList}, each a boolean).
func argPolygon(args []js.Value) (demo.Flat, demo.Options, error) {
	var f demo.Flat
	var o demo.Options
	coords, err := arg(args, 0, "coords")
	if err != nil {
		return f, o, err
	}
	if !coords.InstanceOf(js.Global().Get("Float64Array")) {
		return f, o, errors.New("argument 0 (coords): want a Float64Array")
	}
	view := js.Global().Get("Uint8Array").New(coords.Get("buffer"), coords.Get("byteOffset"), coords.Get("byteLength"))
	b := make([]byte, view.Get("length").Int())
	js.CopyBytesToGo(b, view)
	f.Coords = make([]float64, len(b)/8)
	for i := range f.Coords {
		f.Coords[i] = math.Float64frombits(binary.LittleEndian.Uint64(b[8*i:]))
	}

	starts, err := arg(args, 1, "holeStarts")
	if err != nil {
		return f, o, err
	}
	if !js.Global().Get("Array").Call("isArray", starts).Bool() {
		return f, o, errors.New("argument 1 (holeStarts): want an array")
	}
	f.HoleStarts = make([]int, starts.Length())
	for i := range f.HoleStarts {
		n, err := argInt([]js.Value{starts.Index(i)}, 0, fmt.Sprintf("holeStarts[%d]", i))
		if err != nil {
			return f, o, err
		}
		f.HoleStarts[i] = n
	}

	options, err := arg(args, 2, "options")
	if err != nil {
		return f, o, err
	}
	if options.Type() != js.TypeObject {
		return f, o, fmt.Errorf("argument 2 (options): want an object, got %s", options.Type())
	}
	for name, dst := range map[string]*bool{
		"forcedCuts":   &o.ForcedCuts,
		"constrained":  &o.Constrained,
		"extraPoints":  &o.ExtraPoints,
		"triangleList": &o.TriangleList,
	} {
		v := options.Get(name)
		if v.Type() != js.TypeBoolean {
			return f, o, fmt.Errorf("argument 2 (options): %s: want a boolean, got %s", name, v.Type())
		}
		*dst = v.Bool()
	}
	return f, o, nil
}

// flatToJS converts a polygon to {coords: Float64Array, holeStarts: number[]}.
func flatToJS(f demo.Flat) js.Value {
	starts := make([]any, len(f.HoleStarts))
	for i, s := range f.HoleStarts {
		starts[i] = s
	}
	return js.ValueOf(map[string]any{"coords": float64sToJS(f.Coords), "holeStarts": starts})
}

// bytesToJS copies b to a new Uint8Array.
func bytesToJS(b []byte) js.Value {
	out := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(out, b)
	return out
}

// uint32sToJS copies values to a new Uint32Array.
func uint32sToJS(values []uint32) js.Value {
	b := make([]byte, 4*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint32(b[4*i:], v)
	}
	return js.Global().Get("Uint32Array").New(bytesToJS(b).Get("buffer"))
}

// float64sToJS copies values to a new Float64Array.
func float64sToJS(values []float64) js.Value {
	b := make([]byte, 8*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint64(b[8*i:], math.Float64bits(v))
	}
	return js.Global().Get("Float64Array").New(bytesToJS(b).Get("buffer"))
}

// stringsToJS converts s to a JavaScript array of strings.
func stringsToJS(s []string) js.Value {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return js.ValueOf(out)
}
