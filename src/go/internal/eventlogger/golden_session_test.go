package eventlogger

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	gamelog "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestEventLoggerGoldenSession exercises the full publish → subscribe → format
// → emit pipeline against a deterministic event sequence and diffs the captured
// INFO log against testdata/golden_session.log. Any change to the user-facing
// log shape (rename, reorder, new line) must update the golden — making the
// intent visible in code review.
//
// The fixture intentionally uses only non-coalesced kinds so output ordering
// matches publish ordering. Coalescer behavior is covered separately in
// coalesce_test.go.
func TestEventLoggerGoldenSession(t *testing.T) {
	restore := gamelog.SetTimestampFunc(func() string { return "00:00:00.000" })
	defer restore()

	bus := hooks.NewBus(context.Background(), hooks.Options{
		PoolWorkers: 1, PoolQueue: 256, Name: "eventlogger.golden",
	})
	t.Cleanup(func() { _ = bus.Close() })

	var buf bytes.Buffer
	lg := gamelog.NewForTest(&buf, gamelog.LevelInfo)

	logger, err := Open(bus, lg, Options{Verbose: false})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() {
		_ = logger.Close()
		// Release the test pool from the registry so subsequent runs don't
		// inherit a stale instance via the registry's name-based caching.
		// (eventlogger.format is the standing pool, not test-private; we
		// keep it for the goleak baseline.)
		_ = async.DefaultRegistry()
	})

	// Publish a deterministic sequence covering every non-verbose Kind that
	// is NOT coalesced, in user-recognizable narrative order.
	bus.PublishKind(hooks.EventPlayStart, nil)
	bus.PublishKind(hooks.EventNodeAdded, hooks.NodeEdit{ID: 1, I: 0, J: 0, Type: "regular"})
	bus.PublishKind(hooks.EventNodeAdded, hooks.NodeEdit{ID: 2, I: 1, J: 0, Type: "regular"})
	bus.PublishKind(hooks.EventEdgeAdded, hooks.EdgeEdit{FromID: 1, ToID: 2, FromI: 0, FromJ: 0, ToI: 1, ToJ: 0})
	bus.PublishKind(hooks.EventStartNodeChanged, hooks.StartNodePayload{Row: 0, ID: 1})
	bus.PublishKind(hooks.EventRowAdded, hooks.RowChangePayload{Row: 0, Name: "kick", Instrument: "kick"})
	bus.PublishKind(hooks.EventRowInstrumentChange, hooks.RowChangePayload{Row: 0, OldInstrument: "kick", Instrument: "snare"})
	bus.PublishKind(hooks.EventInsertEffectAdded, hooks.InsertEffectPayload{Channel: "snare", Slot: 0, Type: "reverb"})
	bus.PublishKind(hooks.EventInsertEffectRemoved, hooks.InsertEffectPayload{Channel: "snare", Slot: 0})
	bus.PublishKind(hooks.EventNodeTypeChanged, hooks.NodeTypePayload{ID: 2, OldType: "regular", NewType: "mute"})
	bus.PublishKind(hooks.EventNodeParamsChanged, hooks.NodeParamsPayload{ID: 1, Volume: 0.7})
	bus.PublishKind(hooks.EventEdgeDeleted, hooks.EdgeEdit{FromI: 0, FromJ: 0, ToI: 1, ToJ: 0})
	bus.PublishKind(hooks.EventNodeDeleted, hooks.NodeEdit{ID: 2, I: 1, J: 0})
	bus.PublishKind(hooks.EventRowMute, hooks.RowChangePayload{Row: 0, Mute: true})
	bus.PublishKind(hooks.EventRowSolo, hooks.RowChangePayload{Row: 0, Solo: false})
	bus.PublishKind(hooks.EventSubdivChange, hooks.SubdivPayload{Subdiv: 16})
	bus.PublishKind(hooks.EventLengthChange, hooks.LengthPayload{Length: 64})
	bus.PublishKind(hooks.EventRecordStart, nil)
	bus.PublishKind(hooks.EventRecordStop, nil)
	bus.PublishKind(hooks.EventImport, nil)
	bus.PublishKind(hooks.EventExport, nil)
	bus.PublishKind(hooks.EventSeek, hooks.SeekPayload{Beats: 8})
	bus.PublishKind(hooks.EventRowDeleted, hooks.RowChangePayload{Row: 0})
	bus.PublishKind(hooks.EventPaused, nil)
	bus.PublishKind(hooks.EventResumed, nil)
	bus.PublishKind(hooks.EventPlayStop, nil)

	const wantWritten = 26
	waitFor(t, func() bool { return logger.Stats().Written >= wantWritten }, 2*time.Second)

	got := buf.String()

	goldenPath := filepath.Join("testdata", "golden_session.log")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("update golden: %v", err)
		}
		t.Logf("updated golden at %s", goldenPath)
		return
	}

	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (set UPDATE_GOLDEN=1 to create): %v", err)
	}
	want := string(wantBytes)
	if got != want {
		t.Errorf("golden mismatch.\nGot:\n%s\nWant:\n%s\n(re-run with UPDATE_GOLDEN=1 to refresh after intentional changes)", got, want)
	}
}

