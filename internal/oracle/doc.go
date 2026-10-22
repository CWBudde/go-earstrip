// Package oracle runs the C++ reference implementation (third_party/earstrip/earstrip.hpp) through cgo, for the
// differential tests of go-earstrip. Everything but this file needs the build tag earstrip_cgo and a C++17 compiler:
//
//	go test -tags earstrip_cgo ./internal/oracle/
package oracle
