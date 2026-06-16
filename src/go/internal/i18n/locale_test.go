package i18n

import "testing"

func TestParseLocale(t *testing.T) {
	cases := map[string]Locale{"en": LocaleEN, "es": LocaleES, "": LocaleEN, "zz": LocaleEN}
	for in, want := range cases {
		if got := ParseLocale(in); got != want {
			t.Fatalf("ParseLocale(%q)=%q want %q", in, got, want)
		}
	}
}

func TestAllLocales(t *testing.T) {
	got := AllLocales()
	if len(got) != 2 || got[0] != LocaleEN || got[1] != LocaleES {
		t.Fatalf("AllLocales()=%v", got)
	}
}
