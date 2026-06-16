// Package i18n is the single source of truth for user-facing UI text. It is a
// leaf package (no other internal imports) so any package may depend on it.
package i18n

// Locale identifies a supported language.
type Locale string

const (
	LocaleEN Locale = "en"
	LocaleES Locale = "es" // neutral Latin-American Spanish (es-419)
)

// AllLocales returns every supported locale in display order.
func AllLocales() []Locale { return []Locale{LocaleEN, LocaleES} }

// ParseLocale maps a stored string to a Locale, defaulting to English for empty
// or unrecognised input.
func ParseLocale(s string) Locale {
	switch Locale(s) {
	case LocaleES:
		return LocaleES
	default:
		return LocaleEN
	}
}

func (l Locale) String() string { return string(l) }
