package env

import (
	"os"
	"testing"
	"time"
)

// Removes the scratch variable when the test ends, or it would stay in All() and break TestTableSize. NOT with
// t.Parallel(): a parallel test's cleanup runs after every serial test, TestTableSize included, has read All(). (Calling
// t.Parallel() before registering is fine.)
func scratch(t *testing.T, name string) string {
	t.Helper()
	t.Cleanup(func() { unregister(name) })
	return name
}

func TestBoolTruthySet(t *testing.T) {
	v := Bool(scratch(t, "BEATMO_TEST_BOOL"), false, ScopeTest, "unit test var")
	for _, s := range []string{"1", "true", "TRUE", "True", "yes", "on", "ON"} {
		t.Setenv("BEATMO_TEST_BOOL", s)
		Reload()
		if !v.Get() {
			t.Errorf("Get()=false for %q, want true", s)
		}
	}
	for _, s := range []string{"0", "false", "FALSE", "no", "off"} {
		t.Setenv("BEATMO_TEST_BOOL", s)
		Reload()
		if v.Get() {
			t.Errorf("Get()=true for %q, want false", s)
		}
	}
}

func TestBoolGarbageFallsBackToDefault(t *testing.T) {
	v := Bool(scratch(t, "BEATMO_TEST_BOOL_DEF"), true, ScopeTest, "unit test var")
	t.Setenv("BEATMO_TEST_BOOL_DEF", "banana")
	Reload()
	if !v.Get() {
		t.Errorf("Get()=false for garbage value, want default true")
	}
}

func TestUnsetUsesDefault(t *testing.T) {
	v := Int(scratch(t, "BEATMO_TEST_INT_UNSET"), 42, ScopeTest, "unit test var")
	_ = os.Unsetenv("BEATMO_TEST_INT_UNSET")
	Reload()
	if got := v.Get(); got != 42 {
		t.Errorf("Get()=%d, want 42", got)
	}
}

func TestIntParseErrorFallsBackToDefault(t *testing.T) {
	v := Int(scratch(t, "BEATMO_TEST_INT_BAD"), 7, ScopeTest, "unit test var")
	t.Setenv("BEATMO_TEST_INT_BAD", "not-a-number")
	Reload()
	if got := v.Get(); got != 7 {
		t.Errorf("Get()=%d, want default 7", got)
	}
}

func TestFloatAndDur(t *testing.T) {
	f := Float(scratch(t, "BEATMO_TEST_FLOAT"), 1.5, ScopeTest, "unit test var")
	t.Setenv("BEATMO_TEST_FLOAT", "2.25")
	d := Dur(scratch(t, "BEATMO_TEST_DUR_MS"), 100*time.Millisecond, ScopeTest, "unit test var; value in milliseconds")
	t.Setenv("BEATMO_TEST_DUR_MS", "250")
	Reload()
	if got := f.Get(); got != 2.25 {
		t.Errorf("Float Get()=%v, want 2.25", got)
	}
	if got := d.Get(); got != 250*time.Millisecond {
		t.Errorf("Dur Get()=%v, want 250ms", got)
	}
}

func TestSetForTestOverridesAndRestores(t *testing.T) {
	v := Bool(scratch(t, "BEATMO_TEST_OVERRIDE"), false, ScopeTest, "unit test var")
	Reload()
	if v.Get() {
		t.Fatalf("precondition: want false")
	}
	restore := v.SetForTest(true)
	if !v.Get() {
		t.Errorf("Get()=false after SetForTest(true)")
	}
	restore()
	if v.Get() {
		t.Errorf("Get()=true after restore, want false")
	}
}

func TestSetForTestSurvivesReload(t *testing.T) {
	v := Bool(scratch(t, "BEATMO_TEST_OVERRIDE_RELOAD"), false, ScopeTest, "unit test var")
	Reload()
	restore := v.SetForTest(true)
	defer restore()
	Reload()
	if !v.Get() {
		t.Errorf("Get()=false after Reload(); a test override must survive Reload")
	}
}

