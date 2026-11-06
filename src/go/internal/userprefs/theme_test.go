//go:build !js

package userprefs

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

func TestThemeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	s := NewBackingStore(Options{Path: path, PoolName: uniquePoolName(t), Logger: t.Logf})
	t.Cleanup(func() { _ = s.Close() })
	ts, ok := s.(ThemeStore)
	if !ok {
		t.Fatal("backing store does not implement ThemeStore")
	}
	want := theme.UserDoc{Doc: theme.Doc{Categories: map[string]string{"accent": "orchid-300"}, Tokens: map[string]string{"focus-ring": "#FF2D9E"}}, OverrideCircuits: true}
	if err := ts.SaveTheme(want); err != nil {
		t.Fatal(err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatal(err)
	}
	s2 := NewBackingStore(Options{Path: path, PoolName: uniquePoolName(t), Logger: t.Logf})
	t.Cleanup(func() { _ = s2.Close() })
	got, err := s2.(ThemeStore).LoadTheme()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Doc.Equal(want.Doc) || got.OverrideCircuits != want.OverrideCircuits {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestThemeAbsentIsEmptyAndOmitted(t *testing.T) {
	snap, err := parsePrefs([]byte(`{"schema_version":"0.0.1","language":"es"}`), t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.theme.Doc.IsEmpty() || snap.theme.OverrideCircuits {
		t.Fatalf("absent theme must load empty: %+v", snap.theme)
	}
	out, err := marshalPrefs(snap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"theme"`) {
		t.Fatalf("empty theme must be omitted on write: %s", out)
	}
	snap.theme.OverrideCircuits = true
	out, _ = marshalPrefs(snap)
	if !strings.Contains(string(out), `"override_circuits": true`) {
		t.Fatalf("override flag alone must persist: %s", out)
	}
}
