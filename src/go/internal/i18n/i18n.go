package i18n

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// T returns the message for k in the active locale, falling back to English and
// then to the raw key string. Never panics.
func T(k Key) string {
	loc := currentLocale()
	if v, ok := catalogs[loc][k]; ok {
		return v
	}
	if v, ok := en[k]; ok {
		return v
	}
	return string(k)
}

// Tf resolves k then applies fmt.Sprintf with args.
func Tf(k Key, args ...any) string { return fmt.Sprintf(T(k), args...) }

// Reverse-maps a notification persisted as English text with no key, so history retranslates in the current locale.
func EN(k Key) string {
	if v, ok := en[k]; ok {
		return v
	}
	return string(k)
}

// ActiveLocale returns the active locale.
func ActiveLocale() Locale { return currentLocale() }

// SetLocale switches the active locale and fires every OnChange listener.
func SetLocale(l Locale) {
	prevFace := FontIdentity()
	activeLocale.Store(l)
	if FontIdentity() != prevFace {
		BumpFontGeneration()
	}
	listenersMu.Lock()
	fns := make([]func(), 0, len(listeners))
	for _, fn := range listeners {
		fns = append(fns, fn)
	}
	listenersMu.Unlock()
	for _, fn := range fns {
		fn()
	}
}

// OnChange registers fn to run after every SetLocale. Returns a cancel func.
func OnChange(fn func()) (cancel func()) {
	listenersMu.Lock()
	id := listenerNext
	listenerNext++
	listeners[id] = fn
	listenersMu.Unlock()
	return func() {
		listenersMu.Lock()
		delete(listeners, id)
		listenersMu.Unlock()
	}
}

// --- CJK-ready font seam ---

// Written by SetFontProvider on the UI goroutine while the renderer may read it elsewhere: a bare func var would be a
// multi-word race.
var (
	fontMu       sync.RWMutex
	fontProvider func(Locale) string
)

// SetFontProvider installs the locale→face-identity resolver. nil clears it.
func SetFontProvider(fn func(Locale) string) {
	fontMu.Lock()
	fontProvider = fn
	fontMu.Unlock()
}

// FontIdentity returns the face identity for the active locale, or "" when no
// provider is installed.
func FontIdentity() string {
	fontMu.RLock()
	fn := fontProvider
	fontMu.RUnlock()
	if fn == nil {
		return ""
	}
	return fn(currentLocale())
}

// FontGeneration is a monotonically increasing token the renderer folds into
// its sprite-cache key.
func FontGeneration() int64 { return atomic.LoadInt64(&fontGen) }

// BumpFontGeneration increments the generation.
func BumpFontGeneration() int64 { return atomic.AddInt64(&fontGen, 1) }

// Has reports whether locale l's catalogue defines k (no English fallback) —
// for tests that key strings by data rather than by Go constant.
func Has(l Locale, k Key) bool {
	_, ok := catalogFor(l)[k]
	return ok
}
