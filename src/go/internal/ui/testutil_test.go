package ui

// ─── E2E Test Utilities ─────────────────────────────────────────────────────
//
// Essential test patterns for Go E2E/functional tests:
//
//   1. Always call assertDefaultParityState(t) at the start of every test to
//      reset parity globals and verify clean state.
//   2. After any circuit change (addNode, addEdge, etc.), MUST call
//      g.updateBeatInfos() + g.refreshDrumRow() to sync predictor and UI.
//   3. Use stopPlaybackForTest(g) for clean stop — it resets all sequencer
//      state (indices, pulses, highlights, parity).
//   4. Never access lastTriggeredByRow directly — use thread-safe helpers:
//      setLastTriggeredForTest, lastTriggeredForTest, lastTriggeredRowSnapshotForTest.
//   5. Use advanceFrames(g, N) to tick the game loop N times (~N/60 seconds).
//   6. Use withDefaultStart(t, false) to disable default start node for clean slate.
//   7. Use withDefaultAudio(t) to reset audio channels/instruments at test start.

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// click simulates a mouse click at (x,y) and releases it on the next frame.
func click(g *Game, x, y int) {
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.drum.Update()
	restore()
	g.drum.Update()
}

// clickDrumView simulates a mouse click at (x,y) against a DrumView.
func clickDrumView(t *testing.T, dv *DrumView, x, y int) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while clicking")
	}
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	dv.Update()
	restore()
	dv.Update()
}

func focusTextInput(t *testing.T, dv *DrumView, ti *TextInput) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while focusing text input")
	}
	if ti == nil {
		t.Fatalf("nil text input while focusing")
	}
	r := ti.Rect
	clickDrumView(t, dv, r.Min.X+1, r.Min.Y+1)
	if !ti.Focused() {
		t.Fatalf("text input not focused after click")
	}
}

func startUploadForTest(t *testing.T, dv *DrumView) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while starting upload")
	}
	if dv.uploadBtn() == nil || dv.uploadBtn().OnClick == nil {
		t.Fatalf("upload button missing while starting upload")
	}
	dv.uploadBtn().OnClick()
	if !dv.uploading {
		t.Fatalf("upload not started (uploading=%v naming=%v)", dv.uploading, dv.IsNamingOpen())
	}
}

func waitForUploadResult(t *testing.T, dv *DrumView) uploadResult {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while waiting for upload result")
	}
	for i := 0; i < 10000; i++ {
		select {
		case res := <-dv.uploadCh:
			return res
		default:
			runtime.Gosched()
		}
	}
	t.Fatalf("timeout waiting for upload result")
	return uploadResult{}
}

func waitForNaming(t *testing.T, dv *DrumView) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while waiting for naming")
	}
	for i := 0; i < 60; i++ {
		dv.Update()
		if dv.IsNamingOpen() {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("upload did not enter naming state (uploading=%v naming=%v)", dv.uploading, dv.IsNamingOpen())
}

func startImportForTest(t *testing.T, dv *DrumView) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while starting import")
	}
	if dv.importBtn() == nil || dv.importBtn().OnClick == nil {
		t.Fatalf("import button missing while starting import")
	}
	dv.importBtn().OnClick()
	if !dv.importing {
		t.Fatalf("import not started (importing=%v)", dv.importing)
	}
}

func closeImportForTest(t *testing.T, g *Game) {
	t.Helper()
	if g == nil || g.drum == nil {
		return
	}
	if g.drum.importing {
		for i := 0; i < 3 && g.drum.importing; i++ {
			g.drum.Update()
			runtime.Gosched()
		}
		if g.drum.importing {
			select {
			case g.drum.importCh <- importResult{data: nil, err: nil}:
			default:
			}
			g.drum.Update()
		}
	}
	g.endImportDialog()
}

