package eventlogger

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// TestEventLoggerCoversAllNonVerboseKinds asserts that every non-verbose
// hooks.Kind has a formatter in this package. Adding a new Kind without a
// formatter trips this test, forcing the author to consider whether the new
// Kind belongs in the user-facing INFO narrative.
func TestEventLoggerCoversAllNonVerboseKinds(t *testing.T) {
	for _, k := range hooks.KindAll {
		if hooks.IsVerbose(k) {
			continue
		}
		if _, ok := lookupFormatter(k); !ok {
			t.Errorf("missing formatter for non-verbose kind %q — add one in format.go or justify exclusion", k)
		}
	}
}

// TestEventLoggerCoversVerboseKindsToo asserts that verbose kinds also have
// formatters; we want them to render correctly when callers opt in via
// Options.Verbose.
func TestEventLoggerCoversVerboseKindsToo(t *testing.T) {
	for _, k := range hooks.KindAll {
		if !hooks.IsVerbose(k) {
			continue
		}
		if _, ok := lookupFormatter(k); !ok {
			t.Errorf("missing formatter for verbose kind %q", k)
		}
	}
}

// TestEventLoggerNoUnknownKindsInTable catches typos: every key in the
// formatters map must correspond to an entry in hooks.KindAll. Otherwise a
// renamed Kind would silently lose its formatter.
func TestEventLoggerNoUnknownKindsInTable(t *testing.T) {
	known := make(map[hooks.Kind]struct{}, len(hooks.KindAll))
	for _, k := range hooks.KindAll {
		known[k] = struct{}{}
	}
	for k := range formatters {
		if _, ok := known[k]; !ok {
			t.Errorf("formatter table contains unknown kind %q (not in hooks.KindAll)", k)
		}
	}
}
