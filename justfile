# go-earstrip development commands; `just --list` shows them all

default:
    @just --list

build:
    go build ./...

test:
    go test ./...

# Differential tests against the C++ header through cgo (needs a C++17 compiler)
test-oracle:
    go test -tags earstrip_cgo ./internal/oracle/... -v

# Hash-exact comparison with the C++ results on the bench-data-1 polygons (run fetch-data first)
conformance:
    go test -run 'TestConformance' -v -timeout 30m .

fuzz time="2m":
    go test -run '^$' -fuzz FuzzInvariants -fuzztime {{time}} .

bench:
    go test -run '^$' -bench . -benchmem .

lint:
    golangci-lint run ./...

fmt:
    gofmt -w .

# Download the benchmark polygons (OpenStreetMap, ODbL) into data/
fetch-data *files:
    ./scripts/fetch-data.sh {{files}}