func withDefaultStart(t *testing.T, enabled bool) {
	t.Helper()
	assertDefaultParityState(t)
	prev := enableDefaultStart
	if prev != true {
		t.Fatalf("enableDefaultStart=%v want true (default) before override", prev)
	}
	SetDefaultStartForTest(enabled)
	t.Cleanup(func() {
		SetDefaultStartForTest(prev)
	})
}

func withForceAutoSize(t *testing.T, enabled bool) {
	t.Helper()
	assertDefaultParityState(t)
	prev := forceAutoSize
	if prev != false {
		t.Fatalf("forceAutoSize=%v want false (default) before override", prev)
	}
	forceAutoSize = enabled
	t.Cleanup(func() {
		forceAutoSize = prev
	})
}

func withSmallScreen(t *testing.T, enabled bool) {
	t.Helper()
	prev := forceSmallScreenForTest
	if prev != false {
		t.Fatalf("forceSmallScreenForTest=%v want false (default) before override", prev)
	}
	forceSmallScreenForTest = enabled
	UpdateProfile()
	t.Cleanup(func() {
		forceSmallScreenForTest = prev
		UpdateProfile()
	})
}

func withAudioCatalog(t *testing.T, entries []audio.SoundMeta) {
	t.Helper()
	assertDefaultParityState(t)
	audio.ResetInstruments()
	audio.ResetCatalogForTest(entries)
	t.Cleanup(func() {
		audio.ResetInstruments()
		audio.ResetCatalogForTest(nil)
	})
}

func withDefaultAudio(t *testing.T) {
	t.Helper()
	assertDefaultParityState(t)
	audio.ResetInstruments()
	audio.ResetCatalogForTest(nil)
	t.Cleanup(func() {
		audio.ResetInstruments()
		audio.ResetCatalogForTest(nil)
	})
}

func testLogOutput() io.Writer {
	if testing.Verbose() {
		return os.Stdout
	}
	return io.Discard
}

func advanceFrames(g *Game, frames int) {
	if g == nil || frames <= 0 {
		return
	}
	for i := 0; i < frames; i++ {
		_ = g.Update()
	}
}

// advancePlaybackByAbs advances playback by a fixed number of absolute steps.
// It preserves the current absolute position to keep progression monotonic.
func advancePlaybackByAbs(g *Game, steps int) {
	if g == nil || steps <= 0 {
		return
	}
	start := g.elapsedBeats
	if start < 0 {
		start = 0
	}
	for i := 1; i <= steps; i++ {
		setPlayStartForAbs(g, start+i)
		_ = g.Update()
	}
}

