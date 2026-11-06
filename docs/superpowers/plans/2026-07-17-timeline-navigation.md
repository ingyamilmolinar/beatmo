# Timeline Navigation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users navigate playback to any beat/subdivision — ribbon click-to-seek, FF/RW transport buttons, tappable beat-counter position entry, and desktop keyboard nudges — on top of a single unified transport clock.

**Architecture:** The engine already has a full playhead-seek (`Game.Seek`, wired only to undo today). This plan (1) consolidates the duplicated transport-clock formula into `gamestate.State` as the single source of truth, (2) promotes `Game.Seek` into a subdivision-granular `SeekSteps` chokepoint that also rewinds the timeline (archiving superseded history) and the predictor, and (3) funnels every new UI entry point through one DrumView pending-seek request drained once per frame under `seqMu`.

**Tech Stack:** Go 1.26 (bundled at `.tools/go/bin/go`), Ebiten v2.9, the `internal/ui` Zone/HitArea tree, `internal/gamestate`, `internal/timeline`, `core/engine` predictor, WASM/JS bridge.

## Global Constraints

- **Go binary:** always `../../.tools/go/bin/go` (from `src/go/`), never system `go`.
- **Fast test command:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./<pkg>`.
- **Never stage or commit** unless explicitly asked (project rule). The "Commit" steps below are written per the skill template; in this repo, STOP at each commit step and leave the change uncommitted for the user to review — do not run `git add`/`git commit`.
- **Functional tests over isolation:** drive real `Game.Update` where behavior is user-facing; reproduce functionally before asserting internals.
- **Forbidden glyphs:** no `▶ ▼ ▲ ✕ ≡ ⏸ ■ ↑ ↓ → ←` (or shuttle glyphs `⏪ ⏩`) in `internal/ui/` source — use `IconID` + `DrawIcon`. Guarded by `forbidden_glyphs_test.go`.
- **Event-source attribution:** `EventSeek` carries an `Origin` naming the emitter (`ribbon`/`transport`/`counter`/`keyboard`/`js`/`undo`).
- **Bounded events per gesture (anti-pollution):** never emit `EventSeek` per frame. A hold-shuttle emits once per discrete repeat step. A ribbon tap emits once; a ribbon drag emits at most twice (seek on press for instant feedback, re-seek on release only if the beat changed). This refines the spec's "emit at release only" toward instant press feedback while keeping a tap to a single event.
- **Seek is not undoable** (`ScopeTransport`); it trims only speculative future cells.
- **`SeekSteps` must be called with `seqMu` held** — matches the existing `Seek` (undo) context and the Play/Stop pressed-flag handlers. The single drain site and undo are the only callers.
- **Charter:** logic tests in Go; JS browser tests only for the Go↔WebAudio/Worklet/real-input/bridge boundaries.

---

## File Structure

**Phase A — Clock authority (behavior-preserving refactor)**
- `internal/gamestate/state.go` — add clock methods; rewrite `AnchorForBPMChange`; demote raw anchor setters to `...ForTest`.
- `internal/gamestate/state_clock_test.go` (new) — table + equivalence tests.
- `internal/ui/game_beat_display.go`, `game_sequencer_schedule.go`, `game_sync_ui.go`, `game_update.go`, `game_seek.go` — route the 4 formula sites + `applySeekState` through the authority.

**Phase B — Seek chokepoint**
- `internal/hooks/events.go`, `internal/ui/event_helpers.go` — `SeekPayload{Steps,Origin}` + widened `emitSeek`.
- `internal/timeline/service_trim.go` — `RewindTo(row, abs)`.
- `internal/timeline/service_rewind_test.go` (new).
- `internal/ui/game_timeline.go`, `playback_slate.go` — `timelineRewindTo` wrapper.
- `core/engine/predictor_rewind_test.go` (new) — validate `RebaseAt` backward-seek.
- `internal/ui/game_seek.go` — `SeekSteps(steps, origin)`; `Seek` becomes a wrapper.
- `internal/ui/game_seek_steps_test.go` (new).

**Phase C — DrumView plumbing**
- `internal/ui/drumview_transport.go` (or a new `drumview_seek.go`) — pending-seek request + accessors.
- `internal/ui/game_update.go` — drain site.
- `internal/ui/game_seek_request_test.go` (new).

**Phase D — UI entry points**
- `internal/ui/icons.go`, `drawing_icons.go` — `IconRewind`, `IconFastForward`.
- `internal/ui/transport_zone.go`, `drumview_ctor.go` — FF/RW buttons + wiring.
- `internal/ui/timeline_zone.go`, `drumview_ctor.go` — ribbon seek + ghost + beat-counter popup.
- `internal/ui/keyboard_shortcuts.go`, `game_input_shortcuts.go` — `seek` shortcut node.

**Phase E — Bridge + browser**
- `internal/ui/js_exports_graph_ui.go` — `seekSteps`, `playheadStep`.
- `src/js/wasm_bridge_smoke.browser.test.js` — catalogue entries.
- `src/js/e2e_timeline_seek.browser.test.js`, `xplat_parity.browser.test.js` — repoint/extend.

---

## Task 1: Transport clock authority in `gamestate.State`

**Files:**
- Modify: `src/go/internal/gamestate/state.go`
- Test: `src/go/internal/gamestate/state_clock_test.go` (create)

**Interfaces:**
- Consumes: existing `State` fields (`beatBase`, `audioStart`, `playStart`, `pausedBeats`, `playing`, `paused`).
- Produces:
  - `func (s *State) DtSince(now time.Time, audioNow float64) (dtSec float64, ok bool)`
  - `func (s *State) PosBeats(now time.Time, audioNow float64, bpm int) (beats float64, ok bool)`
  - `func (s *State) PosStep(now time.Time, audioNow float64, bpm, div int) (step int, ok bool)`
  - `func (s *State) TargetStep(now time.Time, audioNow float64, bpm, div int) (step int, ok bool)`
  - `func (s *State) DtOfStep(idx, div, bpm int) float64`
  - `func (s *State) AudioTimeOfStep(idx, div, bpm int) float64`
  - `func (s *State) SeekTo(steps, div int, now time.Time, audioNow float64)`
  - `SetBeatBaseForTest`, `SetAudioStartForTest`, `SetPausedBeatsForTest` (renamed from the raw setters).

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/gamestate/state_clock_test.go`:

```go
package gamestate

import (
	"math"
	"testing"
	"time"
)

func TestPosBeatsPrefersAudioClock(t *testing.T) {
	s := New(120)
	s.SetPlaying(true)
	s.SetBeatBaseForTest(4)   // start at beat 4
	s.SetAudioStartForTest(10) // audio clock anchored at t=10s
	// 120 BPM => 2 beats/sec. 3 audio-seconds later => +6 beats => 10.
	beats, ok := s.PosBeats(time.Now(), 13, 120)
	if !ok {
		t.Fatal("PosBeats ok=false with audio clock available")
	}
	if math.Abs(beats-10) > 1e-9 {
		t.Fatalf("PosBeats=%v want 10", beats)
	}
}

func TestPosBeatsFallsBackToWallClockWhenNoAudio(t *testing.T) {
	s := New(60)
	s.SetPlaying(true)
	s.SetBeatBaseForTest(0)
	// No audio clock (audioStart==0): must use wall clock.
	start := time.Now().Add(-2 * time.Second)
	s.SetPlayStart(start)
	beats, ok := s.PosBeats(time.Now(), 0, 60) // 60 BPM => 1 beat/sec => ~2
	if !ok {
		t.Fatal("PosBeats ok=false with wall clock available")
	}
	if beats < 1.5 || beats > 2.5 {
		t.Fatalf("PosBeats=%v want ~2", beats)
	}
}

func TestPosBeatsNotOkWithNoClock(t *testing.T) {
	s := New(120)
	s.SetPlaying(true)
	// playStart zero AND audioStart zero => no anchor.
	if _, ok := s.PosBeats(time.Now(), 0, 120); ok {
		t.Fatal("PosBeats ok=true with no clock anchor")
	}
}

func TestTargetStepFloorPosStepRound(t *testing.T) {
	s := New(120)
	s.SetPlaying(true)
	s.SetBeatBaseForTest(0)
	s.SetAudioStartForTest(0.0)
	s.SetPlayStart(time.Now().Add(-1 * time.Second)) // wall clock, ~2 beats at 120
	// Force an exact position via beatBase to avoid clock jitter in the assertion:
	s.SetPlayStart(time.Time{})
	s.SetBeatBaseForTest(2.6)
	// No clock => ok=false, but PosStep/TargetStep still return the base position.
	step, _ := s.PosStep(time.Now(), 0, 120, 4) // 2.6*4=10.4 => round 10
	if step != 10 {
		t.Fatalf("PosStep=%d want 10", step)
	}
	tgt, _ := s.TargetStep(time.Now(), 0, 120, 4) // floor(10.4)=10
	if tgt != 10 {
		t.Fatalf("TargetStep=%d want 10", tgt)
	}
}

func TestDtOfStepIsInverseOfPosBeats(t *testing.T) {
	s := New(140)
	s.SetPlaying(true)
	s.SetBeatBaseForTest(3)
	s.SetAudioStartForTest(5)
	div, bpm, idx := 8, 140, 40 // beat = 40/8 = 5.0
	// AudioTimeOfStep(idx) is the audio time at which the playhead reaches idx.
	at := s.AudioTimeOfStep(idx, div, bpm)
	beats, ok := s.PosBeats(time.Now(), at, bpm)
	if !ok {
		t.Fatal("PosBeats ok=false")
	}
	if math.Abs(beats-float64(idx)/float64(div)) > 1e-9 {
		t.Fatalf("round-trip beats=%v want %v", beats, float64(idx)/float64(div))
	}
}

// TestClockSingleSource pins that the display path and the scheduler path
// derive the SAME position from the SAME inputs — the drift this refactor
// eliminates stays eliminated.
func TestClockSingleSource(t *testing.T) {
	s := New(120)
	s.SetPlaying(true)
	s.SetBeatBaseForTest(1.25)
	s.SetAudioStartForTest(2)
	div, bpm := 4, 120
	now, audioNow := time.Now(), 4.5
	beats, ok := s.PosBeats(now, audioNow, bpm)
	if !ok {
		t.Fatal("PosBeats ok=false")
	}
	// Display path: raw beats. Scheduler path: floor(beats*div). Both must
	// come from the identical PosBeats value.
	wantTarget := int(math.Floor(beats * float64(div)))
	gotTarget, _ := s.TargetStep(now, audioNow, bpm, div)
	if gotTarget != wantTarget {
		t.Fatalf("TargetStep=%d want %d (scheduler must equal floor(PosBeats*div))", gotTarget, wantTarget)
	}
}

func TestSeekToPlayingReanchorsClocks(t *testing.T) {
	s := New(120)
	s.SetPlaying(true)
	before := time.Now().Add(-time.Hour)
	s.SetPlayStart(before)
	s.SeekTo(16, 4, time.Now(), 7) // beat 4
	if math.Abs(s.BeatBase()-4) > 1e-9 {
		t.Fatalf("BeatBase=%v want 4", s.BeatBase())
	}
	if s.PlayStart().Equal(before) {
		t.Fatal("SeekTo(playing) must re-anchor playStart")
	}
	if s.AudioStart() != 7 {
		t.Fatalf("AudioStart=%v want 7", s.AudioStart())
	}
}

func TestSeekToStoppedSetsPausedBeats(t *testing.T) {
	s := New(120)
	s.SetPlaying(false)
	s.SeekTo(12, 4, time.Now(), 0)
	if s.PausedBeats() != 12 {
		t.Fatalf("PausedBeats=%d want 12", s.PausedBeats())
	}
	if math.Abs(s.BeatBase()-3) > 1e-9 {
		t.Fatalf("BeatBase=%v want 3", s.BeatBase())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/gamestate/ -run 'TestPosBeats|TestTargetStep|TestDtOfStep|TestClockSingleSource|TestSeekTo'`
