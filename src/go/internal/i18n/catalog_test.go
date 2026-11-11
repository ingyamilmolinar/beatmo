package i18n

import "testing"

func TestCatalogCompleteness(t *testing.T) {
	want := map[Key]bool{}
	for _, k := range AllKeys() {
		want[k] = true
	}
	for _, loc := range AllLocales() {
		cat := catalogFor(loc)
		if cat == nil {
			t.Fatalf("no catalog for %s", loc)
		}
		for k := range want {
			v, ok := cat[k]
			if !ok {
				t.Errorf("locale %s missing key %q", loc, k)
				continue
			}
			if v == "" {
				t.Errorf("locale %s empty value for key %q", loc, k)
			}
		}
		for k := range cat {
			if !want[k] {
				t.Errorf("locale %s has orphan key %q (not in AllKeys())", loc, k)
			}
		}
	}
}