// spinWait yields the scheduler until cond returns true or the step budget is exhausted.
func spinWait(t *testing.T, steps int, cond func() bool) {
	t.Helper()
	for i := 0; i < steps; i++ {
		if cond() {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("condition not met after %d steps", steps)
}

func waitForChan[T any](t *testing.T, ch <-chan T, steps int) T {
	t.Helper()
	var zero T
	if steps < 1 {
		steps = 1
	}
	timeout := time.Duration(steps/100) * time.Millisecond
	if timeout < 25*time.Millisecond {
		timeout = 25 * time.Millisecond
	}
	if timeout > 500*time.Millisecond {
		timeout = 500 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	for i := 0; i < steps; i++ {
		select {
		case v := <-ch:
			return v
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("channel did not receive within %v (%d steps)", timeout, steps)
			return zero
		}
		runtime.Gosched()
		time.Sleep(time.Microsecond)
	}
	t.Fatalf("channel did not receive after %d steps", steps)
	return zero
}

// waitForUpdateCond advances the game loop while waiting for cond to succeed.
func waitForUpdateCond(t *testing.T, g *Game, steps int, cond func() bool) {
	t.Helper()
	for i := 0; i < steps; i++ {
		if g != nil {
			_ = g.Update()
		}
		if cond() {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("condition not met after %d steps", steps)
}

// waitForCount waits until current() reaches at least want.
func waitForCount(t *testing.T, steps int, want int, current func() int) {
	t.Helper()
	for i := 0; i < steps; i++ {
		if current() >= want {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("count did not reach %d after %d steps (got %d)", want, steps, current())
}

func ensureInstrumentAvailable(t *testing.T, g *Game, id string) {
	t.Helper()
	if g == nil || g.drum == nil {
		t.Fatalf("missing drum view while ensuring instrument %q", id)
	}
	t.Cleanup(func() {
		audio.ResetInstruments()
		audio.ResetCatalogForTest(nil)
	})
	g.drum.refreshInstruments()
	if !g.drum.IsInstrumentAvailable(id) {
		_ = audio.RegisterWAV(id, "test://placeholder.wav")
		g.drum.refreshInstruments()
	}
	if !g.drum.IsInstrumentAvailable(id) {
		t.Fatalf("instrument %q not available after registration", id)
	}
}

func setRowMuted(t *testing.T, dv *DrumView, row int, muted bool) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while setting muted=%v", muted)
	}
	if row < 0 || row >= len(dv.Rows) {
		t.Fatalf("row %d out of range while setting muted=%v", row, muted)
	}
	if dv.Rows[row].Muted == muted {
		return
	}
	dv.toggleMute(row)
	if dv.Rows[row].Muted != muted {
		t.Fatalf("row %d muted=%v want %v", row, dv.Rows[row].Muted, muted)
	}
}

func setRowSolo(t *testing.T, dv *DrumView, row int, solo bool) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while setting solo=%v", solo)
	}
	if row < 0 || row >= len(dv.Rows) {
		t.Fatalf("row %d out of range while setting solo=%v", row, solo)
	}
	if dv.Rows[row].Solo == solo {
		return
	}
	dv.toggleSolo(row)
	if dv.Rows[row].Solo != solo {
		t.Fatalf("row %d solo=%v want %v", row, dv.Rows[row].Solo, solo)
	}
}

func clearMuteSolo(t *testing.T, dv *DrumView) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while clearing mute/solo")
	}
	for i := range dv.Rows {
		if dv.Rows[i].Solo {
			dv.toggleSolo(i)
		}
	}
	for i := range dv.Rows {
		if dv.Rows[i].Muted {
			dv.toggleMute(i)
		}
	}
	for i := range dv.Rows {
		if dv.Rows[i].Solo || dv.Rows[i].Muted {
			t.Fatalf("row %d not cleared: muted=%v solo=%v", i, dv.Rows[i].Muted, dv.Rows[i].Solo)
		}
	}
}

func pressPlay(t *testing.T, dv *DrumView) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while pressing play")
	}
	if dv.playBtn() != nil && dv.playBtn().OnClick != nil {
		dv.playBtn().OnClick()
		return
	}
	dv.playPressed = true
}

func pressStop(t *testing.T, dv *DrumView) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while pressing stop")
	}
	if dv.stopBtn() != nil && dv.stopBtn().OnClick != nil {
		dv.stopBtn().OnClick()
		return
	}
	dv.stopPressed = true
}

func pressLenInc(t *testing.T, dv *DrumView) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while pressing length +")
	}
	if dv.lenIncBtn != nil && dv.lenIncBtn.OnClick != nil {
		dv.lenIncBtn.OnClick()
		return
	}
	dv.lenIncPressed = true
}

func pressLenDec(t *testing.T, dv *DrumView) {
	t.Helper()
	if dv == nil {
		t.Fatalf("nil drum view while pressing length -")
	}
	if dv.lenDecBtn != nil && dv.lenDecBtn.OnClick != nil {
		dv.lenDecBtn.OnClick()
		return
	}
	dv.lenDecPressed = true
}

