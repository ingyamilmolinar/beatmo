//go:build test

package audio

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The UI thread never blocks on disk I/O, yet everything is written by the time WaitRecordingFinalized returns.
func TestStopRecording_FinalizeCompletesWithinDeadline(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(env.RecordingsDir.SetForTest(dir))

	opts := RecordingOptions{
		Format: FormatWAV16,
		Instruments: []InstrumentMeta{
			{ID: channel.Of(roster.Kick), Name: "Kick"},
			{ID: channel.Of(roster.Snare), Name: "Snare"},
		},
		BPM: 120,
	}
	if err := StartRecording(opts); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}

	stopStart := time.Now()
	if _, err := StopRecording(); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}
	stopElapsed := time.Since(stopStart)

	// Generous; finalize runs on the lifecycle pool.
	if stopElapsed > 200*time.Millisecond {
		t.Fatalf("StopRecording blocked for %v; finalize should run async", stopElapsed)
	}

	// Now wait for finalize to actually complete. 5s covers slow disks.
	wctx, wcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer wcancel()
	waitStart := time.Now()
	if err := WaitRecordingFinalized(wctx); err != nil {
		t.Fatalf("WaitRecordingFinalized: %v", err)
	}
	waitElapsed := time.Since(waitStart)
	if waitElapsed > 4*time.Second {
		t.Fatalf("finalize took %v after Stop returned; should be sub-second under no load", waitElapsed)
	}

	// Empty sessions write no session.json, but the directory proves the pool ran.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no recording session directory created; finalize did not run")
	}
}

// So shutdown handlers can call it unconditionally.
func TestWaitRecordingFinalized_NoSessionReturnsImmediately(t *testing.T) {
	wctx, wcancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer wcancel()
	start := time.Now()
	err := WaitRecordingFinalized(wctx)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("WaitRecordingFinalized with no session: %v", err)
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Wait blocked for %v with no in-flight finalize; should be near-zero", elapsed)
	}
}

// For shutdown handlers with a deadline.
func TestWaitRecordingFinalized_RespectsContextCancel(t *testing.T) {
	t.Cleanup(env.RecordingsDir.SetForTest(t.TempDir()))
	opts := RecordingOptions{
		Format:      FormatWAV16,
		Instruments: []InstrumentMeta{{ID: channel.Of(roster.Kick), Name: "Kick"}},
		BPM:         120,
	}
	if err := StartRecording(opts); err != nil {
		t.Fatalf("StartRecording: %v", err)
	}
	if _, err := StopRecording(); err != nil {
		t.Fatalf("StopRecording: %v", err)
	}

	// Immediately cancel — Wait should return promptly with ctx.Err.
	wctx, wcancel := context.WithCancel(context.Background())
	wcancel()
	start := time.Now()
	err := WaitRecordingFinalized(wctx)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if elapsed > 100*time.Millisecond {
		t.Fatalf("Wait blocked for %v after cancel; should return promptly", elapsed)
	}

	// Drain the legitimate finalize so other tests don't see leftover state.
	wctx2, wcancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer wcancel2()
	_ = WaitRecordingFinalized(wctx2)
}
