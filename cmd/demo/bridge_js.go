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
	var err error
	if f.Coords, err = argCoords(args, 0); err != nil {
		return f, o, err
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

	o, err = argOptions(args, 2)
	return f, o, err
}

// argBatch reads the arguments coords (a Float64Array of x, y pairs), ringStarts and polygonStarts (Uint32Arrays,
// see demo.Batch) and options, as argPolygon does.
func argBatch(args []js.Value) (demo.Batch, demo.Options, error) {
	var b demo.Batch
	var o demo.Options
	var err error
	if b.Coords, err = argCoords(args, 0); err != nil {
		return b, o, err
	}
	if b.RingStarts, err = argIndices(args, 1, "ringStarts"); err != nil {
		return b, o, err
	}
	if b.PolygonStarts, err = argIndices(args, 2, "polygonStarts"); err != nil {
		return b, o, err
	}
	o, err = argOptions(args, 3)
	return b, o, err
}

// argCoords reads argument i, a Float64Array of coordinates.
func argCoords(args []js.Value, i int) ([]float64, error) {
	b, err := argTypedBytes(args, i, "coords", "Float64Array")
	if err != nil {
		return nil, err
	}
	out := make([]float64, len(b)/8)
	for k := range out {
		out[k] = math.Float64frombits(binary.LittleEndian.Uint64(b[8*k:]))
	}
	return out, nil
}

// argIndices reads argument i, a Uint32Array, as ints.
func argIndices(args []js.Value, i int, name string) ([]int, error) {
	b, err := argTypedBytes(args, i, name, "Uint32Array")
	if err != nil {
		return nil, err
	}
	out := make([]int, len(b)/4)
	for k := range out {
		out[k] = int(binary.LittleEndian.Uint32(b[4*k:]))
	}
	return out, nil
}

// argTypedBytes copies the bytes of argument i, which must be a typed array of the type named, to Go.
func argTypedBytes(args []js.Value, i int, name, typ string) ([]byte, error) {
	v, err := arg(args, i, name)
	if err != nil {
		return nil, err
	}
	if !v.InstanceOf(js.Global().Get(typ)) {
		return nil, fmt.Errorf("argument %d (%s): want a %s", i, name, typ)
	}
	view := js.Global().Get("Uint8Array").New(v.Get("buffer"), v.Get("byteOffset"), v.Get("byteLength"))
	b := make([]byte, view.Get("length").Int())
	js.CopyBytesToGo(b, view)
	return b, nil
}

// argOptions reads argument i, the options {forcedCuts, constrained, extraPoints, triangleList}, each a boolean.
func argOptions(args []js.Value, i int) (demo.Options, error) {
	var o demo.Options
	options, err := arg(args, i, "options")
	if err != nil {
		return o, err
	}
	if options.Type() != js.TypeObject {
		return o, fmt.Errorf("argument %d (options): want an object, got %s", i, options.Type())
	}
	for name, dst := range map[string]*bool{
		"forcedCuts":   &o.ForcedCuts,
		"constrained":  &o.Constrained,
		"extraPoints":  &o.ExtraPoints,
		"triangleList": &o.TriangleList,
	} {
		v := options.Get(name)
		if v.Type() != js.TypeBoolean {
			return o, fmt.Errorf("argument %d (options): %s: want a boolean, got %s", i, name, v.Type())
		}
		*dst = v.Bool()
	}
	return o, nil
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

// float32sToJS copies values to a new Float32Array.
func float32sToJS(values []float32) js.Value {
	b := make([]byte, 4*len(values))
	for i, v := range values {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(v))
	}
	return js.Global().Get("Float32Array").New(bytesToJS(b).Get("buffer"))
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