func assertDefaultParityState(t *testing.T) {
	t.Helper()
	// Reset parity globals so tests don't depend on prior side effects.
	parityWatchDefault = parityWatchOff
	parityFatalEnabled.Store(true)
	if runtime.GOOS == "js" {
		parityWatchDefault = parityWatchLog
		parityFatalEnabled.Store(false)
	}
	if enableDefaultStart != true {
		t.Fatalf("enableDefaultStart=%v want true (default)", enableDefaultStart)
	}
	if forceAutoSize != false {
		t.Fatalf("forceAutoSize=%v want false (default)", forceAutoSize)
	}
	if forceSmallScreenForTest != false {
		t.Fatalf("forceSmallScreenForTest=%v want false (default)", forceSmallScreenForTest)
	}
	if defaultPerfFastPath {
		t.Fatalf("defaultPerfFastPath=%v want false (default)", defaultPerfFastPath)
	}
	if timelineTrace {
		t.Fatalf("timelineTrace=%v want false (default)", timelineTrace)
	}
	if timelineTraceRow != 0 {
		t.Fatalf("timelineTraceRow=%v want 0 (default)", timelineTraceRow)
	}
	if debugGeom {
		t.Fatalf("debugGeom=%v want false (default)", debugGeom)
	}
	if runtime.GOOS == "js" {
		if parityWatchDefault != parityWatchLog {
			t.Fatalf("parityWatchDefault=%v want %v for js tests", parityWatchDefault, parityWatchLog)
		}
		if parityFatalEnabled.Load() {
			t.Fatalf("parityFatalEnabled=%v want false for js tests", parityFatalEnabled.Load())
		}
		return
	}
	if parityWatchDefault != parityWatchOff {
		t.Fatalf("parityWatchDefault=%v want %v for tests (PARITY_WATCH should be unset)", parityWatchDefault, parityWatchOff)
	}
	if !parityFatalEnabled.Load() {
		t.Fatalf("parityFatalEnabled=%v want true for tests (PARITY_FATAL should be unset)", parityFatalEnabled.Load())
	}

	// Auto-reset every package global this function asserts on. Production
	// helpers like (*Game).SetForceMobileProfile mutate these (used by 31
	// scene Setups in scene_catalog.go for the screenshot harness's mobile
	// pass), and tests exercising those scenes via RunSceneMobile rarely
	// register their own t.Cleanup. Without this defense the very first
	// mobile-scene test poisons every subsequent test that calls
	// assertDefaultParityState — a 100+-test cascade.
	//
	// The entry-time assertions above still catch NEW state-leak bugs
	// originating outside the test runner (e.g. init() functions, env
	// vars set in test bootstrap). The cleanup is purely defensive against
	// in-test mutations the test forgot to undo.
	t.Cleanup(func() {
		forceSmallScreenForTest = false
		forceAutoSize = false
		enableDefaultStart = true
		defaultPerfFastPath = false
		timelineTrace = false
		timelineTraceRow = 0
		debugGeom = false
		parityWatchDefault = parityWatchOff
		parityFatalEnabled.Store(true)
		UpdateProfile()
	})
}

func assertDefaultSimpleDraw(t *testing.T, g *Game) {
	t.Helper()
	if g == nil {
		t.Fatalf("nil game in assertDefaultSimpleDraw")
	}
	if g.simpleDraw != RuntimeProf().SimpleDrawDefault {
		t.Fatalf("simpleDraw default=%v want %v before override", g.simpleDraw, RuntimeProf().SimpleDrawDefault)
	}
}

func assertDefaultPredictorThrottle(t *testing.T) {
	t.Helper()
	want := runtime.GOARCH == "wasm"
	if predictorPerfThrottleEnabled != want {
		t.Fatalf("predictorPerfThrottleEnabled=%v want %v before override", predictorPerfThrottleEnabled, want)
	}
}

func assertDefaultRowSnapshotMode(t *testing.T, g *Game) {
	t.Helper()
	if g == nil {
		t.Fatalf("nil game in assertDefaultRowSnapshotMode")
	}
	if g.rowSnapshotMode {
		t.Fatalf("rowSnapshotMode=%v want false before override", g.rowSnapshotMode)
	}
}

