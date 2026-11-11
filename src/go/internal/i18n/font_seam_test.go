package i18n

import "testing"

func TestFontGenerationMonotonic(t *testing.T) {
	g0 := FontGeneration()
	BumpFontGeneration()
	if FontGeneration() <= g0 {
		t.Fatal("FontGeneration must increase after BumpFontGeneration")
	}
}

func TestFontProviderBumpsGenerationOnDistinctFace(t *testing.T) {
	defer SetLocale(LocaleEN)
	defer SetFontProvider(nil)
	SetLocale(LocaleEN)
	SetFontProvider(func(l Locale) string {
		if l == LocaleES {
			return "cjk"
		}
		return "latin"
	})
	g0 := FontGeneration()
	SetLocale(LocaleES)
	if FontGeneration() == g0 {
		t.Fatal("switching to a locale with a distinct face must bump FontGeneration")
	}
}
