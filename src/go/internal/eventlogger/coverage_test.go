package eventlogger

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// A new Kind without a formatter fails, forcing the decision whether it belongs in the INFO narrative.
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

// A key with no hooks.KindAll entry is a renamed Kind that silently lost its formatter.
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
