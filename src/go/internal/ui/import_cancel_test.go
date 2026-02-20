package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestImportEmptyPayloadIgnored(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	var calls int
	dv.onImport = func(_ []byte) error {
		calls++
		return nil
	}
	old := selectJSONAsyncFn
	selectJSONAsyncFn = func(cb func([]byte, error)) {}
	t.Cleanup(func() { selectJSONAsyncFn = old })

	startImportForTest(t, dv)
	dv.importCh <- importResult{data: nil, err: nil}
	dv.Update()

	if calls != 0 {
		t.Fatalf("onImport invoked for empty payload")
	}
	if len(dv.notifs) != 0 {
		t.Fatalf("unexpected notification after empty import: %+v", dv.notifs)
	}

	payload := []byte(`{}`)
	startImportForTest(t, dv)
	dv.importCh <- importResult{data: payload, err: nil}
	dv.Update()

	if calls != 1 {
		t.Fatalf("expected onImport to run once after valid payload, got %d", calls)
	}
}
