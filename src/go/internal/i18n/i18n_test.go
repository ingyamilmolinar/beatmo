package i18n

import "testing"

func TestTFallbackChain(t *testing.T) {
	defer SetLocale(LocaleEN)
	SetLocale(LocaleES)
	if got := T(KeyTransportPlay); got == "" {
		t.Fatal("ES T returned empty")
	}
	if got := T(Key("does.not.exist")); got != "does.not.exist" {
		t.Fatalf("unknown key fallback=%q", got)
	}
}

func TestTfFormats(t *testing.T) {
	defer SetLocale(LocaleEN)
	SetLocale(LocaleEN)
	withTempKey(t, Key("tmp.fmt"), "hello %s", "hola %s")
	if got := Tf(Key("tmp.fmt"), "world"); got != "hello world" {
		t.Fatalf("Tf=%q", got)
	}
}

func TestOnChangeFires(t *testing.T) {
	defer SetLocale(LocaleEN)
	n := 0
	cancel := OnChange(func() { n++ })
	SetLocale(LocaleES)
	SetLocale(LocaleEN)
	if n != 2 {
		t.Fatalf("OnChange fired %d times, want 2", n)
	}
	// cancel() must actually unsubscribe: no further increments after it.
	cancel()
	SetLocale(LocaleES)
	SetLocale(LocaleEN)
	if n != 2 {
		t.Fatalf("OnChange fired after cancel(): n=%d want 2", n)
	}
}

func TestActiveLocale(t *testing.T) {
	defer SetLocale(LocaleEN)
	SetLocale(LocaleES)
	if ActiveLocale() != LocaleES {
		t.Fatalf("ActiveLocale=%q want es", ActiveLocale())
	}
}
