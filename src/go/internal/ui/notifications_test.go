//go:build test

package ui

import (
	"errors"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
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
	if len(dv.notifs) == 0 {
		t.Fatalf("expected a notification after import success")
	}
	if dv.notifs[len(dv.notifs)-1].isErr {
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
	if len(dv.notifs) == 0 || !dv.notifs[len(dv.notifs)-1].isErr {
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
	if len(dv.notifs) == 0 || !dv.notifs[len(dv.notifs)-1].isErr {
		t.Fatalf("expected error notification for upload failure")
	}
}

func TestNotificationOnInvalidBPM(t *testing.T) {
	assertDefaultParityState(t)
	g := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, g)
	dv.calcLayout()
	focusTextInput(t, dv, dv.bpmBox())
	dv.bpmBox().SetText("abc")
	// Simulate Enter to commit invalid BPM
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 200 },
	)
	defer restore()
	dv.Update()
	if dv.bpmErrorAnim == 0 {
		t.Fatalf("expected bpm error animation on invalid entry")
	}
	if len(dv.notifs) == 0 || !dv.notifs[len(dv.notifs)-1].isErr {
		t.Fatalf("expected error notification on invalid BPM")
	}
}