// TestEventLoggerVerboseFiltersByDefault asserts that camera/drag verbose
// events do NOT appear at INFO unless Options.Verbose=true.
func TestEventLoggerVerboseFiltersByDefault(t *testing.T) {
	restore := gamelog.SetTimestampFunc(func() string { return "00:00:00.000" })
	defer restore()

	bus := hooks.NewBus(context.Background(), hooks.Options{PoolWorkers: 1, Name: "eventlogger.verbose-off"})
	t.Cleanup(func() { _ = bus.Close() })
	var buf bytes.Buffer
	lg := gamelog.NewForTest(&buf, gamelog.LevelInfo)
	logger, err := Open(bus, lg, Options{Verbose: false, CoalesceWindow: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	bus.PublishKind(hooks.EventCameraPan, hooks.CameraPanPayload{DX: 1, DY: 2})
	bus.PublishKind(hooks.EventCameraZoom, hooks.CameraZoomPayload{Factor: 1.1})
	bus.PublishKind(hooks.EventDragProgress, hooks.DragProgressPayload{NodeID: 1, I: 0, J: 0})
	bus.PublishKind(hooks.EventPlayStart, nil) // sentinel: must appear

	waitFor(t, func() bool { return logger.Stats().Written >= 1 }, 2*time.Second)
	logger.FlushNow()
	// Give any stray late delivery a moment to misbehave so we can catch it.
	time.Sleep(80 * time.Millisecond)

	out := buf.String()
	if strings.Contains(out, "[camera]") || strings.Contains(out, "[drag]") {
		t.Fatalf("verbose events leaked into INFO with Verbose=false:\n%s", out)
	}
	if !strings.Contains(out, "play started") {
		t.Fatalf("non-verbose event missing:\n%s", out)
	}
}

// TestEventLoggerVerboseEmitsWhenOptedIn confirms the opt-in path works.
func TestEventLoggerVerboseEmitsWhenOptedIn(t *testing.T) {
	restore := gamelog.SetTimestampFunc(func() string { return "00:00:00.000" })
	defer restore()

	bus := hooks.NewBus(context.Background(), hooks.Options{PoolWorkers: 1, Name: "eventlogger.verbose-on"})
	t.Cleanup(func() { _ = bus.Close() })
	var buf bytes.Buffer
	lg := gamelog.NewForTest(&buf, gamelog.LevelInfo)
	logger, err := Open(bus, lg, Options{Verbose: true, CoalesceWindow: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	bus.PublishKind(hooks.EventCameraZoom, hooks.CameraZoomPayload{Factor: 1.5})
	// Force coalesce window to elapse.
	time.Sleep(50 * time.Millisecond)
	logger.FlushNow()

	if !strings.Contains(buf.String(), "[camera] zoom factor=1.50") {
		t.Fatalf("verbose camera zoom missing with Verbose=true:\n%s", buf.String())
	}
}

// TestEventLoggerCoalescesRapidBPM publishes 20 BPMChange events in a tight
// burst and asserts only one INFO line is produced after the window closes.
// This is the end-to-end version of coalesce_test.go's unit test.
func TestEventLoggerCoalescesRapidBPM(t *testing.T) {
	restore := gamelog.SetTimestampFunc(func() string { return "00:00:00.000" })
	defer restore()

	bus := hooks.NewBus(context.Background(), hooks.Options{PoolWorkers: 1, Name: "eventlogger.bpm"})
	t.Cleanup(func() { _ = bus.Close() })
	var buf bytes.Buffer
	lg := gamelog.NewForTest(&buf, gamelog.LevelInfo)
	logger, err := Open(bus, lg, Options{CoalesceWindow: 40 * time.Millisecond})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	for i := 1; i <= 20; i++ {
		bus.PublishKind(hooks.EventBPMChange, float64(100+i))
	}
	// Wait past the coalesce window then flush any stragglers.
	time.Sleep(100 * time.Millisecond)
	logger.FlushNow()

	bpmLineRE := regexp.MustCompile(`\[bpm\] BPM = \d+\.\d`)
	matches := bpmLineRE.FindAllString(buf.String(), -1)
	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 BPM INFO line, got %d:\n%s", len(matches), buf.String())
	}
	if !strings.Contains(matches[0], "120.0") {
		t.Fatalf("expected trailing BPM = 120.0, got %q", matches[0])
	}
}

// waitFor polls cond every 2ms until it returns true or the timeout elapses.
func waitFor(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("waitFor: timeout")
}
