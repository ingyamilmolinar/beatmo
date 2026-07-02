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
	pubFixed(bus, hooks.EventPlayStart, nil)
	pubFixed(bus, hooks.EventNodeAdded, hooks.NodeEdit{ID: 1, I: 0, J: 0, Type: "regular"})
	pubFixed(bus, hooks.EventNodeAdded, hooks.NodeEdit{ID: 2, I: 1, J: 0, Type: "regular"})
	pubFixed(bus, hooks.EventEdgeAdded, hooks.EdgeEdit{FromID: 1, ToID: 2, FromI: 0, FromJ: 0, ToI: 1, ToJ: 0})
	pubFixed(bus, hooks.EventStartNodeChanged, hooks.StartNodePayload{Row: 0, ID: 1})
	pubFixed(bus, hooks.EventRowAdded, hooks.RowChangePayload{Row: 0, Name: "kick", Instrument: "kick"})
	pubFixed(bus, hooks.EventRowInstrumentChange, hooks.RowChangePayload{Row: 0, OldInstrument: "kick", Instrument: "snare"})
	pubFixed(bus, hooks.EventInsertEffectAdded, hooks.InsertEffectPayload{Channel: "snare", Slot: 0, Type: "reverb"})
	pubFixed(bus, hooks.EventInsertEffectRemoved, hooks.InsertEffectPayload{Channel: "snare", Slot: 0})
	pubFixed(bus, hooks.EventNodeTypeChanged, hooks.NodeTypePayload{ID: 2, OldType: "regular", NewType: "mute"})
	pubFixed(bus, hooks.EventNodeParamsChanged, hooks.NodeParamsPayload{ID: 1, Volume: 0.7})
	pubFixed(bus, hooks.EventEdgeDeleted, hooks.EdgeEdit{FromI: 0, FromJ: 0, ToI: 1, ToJ: 0})
	pubFixed(bus, hooks.EventNodeDeleted, hooks.NodeEdit{ID: 2, I: 1, J: 0})
	pubFixed(bus, hooks.EventRowMute, hooks.RowChangePayload{Row: 0, Mute: true})
	pubFixed(bus, hooks.EventRowSolo, hooks.RowChangePayload{Row: 0, Solo: false})
	pubFixed(bus, hooks.EventSubdivChange, hooks.SubdivPayload{Subdiv: 16})
	pubFixed(bus, hooks.EventLengthChange, hooks.LengthPayload{Length: 64})
	pubFixed(bus, hooks.EventRecordStart, nil)
	pubFixed(bus, hooks.EventRecordStop, nil)
	pubFixed(bus, hooks.EventImport, nil)
	pubFixed(bus, hooks.EventExport, nil)
	pubFixed(bus, hooks.EventSeek, hooks.SeekPayload{Beats: 8})
	pubFixed(bus, hooks.EventRowDeleted, hooks.RowChangePayload{Row: 0})
	pubFixed(bus, hooks.EventPaused, nil)
	pubFixed(bus, hooks.EventResumed, nil)
	pubFixed(bus, hooks.EventPlayStop, nil)
	// Round 2 additions exercised in golden:
	pubFixed(bus, hooks.EventCustomWAVLoaded, hooks.CustomWAVPayload{InstrumentID: "snare2", IsUpdate: false})
	pubFixed(bus, hooks.EventInstrumentRenamed, hooks.InstrumentRenamePayload{OldID: "kick", NewID: "kick_v2"})
	pubFixed(bus, hooks.EventSceneApplied, hooks.ScenePayload{Name: "transport_idle"})
	pubFixed(bus, hooks.EventUIStateApplied, hooks.UIStatePayload{Path: "/tmp/ui.json"})
	pubFixed(bus, hooks.EventFavoriteToggled, hooks.FavoritePayload{InstrumentID: "kick", IsFavorite: true})
	// EventRowColorChanged is coalesced; FlushNow below drains it.
	pubFixed(bus, hooks.EventRowColorChanged, hooks.RowColorPayload{Row: 0, Color: 0xFF8040FF})

	const wantWritten = 32
	// Wait for the non-coalesced events first.
	waitFor(t, func() bool { return logger.Stats().Written >= wantWritten-1 }, 2*time.Second)
	// Drain coalesced (row color) so the golden contains it deterministically.
	logger.FlushNow()
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

// TestEventLoggerVerboseFiltersByDefault asserts that drag-progress (still
// verbose) does NOT appear at INFO, while camera pan (promoted out of verbose)
// DOES appear by default.
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

	pubFixed(bus, hooks.EventCameraPan, hooks.CameraPanPayload{DX: 1, DY: 2})
	pubFixed(bus, hooks.EventDragProgress, hooks.DragProgressPayload{NodeID: 1, I: 0, J: 0})
	pubFixed(bus, hooks.EventPlayStart, nil)

	waitFor(t, func() bool { return logger.Stats().Written >= 1 }, 2*time.Second)
	logger.FlushNow()
	time.Sleep(80 * time.Millisecond)

	out := buf.String()
	if strings.Contains(out, "[drag]") {
		t.Fatalf("drag (still verbose) leaked into INFO:\n%s", out)
	}
	if !strings.Contains(out, "[camera] pan") {
		t.Fatalf("camera pan should now appear at INFO by default:\n%s", out)
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

	pubFixed(bus, hooks.EventCameraZoom, hooks.CameraZoomPayload{Factor: 1.5})
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
		pubFixed(bus, hooks.EventBPMChange, float64(100+i))
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

// TestEventLoggerCoalescesRapidScroll publishes 10 EventScroll events in a tight
// burst and asserts only one INFO line is produced after the coalesce window
// closes. Mirror of TestEventLoggerCoalescesRapidBPM, adapted for scroll.
func TestEventLoggerCoalescesRapidScroll(t *testing.T) {
	restore := gamelog.SetTimestampFunc(func() string { return "00:00:00.000" })
	defer restore()

	bus := hooks.NewBus(context.Background(), hooks.Options{PoolWorkers: 1, Name: "eventlogger.scroll"})
	t.Cleanup(func() { _ = bus.Close() })
	var buf bytes.Buffer
	lg := gamelog.NewForTest(&buf, gamelog.LevelInfo)
	logger, err := Open(bus, lg, Options{CoalesceWindow: 40 * time.Millisecond})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })

	for i := 0; i < 10; i++ {
		pubFixed(bus, hooks.EventScroll, hooks.ScrollPayload{Surface: "inst-menu"})
	}
	// Wait past the coalesce window then flush any stragglers.
	time.Sleep(100 * time.Millisecond)
	logger.FlushNow()

	scrollLineRE := regexp.MustCompile(`\[scroll\] scrolled inst-menu`)
	matches := scrollLineRE.FindAllString(buf.String(), -1)
	if len(matches) != 1 {
		t.Fatalf("expected exactly 1 scroll INFO line, got %d:\n%s", len(matches), buf.String())
	}
}

// fixedSource is the deterministic Source attached to every event published
// via pubFixed. Pinning the source keeps the golden file stable across edits
// to this test file (real runtime.Caller-derived sources would shift on
// every refactor and produce noisy goldens).
var fixedSource = hooks.Source{Pkg: "internal/test", File: "fixture.go", Line: 1}

// pubFixed publishes via the source-aware API with a stable injected Source
// so the golden output is deterministic. Production callers go through
// emit helpers in internal/ui/event_helpers.go that use real runtime.Caller
// capture; this fixture trades that fidelity for golden stability.
func pubFixed(bus *hooks.Bus, k hooks.Kind, payload any) {
	bus.PublishWithSource(k, payload, fixedSource)
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
