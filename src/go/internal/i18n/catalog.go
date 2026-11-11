package i18n

import (
	"sync"
	"sync/atomic"
)

var catalogs = map[Locale]map[Key]string{
	LocaleEN: en,
	LocaleES: es,
}

func catalogFor(l Locale) map[Key]string { return catalogs[l] }

var activeLocale atomic.Value // stores Locale

func init() { activeLocale.Store(LocaleEN) }

func currentLocale() Locale {
	if v, ok := activeLocale.Load().(Locale); ok {
		return v
	}
	return LocaleEN
}

var (
	listenersMu  sync.Mutex
	listeners    = map[int]func(){}
	listenerNext int
	fontGen      int64
)
