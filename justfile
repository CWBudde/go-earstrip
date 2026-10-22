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
