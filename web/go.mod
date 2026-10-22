// web/ is the demo's front end, not Go code. This file makes it a module of
// its own so that the root module's ./... (go build, go test, golangci-lint)
// skips it, including the Go files some npm packages ship in node_modules.
module github.com/cwbudde/go-earstrip/web

go 1.24
