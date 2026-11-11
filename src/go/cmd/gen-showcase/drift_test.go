package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDrift ensures that the committed JSON files under internal/assets/templates/
// match exactly what build() produces. If a spec changes, the committed file must
// be regenerated via `go run ./cmd/gen-showcase` — this test catches drift.
func TestDrift(t *testing.T) {
	outDir := filepath.Join("..", "..", "internal", "assets", "templates")
	for _, s := range showcases() {
		s := s
		t.Run(s.Stem, func(t *testing.T) {
			got, err := build(s)
			if err != nil {
				t.Fatalf("build(%s): %v", s.Stem, err)
			}
			// main() appends a trailing newline; match that here.
			want, err := os.ReadFile(filepath.Join(outDir, s.Stem+".json"))
			if err != nil {
				t.Fatalf("read committed file %s.json: %v", s.Stem, err)
			}
			gotWithNL := append(got, '\n')
			if string(gotWithNL) != string(want) {
				t.Errorf("%s: build() output does not match committed JSON.\nRun: go run ./cmd/gen-showcase to regenerate.", s.Stem)
			}
		})
	}
}
