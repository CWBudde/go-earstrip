// Package oracle runs the C++ reference implementation (third_party/earstrip/earstrip.hpp) through cgo, for the
// differential tests of go-earstrip. Everything but this file needs the build tag earstrip_cgo and a C++17 compiler:
//
//	go test -tags earstrip_cgo ./internal/oracle/
//
// go-earstrip orders holes with the same rightmost point as libc++'s std::sort does (Clang, Apple). Built against
// libstdc++ (GCC), the C++ may order such ties differently: one polygon of gom4old.bin then differs. Run the
// differential tests with libc++.
package oracle
