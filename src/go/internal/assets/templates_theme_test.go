package assets

import (
	"encoding/json"
	"testing"
)

// Built-in templates ship the default look; a theme block in one would force
// the author's colors on every user who loads it.
func TestTemplatesCarryNoTheme(t *testing.T) {
	for _, tp := range Templates() {
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(tp.Bytes, &doc); err != nil {
			t.Fatalf("%s: %v", tp.Display, err)
		}
		if _, ok := doc["theme"]; ok {
			t.Errorf("template %s carries a theme block", tp.Display)
		}
	}
}