func assertDefaultPerfFastPath(t *testing.T, g *Game) {
	t.Helper()
	if g == nil {
		t.Fatalf("nil game in assertDefaultPerfFastPath")
	}
	if g.perfMode.FastPathEnabled() {
		t.Fatalf("perf fast path enabled by default; expected disabled before override")
	}
}

func setPlayStartForAbs(g *Game, abs int) {
	if g == nil {
		return
	}
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}
	bpm := g.AppliedBPM()
	if bpm <= 0 {
		bpm = g.drum.BPM()
	}
	if bpm <= 0 {
		bpm = 120
	}
	g.SetBeatBaseForTest(0)
	dtBeats := (float64(abs) + 0.01) / float64(div)
	dtSec := dtBeats * 60.0 / float64(bpm)
	g.SetPlayStartForTest(time.Now().Add(-time.Duration(dtSec * float64(time.Second))))
	g.SetAudioStartForTest(0)
}

func setPlayStartForAbsFloat(g *Game, abs float64) {
	if g == nil {
		return
	}
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}
	bpm := g.AppliedBPM()
	if bpm <= 0 {
		bpm = g.drum.BPM()
	}
	if bpm <= 0 {
		bpm = 120
	}
	g.SetBeatBaseForTest(0)
	dtBeats := (abs + 0.01) / float64(div)
	dtSec := dtBeats * 60.0 / float64(bpm)
	g.SetPlayStartForTest(time.Now().Add(-time.Duration(dtSec * float64(time.Second))))
	g.SetAudioStartForTest(0)
}

// stopPlaybackForTest mirrors the stop-button reset path to ensure
// scheduling restarts from a clean default state.
func stopPlaybackForTest(g *Game) {
	if g == nil {
		return
	}
	g.state.Stop()
	g.audioGen.Add(1)
	g.setPrimaryStep(0)
	g.elapsedBeats = 0
	g.activePulses = nil
	g.activePulse = nil
	g.resetHighlights()
	g.lastFrame = time.Time{}
	if g.drum != nil {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
		g.muteUntilByRow = make([]int, len(g.drum.Rows))
	} else {
		g.nextBeatIdxs = nil
		g.seqNextIdxs = nil
		g.muteUntilByRow = nil
	}
	g.frozenUpToByRow = nil
	g.clearParityState()
	g.parityWatch = parityWatchDefault
	parityFatalEnabled.Store(true)
}

// writeTempWAV creates a minimal valid PCM WAV file and returns its path.
func writeTempWAV(t *testing.T, name string) string {
	t.Helper()
	if name == "" {
		name = "sample.wav"
	}
	dir := t.TempDir()
	path := filepath.Join(dir, name)

	const sampleRate = 44100
	const channels = 1
	const bitsPerSample = 16
	const frames = 64
	dataLen := frames * channels * (bitsPerSample / 8)
	byteRate := sampleRate * channels * (bitsPerSample / 8)
	blockAlign := channels * (bitsPerSample / 8)

	buf := &bytes.Buffer{}
	writeLE := func(v any) {
		if err := binary.Write(buf, binary.LittleEndian, v); err != nil {
			t.Fatalf("write wav header: %v", err)
		}
	}
	buf.WriteString("RIFF")
	writeLE(uint32(36 + dataLen))
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	writeLE(uint32(16))
	writeLE(uint16(1)) // PCM
	writeLE(uint16(channels))
	writeLE(uint32(sampleRate))
	writeLE(uint32(byteRate))
	writeLE(uint16(blockAlign))
	writeLE(uint16(bitsPerSample))
	buf.WriteString("data")
	writeLE(uint32(dataLen))
	buf.Write(make([]byte, dataLen))

	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write wav: %v", err)
	}
	return path
}