Expected: FAIL — `s.PosBeats undefined`, `s.SetBeatBaseForTest undefined`, etc.

- [ ] **Step 3: Add the clock methods and rename the raw setters**

In `src/go/internal/gamestate/state.go`, add `"math"` to the import block. Rename the three raw setters (they are currently `SetBeatBase`, `SetAudioStart`, `SetPausedBeats` around lines 163/193/211) to the `...ForTest` names, then add the authority methods. Replace the existing `SetBeatBase`/`SetAudioStart`/`SetPausedBeats` declarations with:

```go
// SetBeatBaseForTest forces a new beat base (tests only — production code must
// go through SeekTo / the Resume/Pause/Stop transitions).
func (s *State) SetBeatBaseForTest(v float64) { s.beatBase = v }

// SetAudioStartForTest forces the audio clock anchor (tests only).
func (s *State) SetAudioStartForTest(v float64) { s.audioStart = v }

// SetPausedBeatsForTest forces the stored pause index (tests only).
func (s *State) SetPausedBeatsForTest(v int) { s.pausedBeats = v }
```

Keep the existing getters (`BeatBase()`, `AudioStart()`, `PausedBeats()`) unchanged. Add the authority block (anywhere after `AnchorForBPMChange`):

```go
// DtSince returns elapsed seconds since the play anchor, preferring the audio
// clock and falling back to wall clock. ok=false means no anchor is available
// (the caller decides its own fallback). This is the SINGLE place the
// audio-vs-wall clock selection lives.
func (s *State) DtSince(now time.Time, audioNow float64) (dtSec float64, ok bool) {
	if audioNow > 0 && s.audioStart > 0 {
		return audioNow - s.audioStart, true
	}
	if !s.playStart.IsZero() {
		return now.Sub(s.playStart).Seconds(), true
	}
	return 0, false
}

// PosBeats is the ONE transport-position formula: beatBase + elapsed*bpm/60.
// ok mirrors DtSince; when !ok it returns beatBase (no progress). Callers apply
// their own floor/round and monotonic clamping.
func (s *State) PosBeats(now time.Time, audioNow float64, bpm int) (beats float64, ok bool) {
	dt, ok := s.DtSince(now, audioNow)
	if !ok {
		return s.beatBase, false
	}
	return s.beatBase + dt*float64(bpm)/60.0, true
}

// PosStep is the canonical absolute-subdivision position (round).
func (s *State) PosStep(now time.Time, audioNow float64, bpm, div int) (int, bool) {
	if div <= 0 {
		div = 1
	}
	b, ok := s.PosBeats(now, audioNow, bpm)
	return int(math.Round(b * float64(div))), ok
}

// TargetStep is the scheduler's catch-up target (floor).
func (s *State) TargetStep(now time.Time, audioNow float64, bpm, div int) (int, bool) {
	if div <= 0 {
		div = 1
	}
	b, ok := s.PosBeats(now, audioNow, bpm)
	return int(math.Floor(b * float64(div))), ok
}

// DtOfStep is the inverse of PosBeats: elapsed seconds at which the playhead
// reaches subdivision idx, relative to the play anchor.
func (s *State) DtOfStep(idx, div, bpm int) float64 {
	if div <= 0 {
		div = 1
	}
	if bpm <= 0 {
		return 0
	}
	return (float64(idx)/float64(div) - s.beatBase) * 60.0 / float64(bpm)
}

// AudioTimeOfStep is DtOfStep expressed on the audio clock — the ideal audio
// time to schedule subdivision idx.
func (s *State) AudioTimeOfStep(idx, div, bpm int) float64 {
	return s.audioStart + s.DtOfStep(idx, div, bpm)
}

// SeekTo relocates the playhead to absolute subdivision `steps`. When playing,
// it re-anchors the clocks so PosBeats continues smoothly from steps; when
// stopped/paused it records the paused position. This is the single mutation
// that moves the transport position.
func (s *State) SeekTo(steps, div int, now time.Time, audioNow float64) {
	if div <= 0 {
		div = 1
	}
	beat := float64(steps) / float64(div)
	s.beatBase = beat
	s.lastBeat = beat
	s.lastDisplayBeat = beat
	s.lastProg = 0
	s.lastStep = steps
	if s.playing.Load() {
		s.playStart = now
		if audioNow > 0 {
			s.audioStart = audioNow
		} else {
			s.audioStart = 0
		}
		s.justResumed.Store(false)
	} else {
		s.pausedBeats = steps
	}
}
```

- [ ] **Step 4: Rewrite `AnchorForBPMChange` to use `DtSince`**

Replace the body of `AnchorForBPMChange` (state.go ~118-133) with the version that reuses the single clock selection:

```go
func (s *State) AnchorForBPMChange(prevBPM int, now time.Time, audioNow float64) {
	if !s.playing.Load() || prevBPM <= 0 {
		return
	}
	dtSec, ok := s.DtSince(now, audioNow)
	if !ok {
		return
	}
	s.beatBase += dtSec * float64(prevBPM) / 60.0
	s.playStart = now
	if audioNow > 0 {
		s.audioStart = audioNow
	}
}
```

- [ ] **Step 5: Update existing callers of the renamed setters**

The renamed setters break existing callers. Find them and update. In `src/go/internal/ui/game_transport_helpers.go` the test helpers already delegate — update those delegations to the new names:

Run: `cd src/go && grep -rn 'state\.SetBeatBase(\|state\.SetAudioStart(\|state\.SetPausedBeats(\|\.SetBeatBase(\|\.SetAudioStart(\|\.SetPausedBeats(' internal/ internal/gamestate/`

For each **production** hit in `internal/ui/` (e.g. `game_update.go:812-815`, `game_media_session_wasm.go`, `game_seek.go` — these are handled in later tasks via `SeekTo`), leave them for now if the later task replaces them; but to keep the tree compiling at THIS commit, do a mechanical rename of every remaining `SetBeatBase(`→`SetBeatBaseForTest(`, `SetAudioStart(`→`SetAudioStartForTest(`, `SetPausedBeats(`→`SetPausedBeatsForTest(` at call sites that are NOT rewritten later in this plan. (Tasks 2 and 6 replace the `game_seek.go`, `game_update.go`, and media-session sites with `SeekTo`; if you are executing strictly in order, the mechanical rename keeps compilation green until then.)

In `game_transport_helpers.go`, update the helper bodies:

```go
func (g *Game) SetBeatBaseForTest(v float64) { g.state.SetBeatBaseForTest(v) }
func (g *Game) SetAudioStartForTest(v float64) { g.state.SetAudioStartForTest(v) }
func (g *Game) SetPausedBeatsForTest(v int) { g.state.SetPausedBeatsForTest(v) }
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/gamestate/`
Expected: PASS (all clock tests).

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./...`
Expected: builds clean (renames propagated).

- [ ] **Step 7: Commit** (STOP — leave uncommitted per repo rule; do not run git)

```
feat(gamestate): add transport clock authority (PosBeats/SeekTo/DtOfStep)
```

---

## Task 2: Route the UI formula sites through the clock authority

**Files:**
- Modify: `src/go/internal/ui/game_beat_display.go:106-127`
- Modify: `src/go/internal/ui/game_sequencer_schedule.go:94-105`, `:220`, `:362`
- Modify: `src/go/internal/ui/game_sync_ui.go:42-53`
- Test: existing `internal/ui` suite + parity (behavior-preserving).

**Interfaces:**
- Consumes: Task 1's `DtSince`, `PosBeats`, `TargetStep`, `DtOfStep`, `AudioTimeOfStep`.
- Produces: no new API; removes 4 inline formula copies.

This task is a behavior-preserving refactor. There is no new unit test; correctness is proven by the existing `internal/ui` tests and parity checks continuing to pass.

- [ ] **Step 1: Confirm the baseline is green before refactoring**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDisplayBeat|TestSeq|TestSyncUI|Parity' 2>&1 | tail -20`
Expected: PASS (record the baseline; if any are already red, note them — do NOT attribute later failures to your change without proving causation).

- [ ] **Step 2: Replace the display-path formula**

In `game_beat_display.go`, replace lines 106-127 (the steady-state branch that computes `dtSec` and `v := g.state.BeatBase() + dtSec*...`) with:

```go
	bpm := g.AppliedBPM()
	if bpm <= 0 {
		bpm = g.bpm
	}
	if bpm <= 0 {
		return lastDisplay
	}
	v, ok := g.state.PosBeats(time.Now(), audio.Now(), bpm)
	if !ok {
		return lastDisplay
	}
	if v < lastDisplay {
		v = lastDisplay
	}
	g.state.SetLastDisplayBeat(v)
	return v
```

- [ ] **Step 3: Replace the scheduler-path formula (target + lateness + ideal audio)**

