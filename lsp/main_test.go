package main

import "testing"

func TestMainPackageBuilds(t *testing.T) {
	// This test exists so `go test ./...` has something to run before
	// real handlers land. It also pins the package name.
}
