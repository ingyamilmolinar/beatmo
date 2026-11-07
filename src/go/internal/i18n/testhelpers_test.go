package i18n

// Test-only, so it never ships and the catalogs stay immutable after init: that is what makes concurrent T /
// ActiveLocale reads race-free without a lock.
func withTempKey(t interface{ Cleanup(func()) }, k Key, enVal, esVal string) {
	en[k] = enVal
	es[k] = esVal
	t.Cleanup(func() { delete(en, k); delete(es, k) })
}