In `game_sequencer_schedule.go`, replace the `dtSec`/`target` computation (lines ~94-105) with a single snapshot feeding the authority:

```go
	// Single clock snapshot for this tick — used for both target and lateness.
	now := time.Now()
	audioNow := audio.Now()
	dtSec, ok := g.state.DtSince(now, audioNow)
	if !ok {
		return
	}
	target, _ := g.state.TargetStep(now, audioNow, bpm, div)
```

(Keep the surrounding `audioStart := g.state.AudioStart()` line only if still referenced; `beatIdealAudio` below now uses the authority.) Then at the Stage-A lateness site (~line 220) replace:

```go
			beatDtSec := (float64(idx)/float64(div) - baseBeats) * 60.0 / float64(bpm)
```
with:
```go
			beatDtSec := g.state.DtOfStep(idx, div, bpm)
```

And at the ideal-audio site (~line 362) replace:

```go
					beatIdealAudio := audioStart + (float64(idx)/float64(div)-baseBeats)*60.0/float64(bpm)
```
with:
```go
					beatIdealAudio := g.state.AudioTimeOfStep(idx, div, bpm)
```

Remove the now-unused local `baseBeats := g.state.BeatBase()` if the compiler flags it (search for remaining uses first; the BPM-change re-anchor branch at line 70 does not use it).

- [ ] **Step 4: Replace the sync-path formula**

In `game_sync_ui.go`, replace lines 42-53 (the `dtSec`/`absBeats` computation) with:

```go
	absBeats, ok := g.state.PosBeats(time.Now(), audio.Now(), bpm)
	if !ok {
		return
	}
	absDivF := absBeats * float64(div)
	target := int(math.Floor(absDivF))
```

Remove the now-unused `audioStart` local in this function if flagged.

- [ ] **Step 5: Build and run the full UI + parity suite**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./...`
Expected: clean (watch for unused-variable errors from removed locals; delete them).

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS — same result as the Step 1 baseline. If a previously-green test now fails, the refactor changed behavior — investigate (likely a leftover `audioStart>0` guard difference; `DtSince` encodes the exact same guard).

- [ ] **Step 6: Commit** (STOP — leave uncommitted)

```
refactor(ui): route playhead/scheduler/sync through the clock authority
```

---

## Task 3: `SeekPayload` gains `Steps` + `Origin`

**Files:**
- Modify: `src/go/internal/hooks/events.go:393-396`
- Modify: `src/go/internal/ui/event_helpers.go:157-160`
- Modify: `src/go/internal/ui/game_seek.go` (origin constants + emit call)
- Test: `src/go/internal/ui/game_seek_steps_test.go` (added in Task 6 asserts Origin)

**Interfaces:**
- Produces:
  - `hooks.SeekPayload{Beats int; Steps int; Origin string}`
  - `func emitSeek(beats, steps int, origin string)`
  - origin constants in package `ui`: `seekOriginRibbon`, `seekOriginTransport`, `seekOriginCounter`, `seekOriginKeyboard`, `seekOriginJS`, `seekOriginUndo` (string values `"ribbon"`, `"transport"`, `"counter"`, `"keyboard"`, `"js"`, `"undo"`).

- [ ] **Step 1: Extend the payload**

In `src/go/internal/hooks/events.go`, replace the `SeekPayload` struct (lines ~393-396):

```go
// SeekPayload describes a transport seek to a particular position.
type SeekPayload struct {
	Beats  int    `json:"beats"`  // destination whole-beat (Steps/div)
	Steps  int    `json:"steps"`  // destination absolute subdivision index
	Origin string `json:"origin"` // emitter: ribbon|transport|counter|keyboard|js|undo
}
```

- [ ] **Step 2: Widen `emitSeek`**

In `src/go/internal/ui/event_helpers.go`, replace `emitSeek`:

```go
// emitSeek publishes EventSeek with the destination position and its origin.
func emitSeek(beats, steps int, origin string) {
	hooks.PublishWithSource(hooks.EventSeek, hooks.SeekPayload{Beats: beats, Steps: steps, Origin: origin}, hooks.CaptureSource(1))
}
```

- [ ] **Step 3: Add origin constants and fix the sole existing caller**

In `src/go/internal/ui/game_seek.go`, add at the top (after imports):

```go
const (
	seekOriginRibbon   = "ribbon"
	seekOriginTransport = "transport"
	seekOriginCounter  = "counter"
	seekOriginKeyboard = "keyboard"
	seekOriginJS       = "js"
	seekOriginUndo     = "undo"
)
```

The existing `emitSeek(beats)` call at `game_seek.go:15` will be replaced in Task 6 when `Seek` becomes a wrapper. For now, to keep compilation green, update that single call to:

```go
	emitSeek(beats, beats*g.grid.MaxDiv(), seekOriginUndo)
```

- [ ] **Step 4: Build**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test ./...`
Expected: clean.

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/`
Expected: PASS (formatter for EventSeek still returns its message; tag `seek` unchanged; `NumNonVerbose` unchanged since no new Kind).

- [ ] **Step 5: Commit** (STOP — leave uncommitted)

```
feat(hooks): SeekPayload carries Steps + Origin
```

---

## Task 4: `timeline.RewindTo(row, abs)` — archive superseded history

**Files:**
- Modify: `src/go/internal/timeline/service_trim.go`
- Test: `src/go/internal/timeline/service_rewind_test.go` (create)

**Interfaces:**
- Consumes: `ensureArchiveLocked(row)`, `rowArchive.append(abs,val,typ,kind,maxEntries)`, `commitRing.trimAfter(maxAbs)`, `s.immutables`, `s.rows`, `CommitKindPlayback/Import`.
- Produces: `func (s *Service) RewindTo(row, abs int)` — moves immutable Playback/Import commits with key `>= abs` into the cold archive (not delete), cuts the ring at `abs-1`, leaves everything `< abs` untouched.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/timeline/service_rewind_test.go`:

```go
package timeline

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestRewindToArchivesPostAbsImmutables(t *testing.T) {
	s := NewService()
	// Record immutable playback hits at abs 0..9 on row 0.
	for abs := 0; abs < 10; abs++ {
		s.RecordCommitKind(0, abs, abs%2 == 0, model.NodeTypeRegular, CommitKindPlayback, 64, 64)
	}
	// Rewind to abs 5: 5..9 immutables move to archive, 0..4 stay live.
	s.RewindTo(0, 5)

	// 0..4 still resolve via the live tiers.
	for abs := 0; abs < 5; abs++ {
		if _, _, _, ok := s.CommittedKind(0, abs); !ok {
			t.Fatalf("abs %d below seek point should stay committed", abs)
		}
	}
	// 5..9 are no longer in immutables/ring, but remain retrievable in the
	// archive tier (history is archived, not dropped).
	if s.LastImmutableAbs(0) >= 5 {
		t.Fatalf("immutables >= 5 should have been archived, last=%d", s.LastImmutableAbs(0))
	}
	// Archive retains at least one of the superseded entries.
	if got := archivedCountForTest(s, 0); got == 0 {
		t.Fatal("expected archived entries after rewind, got 0")
	}
}

func TestRewindToPreservesBelowAbs(t *testing.T) {
	s := NewService()
	for abs := 0; abs < 6; abs++ {
		s.RecordCommitKind(0, abs, true, model.NodeTypeRegular, CommitKindPlayback, 64, 64)
	}
	s.RewindTo(0, 3)
	val, _, kind, ok := s.CommittedKind(0, 2)
	if !ok || !val || kind != CommitKindPlayback {
		t.Fatalf("abs 2 must be preserved as playback true, got ok=%v val=%v kind=%v", ok, val, kind)
	}
}
```

Add the tiny test-only archive probe at the bottom of the new file (reuse the existing `TierForTest` if it can report the archive tier; if not, add this helper referencing internal fields — it lives in a `_test.go` so it may touch unexported state):

```go
// archivedCountForTest reports how many entries the row's cold archive holds.
func archivedCountForTest(s *Service, row int) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	arch := s.archives[row]
	if arch == nil {
		return 0
	}
	return arch.len()
}
```

If `rowArchive` has no `len()` method, add one in `archive.go`: `func (a *rowArchive) len() int { return len(a.entries) }` (use the actual slice field name from `archive.go`).

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/timeline/ -run TestRewindTo`
Expected: FAIL — `s.RewindTo undefined`.

- [ ] **Step 3: Implement `RewindTo`**

In `src/go/internal/timeline/service_trim.go`, add (mirror the demote idiom from `pruneImmutablesRowLocked` in `service.go:155`):

```go
// RewindTo repositions row `row` for a backward seek to absolute subdivision
// `abs`: immutable Playback/Import commits with key >= abs are moved into the
// cold archive tier (retained, NOT deleted — see the full-history-retention
// rule), the mutable/ring tier is cut at abs-1, and everything below abs is
// left untouched. After a rewind the caller lowers the freeze watermark so the
// rewound region renders from the predictor, not the archive.
func (s *Service) RewindTo(row, abs int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if row < 0 || row >= len(s.rows) {
		return
	}
	// Move post-abs immutables to the archive, then drop them from the sidecar.
	if rowMap := s.immutables[row]; rowMap != nil {
		arch := s.ensureArchiveLocked(row)
		for k, ce := range rowMap {
			if k < abs {
				continue
			}
			if ce.kind == CommitKindPlayback || ce.kind == CommitKindImport {
				arch.append(k, ce.val, ce.typ, ce.kind, s.archiveMaxEntries)
			}
			delete(rowMap, k)
		}
	}
	// Hard-cut the ring at abs-1 (drops any ring entries >= abs, mutable or not;
	// immutables were just archived, so no live copy is lost).
	s.rows[row].commits.trimAfter(abs - 1)
}
```

Verify the ring field/method names against `commit_ring.go` (`s.rows[row].commits` is the `commitRing`; `trimAfter(maxAbs)` exists per the survey). If `ensureArchiveLocked` returns `*rowArchive` with an `append` signature different from `(abs, val, typ, kind, maxEntries)`, match the real signature in `archive.go`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/timeline/ -run TestRewindTo`
Expected: PASS.

