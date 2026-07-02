//go:build test

package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestNotificationRetranslatesOnLocaleSwitch is the core requirement: a stored
// notification (and thus the history popup) must display in the CURRENT locale,
// not the locale it was created in. Notifications are stored as a translatable
// reference (i18n key + args) and rendered at display time, so switching to
// Spanish re-renders the whole history in Spanish.
func TestNotificationRetranslatesOnLocaleSwitch(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	dv := g.drum

	i18n.SetLocale(i18n.LocaleEN)
	dv.notifyInfoKey(i18n.KeyNotifRenamedInstrument, "Kick-1")
	if got := dv.notifStore.Latest().display(); got != "Renamed instrument to: Kick-1" {
		t.Fatalf("EN display = %q, want %q", got, "Renamed instrument to: Kick-1")
	}

	i18n.SetLocale(i18n.LocaleES)
	if got := dv.notifStore.Latest().display(); got != "Instrumento renombrado a: Kick-1" {
		t.Fatalf("ES display after switch = %q, want %q (history did not retranslate)", got, "Instrumento renombrado a: Kick-1")
	}
}

// TestUndoNotificationRetranslatesOnLocaleSwitch proves undo/redo notifications
// retranslate BOTH the "Undo:" prefix AND the action name when the locale flips.
func TestUndoNotificationRetranslatesOnLocaleSwitch(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	g := newTestGameForUndo(t)
	g.tryAddNode(7, 7, model.NodeTypeRegular)

	i18n.SetLocale(i18n.LocaleEN)
	g.performUndo()
	if got := g.drum.notifStore.Latest().display(); got != "Undo: add node" {
		t.Fatalf("EN undo display = %q, want %q", got, "Undo: add node")
	}

	i18n.SetLocale(i18n.LocaleES)
	if got := g.drum.notifStore.Latest().display(); got != "Deshacer: añadir nodo" {
		t.Fatalf("ES undo display after switch = %q, want %q", got, "Deshacer: añadir nodo")
	}
}

// TestNotificationRecordRoundTripsKeyArgs proves the persisted history carries
// the key + args (not just frozen text), so a reloaded history retranslates.
func TestNotificationRecordRoundTripsKeyArgs(t *testing.T) {
	in := []notification{{key: i18n.KeyNotifRenamedInstrument, args: []string{"Kick-1"}, unixMs: 5}}
	out := recordsToNotifs(notifsToRecords(in))
	if len(out) != 1 {
		t.Fatalf("round-trip dropped the record")
	}
	if out[0].key != i18n.KeyNotifRenamedInstrument {
		t.Fatalf("key lost in round-trip: %q", out[0].key)
	}
	if len(out[0].args) != 1 || out[0].args[0] != "Kick-1" {
		t.Fatalf("args lost in round-trip: %v", out[0].args)
	}
}

// TestNoUnlocalizedNotificationProducers is the exhaustiveness guard: no
// production code may push a PRE-RENDERED i18n string into a notification
// (notifyInfo/notifyError(i18n.T/Tf(...))). Such a string freezes in the
// creation-time locale and won't retranslate in the history. Every producer
// must use the keyed API (notifyInfoKey/notifyErrorKey) so the notification
// carries its key + args and renders in the current locale.
func TestNoUnlocalizedNotificationProducers(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	bad := regexp.MustCompile(`notify(Info|Error)\(i18n\.(T|Tf)\(`)
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if bad.Match(b) {
			t.Errorf("%s pushes a pre-rendered i18n notification (notify*(i18n.T/Tf(...))) — "+
				"use notifyInfoKey/notifyErrorKey so it retranslates in the history", f)
		}
	}
}

// TestLegacyTextNotificationRetranslates is the bug the user hit: a notification
// persisted as a frozen ENGLISH string (no key — created before keyed storage or
// in a prior English session) must STILL render in the current locale when the
// history is drawn. display() reverse-maps the rendered text back to its key.
func TestLegacyTextNotificationRetranslates(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	n := notification{text: "Renamed instrument to: Kick-1"} // legacy: text only, no key
	i18n.SetLocale(i18n.LocaleES)
	if got := n.display(); got != "Instrumento renombrado a: Kick-1" {
		t.Fatalf("legacy text notification did not retranslate: got %q, want %q", got, "Instrumento renombrado a: Kick-1")
	}
}

// TestLegacyUndoTextNotificationRetranslates proves a frozen "Undo: <action>"
// history line retranslates BOTH the prefix and the action name.
func TestLegacyUndoTextNotificationRetranslates(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	n := notification{text: "Undo: edit sample"}
	i18n.SetLocale(i18n.LocaleES)
	if got := n.display(); got != "Deshacer: editar muestra" {
		t.Fatalf("legacy undo text did not retranslate: got %q, want %q", got, "Deshacer: editar muestra")
	}
}

// TestLegacyRecordingSavedRetranslates proves an old entry rendered with the
// former %d format ("drops=0") still matches the current %s template and
// retranslates.
func TestLegacyRecordingSavedRetranslates(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	n := notification{text: "Recording saved (drops=0): beatmo-rec.zip"}
	i18n.SetLocale(i18n.LocaleES)
	if got := n.display(); got != "Grabación guardada (descartes=0): beatmo-rec.zip" {
		t.Fatalf("legacy recording text did not retranslate: got %q", got)
	}
}

// TestUnknownTextNotificationStaysVerbatim proves a non-notification plain string
// (no template match) is shown verbatim, never garbled.
func TestUnknownTextNotificationStaysVerbatim(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	n := notification{text: "a totally custom message"}
	i18n.SetLocale(i18n.LocaleES)
	if got := n.display(); got != "a totally custom message" {
		t.Fatalf("unknown text should stay verbatim, got %q", got)
	}
}
