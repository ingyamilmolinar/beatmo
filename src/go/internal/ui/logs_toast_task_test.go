//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// A progress indicator that vanishes while the work still runs is worse than none.
func TestToastTaskIsSticky(t *testing.T) {
	var tt logsToast
	tt.beginTask(i18n.KeyNotifCircuitLoading, 10)
	for i := 0; i < logsToastTTLFrames*3; i++ {
		tt.tick()
	}
	if !tt.active() {
		t.Fatal("task toast expired on the error TTL; it must be sticky")
	}
	tt.endTask()
	if tt.active() {
		t.Fatal("task toast still active after endTask")
	}
}

// Built from an i18n key each frame, so a locale switch mid-load is reflected.
func TestToastTaskRendersLocalisedProgress(t *testing.T) {
	var tt logsToast
	tt.beginTask(i18n.KeyNotifCircuitLoading, 120)
	tt.updateTask(42)
	got := tt.label()
	if !strings.Contains(got, "42") || !strings.Contains(got, "120") {
		t.Errorf("task label %q missing progress counts", got)
	}
}

func TestToastErrorPathUnchanged(t *testing.T) {
	var tt logsToast
	tt.push("boom")
	if !tt.active() {
		t.Fatal("error toast inactive right after push")
	}
	for i := 0; i < logsToastTTLFrames+1; i++ {
		tt.tick()
	}
	if tt.active() {
		t.Fatal("error toast did not expire on its TTL")
	}
}