func TestIsSetDistinguishesAbsentFromDefaultValued(t *testing.T) {
	v := Int(scratch(t, "BEATMO_TEST_ISSET"), 5, ScopeTest, "unit test var")

	_ = os.Unsetenv("BEATMO_TEST_ISSET")
	Reload()
	if v.IsSet() {
		t.Errorf("IsSet()=true when unset, want false")
	}

	// A value equal to the default is still explicitly set.
	t.Setenv("BEATMO_TEST_ISSET", "5")
	Reload()
	if !v.IsSet() {
		t.Errorf("IsSet()=false for an explicit value equal to the default, want true")
	}

	// A present-but-unparseable value counts as unset so callers guarding on
	// IsSet do not write a fallback over a builder-supplied value.
	t.Setenv("BEATMO_TEST_ISSET", "banana")
	Reload()
	if v.IsSet() {
		t.Errorf("IsSet()=true for an unparseable value, want false")
	}
	if got := v.Get(); got != 5 {
		t.Errorf("Get()=%d for an unparseable value, want default 5", got)
	}
}

func TestFloatIsSetUnparseableFallsBackToDefault(t *testing.T) {
	v := Float(scratch(t, "BEATMO_TEST_FLOAT_ISSET"), 1.5, ScopeTest, "unit test var")
	t.Setenv("BEATMO_TEST_FLOAT_ISSET", "not-a-float")
	Reload()
	if v.IsSet() {
		t.Errorf("IsSet()=true for an unparseable float, want false")
	}
	if got := v.Get(); got != 1.5 {
		t.Errorf("Get()=%v for an unparseable float, want default 1.5", got)
	}
}

func TestDurIsSetUnparseableFallsBackToDefault(t *testing.T) {
	v := Dur(scratch(t, "BEATMO_TEST_DUR_ISSET_MS"), 100*time.Millisecond, ScopeTest, "unit test var; value in milliseconds")
	t.Setenv("BEATMO_TEST_DUR_ISSET_MS", "not-a-duration")
	Reload()
	if v.IsSet() {
		t.Errorf("IsSet()=true for an unparseable duration, want false")
	}
	if got := v.Get(); got != 100*time.Millisecond {
		t.Errorf("Get()=%v for an unparseable duration, want default 100ms", got)
	}
}

// An unrecognised bool string is still a value the environment provided (there is no "invalid" bool literal), so
// IsSet is true while Get falls back; a numeric parse error is unambiguous, so those report unset.
func TestBoolGarbageIsSetTrue(t *testing.T) {
	v := Bool(scratch(t, "BEATMO_TEST_BOOL_ISSET_GARBAGE"), false, ScopeTest, "unit test var")
	t.Setenv("BEATMO_TEST_BOOL_ISSET_GARBAGE", "banana")
	Reload()
	if !v.IsSet() {
		t.Errorf("IsSet()=false for a present-but-unrecognized bool value, want true")
	}
	if v.Get() {
		t.Errorf("Get()=true for a present-but-unrecognized bool value, want default false")
	}
}

func TestAllIsSortedAndContainsRegistered(t *testing.T) {
	Str(scratch(t, "BEATMO_TEST_ZZZ"), "", ScopeTest, "unit test var")
	Str(scratch(t, "BEATMO_TEST_AAA"), "", ScopeTest, "unit test var")
	all := All()
	var prev string
	found := map[string]bool{}
	for _, v := range all {
		if prev != "" && v.Name() < prev {
			t.Fatalf("All() not sorted: %q after %q", v.Name(), prev)
		}
		prev = v.Name()
		found[v.Name()] = true
	}
	for _, want := range []string{"BEATMO_TEST_AAA", "BEATMO_TEST_ZZZ"} {
		if !found[want] {
			t.Errorf("All() missing %q", want)
		}
	}
}

func TestDuplicateNamePanics(t *testing.T) {
	// Cleanup registered before the panic: the first Str registers the name, and t.Cleanup runs regardless.
	name := scratch(t, "BEATMO_TEST_DUP")
	defer func() {
		if recover() == nil {
			t.Errorf("registering a duplicate name did not panic")
		}
	}()
	Str(name, "", ScopeTest, "unit test var")
	Str(name, "", ScopeTest, "unit test var")
}
