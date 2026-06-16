//go:build test

package ui

import (
	"errors"
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestNotificationOnImportSuccess(t *testing.T) {
	assertDefaultParityState(t)
	g := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, g)
	// The real onImport (wired by Game) queues data and returns nil; the actual
	// import and notification happen in Game.Update(). For this standalone test,
	// we simulate the notification that Game.Update() would show after a
	// successful import.
	dv.onImport = func(_ []byte) error {
		dv.notifyInfo("Imported project JSON")
		return nil
	}
	old := selectJSONAsyncFn
	selectJSONAsyncFn = func(cb func([]byte, error)) {}
	t.Cleanup(func() { selectJSONAsyncFn = old })
	startImportForTest(t, dv)
	dv.importCh <- importResult{data: []byte("{}"), err: nil}
	dv.Update()
	if dv.notifStore.Len() == 0 {
		t.Fatalf("expected a notification after import success")
	}
	if dv.notifStore.Latest().isErr {
		t.Fatalf("expected info notification, got error")
	}
}

func TestNotificationOnImportError(t *testing.T) {
	assertDefaultParityState(t)
	g := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, g)
	dv.onImport = func(_ []byte) error { return errors.New("bad json") }
	old := selectJSONAsyncFn
	selectJSONAsyncFn = func(cb func([]byte, error)) {}
	t.Cleanup(func() { selectJSONAsyncFn = old })
	startImportForTest(t, dv)
	dv.importCh <- importResult{data: []byte("{"), err: nil}
	dv.Update()
	if dv.notifStore.Len() == 0 || !dv.notifStore.Latest().isErr {
		t.Fatalf("expected error notification after import error")
	}
}

func TestNotificationOnUploadError(t *testing.T) {
	assertDefaultParityState(t)
	g := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, g)
	startUploadForTest(t, dv)
	_ = waitForUploadResult(t, dv)
	dv.uploadCh <- uploadResult{path: "", err: errors.New("select failed")}
	dv.Update()
	if dv.notifStore.Len() == 0 || !dv.notifStore.Latest().isErr {
		t.Fatalf("expected error notification for upload failure")
	}
}

func TestNotificationOnInvalidBPM(t *testing.T) {
	assertDefaultParityState(t)
	g := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, g)
	dv.calcLayout()
	// Open the shared editor, type an invalid value, then commit.
	dv.transportZone.openBPMEditor()
	ed := dv.transportZone.paramEditor
	ed.ti.SetText("abc")
	ed.commit()

	if ed.errorAnim == 0 {
		t.Fatalf("expected editor error animation on invalid entry")
	}
	if dv.notifStore.Len() == 0 || !dv.notifStore.Latest().isErr {
		t.Fatalf("expected error notification on invalid BPM")
	}
}
