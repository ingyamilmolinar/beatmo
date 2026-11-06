//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestToastTaskIsSticky pins that a task toast does NOT expire on the error
// toast's TTL. A progress indicator that vanishes after 4s while the work is
// still running is worse than none.
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

// TestToastTaskRendersLocalisedProgress pins that the label is built from an
// i18n key each frame (so a mid-load locale switch is reflected) and carries
// the done/total counts.
func TestToastTaskRendersLocalisedProgress(t *testing.T) {
	var tt logsToast
	tt.beginTask(i18n.KeyNotifCircuitLoading, 120)
	tt.updateTask(42)
	got := tt.label()
	if !strings.Contains(got, "42") || !strings.Contains(got, "120") {
		t.Errorf("task label %q missing progress counts", got)
	}
}

// TestToastErrorPathUnchanged pins that the pre-existing error toast behaviour
// (plain string, TTL expiry) is untouched by the task mode.
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
