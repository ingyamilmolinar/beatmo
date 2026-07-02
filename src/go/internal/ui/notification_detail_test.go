//go:build test

package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestImportSourceName reduces a file path / template name to the short label
// shown in the "Loaded …" notification (basename, separator-agnostic).
func TestImportSourceName(t *testing.T) {
	cases := map[string]string{
		"/home/u/beat.json":      "beat.json",
		`C:\songs\my track.json`: "my track.json",
		"song.json":              "song.json",
		"Salsa":                  "Salsa",
		"":                       "",
	}
	for in, want := range cases {
		if got := importSourceName(in); got != want {
			t.Errorf("importSourceName(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestImportNotificationNamesSource drives a real import through the production
// drain (onImport → pendingImportData → g.Update → notify) and asserts the
// success notification names the loaded source (a template / JSON filename) and
// its node/row counts — not the bare "Imported project JSON".
func TestImportNotificationNamesSource(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}
	if err := g.drum.onImport(data, "Salsa"); err != nil {
		t.Fatalf("onImport: %v", err)
	}
	restore := stubAllInput(0, 0, map[ebiten.Key]bool{}, map[ebiten.Key]bool{}, false)
	defer restore()
	if err := g.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := g.drum.notifStore.Latest().display()
	want := fmt.Sprintf("Loaded Salsa (%d nodes, %d rows)", len(g.graph.Nodes), len(g.drum.Rows))
	if got != want {
		t.Fatalf("import notification = %q, want %q", got, want)
	}
	// Retranslates: source name + counts survive a locale switch.
	i18n.SetLocale(i18n.LocaleES)
	if got := g.drum.notifStore.Latest().display(); !strings.Contains(got, "Salsa") || !strings.HasPrefix(got, "Cargado") {
		t.Fatalf("ES import notification = %q, want it to start \"Cargado\" and contain \"Salsa\"", got)
	}
}

// TestInvalidBPMNotificationShowsValueAndRange asserts the invalid-BPM toast
// names the rejected value and the valid range, not a bare "Invalid BPM".
func TestInvalidBPMNotificationShowsValueAndRange(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	n := notification{key: i18n.KeyNotifInvalidBPMDetail, args: []string{"5000", "1000"}}
	if got := n.display(); got != "Invalid BPM: 5000 (1–1000)" {
		t.Fatalf("invalid BPM detail = %q, want %q", got, "Invalid BPM: 5000 (1–1000)")
	}
}
