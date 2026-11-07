//go:build test

package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Basename, whatever the separator.
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

// Through the production drain (onImport → pendingImportData → Update → notify).
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
	// The import's own line, not Latest(): the off-thread warm-up posts "Circuit ready" right after, racing this read.
	var imported *notification
	for _, n := range g.drum.notifStore.History() {
		if n.key == i18n.KeyNotifImportedNamed {
			n := n
			imported = &n
			break
		}
	}
	want := fmt.Sprintf("Loaded Salsa (%d nodes, %d rows)", len(g.graph.Nodes), len(g.drum.Rows))
	if imported == nil {
		t.Fatalf("no import notification posted; want %q", want)
	}
	if got := imported.display(); got != want {
		t.Fatalf("import notification = %q, want %q", got, want)
	}
	// Retranslates: source name + counts survive a locale switch.
	i18n.SetLocale(i18n.LocaleES)
	if got := imported.display(); !strings.Contains(got, "Salsa") || !strings.HasPrefix(got, "Cargado") {
		t.Fatalf("ES import notification = %q, want it to start \"Cargado\" and contain \"Salsa\"", got)
	}
}

func TestInvalidBPMNotificationShowsValueAndRange(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)
	n := notification{key: i18n.KeyNotifInvalidBPMDetail, args: []string{"5000", "1000"}}
	if got := n.display(); got != "Invalid BPM: 5000 (1–1000)" {
		t.Fatalf("invalid BPM detail = %q, want %q", got, "Invalid BPM: 5000 (1–1000)")
	}
}
