// Package oracle differentially tests vt10x against charmbracelet/x/vt.
//
// It is a separate module so that x/vt and its dependency tree never enter
// vt10x's own go.mod, and so never enter the module graph of vt10x's consumers.
// Run it with: go test ./...   (from this directory).
package oracle
