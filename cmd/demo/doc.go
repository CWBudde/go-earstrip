// Command demo is the browser demo of go-earstrip: built for js/wasm by "just build-demo" and loaded by
// web/src/wasm/earstrip.ts, it publishes globalThis.earstrip, whose functions the web app calls.
//
// main_js.go defines those functions, bridge_js.go converts arguments and results and defines how errors reach
// JavaScript. The computations live in internal/demo.
//
// The _js.go files build for GOOS=js only; on other systems the package is main_other.go, which exits with a hint,
// so that "go build ./..." works.
package main
