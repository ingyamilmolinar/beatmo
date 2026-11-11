package i18n

// withTempKey injects a temporary message into both catalogs for the duration
// of a test, restoring it via t.Cleanup. Test-only: kept in a _test.go file so
// it never ships in a production binary and the en/es maps stay provably
// immutable post-init (which is what makes concurrent T/ActiveLocale reads
// race-free without locking the catalogs).
func withTempKey(t interface{ Cleanup(func()) }, k Key, enVal, esVal string) {
	en[k] = enVal
	es[k] = esVal
	t.Cleanup(func() { delete(en, k); delete(es, k) })
}
