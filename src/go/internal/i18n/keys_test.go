package i18n

import "testing"

func TestAllKeysNonEmptyAndUnique(t *testing.T) {
	seen := map[Key]bool{}
	for _, k := range AllKeys() {
		if k == "" {
			t.Fatal("empty Key in AllKeys()")
		}
		if seen[k] {
			t.Fatalf("duplicate Key %q in AllKeys()", k)
		}
		seen[k] = true
	}
	if len(AllKeys()) == 0 {
		t.Fatal("AllKeys() is empty")
	}
}
