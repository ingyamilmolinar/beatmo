package ui

// Codegen entry point for the design system. The generator reads DESIGN.md
// and emits design_tokens.gen.go (Phase 0 — primitives only). Phase 2 will
// add design_components.gen.go and design_iconcolor.gen.go.
//
// Run via `make gen-design-tokens` from the repo root, or `go generate
// ./internal/ui/...` from src/go. Generated files are committed.

//go:generate go run ../../cmd/gen_design_tokens -design ../../../DESIGN.md -out design_tokens.gen.go -out-components design_components.gen.go -out-profile design_profile.gen.go
