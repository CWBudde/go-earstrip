# go-earstrip development commands; `just --list` shows them all
# The data files are looked up in $EARSTRIP_DATA, else in the nearest data/ directory upwards (a git worktree
# finds the main checkout's); without them the data tests and benchmarks skip.

default:
    @just --list

build:
    go build ./...

test:
    go test ./...

# Differential tests against the C++ header through cgo (needs a C++17 compiler); -short samples z13
test-oracle *args:
    go test -tags earstrip_cgo -timeout 60m {{args}} ./internal/oracle/...

# The C++ through the oracle against the hashes stored in z9.bin / z13.bin: checks loader, hash and oracle
oracle-hashes:
    go test -tags earstrip_cgo -run TestOracleHashes -v ./internal/oracle/

# Hash-exact comparison with the C++ results on the bench-data-1 polygons (run fetch-data first)
conformance:
    go test -run 'TestConformance' -v -timeout 30m .

fuzz time="2m":
    go test -run '^$' -fuzz FuzzInvariants -fuzztime {{time}} .

# Fuzz go-earstrip against the C++ (cgo)
fuzz-diff time="2m":
    go test -tags earstrip_cgo -run '^$' -fuzz FuzzDifferential -fuzztime {{time}} ./internal/oracle/

bench:
    go test -run '^$' -bench . -benchmem .

# bench/bench.cpp's numbers for a data file: just bench-file data/z13.bin -r 7 --check
bench-file file *args:
    go run ./cmd/earstrip-bench {{file}} {{args}}

lint:
    golangci-lint run ./...
    golangci-lint run --build-tags earstrip_cgo ./internal/oracle/...

fmt:
    gofmt -w .

# Download the benchmark polygons (OpenStreetMap, ODbL) into data/
fetch-data *files:
    ./scripts/fetch-data.sh {{files}}

# Build the browser demo: cmd/demo as js/wasm into web/public, then the Vite app into web/dist
build-demo: _demo-wasm
    npm --prefix web ci
    npm --prefix web run build

# Serve the browser demo with Vite's dev server (http://localhost:5173/go-earstrip/)
dev-demo: _demo-wasm _web-deps
    npm --prefix web run dev

# Lint the js/wasm-only demo glue in cmd/demo
lint-wasm:
    GOOS=js GOARCH=wasm golangci-lint run ./cmd/demo/...

# Lint and type-check the demo front end
lint-web: _web-deps
    npm --prefix web run lint
    npm --prefix web run typecheck

# Install the locked web dependencies unless web/node_modules exists (build-demo always runs npm ci)
_web-deps:
    [ -d web/node_modules ] || npm --prefix web ci

# Build cmd/demo as web/public/demo.wasm and copy the Go toolchain's wasm_exec.js next to it
_demo-wasm:
    GOOS=js GOARCH=wasm go build -trimpath -ldflags='-s -w' -o web/public/demo.wasm ./cmd/demo
    cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/public/
