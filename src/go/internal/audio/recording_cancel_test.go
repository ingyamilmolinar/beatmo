//go:build test

package audio

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestStopRecording_FinalizeCompletesWithinDeadline asserts the
// async-finalize pipeline returns control to the caller fast (the UI
// thread should never block on disk I/O) and the on-disk artifacts are
// fully written before WaitRecordingFinalized returns. The plan calls
// out finalize as the "no context.Context" path — this test pins the
// invariant that, even without cancellation, finalize completes
// promptly under normal load.
func TestStopRecording_FinalizeCompletesWithinDeadline(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BEATMO_RECORDINGS_DIR", dir)

	opts := RecordingOptions{
		Format: FormatWAV16,
		Instruments: []InstrumentMeta{
			{ID: "kick", Name: "Kick"},
			{ID: "snare", Name: "Snare"},
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

	// Stop should return fast — finalize runs async on the lifecycle
	// pool. 200ms is generous; in practice this returns in <10ms even
	// on slow CI.
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

	// Verify a session directory was created. With no audio fed via Tap
	// we don't get session.json (SaveRecording skips empty sessions),
	// but the recording dir itself proves the lifecycle pool ran.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no recording session directory created; finalize did not run")
	}
}

// TestWaitRecordingFinalized_NoSessionReturnsImmediately documents the
// contract: with no in-flight finalize, Wait is a fast path so callers
// (CLI bench, shutdown handlers) can call it unconditionally.
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

// TestWaitRecordingFinalized_RespectsContextCancel verifies that if the
// caller's context is cancelled while waiting, Wait returns ctx.Err()
// promptly instead of blocking until finalize completes. Important for
// shutdown handlers that have a deadline.
func TestWaitRecordingFinalized_RespectsContextCancel(t *testing.T) {
	t.Setenv("BEATMO_RECORDINGS_DIR", t.TempDir())
	opts := RecordingOptions{
		Format:      FormatWAV16,
		Instruments: []InstrumentMeta{{ID: "kick", Name: "Kick"}},
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