Run the full timeline suite to catch archive/ring regressions:
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/timeline/`
Expected: PASS (including `replace_demote_archive_test.go`, `tier_probe_test.go`).

- [ ] **Step 5: Commit** (STOP — leave uncommitted)

```
feat(timeline): RewindTo archives post-seek immutables instead of losing them
```

---

## Task 5: Validate predictor `RebaseAt` for backward seek

**Files:**
- Test: `src/go/core/engine/predictor_rewind_test.go` (create)
- Possibly modify: `src/go/core/engine/predictor_compute.go` (only if `RebaseAt` does not hold `p.mu`)

**Interfaces:**
- Consumes: existing `Predictor.RebaseAt(idx int)`, `Ensure(horizon int)`, `WindowStart()`, `Horizon()`, `SetPaths(...)`, `VisibleAt/TriggeredAt`.
- Produces: no new API (SeekSteps calls `RebaseAt` directly). This task confirms reuse and thread-safety.

- [ ] **Step 1: Write the failing (or validating) test**

Create `src/go/core/engine/predictor_rewind_test.go`. Build a minimal single-row looping graph, advance the window forward past a point, then `RebaseAt` back below `WindowStart` and assert the window resets to the target and re-simulates (no frozen-empty past):

```go
package engine

import "testing"

func TestRebaseAtBackwardSeekResetsWindow(t *testing.T) {
	p := newSingleRowLoopPredictorForTest(t) // helper below or reuse an existing test ctor
	// Drive the window forward.
	p.Ensure(200)
	if p.WindowStart() == 0 && p.Horizon() < 200 {
		t.Fatalf("Ensure(200) did not extend horizon: start=%d horizon=%d", p.WindowStart(), p.Horizon())
	}
	// Force eviction so windowStart advances above 0 (cap is 4096 by default;
	// use a large horizon if the row is short — otherwise assert the rebase path
	// directly).
	// Backward seek: rebase to abs 8 (below current horizon).
	p.RebaseAt(8)
	if p.WindowStart() > 8 {
		t.Fatalf("after RebaseAt(8), WindowStart=%d must be <= 8", p.WindowStart())
	}
	// Re-simulate forward and confirm the start-of-loop hit is visible again
	// (i.e. the region was re-computed, not frozen empty).
	p.Ensure(40)
	sawHit := false
	for idx := 8; idx < 40; idx++ {
		if p.VisibleAt(0, idx) {
			sawHit = true
			break
		}
	}
	if !sawHit {
		t.Fatal("RebaseAt+Ensure produced no visible hits — window frozen empty (regression)")
	}
}
```

For `newSingleRowLoopPredictorForTest`, reuse whatever the existing predictor tests use to construct a `*Predictor` with a simple looping path (grep `core/engine/*_test.go` for an existing constructor/helper such as one that calls `SetPaths` with a single regular-node loop; wire it into this test rather than inventing a new graph builder).

- [ ] **Step 2: Run the test**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./core/engine/ -run TestRebaseAtBackwardSeek`
Expected: PASS if `RebaseAt` already behaves correctly (the likely outcome per the survey). If it FAILS or DATA-RACES, proceed to Step 3.

- [ ] **Step 3: (Only if needed) ensure `RebaseAt` locks `p.mu`**

Run the race check: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./core/engine/ -run TestRebaseAt`

If `RebaseAt` (predictor_compute.go:362) does not acquire `p.mu.Lock()` at entry (compare with `Ensure`/`SetPaths`), wrap its body:

```go
func (p *Predictor) RebaseAt(idx int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rebaseAtLocked(idx) // rename the existing body to rebaseAtLocked
}
```

and rename the existing method body to `rebaseAtLocked`, updating any in-package callers that already hold the lock to call `rebaseAtLocked`.

- [ ] **Step 4: Confirm green**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./core/engine/`
Expected: PASS.

- [ ] **Step 5: Commit** (STOP — leave uncommitted)

```
test(engine): pin RebaseAt backward-seek window reset for playhead seeking
```

---

## Task 6: `Game.SeekSteps(steps, origin)` — the seek chokepoint

**Files:**
- Modify: `src/go/internal/ui/game_seek.go`
- Modify: `src/go/internal/ui/game_timeline.go`, `playback_slate.go` (add `timelineRewindTo`)
- Test: `src/go/internal/ui/game_seek_steps_test.go` (create)

**Interfaces:**
- Consumes: `state.SeekTo` (Task 1), `timeline.RewindTo` (Task 4), `predictor.RebaseAt` (Task 5), `emitSeek` (Task 3), `clampRowFreeze` (`game_row_helpers.go:51`), `timelineTrimAfterMutable`, `resetOriginSequences`, `spawnPulseFromRow`.
- Produces:
  - `func (g *Game) SeekSteps(steps int, origin string)` — clamps, seeks the clock, rewinds timeline + predictor per row, realigns counters, resets highlights, emits one `EventSeek`.
  - `func (g *Game) Seek(beats int)` — thin wrapper: `SeekSteps(beats*div, seekOriginUndo)`.
  - `func (g *Game) timelineRewindTo(row, abs int)` — Game/slate wrapper over `Service.RewindTo` + `clampRowFreeze`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/game_seek_steps_test.go`. Use the standard test harness (see `testutil_test.go` header — `assertDefaultParityState`, `updateBeatInfos`, `stopPlaybackForTest`). Assert seek-while-stopped sets position, seek-while-playing re-anchors, and one `EventSeek` with the right origin fires:

```go
package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestSeekStepsStoppedSetsPosition(t *testing.T) {
	g := newGameWithStart(t) // internal package-ui constructor with a start node (playable)
	div := g.grid.MaxDiv()
	g.SeekSteps(4*div, seekOriginRibbon) // beat 4
	if got := g.playheadAbsSubdiv(); got != 4*div {
		t.Fatalf("playhead=%d want %d", got, 4*div)
	}
}

func TestSeekStepsEmitsEventWithOrigin(t *testing.T) {
	g := newGameWithStart(t)
	// hooks.Bus is ASYNC (worker pool, no ordering) — observe via a buffered
	// channel + timeout rather than a synchronous count.
	ch := make(chan hooks.SeekPayload, 4)
	unsub := hooks.Subscribe(hooks.EventSeek, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.SeekPayload); ok {
			select {
			case ch <- p:
			default:
			}
		}
	})
	defer unsub()
	div := g.grid.MaxDiv()
	g.SeekSteps(6*div, seekOriginKeyboard)
	select {
	case p := <-ch:
		if p.Origin != seekOriginKeyboard || p.Steps != 6*div {
			t.Fatalf("payload=%+v want origin=keyboard steps=%d", p, 6*div)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventSeek observed within 1s")
	}
}

func TestSeekStepsClampsNegative(t *testing.T) {
	g := newGameWithStart(t)
	g.SeekSteps(-100, seekOriginJS)
	if got := g.playheadAbsSubdiv(); got != 0 {
		t.Fatalf("playhead=%d want 0 (clamped)", got)
	}
}
```

Adjust `newTestGame`, `hooks.Subscribe`/`DrainForTest` to the real helpers in this package (grep `internal/ui/*_test.go` and `internal/hooks` for the actual subscription + drain API; if the bus has no synchronous drain, use the existing pattern other tests use to observe `hooks.Bus` events, e.g. a test sink).

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSeekSteps`
Expected: FAIL — `g.SeekSteps undefined`.

- [ ] **Step 3: Add the `timelineRewindTo` wrapper**

In `src/go/internal/ui/playback_slate.go` (alongside `timelineTrimAfterMutable` at ~line 44), add:

```go
func (s *playbackSlate) timelineRewindTo(row, abs int) {
	if s.g == nil {
		return
	}
	s.g.timelineService().RewindTo(row, abs)
}
```

In `src/go/internal/ui/game_timeline.go` (alongside `timelineTrimAfterMutable` at ~line 152), add:

```go
func (g *Game) timelineRewindTo(row, abs int) {
	g.playbackSlate.timelineRewindTo(row, abs)
	// Lower the freeze watermark so the rewound region renders from the
	// predictor (live re-simulation), not the archived past.
	g.clampRowFreeze(row, abs-1)
}
```

- [ ] **Step 4: Implement `SeekSteps` and reduce `Seek` to a wrapper**

Replace the whole `Seek` function in `game_seek.go` (lines 11-63) with:

```go
// SeekSteps repositions the playhead to absolute subdivision `steps`. It is the
// single engine chokepoint for all user navigation (ribbon, FF/RW, counter,
// keyboard, JS) and for undo restore. Must be called with seqMu held.
func (g *Game) SeekSteps(steps int, origin string) {
	if steps < 0 {
		steps = 0
	}
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}
	if maxSteps := g.drum.timelineBeats * div; maxSteps > 0 && steps > maxSteps {
		steps = maxSteps
	}
	emitSeek(steps/div, steps, origin)
	g.resetHighlights()
	g.activePulses = nil
	g.activePulse = nil

	rows := len(g.drum.Rows)
	trim := steps - 1
	if rows > 0 {
		if len(g.frozenUpToByRow) != rows {
			old := g.frozenUpToByRow
			g.frozenUpToByRow = make([]int, rows)
			for i := range g.frozenUpToByRow {
				g.frozenUpToByRow[i] = -1
			}
			copy(g.frozenUpToByRow, old)
		}
		newLimit := trim
		if newLimit < -1 {
			newLimit = -1
		}
		for row := 0; row < rows; row++ {
			// Archive superseded history and lower the freeze watermark.
			g.timelineRewindTo(row, steps)
			if g.frozenUpToByRow[row] > newLimit {
				g.frozenUpToByRow[row] = newLimit
			}
			maxKeep := g.frozenUpToByRow[row]
			if maxKeep < newLimit {
				maxKeep = newLimit
			}
			g.timelineTrimAfterMutable(row, maxKeep)
		}
	}

	// Rewind the predictor window to the seek target and re-simulate forward.
	if g.engine != nil && g.engine.Predictor != nil {
		g.engine.Predictor.RebaseAt(steps)
		g.engine.Predictor.Ensure(steps + 1)
	}

	if g.Playing() {
		for row := range g.drum.Rows {
			g.spawnPulseFromRow(row, steps)
		}
	} else {
		for i := range g.nextBeatIdxs {
			g.nextBeatIdxs[i] = steps
		}
	}
	g.applySeekState(steps)
	g.resetOriginSequences()
	g.muteUntilByRow = make([]int, len(g.drum.Rows))
	g.seqNextIdxs = make([]int, len(g.drum.Rows))
}

// Seek repositions by whole beats — the undo-restore entry point.
func (g *Game) Seek(beats int) {
	if beats < 0 {
		beats = 0
	}
	g.SeekSteps(beats*g.grid.MaxDiv(), seekOriginUndo)
}
```

- [ ] **Step 5: Collapse `applySeekState` onto `SeekTo`**

Replace `applySeekState` (game_seek.go:65-90) — drop the `anchorClocks bool` param (it is derivable from playing state) and delegate the anchor mutation to `state.SeekTo`:

```go
func (g *Game) applySeekState(steps int) {
	g.elapsedBeats = steps
	div := max1(g.grid.MaxDiv())
	g.state.SeekTo(steps, div, time.Now(), audio.Now())
}
```

Update the other two callers that passed `anchorClocks`:
- `setPrimaryStep` (game_seek.go:98): `g.applySeekState(step)`
- `updateDrumTracking` (game_seek.go:117): `g.applySeekState(g.state.PausedBeats())`

(Both previously passed `false`; since they only run when not playing, `SeekTo`'s playing-derived branch matches — it takes the paused branch.)

- [ ] **Step 6: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSeekSteps|TestSeek|Parity|Undo'`
Expected: PASS. In particular the undo tests (which call `Seek`) must stay green — the wrapper preserves their behavior.

Run the full UI suite:
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS.

- [ ] **Step 7: Commit** (STOP — leave uncommitted)

```
feat(ui): SeekSteps chokepoint — clock + timeline + predictor rewind
```

---

## Task 7: DrumView pending-seek request + Game drain site

**Files:**
- Create: `src/go/internal/ui/drumview_seek.go`
- Modify: `src/go/internal/ui/game_update.go` (drain near the Play/Stop pressed handling, ~line 855)
- Test: `src/go/internal/ui/game_seek_request_test.go` (create)

**Interfaces:**
- Produces (on `*DrumView`):
  - `func (dv *DrumView) RequestSeekSteps(steps int, origin string)` — absolute.
  - `func (dv *DrumView) RequestSeekDeltaBeats(deltaBeats int, origin string)` — relative to current playhead (resolved in Game).
  - `func (dv *DrumView) TakeSeekRequest() (steps int, deltaBeats int, isDelta bool, origin string, ok bool)`
- Consumes (in Game): `g.SeekSteps`, `g.playheadAbsSubdiv`, `g.grid.MaxDiv`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/game_seek_request_test.go`:

```go
package ui

import "testing"

func TestSeekRequestAbsoluteDrains(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.drum.RequestSeekSteps(5*div, seekOriginRibbon)
	g.drainSeekRequest() // the helper the Update drain site calls
	if got := g.playheadAbsSubdiv(); got != 5*div {
		t.Fatalf("playhead=%d want %d", got, 5*div)
	}
	// Request is one-shot.
	if _, _, _, _, ok := g.drum.TakeSeekRequest(); ok {
		t.Fatal("seek request should be consumed after drain")
	}
}

func TestSeekRequestDeltaResolvesAgainstPlayhead(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(8*div, seekOriginJS) // start at beat 8
	g.drum.RequestSeekDeltaBeats(-2, seekOriginTransport)
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 6*div {
		t.Fatalf("playhead=%d want %d (8-2 beats)", got, 6*div)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSeekRequest`
Expected: FAIL — `RequestSeekSteps undefined`, `drainSeekRequest undefined`.

- [ ] **Step 3: Implement the DrumView request store**

Create `src/go/internal/ui/drumview_seek.go`:

```go
package ui

// seekRequest is a one-shot navigation intent set by any UI entry point
// (ribbon, FF/RW, counter, keyboard) and drained once per frame by
// Game.Update under seqMu. Absolute requests carry steps; delta requests carry
// a beat offset resolved against the live playhead in Game.
type seekRequest struct {
	steps      int
	deltaBeats int
	isDelta    bool
	origin     string
}

// RequestSeekSteps queues an absolute seek to subdivision `steps`.
func (dv *DrumView) RequestSeekSteps(steps int, origin string) {
	dv.pendingSeek = &seekRequest{steps: steps, origin: origin}
}

// RequestSeekDeltaBeats queues a relative seek of `deltaBeats` whole beats.
func (dv *DrumView) RequestSeekDeltaBeats(deltaBeats int, origin string) {
	dv.pendingSeek = &seekRequest{deltaBeats: deltaBeats, isDelta: true, origin: origin}
}

// TakeSeekRequest returns and clears any queued request (last-wins per frame).
func (dv *DrumView) TakeSeekRequest() (steps, deltaBeats int, isDelta bool, origin string, ok bool) {
	r := dv.pendingSeek
	if r == nil {
		return 0, 0, false, "", false
	}
	dv.pendingSeek = nil
	return r.steps, r.deltaBeats, r.isDelta, r.origin, true
}
```

Add the field to the `DrumView` struct (find the struct in `drumview.go`/`drumview_ctor.go`; add near other transient input fields):

```go
	pendingSeek *seekRequest
```

- [ ] **Step 4: Add the Game drain helper and call it in Update**

In `game_seek.go`, add:

```go
// drainSeekRequest applies any queued DrumView seek request. Must run with
// seqMu held (called from Game.Update alongside Play/Stop pressed handling).
func (g *Game) drainSeekRequest() {
	steps, deltaBeats, isDelta, origin, ok := g.drum.TakeSeekRequest()
	if !ok {
		return
	}
	if isDelta {
		div := max1(g.grid.MaxDiv())
		cur := g.playheadAbsSubdiv()
		snapped := int(math.Round(float64(cur)/float64(div))) * div // snap to whole beat first
		steps = snapped + deltaBeats*div
	}
	g.SeekSteps(steps, origin)
}
```

In `game_update.go`, immediately after the `if g.drum.StopPressed() { ... }` block (ends ~line 880), add:

```go
	g.drainSeekRequest()
```

(Placed after Stop so a same-frame Stop-then-seek resolves to the seek target; this is the proven-safe locked region shared with Play/Stop pressed handling.)

- [ ] **Step 5: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSeekRequest`
Expected: PASS.

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS.

- [ ] **Step 6: Commit** (STOP — leave uncommitted)

```
feat(ui): DrumView pending-seek request drained once per frame
```

---

## Task 8: `IconRewind` + `IconFastForward`

**Files:**
- Modify: `src/go/internal/ui/icons.go` (enum + switch)
- Modify: `src/go/internal/ui/drawing_icons.go` (vector paths)
- Test: `src/go/internal/ui/icons_test.go` (add) + `forbidden_glyphs_test.go` (must stay green)

**Interfaces:**
- Produces: `IconRewind IconID = "rewind"`, `IconFastForward IconID = "fast-forward"`, drawable via `DrawIcon`.

- [ ] **Step 1: Write the failing test**

Add to `src/go/internal/ui/icons_test.go` (create if absent):

```go
package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestSeekIconsDraw(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	r := image.Rect(0, 0, 24, 24)
	for _, id := range []IconID{IconRewind, IconFastForward} {
		if !DrawIcon(dst, id, r, colTextPrimary) {
			t.Fatalf("DrawIcon returned false for %q (not registered in switch)", id)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSeekIconsDraw`
Expected: FAIL — `IconRewind undefined`.

- [ ] **Step 3: Add the enum consts**

In `icons.go`, in the `IconID` const block (lines ~10-56), add:

```go
	IconRewind      IconID = "rewind"
	IconFastForward IconID = "fast-forward"
```

- [ ] **Step 4: Add the vector paths**

In `drawing_icons.go`, add two `var` glyphs, reusing the exact primitive `drawPlayIcon` uses — `iconFillPath(dst, c, [][2]float32{...}, col)` on the 24×24 `iconCanvas` (play is the single triangle `{{8,5},{19,12},{8,19}}`, `drawing_icons.go:26`). Fast-forward is a right-pointing double triangle; rewind mirrors the X axis:

```go
var drawFastForwardIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconFillPath(dst, c, [][2]float32{{4, 6}, {12, 12}, {4, 18}}, col)
	iconFillPath(dst, c, [][2]float32{{12, 6}, {20, 12}, {12, 18}}, col)
}

var drawRewindIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconFillPath(dst, c, [][2]float32{{20, 6}, {12, 12}, {20, 18}}, col)
	iconFillPath(dst, c, [][2]float32{{12, 6}, {4, 12}, {12, 18}}, col)
}
```

Keep the glyphs as `var` (not `func`) so tests can stub them, matching the file's convention.

- [ ] **Step 5: Register in the switch**

In `icons.go`, in `drawIconByID` (lines ~63-151), add before `default:`:

```go
	case IconRewind:
		drawRewindIcon(dst, r, col)
	case IconFastForward:
		drawFastForwardIcon(dst, r, col)
```

- [ ] **Step 6: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSeekIconsDraw|Forbidden'`
Expected: PASS (icons draw; no forbidden glyph introduced — we used IconIDs, not runes).

- [ ] **Step 7: Commit** (STOP — leave uncommitted)

```
feat(ui): rewind / fast-forward icons
```

---

## Task 9: FF / RW transport buttons

**Files:**
- Modify: `src/go/internal/ui/transport_zone.go` (callbacks, buttons, `.Repeat`, layout, hit areas)
- Modify: `src/go/internal/ui/drumview_ctor.go` (wire callbacks → `dv.RequestSeekDeltaBeats`)
- Test: `src/go/internal/ui/transport_seek_buttons_test.go` (create)

**Interfaces:**
- Consumes: `NewButton`, `Button.Repeat`, `Button.Icon`, `TransportCallbacks`, `dv.RequestSeekDeltaBeats`.
- Produces: `TransportCallbacks.OnRewind func()`, `OnFastForward func()`; buttons `z.rwBtn`, `z.ffBtn`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/transport_seek_buttons_test.go`. Drive the callback and assert a delta-seek request is queued on the DrumView:

```go
package ui

import "testing"

func TestFastForwardButtonRequestsForwardSeek(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(4*div, seekOriginJS) // start at beat 4
	// Simulate the FF button firing its OnClick.
	g.drum.transportZone.callbacks.OnFastForward() // unexported field, reachable from package ui
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 5*div {
		t.Fatalf("playhead=%d want %d (+1 beat)", got, 5*div)
	}
}

func TestRewindButtonRequestsBackwardSeek(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(4*div, seekOriginJS)
	g.drum.transportZone.callbacks.OnRewind()
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 3*div {
		t.Fatalf("playhead=%d want %d (-1 beat)", got, 3*div)
	}
}
```

Adjust the path to the callbacks (`g.drum.transportZone.callbacks`) to the real field names — grep `transport_zone.go` / `drumview_ctor.go` for how the zone and its `callbacks TransportCallbacks` are reachable from `DrumView`. If the callbacks struct is unexported and unreachable from the test, instead call a small exported test shim or drive the button's `OnClick` directly via an exported accessor already used by other transport tests (see `transport_test.go` for the established pattern).

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestFastForwardButton|TestRewindButton'`
Expected: FAIL — `OnFastForward`/`OnRewind` undefined.

- [ ] **Step 3: Add callback fields**

In `transport_zone.go`, add to `TransportCallbacks` (struct at ~line 17):

```go
	OnRewind      func() // seek -1 beat
	OnFastForward func() // seek +1 beat
```

- [ ] **Step 4: Construct the buttons with accelerating hold-repeat**

In `transport_zone.go`, next to the stop/record button construction (~line 184), add:

```go
	z.rwBtn = NewButton("", nil, func() {
		hapticTransportTap()
		if z.callbacks.OnRewind != nil {
			z.callbacks.OnRewind()
		}
	})
	z.rwBtn.Icon = string(IconRewind)
	z.rwBtn.Repeat = true // hold = accelerating repeat via repeatTick

	z.ffBtn = NewButton("", nil, func() {
		hapticTransportTap()
		if z.callbacks.OnFastForward != nil {
			z.callbacks.OnFastForward()
		}
	})
	z.ffBtn.Icon = string(IconFastForward)
	z.ffBtn.Repeat = true
```

Add the fields to the `TransportZone` struct (near `playBtn/stopBtn/recordBtn` at ~line 55):

```go
	rwBtn *Button
	ffBtn *Button
```

- [ ] **Step 5: Lay out and register hit areas**

In the desktop layout (SetRect, ~line 738) place `rwBtn` left of Play and `ffBtn` right of Stop by extending the grid `weights` and adding cells. Add two weight entries (value `1`) and:

```go
	z.rwBtn.SetRect(safeInsetTransport(row0Grid.Cell(rwCol, 0), pad))
	z.ffBtn.SetRect(safeInsetTransport(row0Grid.Cell(ffCol, 0), pad))
```

with `rwCol`/`ffCol` the new column indices (renumber the existing cells accordingly). Mirror the same in the square-unit desktop path (~line 849) and the mobile bottom-bar path if the transport group appears there. Apply styles in `applyProfile` (~line 329) alongside stop/record:

```go
	z.rwBtn.Style = p.StopBtnStyle
	z.rwBtn.IconColor = colTextPrimary
	z.ffBtn.Style = p.StopBtnStyle
	z.ffBtn.IconColor = colTextPrimary
```

Register hit areas in the `simpleButtons` slice (~line 922):

```go
		{z.rwBtn, "transport-rewind", true},
		{z.ffBtn, "transport-fastforward", true},
```

- [ ] **Step 6: Wire the callbacks in `drumview_ctor.go`**

Where `TransportCallbacks` is constructed (grep `drumview_ctor.go` for `OnStop:` / `OnPlayToggle:`), add:

```go
		OnRewind:      func() { dv.RequestSeekDeltaBeats(-1, seekOriginTransport) },
		OnFastForward: func() { dv.RequestSeekDeltaBeats(1, seekOriginTransport) },
```

- [ ] **Step 7: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestFastForwardButton|TestRewindButton|Transport'`
Expected: PASS. Watch `transport_no_overlap_test.go`, `transport_width_test.go`, `transport_equal_squares_test.go` — if the new columns break the width/overlap assertions, update those tests' expected button counts/rects with a one-line rationale (added FF/RW).

- [ ] **Step 8: Commit** (STOP — leave uncommitted)

```
feat(ui): FF / RW transport buttons with accelerating hold-shuttle
```

---

## Task 10: Ribbon click-to-seek + drag ghost

**Files:**
- Modify: `src/go/internal/ui/timeline_zone.go` (scrub adapter, ghost draw, callbacks)
- Modify: `src/go/internal/ui/drumview_ctor.go` (wire new callbacks; remove `OnScrubPosition` wiring)
- Test: `src/go/internal/ui/timeline_seek_test.go` (create)

**Interfaces:**
- Consumes: `dv.RequestSeekSteps`, `z.callbacks.TimelineUnitsPerBeat/TimelineBeats/PlayheadStep`.
- Produces:
  - `TimelineCallbacks.OnSeekToStep func(step int, origin string)`
  - `TimelineCallbacks.PlayheadStep func() int`
  - Removes `TimelineCallbacks.OnScrubPosition` and its ctor wiring.
  - Ribbon adapter: press → seek to nearest beat; drag → ghost preview (`z.scrubGhostBeat`, `z.scrubbing`); release → commit final.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/timeline_seek_test.go`. Drive the scrub adapter's press/release and assert a beat-snapped seek request is queued:

```go
package ui

import "testing"

func TestRibbonPressSeeksToBeat(t *testing.T) {
	g := newGameWithStart(t)
	updateOnce(g) // lay out zones so timelineBarRect is populated (use the real one-frame-update helper)
	z := g.drum.timelineZone
	bar := z.timelineBarRect
	// Press at ~40% across the bar.
	x := bar.Min.X + bar.Dx()*2/5
	adapter := &timelineScrubHitAdapter{zone: z}
	adapter.OnPress(x, bar.Min.Y+bar.Dy()/2)
	adapter.OnRelease(x, bar.Min.Y+bar.Dy()/2)
	g.drainSeekRequest()
	// Expected beat = round(0.4 * timelineBeats); assert the playhead landed on
	// a whole-beat boundary matching that.
	div := g.grid.MaxDiv()
	if got := g.playheadAbsSubdiv(); got%div != 0 {
		t.Fatalf("ribbon seek must snap to a whole beat, got step=%d (div=%d)", got, div)
	}
}
```

Wire `newTestGameWithTimeline` / `g.drum.timelineZone` to the real harness + field (grep `timeline_zone` tests such as `timeline_ribbon_coordinate_systems`-related tests for how the zone is constructed and laid out in a test).

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestRibbonPressSeeks`
Expected: FAIL (either compile error on new fields, or the old offset behavior leaves the playhead at 0).

- [ ] **Step 3: Add callbacks + ghost state; redesign the scrub adapter**

In `timeline_zone.go`, in `TimelineCallbacks` (struct at line 23) remove `OnScrubPosition func(newOffset int)` (line 55) and add:

```go
	OnSeekToStep func(step int, origin string) // ribbon seek commit
	PlayheadStep func() int                    // current absolute subdivision
```

Add ghost state to `TimelineZone` (near `scrubbing bool`, line 87):

```go
	scrubGhostBeat int // beat under the pointer during a scrub drag
	scrubPressBeat int // beat committed on press (so a tap re-seeks at most once)
```

Add a helper that maps a pointer X to a beat-snapped subdivision step:

```go
// ribbonStepAt maps pointer X on the timeline bar to the nearest whole-beat
// absolute subdivision step.
func (z *TimelineZone) ribbonStepAt(x int) int {
	bar := z.timelineBarRect
	if bar.Dx() <= 0 {
		return 0
	}
	if x < bar.Min.X {
		x = bar.Min.X
	} else if x >= bar.Max.X {
		x = bar.Max.X - 1
	}
	frac := float64(x-bar.Min.X) / float64(bar.Dx())
	beats := 0
	if z.callbacks.TimelineBeats != nil {
		beats = z.callbacks.TimelineBeats()
	}
	beat := int(math.Round(frac * float64(beats)))
	if beat < 0 {
		beat = 0
	} else if beat > beats {
		beat = beats
	}
	div := 1
	if z.callbacks.TimelineUnitsPerBeat != nil {
		if d := z.callbacks.TimelineUnitsPerBeat(); d > 0 {
			div = d
		}
	}
	return beat * div
}
```

(Add `"math"` to the file's imports if not present.) Replace the `timelineScrubHitAdapter` methods (lines 1363-1445) — keep the multi-touch/haptic/self-release guards, change the payload from offset to seek:

```go
func (h *timelineScrubHitAdapter) OnPress(x, y int) InputResult {
	if globalTouchState != nil && globalTouchState.RecentMultiTouch() {
		return InputIgnored
	}
	z := h.zone
	z.scrubbing = true
	z.scrubGhostBeat = z.ribbonStepAt(x)
	z.scrubPressBeat = z.scrubGhostBeat // remember where the gesture started
	hapticEmit(8)
	if z.callbacks.OnSeekToStep != nil {
		z.callbacks.OnSeekToStep(z.scrubGhostBeat, seekOriginRibbon) // seek on press
	}
	return InputCaptured
}

func (h *timelineScrubHitAdapter) OnDrag(x, y int) {
	z := h.zone
	if !z.scrubbing {
		return
	}
	skirt := z.timelineBarRect.Inset(-4)
	if y < skirt.Min.Y || y >= skirt.Max.Y {
		z.scrubbing = false
		return
	}
	z.scrubGhostBeat = z.ribbonStepAt(x) // ghost only — no seek during drag
}

func (h *timelineScrubHitAdapter) OnRelease(x, y int) {
	z := h.zone
	if z.scrubbing && z.callbacks.OnSeekToStep != nil {
		final := z.ribbonStepAt(x)
		// Re-seek on release ONLY if the gesture moved to a different beat than
		// the press already committed. A pure tap (final == press beat) emits
		// ONE EventSeek; a drag emits at most two (press start + release end).
		if final != z.scrubPressBeat {
			z.callbacks.OnSeekToStep(final, seekOriginRibbon)
		}
	}
	z.scrubbing = false
}

func (h *timelineScrubHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }
```

Delete the old `scrubTo` method (its offset logic is gone). Grep for any remaining `scrubTo` references and remove them.

- [ ] **Step 4: Draw the ghost during a scrub**

In `drawTimelineBar` (~line 346), after the playback cursor draw, add a ghost line + beat chip when scrubbing. Add near the end of the bar-draw body:

```go
	if z.scrubbing {
		z.drawScrubGhost(screen)
	}
```

and define:

```go
// drawScrubGhost renders the pending seek position (a vertical line + beat
// chip) while the user drags the ribbon.
func (z *TimelineZone) drawScrubGhost(screen *ebiten.Image) {
	bar := z.timelineBarRect
	beats := 1
	if z.callbacks.TimelineBeats != nil {
		if b := z.callbacks.TimelineBeats(); b > 0 {
			beats = b
		}
	}
	div := 1
	if z.callbacks.TimelineUnitsPerBeat != nil {
		if d := z.callbacks.TimelineUnitsPerBeat(); d > 0 {
			div = d
		}
	}
	ghostBeat := z.scrubGhostBeat / div
	frac := float64(ghostBeat) / float64(beats)
	gx := bar.Min.X + int(frac*float64(bar.Dx()))
	drawRect(screen, image.Rect(gx, bar.Min.Y, gx+1, bar.Max.Y), colAccentPrimary)
	// Beat chip near the top of the bar.
	DrawTextAt(screen, strconv.Itoa(ghostBeat), gx+3, bar.Min.Y+2, colTextPrimary)
}
```

Use the real accent-color token and text helper already used in `drawBeatCounter`/`drawTimelineBar` (grep for `DrawTextAt`, `colAccent*` in this file; match them). Add `"strconv"` and `"image"` imports if missing.

- [ ] **Step 5: Rewire `drumview_ctor.go`**

In `drumview_ctor.go`, find the `TimelineCallbacks{...}` literal (~line 1007, the `OnScrubPosition` block). Remove the `OnScrubPosition:` entry and add:

```go
		OnSeekToStep: func(step int, origin string) {
			dv.RequestSeekSteps(step, origin)
		},
		PlayheadStep: func() int {
			if dv.game != nil {
				return dv.game.playheadAbsSubdiv()
			}
			return 0
		}, // dv.game is DrumView's *Game back-ref (drumview.go:532)
```

Use the real back-reference from `DrumView` to `Game` (grep `drumview` for the game pointer field — it may be `dv.game` or reached via a callback; match the existing pattern, e.g. how `IsPlaying`/`BPM` callbacks reach engine state).

- [ ] **Step 6: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestRibbon|Timeline'`
Expected: PASS. `beat_counter_input_isolation_test.go` and any timeline scrub test that asserted offset changes will need updating — the ribbon no longer moves offset. Update those to assert seek (or move them under Task 14's e2e repoint) with a one-line rationale.

- [ ] **Step 7: Commit** (STOP — leave uncommitted)

```
feat(ui): ribbon click-to-seek with drag ghost preview
```

---

## Task 11: Beat-counter position entry

**Files:**
- Modify: `src/go/internal/ui/timeline_zone.go` (host `ParamValueEditor`, convert `beatCounterHitAdapter`, lifecycle)
- Modify: `src/go/internal/ui/drumview_ctor.go` (wire callbacks)
- Test: `src/go/internal/ui/beat_counter_entry_test.go` (create)

**Interfaces:**
- Consumes: `ParamValueEditor`, `ValueSpec{Format,Parse}`, `ValueOpen`, `dv.RequestSeekSteps`, `z.callbacks.PlayheadStep/TimelineUnitsPerBeat/TimelineBeats`.
- Produces: `z.posEditor *ParamValueEditor`; converted `beatCounterHitAdapter` that opens the position editor; a `positionValueSpec(div, maxSteps int) ValueSpec` whose value is carried as **integer steps** and whose text grammar is `beat` or `beat.subdiv`.

- [ ] **Step 1: Write the failing test (parser is the load-bearing part)**

Create `src/go/internal/ui/beat_counter_entry_test.go`:

```go
package ui

import "testing"

func TestPositionSpecParsesBeatAndSubdiv(t *testing.T) {
	div := 8
	maxSteps := 64 * div
	spec := positionValueSpec(div, maxSteps)

	cases := []struct {
		text string
		want float64 // steps
		ok   bool
	}{
		{"17", float64(17 * div), true},        // whole beat
		{"17.3", float64(17*div + 3), true},    // beat 17, subdivision index 3
		{"0.0", 0, true},                       // origin
		{"2.7", float64(2*div + 7), true},      // last subdiv at div=8
		{"2.9", float64(2*div + 7), true},      // subdiv clamped to div-1
		{"", 0, false},                         // empty rejected
		{"abc", 0, false},                      // garbage rejected
	}
	for _, c := range cases {
		got, ok := spec.Parse(c.text)
		if ok != c.ok {
			t.Fatalf("Parse(%q) ok=%v want %v", c.text, ok, c.ok)
		}
		if ok && got != c.want {
			t.Fatalf("Parse(%q)=%v want %v", c.text, got, c.want)
		}
	}
}

func TestPositionSpecFormatsBeatDotSubdiv(t *testing.T) {
	div := 8
	spec := positionValueSpec(div, 64*div)
	if s := spec.Format(float64(17*div + 3)); s != "17.3" {
		t.Fatalf("Format(139)=%q want 17.3", s)
	}
	if s := spec.Format(float64(17 * div)); s != "17" {
		t.Fatalf("Format(136)=%q want 17 (no .0 suffix on beat boundary)", s)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestPositionSpec`
Expected: FAIL — `positionValueSpec undefined`.

- [ ] **Step 3: Implement `positionValueSpec`**

Create `src/go/internal/ui/timeline_position_spec.go`:

```go
package ui

import (
	"strconv"
	"strings"
)

// positionValueSpec builds the beat-counter editor spec. The carried value is
// an ABSOLUTE SUBDIVISION STEP (integer, stored in the float64). Text grammar:
//
//	"B"      -> beat B, subdivision 0
//	"B.S"    -> beat B, subdivision index S (0-based, clamped to [0, div-1])
//
// Parsing operates on the raw string (not a float), so "17.10" is unambiguous
// (beat 17, subdiv 10) — a float editor could not distinguish .10 from .1.
func positionValueSpec(div, maxSteps int) ValueSpec {
	if div <= 0 {
		div = 1
	}
	return ValueSpec{
		MinW:   64,
		MaxLen: 8,
		Format: func(v float64) string {
			steps := int(v)
			if steps < 0 {
				steps = 0
			}
			beat := steps / div
			sub := steps % div
			if sub == 0 {
				return strconv.Itoa(beat)
			}
			return strconv.Itoa(beat) + "." + strconv.Itoa(sub)
		},
		Parse: func(text string) (float64, bool) {
			text = strings.TrimSpace(text)
			if text == "" {
				return 0, false
			}
			beatPart, subPart := text, ""
			if i := strings.IndexByte(text, '.'); i >= 0 {
				beatPart, subPart = text[:i], text[i+1:]
			}
			beat, err := strconv.Atoi(strings.TrimSpace(beatPart))
			if err != nil || beat < 0 {
				return 0, false
			}
			sub := 0
			if subPart != "" {
				s, err := strconv.Atoi(strings.TrimSpace(subPart))
				if err != nil || s < 0 {
					return 0, false
				}
				sub = s
			}
			if sub > div-1 {
				sub = div - 1 // clamp to the last subdivision in the beat
			}
			steps := beat*div + sub
			if steps < 0 {
				steps = 0
			}
			if maxSteps > 0 && steps > maxSteps {
				steps = maxSteps
			}
			return float64(steps), true
		},
	}
}
```

- [ ] **Step 4: Host the editor in `TimelineZone` and drive its lifecycle**

In `timeline_zone.go`, add a field to `TimelineZone` (near `portal *OverlayPortal`):

```go
	posEditor *ParamValueEditor
```

In `(z *TimelineZone) Update()` (line 214), drive the editor and block input while it's open (mirror `transport_zone.go`'s `paramEditor.Update()` handling):

```go
	if z.posEditor != nil && z.posEditor.Active() {
		z.posEditor.Update()
	}
```

In `(z *TimelineZone) Draw(screen)` (line 224), after the bar/content draw, add:

```go
	if z.posEditor != nil && z.posEditor.Active() {
		z.posEditor.Draw(screen)
	}
```

Add an open method:

```go
// openPositionEditor pops the numeric position editor over the beat counter.
func (z *TimelineZone) openPositionEditor() {
	if z.posEditor == nil {
		z.posEditor = NewParamValueEditor()
	}
	div := 1
	if z.callbacks.TimelineUnitsPerBeat != nil {
		if d := z.callbacks.TimelineUnitsPerBeat(); d > 0 {
			div = d
		}
	}
	beats := 0
	if z.callbacks.TimelineBeats != nil {
		beats = z.callbacks.TimelineBeats()
	}
	maxSteps := beats * div
	anchor := z.rect
	if z.callbacks.BeatCounterRect != nil {
		anchor = z.callbacks.BeatCounterRect()
	}
	z.posEditor.OpenValue(ValueOpen{
		Spec:          positionValueSpec(div, maxSteps),
		Anchor:        anchor,
		Clamp:         z.rect,
		MobileInputID: "position",
		Get: func() float64 {
			if z.callbacks.PlayheadStep != nil {
				return float64(z.callbacks.PlayheadStep())
			}
			return 0
		},
		Set: func(v float64) {
			if z.callbacks.OnSeekToStep != nil {
				z.callbacks.OnSeekToStep(int(v), seekOriginCounter)
			}
		},
	})
}
```

- [ ] **Step 5: Convert the beat-counter hit adapter from absorber to opener**

In `timeline_zone.go`, replace the `beatCounterHitAdapter` (lines ~1223-1230) so it holds the zone and opens the editor:

```go
type beatCounterHitAdapter struct{ zone *TimelineZone }

func (h *beatCounterHitAdapter) OnPress(x, y int) InputResult {
	if h.zone != nil {
		h.zone.openPositionEditor()
	}
	return InputConsumed
}
func (h *beatCounterHitAdapter) OnDrag(x, y int)                    {}
func (h *beatCounterHitAdapter) OnRelease(x, y int)                 {}
func (h *beatCounterHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }
```

At the registration site (~line 1195), pass the zone:

```go
			Handler:  &beatCounterHitAdapter{zone: z},
```

- [ ] **Step 6: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestPositionSpec|TestBeatCounter|Timeline'`
Expected: PASS. If `beat_counter_input_isolation_test.go` asserted the adapter consumes-and-does-nothing, update it to allow the popup-open (the press is still `InputConsumed`, so isolation holds).

- [ ] **Step 7: Commit** (STOP — leave uncommitted)

```
feat(ui): tappable beat counter opens beat.subdivision position entry
```

---

## Task 12: Keyboard seek shortcuts

**Files:**
- Modify: `src/go/internal/ui/keyboard_shortcuts.go` (z-const + `r.add`)
- Modify: `src/go/internal/ui/game_input_shortcuts.go` (`seekShortcut`)
- Test: `src/go/internal/ui/keyboard_seek_test.go` (create) + extend the router key-collision test

**Interfaces:**
- Consumes: `keyboardShortcutRouter.add`, `shortcutFunc`, `dv.RequestSeekDeltaBeats`, `dv.RequestSeekSteps`.
- Produces: `ZShortcutSeek = 25`; `func (g *Game) seekShortcut() bool` bound to keys `KeyPeriod`, `KeyComma`, `KeyHome` (with Shift for ±4).

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/keyboard_seek_test.go`. Assert the router registers the node and that its keys don't collide with existing nodes:

```go
package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestSeekShortcutNodeRegistered(t *testing.T) {
	g := newGameWithStart(t)
	r := newKeyboardShortcutRouter(g)
	keys := r.keysOf("seek")
	if len(keys) == 0 {
		t.Fatal("seek shortcut node not registered")
	}
	want := map[ebiten.Key]bool{ebiten.KeyPeriod: true, ebiten.KeyComma: true, ebiten.KeyHome: true}
	for _, k := range keys {
		if !want[k] {
			t.Fatalf("unexpected seek key %v", k)
		}
	}
}

func TestSeekKeysDoNotCollide(t *testing.T) {
	g := newGameWithStart(t)
	r := newKeyboardShortcutRouter(g)
	seen := map[ebiten.Key]string{}
	for _, name := range r.nodeNames() {
		for _, k := range r.keysOf(name) {
			if other, dup := seen[k]; dup && other != name {
				t.Fatalf("key %v claimed by both %q and %q", k, other, name)
			}
			seen[k] = name
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSeekShortcut|TestSeekKeys'`
Expected: FAIL — node `"seek"` not registered.

- [ ] **Step 3: Add the z-constant and register the node**

In `keyboard_shortcuts.go`, add to the z-const block (~line 45):

```go
	ZShortcutSeek = 25 // above transport (20), below audio panel (30)
```

In `newKeyboardShortcutRouter` (~line 128), add after the transport registration:

```go
	r.add("seek", ZShortcutSeek, true, []ebiten.Key{
		ebiten.KeyPeriod, ebiten.KeyComma, ebiten.KeyHome,
	}, shortcutFunc(g.seekShortcut))
```

- [ ] **Step 4: Implement `seekShortcut`**

In `game_input_shortcuts.go`, add (model on `transportShortcut` at line 186):

```go
// seekShortcut handles playhead navigation keys: "." = +1 beat, "," = -1 beat
// (Shift = ±4), Home = seek to 0. Held keys repeat via isKeyJustPressed edges
// plus auto-repeat from the OS; use JustPressed so a tap = one beat.
func (g *Game) seekShortcut() bool {
	if g.drum == nil {
		return false
	}
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	step := 1
	if shift {
		step = 4
	}
	acted := false
	if isKeyJustPressed(ebiten.KeyPeriod) {
		g.drum.RequestSeekDeltaBeats(step, seekOriginKeyboard)
		acted = true
	}
	if isKeyJustPressed(ebiten.KeyComma) {
		g.drum.RequestSeekDeltaBeats(-step, seekOriginKeyboard)
		acted = true
	}
	if isKeyJustPressed(ebiten.KeyHome) {
		g.drum.RequestSeekSteps(0, seekOriginKeyboard)
		acted = true
	}
	return acted
}
```

- [ ] **Step 5: Run the tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSeek|Keyboard|Shortcut'`
Expected: PASS — including any existing router test.

- [ ] **Step 6: Commit** (STOP — leave uncommitted)

```
feat(ui): keyboard seek shortcuts (. , Home; Shift = ±4)
```

---

## Task 13: JS exports `seekSteps` + `playheadStep`

**Files:**
- Modify: `src/go/internal/ui/js_exports_graph_ui.go`
- Modify: `src/js/wasm_bridge_smoke.browser.test.js` (catalogue)
- Test: `src/go/internal/ui/js_exports_catalogue_drift_test.go` must stay green (update its expected set)

**Interfaces:**
- Consumes: `jsFn`, `jsArgs`, `dv.RequestSeekSteps`, `g.playheadAbsSubdiv`.
- Produces: JS globals `seekSteps(steps)` and `playheadStep()`.

- [ ] **Step 1: Add the exports**

In `js_exports_graph_ui.go`, near `clickTimelineAt` (~line 1339), add:

```go
	js.Global().Set("seekSteps", jsFn(func(args jsArgs) any {
		if g.drum == nil || args.Len() < 1 {
			return nil
		}
		g.drum.RequestSeekSteps(args.Int(0), seekOriginJS)
		return nil
	}))
	js.Global().Set("playheadStep", jsFn(func(args jsArgs) any {
		return g.playheadAbsSubdiv()
	}))
```

Note: `seekSteps` queues a request drained on the next `Update` under `seqMu` (do NOT call `SeekSteps` directly here — the JS callback is not inside the locked region).

- [ ] **Step 2: Update the catalogue drift test expectation**

Run the drift test to see the failure and the expected-set location:
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestJsExportsCatalogue`
Expected: FAIL listing `seekSteps` / `playheadStep` as undocumented. Add both names to the expected set the test reads (follow the failure message to the exact slice/map in `js_exports_catalogue_drift_test.go`).

- [ ] **Step 3: Add catalogue entries in the JS smoke test**

In `src/js/wasm_bridge_smoke.browser.test.js`, add (alphabetical-ish, matching neighbors):

```js
  { name: "playheadStep", args: [], returns: "any" },
  { name: "seekSteps", args: [0] },
```

- [ ] **Step 4: Verify**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestJsExportsCatalogue`
Expected: PASS.

Run (if a WASM toolchain is available): `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js` from repo root.
Expected: PASS (both exports exist and are callable). If no WASM build is available in this environment, note it and rely on the Go drift test.

- [ ] **Step 5: Commit** (STOP — leave uncommitted)

```
feat(wasm): seekSteps / playheadStep JS exports
```

---

## Task 14: Browser test repoint + xplat parity

**Files:**
- Modify: `src/js/e2e_timeline_seek.browser.test.js`
- Modify: `src/js/xplat_parity.browser.test.js`

**Interfaces:**
- Consumes: `seekSteps`, `playheadStep`, `timelineRect`, `timelineUnitsPerBeat`.

- [ ] **Step 1: Repoint `e2e_timeline_seek` to the playhead**

Open `src/js/e2e_timeline_seek.browser.test.js`. It currently clicks the timeline rect and asserts `drumOffset()` changed. Change the assertion to prove a **playhead seek** now: after clicking ~40% across `timelineRect()`, assert `playheadStep()` changed to a whole-beat boundary (`step % timelineUnitsPerBeat() === 0`) and is non-zero. Update the top-of-file comment to reflect the new semantics (ribbon = seek, not offset). Keep a separate assertion that grid-drag / two-finger pan still change `drumOffset()` if the test already exercises those; otherwise leave view-scroll coverage to existing touch tests.

- [ ] **Step 2: Add a seek step to `xplat_parity`**

In `src/js/xplat_parity.browser.test.js`, add a scenario: call `seekSteps(N)` (a mid-song subdivision), advance, and assert the WASM predictor/audio golden matches native for the post-seek region — reuse the file's existing compare harness (mirror an existing scenario's shape; call `seekSteps` before the capture and compare the same way).

- [ ] **Step 3: Run the browser tests**

Run: `GO=$(pwd)/.tools/go/bin/go node src/js/e2e_timeline_seek.browser.test.js` (from repo root)
Expected: PASS.

Run: `GO=$(pwd)/.tools/go/bin/go node src/js/xplat_parity.browser.test.js`
Expected: PASS. (If the environment lacks a browser/WASM toolchain, note that these must be run in CI or a browser-capable env before merge.)

- [ ] **Step 4: Commit** (STOP — leave uncommitted)

```
test(js): repoint timeline-seek e2e to playhead; xplat seek parity
```

---

## Final verification

- [ ] **Full fast suite:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...` — PASS.
- [ ] **Race check on touched concurrency:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -race ./internal/timeline/ ./core/engine/` — PASS.
- [ ] **Real-Ebiten transport/timeline:** `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/ui/ -run 'Transport|Timeline|Seek'` — PASS.
- [ ] **Manual smoke via the `run` skill:** start the desktop app, play, click the ribbon mid-song (playhead jumps), tap FF/RW (steps ±1 beat, hold accelerates), tap the beat counter and type `8.3` (jumps to beat 8 subdiv 3), press `.`/`,`/`Home`. Confirm audio follows and the ribbon/cursor stay in sync.
- [ ] **Parity:** confirm no parity panics during seek-while-playing (the discipline the clock refactor + predictor RebaseAt must preserve).

---

## Notes for the implementer

- **Resolved names (verified during planning):** internal `package ui` tests construct a playable game with `newGameWithStart(t) *Game` (alternatives: `newTestGameForUndo`, `newTestGameForParity`). `Game`'s DrumView is `g.drum`; DrumView back-ref to Game is `dv.game` (`drumview.go:532`); zones are `dv.timelineZone` (`drumview.go:303`) and `dv.transportZone` (`drumview.go:297`), the latter with an unexported `callbacks TransportCallbacks` field reachable from package-`ui` tests. `hooks.Subscribe(kind, fn) func()` returns an unsub func, but the bus is async (worker pool) — observe events via a channel + timeout, never a synchronous count. The **one genuinely unverified helper** is `updateOnce(g)` (drive a single `Game.Update` so zones lay out and `timelineBarRect` populates) — grep `testutil_test.go` / existing timeline tests for the real one-frame-update idiom before Task 10.
- **`seqMu` discipline:** never call `SeekSteps` outside the drain site or undo. Every UI entry point sets a DrumView request; the drain runs under the lock. `timeline.RewindTo` and `predictor.RebaseAt` take their own internal locks — independent of `seqMu`, no reentrancy.
- **Archive is first-pass retention:** `rowArchive.append` refuses to overwrite an existing archived Playback/Import at the same abs, so repeated rewinds over the same region retain the *first* superseded value, not every pass. This satisfies full-history-retention (nothing the user played is lost on the first supersede) without unbounded growth; the live ribbon always shows the latest reality because ring/sidecar re-records shadow the archive on read.
- **If `transport_no_overlap_test.go` / width tests fail** after adding FF/RW: they encode the exact button count/geometry — update expectations with a one-line rationale (added two transport buttons), do not loosen the assertion.
