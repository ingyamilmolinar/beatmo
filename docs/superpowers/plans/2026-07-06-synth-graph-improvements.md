# Synth-Tab Graph Accuracy + Kid-Readability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the Synth tab's graphs accurate (every knob visibly moves a truthful picture) and kid-readable: a three-card right pane (full-note "Your sound" + "Up close" steady wave + per-knob focus graph), honest per-knob renderers with story cues, and (wave 2) a living pipeline chip strip.

**Architecture:** All display-only Go in `src/go/internal/ui/`, plus one additive audio primitive (`RenderInstrumentPreviewWithOverrides`). The existing `synthMirror` hash-gated cache gains a second (full-note) slot; the focus-graph routing table gets dedicated LFO/burst/pitch-env/post renderers; the chip strip gets arrows + waveform watermarks. Spec: `docs/superpowers/specs/2026-07-06-synth-graph-improvements-design.md`.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), Ebiten (stubbed under `-tags test`), design-token pipeline (`DESIGN.md` → `make gen-design-tokens`), i18n (`internal/i18n`, EN + es-419).

## Global Constraints

- **NEVER `git add`, `git commit`, `git revert`, or `rm` any file.** All changes stay uncommitted in the working tree (standing user policy). Plan steps therefore have NO commit steps.
- Go binary: always `../../.tools/go/bin/go` from `src/go` (absolute: `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`).
- Fast test command shape: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestName'` (same for `./internal/audio/`).
- If the package build is red from CONCURRENT unrelated WIP, verify your own work with `-run` filters; never delete others' files (see memory: verify via `-run` when full build is red).
- No inline `color.RGBA{}` literals in `internal/ui/` (token-discipline ratchet); use existing tokens/`WithAlpha` buckets only.
- No forbidden glyphs (`▶▼▲✕≡⏸■↑↓→←`) in `internal/ui/` source — use `IconID` + `DrawIcon`. i18n catalog strings must not contain them either (keep legend text glyph-free).
- Every new user-visible string: add a `Key` const in `src/go/internal/i18n/keys.go`, entries in `catalog_en.go` AND `catalog_es.go`, and extend the key list around `keys.go:579` if the new key's group is enumerated there (follow `KeyCapYourSound`'s pattern).
- New density-sized values: declare in `DESIGN.md` `densities:` under ALL THREE densities (compact ~line 592 block, comfortable ~681, spacious ~770), check `scripts/gen_design_tokens/schema.json` accepts the key, run `make gen-design-tokens`, read via `Profile().DensityValues().X`.
- Tests that build a `synthMirror` via `newSynthMirror()` must `t.Cleanup` with `closeForTest()`; tests exercising only cache logic use `&synthMirror{}` directly (NO pool — shared-pool Release breaks sibling tests).
- The bidirectional sweep gate `TestFocusGraph_RespondsToEveryKnob` must pass at the end of every task that touches renderers/routing.

## File Structure (new/modified)

| File | Role |
|---|---|
| `src/go/internal/audio/preview_render.go` | + `RenderInstrumentPreviewWithOverrides` (Task 1) |
| `src/go/internal/audio/preview_override_test.go` | NEW — override-form tests (Task 1) |
| `src/go/internal/ui/synth_full_note.go` | NEW — adaptive window, min/max columns, `drawSynthFullNote` (Tasks 2, 4) |
| `src/go/internal/ui/synth_full_note_test.go` | NEW — tests for the above |
| `src/go/internal/ui/synth_mirror.go` | + full-note cache slot + `updateSynthFullNoteLive` (Task 3) |
| `src/go/internal/ui/synth_mirror_full_test.go` | NEW — full-slot cache tests (Task 3) |
| `src/go/internal/ui/synth_focus_graph.go` | 3-way split, routing table update, bypass hint, legend (Tasks 5, 6, 10, 11) |
| `src/go/internal/ui/synth_concept_viz.go` | new renderers `conceptLFO`/`conceptPitchEnvSweep`/`conceptBurst`/`conceptPostWave`, interpolation fix, cue helpers (Tasks 6–9, 11) |
| `src/go/internal/ui/synth_lfo_burst_viz_test.go` | NEW — LFO/burst/pitch-env renderer tests (Tasks 6, 7) |
| `src/go/internal/ui/synth_panel_zone.go` | `drawSynthTab` wiring (Task 5); chip strip arrows+watermarks (Task 15) |
| `src/go/internal/ui/synth_focus_sweep_test.go` | shrink exemptions (Tasks 7, 6, 12) |
| `src/go/internal/i18n/keys.go`, `catalog_en.go`, `catalog_es.go` | new keys (Tasks 4, 10, 11) |
| `DESIGN.md` + `src/go/internal/ui/design_density.gen.go` | `synthRightCardMinH` token (Task 5) |
| `src/go/internal/ui/scene_catalog.go` | new crop scenes (Task 13) |
| `src/go/internal/ui/synth_stage_thumbs.go` (+`_test.go`) | NEW, wave 2 — per-stage thumbnail cache (Task 14) |

Execution order: Tasks 1→13 are wave 1 (ship-worthy on their own). Tasks 14–15 are wave 2 (living pipeline strip). Task 16 is final verification.

---

### Task 1: `audio.RenderInstrumentPreviewWithOverrides`

**Files:**
- Modify: `src/go/internal/audio/preview_render.go:78-92`
- Create: `src/go/internal/audio/preview_override_test.go`

**Interfaces:**
- Produces: `func RenderInstrumentPreviewWithOverrides(instrumentID string, override map[string]float64, durationMs int) []float64` — identical to `RenderInstrumentPreview` but params in `override` win over the instrument's merged params. `RenderInstrumentPreview(id, ms)` must remain byte-identical to `RenderInstrumentPreviewWithOverrides(id, nil, ms)`.
- Consumes: existing `previewParamLookup(instrumentID, override)` (already takes an override map — `RenderInstrumentPreviewWave` uses it; `RenderInstrumentPreview` currently passes `nil` at line 92).

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package audio

import "testing"

// The override form must (a) equal the base form when override is nil and
// (b) change the render when an override pins a param the preview reads.
func TestRenderInstrumentPreviewWithOverrides_NilMatchesBase(t *testing.T) {
	const inst = "preview-override-test"
	BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { ResetInstrumentParams(inst) })

	base := RenderInstrumentPreview(inst, 120)
	viaNil := RenderInstrumentPreviewWithOverrides(inst, nil, 120)
	if previewHash(base) != previewHash(viaNil) {
		t.Fatalf("nil-override render differs from RenderInstrumentPreview")
	}
	if len(base) == 0 {
		t.Fatalf("empty preview render")
	}
}

func TestRenderInstrumentPreviewWithOverrides_OverrideChangesOutput(t *testing.T) {
	const inst = "preview-override-test-2"
	BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { ResetInstrumentParams(inst) })

	base := RenderInstrumentPreview(inst, 120)
	shifted := RenderInstrumentPreviewWithOverrides(inst, map[string]float64{"fundamental": 550}, 120)
	if previewHash(base) == previewHash(shifted) {
		t.Fatalf("fundamental override did not change the preview render")
	}
	// The override must NOT leak into instrument state.
	after := RenderInstrumentPreview(inst, 120)
	if previewHash(base) != previewHash(after) {
		t.Fatalf("override mutated persistent instrument params")
	}
}
```

(`previewHash` and `BindInstrumentToRecipe` already exist — see `preview_render_responsiveness_test.go:12,42`.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run 'TestRenderInstrumentPreviewWithOverrides'`
Expected: FAIL — `undefined: RenderInstrumentPreviewWithOverrides`.

- [ ] **Step 3: Implement**

In `preview_render.go`, rename the existing `RenderInstrumentPreview` body and add a thin delegate. Only the signature line and the `previewParamLookup` call change:

```go
// RenderInstrumentPreview returns float64 mono samples for one note of the
// instrument's resolved recipe params. durationMs is clamped so the call stays
// cheap. Safe to call from a worker goroutine (reads param maps only).
func RenderInstrumentPreview(instrumentID string, durationMs int) []float64 {
	return RenderInstrumentPreviewWithOverrides(instrumentID, nil, durationMs)
}

// RenderInstrumentPreviewWithOverrides is RenderInstrumentPreview with an
// explicit override map layered over the instrument's merged params (override
// wins). Lets read-only callers (focus-graph ghosts, pipeline thumbnails)
// render "what if" notes without mutating instrument state.
func RenderInstrumentPreviewWithOverrides(instrumentID string, override map[string]float64, durationMs int) []float64 {
	// ... the ENTIRE former RenderInstrumentPreview body, unchanged except:
	get, has, lookup := previewParamLookup(instrumentID, override) // was: nil
	// ...
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: same command as Step 2. Expected: PASS.
Also run the existing guards: `... -run 'TestRenderInstrumentPreview'` — all PASS (delegation must not change behavior).

---

### Task 2: Adaptive full-note window — `synthFullNoteWindowMs`

**Files:**
- Create: `src/go/internal/ui/synth_full_note.go`
- Create: `src/go/internal/ui/synth_full_note_test.go`

**Interfaces:**
- Produces: `func synthFullNoteWindowMs(instID string) int` — display window ≈ 1.2×(attack+decay+release) seconds in ms, clamped to [120, 1000]. Non-modular recipes (no visible `amp_attack`) use the `decay` param fallback (default 0.3 s), mirroring `RenderInstrumentPreview`'s own fallback (`preview_render.go:138-141`).
- Produces: `func pcmMinMaxColumns(pcm []float64, cols int) (mins, maxs []float64)` — per-column min/max bins for DAW-clip-style drawing (used by Task 4).
- Consumes: `conceptMergedParams(instID)` (`synth_concept_viz.go:161`), `synthADSRParamsFor(instID)` (`synth_preview_layer.go:607`).

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestSynthFullNoteWindowMs_TracksEnvelope(t *testing.T) {
	const inst = "fullnote-window-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })

	audio.SetInstrumentParam(inst, "amp_attack", 0.01)
	audio.SetInstrumentParam(inst, "amp_decay", 0.10)
	audio.SetInstrumentParam(inst, "amp_release", 0.05)
	short := synthFullNoteWindowMs(inst)

	audio.SetInstrumentParam(inst, "amp_decay", 0.60)
	long := synthFullNoteWindowMs(inst)

	if !(long > short) {
		t.Fatalf("window should grow with decay: short=%d long=%d", short, long)
	}
	if short < 120 || long > 1000 {
		t.Fatalf("window out of clamp range: short=%d long=%d", short, long)
	}
}

func TestSynthFullNoteWindowMs_Clamps(t *testing.T) {
	const inst = "fullnote-window-clamp-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })

	audio.SetInstrumentParam(inst, "amp_attack", 0.001)
	audio.SetInstrumentParam(inst, "amp_decay", 0.001)
	audio.SetInstrumentParam(inst, "amp_release", 0.001)
	if got := synthFullNoteWindowMs(inst); got != 120 {
		t.Fatalf("tiny envelope must clamp to 120ms, got %d", got)
	}
	audio.SetInstrumentParam(inst, "amp_decay", 5.0)
	if got := synthFullNoteWindowMs(inst); got != 1000 {
		t.Fatalf("huge envelope must clamp to 1000ms, got %d", got)
	}
}

func TestSynthFullNoteWindowMs_NonModularFallback(t *testing.T) {
	// Bespoke drum recipe: no visible amp_attack -> decay-based fallback, still in range.
	const inst = "fullnote-window-drum-test"
	audio.BindInstrumentToRecipe(inst, "drum-kick-punchy")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	got := synthFullNoteWindowMs(inst)
	if got < 120 || got > 1000 {
		t.Fatalf("fallback window out of range: %d", got)
	}
}

func TestPcmMinMaxColumns(t *testing.T) {
	pcm := []float64{0, 1, -1, 0.5, -0.5, 0, 0.25, -0.25}
	mins, maxs := pcmMinMaxColumns(pcm, 4)
	if len(mins) != 4 || len(maxs) != 4 {
		t.Fatalf("want 4 columns, got %d/%d", len(mins), len(maxs))
	}
	if maxs[0] != 1 || mins[0] != 0 {
		t.Fatalf("col0 should span [0,1], got [%g,%g]", mins[0], maxs[0])
	}
	if mins[1] != -1 {
		t.Fatalf("col1 min should be -1, got %g", mins[1])
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSynthFullNoteWindowMs|TestPcmMinMaxColumns'`
Expected: FAIL — `undefined: synthFullNoteWindowMs`.

- [ ] **Step 3: Implement**

```go
package ui

// synth_full_note.go — the full-note "Your sound" card: adaptive display
// window + DAW-clip-style min/max column drawing over the real rendered hit
// (audio.RenderInstrumentPreview), which responds to EVERY stage param —
// envelope, LFO, burst, pitch-env — unlike the cycle-normalized "Up close"
// steady wave.

const (
	synthFullNoteWindowMinMs = 120
	synthFullNoteWindowMaxMs = 1000
)

// synthFullNoteWindowMs picks the display window for the full-note card from
// the instrument's effective envelope (≈1.2×(attack+decay+release)), clamped
// so a short kick fills the band and a pad isn't truncated. Non-modular
// recipes fall back to their decay knob, mirroring RenderInstrumentPreview's
// own amp fallback.
func synthFullNoteWindowMs(instID string) int {
	a, d, _, r, ok := synthADSRParamsFor(instID)
	if !ok {
		merged := conceptMergedParams(instID)
		d = merged["decay"]
		if d <= 0 {
			d = 0.3
		}
		a, r = 0.005, 0.08
	}
	ms := int(1.2 * (a + d + r) * 1000)
	if ms < synthFullNoteWindowMinMs {
		ms = synthFullNoteWindowMinMs
	}
	if ms > synthFullNoteWindowMaxMs {
		ms = synthFullNoteWindowMaxMs
	}
	return ms
}

// pcmMinMaxColumns bins pcm into cols columns and returns each column's
// min/max — the classic DAW-clip envelope drawing input.
func pcmMinMaxColumns(pcm []float64, cols int) (mins, maxs []float64) {
	if cols < 1 || len(pcm) == 0 {
		return nil, nil
	}
	mins = make([]float64, cols)
	maxs = make([]float64, cols)
	for c := 0; c < cols; c++ {
		lo := c * len(pcm) / cols
		hi := (c + 1) * len(pcm) / cols
		if hi <= lo {
			hi = lo + 1
		}
		if hi > len(pcm) {
			hi = len(pcm)
		}
		mn, mx := pcm[lo], pcm[lo]
		for _, v := range pcm[lo:hi] {
			if v < mn {
				mn = v
			}
			if v > mx {
				mx = v
			}
		}
		mins[c], maxs[c] = mn, mx
	}
	return mins, maxs
}
```

- [ ] **Step 4: Run tests to verify PASS** (same command as Step 2).

---

### Task 3: `synthMirror` full-note cache slot

**Files:**
- Modify: `src/go/internal/ui/synth_mirror.go`
- Create: `src/go/internal/ui/synth_mirror_full_test.go`

**Interfaces:**
- Produces (methods on `*synthMirror`): `renderLiveFull(inst, hash string, render func(string) []float64)`, `renderNowFull(...same...)`, `snapshotFull() (pcm, ghost []float64)` — exact analogues of `renderLive`/`renderNow`/`snapshot` over new fields `lastHashFull string; pcmFull, ghostFull []float64`. NO pool involvement (the live path is synchronous and hash-gated, like `renderLive`).
- Produces: `func (dv *DrumView) updateSynthFullNoteLive(instID string)` — per-frame driver; render closure = `audio.RenderInstrumentPreview(i, synthFullNoteWindowMs(i))`; hash = existing `dv.synthParamsHash(instID)` (the window derives from the same params, so the hash already covers it).
- Also extend `(dv *DrumView) renderSynthMirrorNow(instID)` to ALSO fill the full slot via `renderNowFull` (deterministic screenshots).

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import "testing"

// Cache-logic-only tests: use &synthMirror{} directly (NO pool — a shared
// "synth.preview" Release would break sibling mirror tests).
func TestSynthMirror_RenderLiveFullHashGated(t *testing.T) {
	m := &synthMirror{}
	calls := 0
	render := func(string) []float64 { calls++; return []float64{float64(calls)} }

	m.renderLiveFull("inst", "h1", render)
	m.renderLiveFull("inst", "h1", render)
	if calls != 1 {
		t.Fatalf("same hash must not re-render: %d calls", calls)
	}
	m.renderLiveFull("inst", "h2", render)
	if calls != 2 {
		t.Fatalf("new hash must re-render: %d calls", calls)
	}
	pcm, ghost := m.snapshotFull()
	if len(pcm) != 1 || pcm[0] != 2 {
		t.Fatalf("live full pcm wrong: %v", pcm)
	}
	if len(ghost) != 1 || ghost[0] != 1 {
		t.Fatalf("prior render must demote to full ghost: %v", ghost)
	}
}

func TestSynthMirror_FullSlotIndependentOfSteadySlot(t *testing.T) {
	m := &synthMirror{}
	m.renderLive("inst", "hs", func(string) []float64 { return []float64{9} })
	m.renderLiveFull("inst", "hf", func(string) []float64 { return []float64{7} })
	steady, _ := m.snapshot()
	full, _ := m.snapshotFull()
	if steady[0] != 9 || full[0] != 7 {
		t.Fatalf("slots must be independent: steady=%v full=%v", steady, full)
	}
}
```

- [ ] **Step 2: Run to verify failure** — `-run 'TestSynthMirror_RenderLiveFullHashGated|TestSynthMirror_FullSlotIndependent'`. Expected: FAIL `undefined`.

- [ ] **Step 3: Implement**

Add to the `synthMirror` struct (after `ghost []float64`):

```go
	// Full-note slot: the whole rendered hit (RenderInstrumentPreview) for the
	// "Your sound" card, cached independently of the steady-wave slot.
	lastHashFull string
	pcmFull      []float64
	ghostFull    []float64
```

Add methods (mirror `renderLive`/`renderNow`/`snapshot` exactly, over the Full fields):

```go
// renderLiveFull is renderLive over the full-note slot: synchronous,
// hash-gated, prior render demoted to ghostFull on a real change.
func (m *synthMirror) renderLiveFull(inst, hash string, render func(inst string) []float64) {
	m.mu.Lock()
	if hash == m.lastHashFull && m.pcmFull != nil {
		m.mu.Unlock()
		return
	}
	m.lastHashFull = hash
	m.ghostFull = m.pcmFull
	m.pcmFull = render(inst)
	m.ready = true
	m.mu.Unlock()
}

// renderNowFull fills the full-note slot synchronously for scene Setup.
func (m *synthMirror) renderNowFull(inst, hash string, render func(inst string) []float64) {
	m.mu.Lock()
	m.lastHashFull = hash
	m.ghostFull = m.pcmFull
	m.pcmFull = render(inst)
	m.ready = true
	m.mu.Unlock()
}

// snapshotFull returns copies of the full-note live + ghost PCM.
func (m *synthMirror) snapshotFull() (pcm, ghost []float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pcmFull != nil {
		pcm = make([]float64, len(m.pcmFull))
		copy(pcm, m.pcmFull)
	}
	if m.ghostFull != nil {
		ghost = make([]float64, len(m.ghostFull))
		copy(ghost, m.ghostFull)
	}
	return pcm, ghost
}
```

Add the DrumView driver (next to `updateSynthMirrorLive`):

```go
// updateSynthFullNoteLive re-renders the full-note "Your sound" card in real
// time — hash-gated like updateSynthMirrorLive, so idle frames cost one
// string compare.
func (dv *DrumView) updateSynthFullNoteLive(instID string) {
	if dv == nil || instID == "" {
		return
	}
	if dv.synthMirror == nil {
		dv.synthMirror = newSynthMirror()
	}
	dv.synthMirror.renderLiveFull(instID, dv.synthParamsHash(instID), func(i string) []float64 {
		return audio.RenderInstrumentPreview(i, synthFullNoteWindowMs(i))
	})
}
```

And in `renderSynthMirrorNow`, after the existing `renderNow` call, add:

```go
	dv.synthMirror.renderNowFull(instID, dv.synthParamsHash(instID), func(i string) []float64 {
		return audio.RenderInstrumentPreview(i, synthFullNoteWindowMs(i))
	})
```

- [ ] **Step 4: Run tests to verify PASS**, plus the existing mirror suite: `-run 'TestSynthMirror'` all green.

---

### Task 4: `drawSynthFullNote` + "Up close" rename (i18n)

**Files:**
- Modify: `src/go/internal/ui/synth_full_note.go` (add draw func)
- Modify: `src/go/internal/ui/synth_preview_layer.go:142` (caption key swap)
- Modify: `src/go/internal/i18n/keys.go`, `catalog_en.go`, `catalog_es.go`
- Test: `src/go/internal/ui/synth_full_note_test.go` (extend)

**Interfaces:**
- Produces: `func drawSynthFullNote(dst *ebiten.Image, rect image.Rectangle, mirror *synthMirror)` — card chrome identical to `drawSynthMirror` (rounded `TokenSurface1` + `TokenBorderSubtle`, caption at caption scale), caption = `i18n.T(i18n.KeyCapYourSound)`; body = min/max column envelope of `mirror.snapshotFull()` with ghost first, centre line last.
- Produces: i18n key `KeyCapUpClose` = `"cap.up_close"`, EN `"Up close"`, ES `"De cerca"`. `drawSynthMirror`'s caption switches from `KeyCapYourSound` to `KeyCapUpClose`.

- [ ] **Step 1: Add i18n keys**

In `keys.go` (next to `KeyCapYourSound`, line ~262): `KeyCapUpClose Key = "cap.up_close"`. Add `KeyCapUpClose` beside `KeyCapYourSound` in the enumerated list at keys.go:579 if that list is a completeness gate (check the surrounding comment; mirror what `KeyCapYourSound` participates in). In `catalog_en.go:251` area: `KeyCapUpClose: "Up close",`. In `catalog_es.go:251` area: `KeyCapUpClose: "De cerca",`.

- [ ] **Step 2: Write the failing test**

```go
// Append to synth_full_note_test.go. fingerprintFocus lives in
// synth_focus_sweep_test.go and hashes an offscreen render.
func TestDrawSynthFullNote_RespondsToAmpDecay(t *testing.T) {
	const inst = "fullnote-draw-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	audio.SetInstrumentParam(inst, "env_enabled", 1)

	rect := image.Rect(0, 0, 260, 90)
	render := func(m *synthMirror) uint64 {
		return fingerprintFocus(t, func(img *ebiten.Image) { drawSynthFullNote(img, rect, m) })
	}

	m := &synthMirror{}
	audio.SetInstrumentParam(inst, "amp_decay", 0.05)
	m.renderNowFull(inst, "h-short", func(i string) []float64 {
		return audio.RenderInstrumentPreview(i, synthFullNoteWindowMs(i))
	})
	short := render(m)

	audio.SetInstrumentParam(inst, "amp_decay", 1.2)
	m.renderNowFull(inst, "h-long", func(i string) []float64 {
		return audio.RenderInstrumentPreview(i, synthFullNoteWindowMs(i))
	})
	long := render(m)

	if short == long {
		t.Fatalf("full-note card identical for 0.05s vs 1.2s decay — envelope not depicted")
	}
}
```

(Adjust `fingerprintFocus`'s exact signature to what `synth_focus_sweep_test.go` defines — it already renders a closure to an offscreen image and hashes pixels; reuse as-is.)

- [ ] **Step 3: Run to verify failure** — `undefined: drawSynthFullNote`.

- [ ] **Step 4: Implement**

```go
// drawSynthFullNote paints the full-note "Your sound" card: the whole rendered
// hit as a DAW-clip min/max envelope, so envelope/LFO/burst/pitch knobs all
// visibly reshape it. Ghost (pre-drag render) drawn faint underneath.
func drawSynthFullNote(dst *ebiten.Image, rect image.Rectangle, mirror *synthMirror) {
	if rect.Dx() < 32 || rect.Dy() < 24 {
		return
	}
	drawRoundedRect(dst, rect, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, rect, TokenBorderSubtle(), RadiusSM, false)

	pad := SpaceSM
	captionScale := FontSizeCaption / FontSizeBody
	captionH := int(float64(TextHeight()) * captionScale)
	DrawTextColorAtScale(dst, i18n.T(i18n.KeyCapYourSound), rect.Min.X+pad, rect.Min.Y+pad/2, TokenTextSecondary(), captionScale)

	traceRect := image.Rect(rect.Min.X+pad, rect.Min.Y+pad+captionH, rect.Max.X-pad, rect.Max.Y-pad)
	if traceRect.Dx() < 8 || traceRect.Dy() < 6 {
		return
	}
	drawRect(dst, traceRect, WithAlpha(genColorVizScopeBg, AlphaOverlay), true)
	if mirror == nil {
		return
	}
	pcm, ghost := mirror.snapshotFull()
	midY := traceRect.Min.Y + traceRect.Dy()/2
	half := float64(traceRect.Dy()-2) / 2
	drawCols := func(buf []float64, gain float64, col, fill interface {
		RGBA() (r, g, b, a uint32)
	}) {
		mins, maxs := pcmMinMaxColumns(buf, traceRect.Dx())
		for c := range mins {
			yHi := midY - int(clampUnit(maxs[c]*gain)*half)
			yLo := midY - int(clampUnit(mins[c]*gain)*half)
			if yLo <= yHi {
				yLo = yHi + 1
			}
			x := traceRect.Min.X + c
			if fill != nil {
				drawRect(dst, image.Rect(x, yHi, x+1, yLo), fill, true)
			}
			if col != nil {
				drawRect(dst, image.Rect(x, yHi, x+1, yHi+1), col, true)
				drawRect(dst, image.Rect(x, yLo-1, x+1, yLo), col, true)
			}
		}
	}
	if len(ghost) > 1 {
		drawCols(ghost, mirrorTraceGain(ghost), WithAlpha(genColorBorder, AlphaSubtle), nil)
	}
	if len(pcm) > 1 {
		drawCols(pcm, mirrorTraceGain(pcm), colWaveTrace, colSynthOscFill)
	}
	drawRect(dst, image.Rect(traceRect.Min.X, midY, traceRect.Max.X, midY+1), WithAlpha(genColorBorder, AlphaSubtle), true)
}

// clampUnit clamps v to [-1, 1].
func clampUnit(v float64) float64 {
	if v > 1 {
		return 1
	}
	if v < -1 {
		return -1
	}
	return v
}
```

Then in `synth_preview_layer.go:142` change the steady mirror's caption:

```go
	DrawTextColorAtScale(dst, i18n.T(i18n.KeyCapUpClose), rect.Min.X+pad, rect.Min.Y+pad/2, TokenTextSecondary(), captionScale)
```

- [ ] **Step 5: Run tests** — new test PASS; also `-run 'TestSynthMirror|TestI18n'` green (i18n completeness tests, if present, now see both catalogs updated).

---

### Task 5: Three-card split + `drawSynthTab` wiring + density token + mobile collapse

**Files:**
- Modify: `DESIGN.md` (densities blocks ~592/~681/~770), run `make gen-design-tokens`
- Modify: `src/go/internal/ui/synth_focus_graph.go:26-41`
- Modify: `src/go/internal/ui/synth_panel_zone.go:1812-1836`
- Test: `src/go/internal/ui/synth_focus_graph_split_test.go` (extend), `src/go/internal/ui/synth_mobile_focus_test.go` (adjust)

**Interfaces:**
- Produces: density token `SynthRightCardMinH` (`densities.synthRightCardMinH`): compact 34, comfortable 40, spacious 44.
- Produces: `func splitSynthRightPane3(r image.Rectangle) (full, up, focus image.Rectangle)` — shares ≈30/26/44 top-to-bottom with 1px gaps. Collapse order: if `r.Dy() < 3*minBand+2` the "Up close" band collapses (returns empty `up`) and the pane falls back to `splitSynthRightPane(r)` semantics as (full, focus); `splitSynthRightPane` itself already collapses the top band below `2*minBand`, leaving focus-only. minBand = `Profile().DensityValues().SynthRightCardMinH`.
- Consumes: `drawSynthFullNote` (Task 4), `updateSynthFullNoteLive` (Task 3).

- [ ] **Step 1: Add the density token**

In `DESIGN.md`, inside each `densities:` block (compact ~line 592 near `synthFocusGraphH: 96`, comfortable ~681, spacious ~770) add:

```yaml
    synthRightCardMinH:    34   # compact
    synthRightCardMinH:    40   # comfortable
    synthRightCardMinH:    44   # spacious
```

Check `scripts/gen_design_tokens/schema.json` — if densities keys are enumerated there, add `synthRightCardMinH`. Run `make gen-design-tokens` from the repo root; confirm `design_density.gen.go` gains `SynthRightCardMinH`. Run the drift guards: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot' ./internal/ui/`.

- [ ] **Step 2: Write the failing tests**

Append to `synth_focus_graph_split_test.go`:

```go
func TestSplitSynthRightPane3_SharesAndOrder(t *testing.T) {
	r := image.Rect(0, 0, 280, 400)
	full, up, focus := splitSynthRightPane3(r)
	if full.Empty() || up.Empty() || focus.Empty() {
		t.Fatalf("tall pane must host all three cards: %v %v %v", full, up, focus)
	}
	if !(full.Max.Y < up.Min.Y && up.Max.Y < focus.Min.Y) {
		t.Fatalf("cards must stack full/up/focus: %v %v %v", full, up, focus)
	}
	if !(focus.Dy() > full.Dy() && full.Dy() > up.Dy()) {
		t.Fatalf("share order must be focus > full > up: %d %d %d", full.Dy(), up.Dy(), focus.Dy())
	}
	if focus.Max.Y != r.Max.Y || full.Min.Y != r.Min.Y {
		t.Fatalf("cards must tile the pane")
	}
}

func TestSplitSynthRightPane3_UpCloseCollapsesFirst(t *testing.T) {
	minBand := Profile().DensityValues().SynthRightCardMinH
	r := image.Rect(0, 0, 280, 3*minBand-1)
	full, up, focus := splitSynthRightPane3(r)
	if !up.Empty() {
		t.Fatalf("short pane must collapse Up-close first, got %v", up)
	}
	if full.Empty() || focus.Empty() {
		t.Fatalf("two-card fallback must keep full-note + focus: %v %v", full, focus)
	}
}
```

- [ ] **Step 3: Run to verify failure** — `undefined: splitSynthRightPane3`.

- [ ] **Step 4: Implement the split** (in `synth_focus_graph.go`, below `splitSynthRightPane`):

```go
// splitSynthRightPane3 carves the right pane into three stacked cards:
// full-note "Your sound" (~30%), "Up close" steady wave (~26%), and the focus
// graph (~44%, the star). Collapse order when short: "Up close" first (most
// redundant with the focus graph), then the two-card split's own rules.
func splitSynthRightPane3(r image.Rectangle) (full, up, focus image.Rectangle) {
	minBand := Profile().DensityValues().SynthRightCardMinH
	if r.Dy() < 3*minBand+2 {
		f, foc := splitSynthRightPane(r)
		return f, image.Rectangle{}, foc
	}
	fullH := r.Dy() * 30 / 100
	upH := r.Dy() * 26 / 100
	if fullH < minBand {
		fullH = minBand
	}
	if upH < minBand {
		upH = minBand
	}
	y1 := r.Min.Y + fullH
	y2 := y1 + 1 + upH
	full = image.Rect(r.Min.X, r.Min.Y, r.Max.X, y1)
	up = image.Rect(r.Min.X, y1+1, r.Max.X, y2)
	focus = image.Rect(r.Min.X, y2+1, r.Max.X, r.Max.Y)
	return full, up, focus
}
```

Also update `splitSynthRightPane`'s `const minBand = 40` to `minBand := Profile().DensityValues().SynthRightCardMinH` so both splits share the token (check `synth_focus_graph_split_test.go`'s existing 42/58 assertions still hold — they assert proportions, not the constant).

- [ ] **Step 5: Rewire `drawSynthTab`** (`synth_panel_zone.go:1812-1836`) — BOTH the desktop and mobile blocks become:

```go
	if r := dv.instEditorPreviewRect; !r.Empty() {
		dv.updateSynthMirrorLive(resolved)
		dv.updateSynthFullNoteLive(resolved)
		fullR, upR, focusR := splitSynthRightPane3(r)
		if !fullR.Empty() {
			drawSynthFullNote(dst, fullR, dv.synthMirror)
		}
		if !upR.Empty() {
			drawSynthMirror(dst, upR, dv.synthMirror)
		}
		dv.drawSynthFocusGraph(dst, focusR, resolved)
	}
	if mb := dv.instEditorMobileFocusRect; !mb.Empty() {
		dv.updateSynthMirrorLive(resolved)
		dv.updateSynthFullNoteLive(resolved)
		fullR, upR, focusR := splitSynthRightPane3(mb)
		if !fullR.Empty() {
			drawSynthFullNote(dst, fullR, dv.synthMirror)
		}
		if !upR.Empty() {
			drawSynthMirror(dst, upR, dv.synthMirror)
		}
		dv.drawSynthFocusGraph(dst, focusR, resolved)
	}
```

- [ ] **Step 6: Run the affected suites and adjust expectations**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSplitSynthRightPane|TestMobileSynthTab|TestSynthFocus'`
Expected: new tests PASS. If `TestMobileSynthTab_WavesRightOfKnobs` / `_FocusGraphRespondsToKnobSelect` assert the OLD two-band split geometry, update their geometry expectations to the three-card split (keep their intent: waves right of knobs, focus responds to selection). Do not weaken any assertion about knob-column usable height.

---

### Task 6: Focus routing split — `conceptLFO` + `conceptPitchEnvSweep`

**Files:**
- Modify: `src/go/internal/ui/synth_concept_viz.go` (new renderers, near `conceptMotion` at line 686)
- Modify: `src/go/internal/ui/synth_focus_graph.go:49-73` (routing table)
- Modify: `src/go/internal/ui/synth_focus_sweep_test.go:62-65` (remove `lfo_delay` exemption)
- Create: `src/go/internal/ui/synth_lfo_burst_viz_test.go`

**Interfaces:**
- Produces: `func lfoCurveFromParams(merged map[string]float64, n int) []float64` — wobble over a 1.0 s window: `depth·amp(t)·sin(2π·rate·t)` where `amp(t)` is 0 before `lfo_delay` seconds then ramps to 1 over 0.25 s (models the vibrato onset so `lfo_delay` becomes showable).
- Produces: renderers `conceptLFO` and `conceptPitchEnvSweep`, both `conceptRenderer`-shaped.
- Routing table becomes: `"pitch"` → `conceptPitchWave`, `"pitchenv"` → `conceptPitchEnvSweep`, `"lfo"` → `conceptLFO`, `"burst"` → `conceptBurst` (Task 7 — until then leave `"burst": conceptMotion` and route it in Task 7). `conceptMotion` stays in source (existing tests call it directly).

- [ ] **Step 1: Write the failing tests**

```go
//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func fpConcept(t *testing.T, r conceptRenderer, inst string, def audio.ParamDef) uint64 {
	t.Helper()
	rect := image.Rect(0, 0, 260, 110)
	return fingerprintFocus(t, func(img *ebiten.Image) { r(img, rect, inst, def, nil) })
}

func lfoDefFor(t *testing.T, inst, name string) audio.ParamDef {
	t.Helper()
	reg := audio.RecipeRegistrations()[audio.RecipeForInstrument(inst)]
	for _, d := range reg.Params {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("param %s not found", name)
	return audio.ParamDef{}
}

func TestConceptLFO_RespondsToRateDepthDelay(t *testing.T) {
	const inst = "lfo-viz-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	for _, name := range []string{"lfo_rate", "lfo_depth", "lfo_delay"} {
		def := lfoDefFor(t, inst, name)
		audio.ResetInstrumentParams(inst)
		audio.SetInstrumentParam(inst, "lfo_depth", 0.6) // wobble visible for rate/delay sweeps
		audio.SetInstrumentParam(inst, name, def.Min)
		lo := fpConcept(t, conceptLFO, inst, def)
		audio.SetInstrumentParam(inst, name, def.Max)
		hi := fpConcept(t, conceptLFO, inst, def)
		if lo == hi {
			t.Errorf("conceptLFO unresponsive to %s", name)
		}
	}
}

func TestConceptPitchEnvSweep_Responds(t *testing.T) {
	const inst = "penv-viz-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	for _, name := range []string{"pitchenv_amt", "pitchenv_decay"} {
		def := lfoDefFor(t, inst, name)
		audio.ResetInstrumentParams(inst)
		if name == "pitchenv_decay" {
			audio.SetInstrumentParam(inst, "pitchenv_amt", 12) // sweep visible for decay sweep
		}
		audio.SetInstrumentParam(inst, name, def.Min)
		lo := fpConcept(t, conceptPitchEnvSweep, inst, def)
		audio.SetInstrumentParam(inst, name, def.Max)
		hi := fpConcept(t, conceptPitchEnvSweep, inst, def)
		if lo == hi {
			t.Errorf("conceptPitchEnvSweep unresponsive to %s", name)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure** — `undefined: conceptLFO`.

- [ ] **Step 3: Implement** (in `synth_concept_viz.go`, after `conceptMotion`):

```go
// ── conceptLFO — the LFO's own story: wobble with onset ──────────────────────

// lfoCurveFromParams samples the LFO wobble over a 1.0s window, INCLUDING the
// vibrato onset: silent until lfo_delay seconds, then a 0.25s ramp to full
// depth. Pure math, live during drag.
func lfoCurveFromParams(merged map[string]float64, n int) []float64 {
	if n < 2 {
		n = 2
	}
	rate := merged["lfo_rate"]
	depth := merged["lfo_depth"]
	delay := merged["lfo_delay"]
	const windowSec = 1.0
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		ts := float64(i) / float64(n-1) * windowSec
		amp := 1.0
		if delay > 0 {
			switch {
			case ts < delay:
				amp = 0
			case ts < delay+0.25:
				amp = (ts - delay) / 0.25
			}
		}
		out[i] = depth * amp * math.Sin(2*math.Pi*rate*ts)
	}
	return out
}

// lfoParamsDiffer reports whether the ghost snapshot's LFO inputs differ.
func lfoParamsDiffer(ghost, live map[string]float64) bool {
	if ghost == nil {
		return false
	}
	for _, k := range []string{"lfo_rate", "lfo_depth", "lfo_delay"} {
		if ghost[k] != live[k] {
			return true
		}
	}
	return false
}

// conceptLFO draws the pure LFO wobble (rate/depth/onset-delay) — no pitch-env
// or burst mixed in, so the picture tells only the LFO's story.
func conceptLFO(dst *ebiten.Image, rect image.Rectangle, instID string, _ audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)
	n := rect.Dx()
	merged := conceptMergedParams(instID)
	midYf := float64(rect.Min.Y+rect.Max.Y) / 2
	half := float64(rect.Dy()-2) / 2
	yAt := func(v float64) int {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		return int(math.Round(midYf - v*half))
	}
	mid := int(math.Round(midYf))
	if lfoParamsDiffer(ghost, merged) {
		conceptDrawFilledCurve(dst, rect, lfoCurveFromParams(ghost, n), yAt, mid, nil, colConceptGhost)
	}
	conceptDrawFilledCurve(dst, rect, lfoCurveFromParams(merged, n), yAt, mid,
		WithAlpha(colAccent, AlphaSubtle), WithAlpha(colAccent, AlphaStrong))
	drawConceptTimeRuler(dst, rect, "1s") // Task 11 adds this helper; until then omit this line
}

// conceptPitchEnvSweep draws the pitch envelope's own story: a bipolar
// pitch-vs-time sweep (reuses the FM pitch-env sample math over the modular
// pitchenv_amt / pitchenv_decay params).
func conceptPitchEnvSweep(dst *ebiten.Image, rect image.Rectangle, instID string, _ audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)
	merged := conceptMergedParams(instID)
	n := rect.Dx()
	const windowSec, maxAmt = 0.4, 24.0
	midYf := float64(rect.Min.Y+rect.Max.Y) / 2
	half := float64(rect.Dy()-2) / 2
	yAt := func(v float64) int {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		return int(math.Round(midYf - v*half))
	}
	mid := int(math.Round(midYf))
	amt, dec := merged["pitchenv_amt"], merged["pitchenv_decay"]
	if ghost != nil && (ghost["pitchenv_amt"] != amt || ghost["pitchenv_decay"] != dec) {
		conceptDrawFilledCurve(dst, rect, fmPitchEnvSamples(ghost["pitchenv_amt"], ghost["pitchenv_decay"], maxAmt, windowSec, n), yAt, mid, nil, colConceptGhost)
	}
	conceptDrawFilledCurve(dst, rect, fmPitchEnvSamples(amt, dec, maxAmt, windowSec, n), yAt, mid,
		WithAlpha(colAccent, AlphaSubtle), WithAlpha(colAccent, AlphaStrong))
}
```

Update the routing table in `synth_focus_graph.go`:

```go
	"pitch":    conceptPitchWave,
	"pitchenv": conceptPitchEnvSweep,
	"lfo":      conceptLFO,
	"burst":    conceptMotion, // Task 7 replaces with conceptBurst
```

Remove the `lfo_delay` entry from `focusSweepExemptByName` (`synth_focus_sweep_test.go:65`) and update the comment block above it. `lfo_target` STAYS exempt (enum routing choice — no wobble-shape change).

- [ ] **Step 4: Run tests** — new tests PASS + `-run 'TestFocusGraph_RespondsToEveryKnob|TestSynthFocusRenderer_EveryGroupExplicit|TestConceptMotion|TestSynthMotion'` all green.

---

### Task 7: `conceptBurst` — hit-spike timeline

**Files:**
- Modify: `src/go/internal/ui/synth_concept_viz.go` (new renderer after `drawBurstMarkers`)
- Modify: `src/go/internal/ui/synth_focus_graph.go` (route `"burst"` → `conceptBurst`)
- Modify: `src/go/internal/ui/synth_focus_sweep_test.go:57-60` (remove `burst4_off` exemption)
- Test: `src/go/internal/ui/synth_lfo_burst_viz_test.go` (extend)

**Interfaces:**
- Produces: `conceptBurst` (conceptRenderer). Draws, over a 0..0.25 s timeline (0.25 = `burst*_off` Max, same as `drawBurstMarkers`): one vertical spike per hit 1–4 at x=offset, height=amp (with sharpness boost, same mapping as `drawBurstMarkers`), width from `burst_sharp`. **Silent hits (amp ≤ 0) draw a DIM PLACEHOLDER spike at 15% height** so `burst4_off` still positions something visible → responsive. The hit the SELECTED knob belongs to draws in `WithAlpha(colAccent, AlphaStrong)`; other hits `WithAlpha(colAccent, AlphaMedium)`; placeholders `colConceptGhost`. `burst_sharp` selected → all spikes accent-strong (it shapes every hit). Ghost: when the ghost map differs for the selected hit's off/amp, a `colConceptGhost` spike marks the pre-drag position.
- Produces: `func burstHitIndexForKnob(name string) int` — `"burst2_off"`/`"burst2_amp"` → 2; `"burst_sharp"`/anything else → 0 (meaning "all").

- [ ] **Step 1: Write the failing tests** (append to `synth_lfo_burst_viz_test.go`):

```go
func TestConceptBurst_EveryBurstKnobResponds(t *testing.T) {
	const inst = "burst-viz-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	names := []string{
		"burst1_off", "burst1_amp", "burst2_off", "burst2_amp",
		"burst3_off", "burst3_amp", "burst4_off", "burst4_amp", "burst_sharp",
	}
	for _, name := range names {
		def := lfoDefFor(t, inst, name)
		audio.ResetInstrumentParams(inst)
		audio.SetInstrumentParam(inst, name, def.Min)
		lo := fpConcept(t, conceptBurst, inst, def)
		audio.SetInstrumentParam(inst, name, def.Max)
		hi := fpConcept(t, conceptBurst, inst, def)
		if lo == hi {
			t.Errorf("conceptBurst unresponsive to %s (burst4_off must respond via the dim placeholder spike)", name)
		}
	}
}

func TestConceptBurst_SelectedHitHighlighted(t *testing.T) {
	const inst = "burst-hl-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	d1 := lfoDefFor(t, inst, "burst1_off")
	d2 := lfoDefFor(t, inst, "burst2_off")
	// Same params, different selected knob ⇒ different highlight ⇒ different pixels.
	if fpConcept(t, conceptBurst, inst, d1) == fpConcept(t, conceptBurst, inst, d2) {
		t.Fatalf("selected-hit highlight not visible: burst1 vs burst2 selection paint identically")
	}
}

func TestBurstHitIndexForKnob(t *testing.T) {
	for name, want := range map[string]int{
		"burst1_off": 1, "burst3_amp": 3, "burst_sharp": 0, "lfo_rate": 0,
	} {
		if got := burstHitIndexForKnob(name); got != want {
			t.Errorf("burstHitIndexForKnob(%s)=%d want %d", name, got, want)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure** — `undefined: conceptBurst`.

- [ ] **Step 3: Implement** (reuse the geometry mapping from `drawBurstMarkers` at `synth_concept_viz.go:740-804`):

```go
// ── conceptBurst — the burst stage's own story: a timeline of hit spikes ────

// burstHitIndexForKnob maps a burstN_* knob to its hit number (1-4); 0 means
// "no single hit" (burst_sharp shapes all hits; non-burst names).
func burstHitIndexForKnob(name string) int {
	if strings.HasPrefix(name, "burst") && len(name) > 5 && name[5] >= '1' && name[5] <= '4' {
		return int(name[5] - '0')
	}
	return 0
}

// conceptBurst draws one spike per burst hit on a 0..0.25s timeline:
// x = when (burstN_off), height = how loud (burstN_amp, sharpness-boosted),
// width = how clicky (burst_sharp). SILENT hits (amp<=0) draw a dim
// placeholder spike at 15% height so even a muted hit's timing knob shows
// where the hit would land. The selected knob's hit is highlighted.
func conceptBurst(dst *ebiten.Image, rect image.Rectangle, instID string, def audio.ParamDef, ghost map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)
	merged := conceptMergedParams(instID)
	selected := burstHitIndexForKnob(def.Name)

	const maxOff = 0.25
	sharp := merged["burst_sharp"]
	if sharp <= 0 {
		sharp = 40
	}
	sf := (sharp - 1.0) / (200.0 - 1.0)
	if sf < 0 {
		sf = 0
	} else if sf > 1 {
		sf = 1
	}
	tickW := 3 - int(math.Round(sf*2))
	if tickW < 1 {
		tickW = 1
	}
	usableH := rect.Dy() - 2
	if usableH < 1 {
		usableH = 1
	}
	baseY := rect.Max.Y - 1

	xFor := func(off float64) int {
		frac := off / maxOff
		if frac < 0 {
			frac = 0
		} else if frac > 1 {
			frac = 1
		}
		return rect.Min.X + int(math.Round(frac*float64(rect.Dx()-1-tickW)))
	}
	spike := func(x, h int, col interface {
		RGBA() (r, g, b, a uint32)
	}) {
		if h < 1 {
			h = 1
		}
		drawRect(dst, image.Rect(x, baseY-h, x+tickW, baseY), col, true)
	}

	for hit := 1; hit <= 4; hit++ {
		offName := fmt.Sprintf("burst%d_off", hit)
		ampName := fmt.Sprintf("burst%d_amp", hit)
		amp := merged[ampName]
		x := xFor(merged[offName])

		// Ghost spike for the selected hit when the pre-drag off/amp differ.
		if ghost != nil && (selected == hit || selected == 0) {
			gOff, gAmp := ghost[offName], ghost[ampName]
			if gOff != merged[offName] || gAmp != amp {
				gh := int(math.Round(math.Max(gAmp, 0.15) * (0.6 + 0.4*sf) * float64(usableH)))
				spike(xFor(gOff), gh, colConceptGhost)
			}
		}

		if amp <= 0 {
			// Dim placeholder: the hit is silent, but its TIMING still shows.
			spike(x, int(math.Round(0.15*float64(usableH))), colConceptGhost)
			continue
		}
		if amp > 1 {
			amp = 1
		}
		hFrac := amp * (0.6 + 0.4*sf)
		if hFrac > 1 {
			hFrac = 1
		}
		col := WithAlpha(colAccent, AlphaMedium)
		if selected == hit || selected == 0 {
			col = WithAlpha(colAccent, AlphaStrong)
		}
		spike(x, int(math.Round(hFrac*float64(usableH))), col)
	}
	drawRect(dst, image.Rect(rect.Min.X, baseY, rect.Max.X, baseY+1), WithAlpha(genColorBorder, AlphaSubtle), true)
}
```

(Add `"fmt"` and `"strings"` to the file's imports if not already present — `strings` is, for `isFMDecayKnob`.)

Route it: in `synth_focus_graph.go` change `"burst": conceptMotion` → `"burst": conceptBurst`. Remove the `burst4_off` entry from `focusSweepExemptByName` and update its comment block.

- [ ] **Step 4: Run tests** — new tests + `-run 'TestFocusGraph_RespondsToEveryKnob'` PASS (sweep log should show 2 fewer exemptions than before Task 6/7: `lfo_delay`, `burst4_off` gone).

---

### Task 8: `conceptPostWave` — before/after wave for POST knobs

**Files:**
- Modify: `src/go/internal/ui/synth_concept_viz.go` (new renderer after `conceptPost`)
- Modify: `src/go/internal/ui/synth_focus_graph.go` (route `"post"` → `conceptPostWave`)
- Test: `src/go/internal/ui/synth_lfo_burst_viz_test.go` (extend)

**Interfaces:**
- Produces: `conceptPostWave` (conceptRenderer) — main trace = steady output wave with the current params (post stage forced on), ghost = the same wave with the SELECTED knob pinned to its neutral reference (`postNeutralRef`). Uses `audio.RenderInstrumentPreviewWave(instID, override, 3, 256)`.
- Produces: `func postNeutralRef(def audio.ParamDef) float64` — `"gain"` → 1, `"tune"` → 0, default → `def.Min` (drive → 0).
- `conceptPost` (the transfer-function diagonal) stays in source for its direct tests but leaves the live routing.

- [ ] **Step 1: Write the failing tests**:

```go
func TestConceptPostWave_DriveMorphsWave(t *testing.T) {
	const inst = "post-viz-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	def := lfoDefFor(t, inst, "drive")
	audio.SetInstrumentParam(inst, def.Name, def.Min)
	lo := fpConcept(t, conceptPostWave, inst, def)
	audio.SetInstrumentParam(inst, def.Name, def.Max)
	hi := fpConcept(t, conceptPostWave, inst, def)
	if lo == hi {
		t.Fatalf("conceptPostWave: max drive paints the same wave as no drive")
	}
}

func TestPostNeutralRef(t *testing.T) {
	if postNeutralRef(audio.ParamDef{Name: "gain", Min: 0}) != 1 {
		t.Fatal("gain neutral must be unity (Min=0 would be silence, hiding the knob's effect)")
	}
	if postNeutralRef(audio.ParamDef{Name: "drive", Min: 0}) != 0 {
		t.Fatal("drive neutral must be 0 (no drive)")
	}
}
```

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement**:

```go
// ── conceptPostWave — before/after OUTPUT WAVE for the POST stage ────────────
//
// Replaces the abstract tanh transfer-function diagonal in the focus graph: a
// kid sees the wave itself get squashed/edgier, with the faint ghost showing
// the wave when THIS knob sits at its neutral value.

// postNeutralRef is the "knob doing nothing" reference value for a POST knob.
func postNeutralRef(def audio.ParamDef) float64 {
	switch def.Name {
	case "gain":
		return 1
	case "tune":
		return 0
	}
	return def.Min
}

func conceptPostWave(dst *ebiten.Image, rect image.Rectangle, instID string, def audio.ParamDef, _ map[string]float64) {
	if rect.Dx() < 8 || rect.Dy() < 6 {
		return
	}
	conceptBgFill(dst, rect)
	const cycles, pts = 3, 256

	midY := (rect.Min.Y + rect.Max.Y) / 2
	half := float64(rect.Dy()-2) / 2
	yAt := func(v float64) int {
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		return int(math.Round(float64(midY) - v*half))
	}

	solid := map[string]float64{}
	neutral := map[string]float64{def.Name: postNeutralRef(def)}
	if en, ok := conceptStageEnableParam("post"); ok {
		solid[en] = 1
		neutral[en] = 1
	}
	cw := audio.RenderInstrumentPreviewWave(instID, solid, cycles, pts)
	gw := audio.RenderInstrumentPreviewWave(instID, neutral, cycles, pts)
	zoomLen := len(autoZoomWindow(cw, cycles))
	if zoomLen > 0 && zoomLen <= len(cw) {
		cw = cw[:zoomLen]
	}
	if zoomLen > 0 && zoomLen <= len(gw) {
		gw = gw[:zoomLen]
	}
	if len(gw) > 0 && !sameWave(gw, cw) {
		conceptDrawFilledCurve(dst, rect, gw, yAt, midY, nil, colConceptGhost)
	}
	conceptDrawFilledCurve(dst, rect, cw, yAt, midY, colConceptFill, colConceptStroke)
}
```

Route it: `"post": conceptPostWave` in `synthFocusRenderers`.

- [ ] **Step 4: Run tests** — new tests + full sweep + `-run 'TestConceptPost'` (legacy direct tests still pass).

---

### Task 9: Smooth interpolated curves + resolution bump (kill the stair-steps)

**Files:**
- Modify: `src/go/internal/ui/synth_concept_viz.go:194-232` (`conceptDrawFilledCurve`), `conceptFM`'s and `conceptFocusLevelWave`'s sampling consts
- Test: `src/go/internal/ui/synth_concept_viz_test.go` (extend)

**Interfaces:**
- `conceptDrawFilledCurve` keeps its exact signature but (a) linearly interpolates between neighboring samples per column instead of nearest-sample stepping, and (b) bridges consecutive columns' strokes with a vertical span so steep curves have no gaps. Every existing caller benefits with no call-site change.
- Sampling consts: `conceptFM` (`synth_concept_viz.go:497`) and `conceptFocusLevelWave` (`synth_focus_graph.go:96`) change `const cycles, pts = 3, 64` → `3, 256`.

- [ ] **Step 1: Write the failing test**

```go
// A steep two-point ramp across a wide rect must paint a CONNECTED stroke:
// with nearest-sample stepping + unbridged 2px strokes, a tall rect leaves
// vertical gaps between columns. Count stroke-colored pixels per column and
// assert adjacent columns' stroke spans overlap or touch.
func TestConceptDrawFilledCurve_BridgesSteepColumns(t *testing.T) {
	rect := image.Rect(0, 0, 40, 120)
	img := ebiten.NewImage(rect.Dx(), rect.Dy())
	samples := []float64{-1, 1} // one maximally steep segment
	yAt := func(v float64) int { return 60 - int(v*58) }
	conceptDrawFilledCurve(img, rect, samples, yAt, 60, nil, colConceptStroke)

	spanFor := func(x int) (top, bot int, ok bool) {
		top, bot = -1, -1
		for y := 0; y < rect.Dy(); y++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a > 0 {
				if top < 0 {
					top = y
				}
				bot = y
			}
		}
		return top, bot, top >= 0
	}
	prevTop, prevBot, ok := spanFor(0)
	if !ok {
		t.Fatal("no stroke in column 0")
	}
	for x := 1; x < rect.Dx(); x++ {
		top, bot, ok := spanFor(x)
		if !ok {
			t.Fatalf("no stroke in column %d", x)
		}
		if top > prevBot+1 || bot < prevTop-1 {
			t.Fatalf("stroke gap between columns %d and %d: [%d,%d] vs [%d,%d]", x-1, x, prevTop, prevBot, top, bot)
		}
		prevTop, prevBot = top, bot
	}
}
```

(This runs under the ebitenstub backend where `img.At` reads work — the established pixel-testing pattern; see memory `reference_key_learnings_and_completed_plans`.)

- [ ] **Step 2: Run to verify failure** — gaps found.

- [ ] **Step 3: Implement** — replace the loop body of `conceptDrawFilledCurve`:

```go
	n := len(samples)
	if n < 2 {
		return
	}
	w := rect.Dx()
	if w < 2 {
		return
	}
	valueAt := func(px int) float64 {
		f := float64(px) * float64(n-1) / float64(w-1)
		i0 := int(f)
		if i0 >= n-1 {
			return samples[n-1]
		}
		frac := f - float64(i0)
		return samples[i0] + frac*(samples[i0+1]-samples[i0])
	}
	prevY := yAt(valueAt(0))
	for px := 0; px < w; px++ {
		x := rect.Min.X + px
		y := yAt(valueAt(px))
		if fillCol != nil {
			yTop, yBot := y, fillBaseline
			if yTop > yBot {
				yTop, yBot = yBot, yTop
			}
			if yBot > yTop {
				drawRect(dst, image.Rect(x, yTop, x+1, yBot), fillCol, true)
			}
		}
		if strokeCol != nil {
			// Bridge to the previous column so steep curves stay connected.
			y0, y1 := y, prevY
			if y0 > y1 {
				y0, y1 = y1, y0
			}
			y1 += conceptStrokeThickness
			if y0 < rect.Min.Y {
				y0 = rect.Min.Y
			}
			if y1 > rect.Max.Y {
				y1 = rect.Max.Y
			}
			if y1 > y0 {
				drawRect(dst, image.Rect(x, y0, x+1, y1), strokeCol, true)
			}
		}
		prevY = y
	}
```

Then bump the two `const cycles, pts = 3, 64` declarations (`conceptFM`, `conceptFocusLevelWave`) to `3, 256`.

- [ ] **Step 4: Run the full renderer suites**

Run: `-run 'TestConcept|TestSynthFocus|TestFocusGraph_RespondsToEveryKnob|TestSynthMirror'` — all PASS. If any existing test pinned exact pixel fingerprints of the old stepping, update the pinned values (intent unchanged: responsiveness, not exact pixels). Run the alloc-budget guard: `-run 'TestEQPanelDrawAlloc'` — if the synth tab's budget line fails, update `perTabAllocBudget` with a one-line rationale ("focus-curve interpolation bridges strokes: +1 rect/column worst-case").

---

### Task 10: Bypassed-stage honesty (dim + "OFF" hint)

**Files:**
- Modify: `src/go/internal/ui/synth_concept_viz.go:68-80` (`conceptStageEnableParam` — add missing groups)
- Modify: `src/go/internal/ui/synth_focus_graph.go:153-189` (`drawSynthFocusGraph`)
- Modify: `src/go/internal/i18n/keys.go`, `catalog_en.go`, `catalog_es.go`
- Test: `src/go/internal/ui/synth_focus_graph_test.go` (extend)

**Interfaces:**
- `conceptStageEnableParam` gains: `"env"` → `"env_enabled"`, `"lfo"` → `"lfo_enabled"`, `"burst"` → `"burst_enabled"`, `"pitchenv"` → `"pitchenv_enabled"`.
- i18n: `KeyCapStageOff Key = "cap.stage_off"`, EN `"OFF — turn the stage on to hear this"`, ES `"APAGADO — enciende la etapa para oírlo"` (em-dash is not a forbidden glyph).
- `drawSynthFocusGraph`: after the renderer runs, if the selected knob's stage has an enable param AND `!synthStageEnabled(instID, en)`, paint `WithAlpha(TokenSurface1(), AlphaStrong)` over `graphR` (same scrim as `dimPlotIfBypassed`) and the hint text bottom-left.

- [ ] **Step 1: Write the failing test**

```go
func TestFocusGraph_BypassedStageDimsAndHints(t *testing.T) {
	g := newSynthTabGameForTest(t) // reuse the established synth-tab game helper in this file/suite
	dv := g.drum
	inst := dv.resolveSynthInstrument("")
	// Select the LFO section + its rate knob, stage OFF (lfo_enabled defaults 0).
	dv.selectSynthSection(synthSectionLFO)
	audio.SetInstrumentParam(inst, "lfo_enabled", 0)
	rect := image.Rect(0, 0, 300, 160)
	off := fingerprintFocus(t, func(img *ebiten.Image) { dv.drawSynthFocusGraph(img, rect, inst) })
	audio.SetInstrumentParam(inst, "lfo_enabled", 1)
	on := fingerprintFocus(t, func(img *ebiten.Image) { dv.drawSynthFocusGraph(img, rect, inst) })
	if off == on {
		t.Fatalf("bypassed LFO stage must dim the focus graph + show the OFF hint")
	}
}
```

(Adapt the game/section-selection helper names to what `synth_focus_graph_test.go` already uses — `synth_focus_scene_test.go` selects sections for the crop scenes; reuse that mechanism. The assertion is fingerprint-level, not text-pixel-level.)

- [ ] **Step 2: Run to verify failure** (with the stage-enable additions absent, LFO has no scrim → fingerprints equal apart from renderer output; ensure the test fails for the right reason: force `lfo_depth` to a fixed value in both renders so the curve itself is identical and only the scrim can differ).

- [ ] **Step 3: Implement** — extend `conceptStageEnableParam`:

```go
	case "env":
		return "env_enabled", true
	case "lfo":
		return "lfo_enabled", true
	case "burst":
		return "burst_enabled", true
	case "pitchenv":
		return "pitchenv_enabled", true
```

Add the i18n key (all three files). In `drawSynthFocusGraph`, after the renderer dispatch line:

```go
	synthFocusRendererForKnob(def)(dst, graphR, instID, def, dv.synthConceptGhost(kIdx))
	// Honesty scrim: the picture previews what the knob WOULD do, but when the
	// stage is off the sound doesn't change — dim the graph and say so.
	if en, ok := conceptStageEnableParam(def.Group); ok && !synthStageEnabled(instID, en) {
		drawRect(dst, graphR, WithAlpha(TokenSurface1(), AlphaStrong), true)
		DrawTextColorAtScale(dst, i18n.T(i18n.KeyCapStageOff),
			graphR.Min.X+SpaceSM, graphR.Max.Y-captionH-2, TokenTextSecondary(), captionScale)
	}
```

(`i18n` import already present via the caption path? — `synth_focus_graph.go` does not import i18n yet; add it.)

- [ ] **Step 4: Run tests** — new test + the FULL sweep. **Important:** the sweep renders via `synthFocusRendererForKnob` directly (not `drawSynthFocusGraph`), so the scrim does not affect Min/Max fingerprints — verify `TestFocusGraph_RespondsToEveryKnob` stays green.

---

### Task 11: Kid-level story cues (filter marker + words, envelope words, time ruler, before/after legend)

**Files:**
- Modify: `src/go/internal/ui/synth_concept_viz.go` (cue helpers + `conceptFilter` + `conceptEnvelope` cues; wire `drawConceptTimeRuler` into `conceptLFO`, `conceptBurst`, `conceptPitchEnvSweep`, `conceptFMEnvelope`)
- Modify: `src/go/internal/ui/synth_focus_graph.go` (before/after legend in caption row)
- Modify: `src/go/internal/i18n/keys.go`, `catalog_en.go`, `catalog_es.go`
- Test: `src/go/internal/ui/synth_concept_cues_test.go` (NEW)

**Interfaces:**
- i18n keys (EN / ES): `KeyGlossDull` = "dull"/"opaco", `KeyGlossBright` = "bright"/"brillante", `KeyCapStarts` = "starts"/"empieza", `KeyCapFades` = "fades"/"se apaga", `KeyCapBeforeAfter` = "before / after"/"antes / después".
- Produces: `func conceptFilterCutoffX(rect image.Rectangle, cutoffHz float64) int` — log-Hz x mapping (20..22000, same constants as `conceptFilter`).
- Produces: `func drawConceptTimeRuler(dst *ebiten.Image, rect image.Rectangle, endLabel string)` — baseline tick marks at 0/25/50/75/100% (2px tall, `colConceptGhost`) + "0" left / endLabel right at caption scale in `TokenTextSecondary`. **Gated: no-op when `rect.Dy() < 40` or `rect.Dx() < 120`** (tiny bands stay clean).
- Cue gating: all in-graph words/markers draw only when `rect.Dy() >= 48 && rect.Dx() >= 160` — the focus graph qualifies; small legacy bands skip. Markers use `DrawIcon(IconChevronDown, …)` (exists at `icons.go:28`); NO glyph characters.

- [ ] **Step 1: Write the failing tests**

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

func TestConceptFilterCutoffX_Mapping(t *testing.T) {
	r := image.Rect(0, 0, 200, 100)
	left := conceptFilterCutoffX(r, 20)
	mid := conceptFilterCutoffX(r, 663)   // ~ geometric middle of 20..22000
	right := conceptFilterCutoffX(r, 22000)
	if left != r.Min.X {
		t.Fatalf("20Hz must map to the left edge, got %d", left)
	}
	if right < r.Max.X-2 {
		t.Fatalf("22kHz must map to the right edge, got %d", right)
	}
	if !(left < mid && mid < right) {
		t.Fatalf("mapping not monotonic: %d %d %d", left, mid, right)
	}
	lo := conceptFilterCutoffX(r, 100)
	hi := conceptFilterCutoffX(r, 1000)
	if hi-lo < r.Dx()/4 {
		t.Fatalf("mapping must be log-scaled (a decade spans >=1/4 width), got %d px", hi-lo)
	}
}

func TestConceptFilter_CuesGatedByRectSize(t *testing.T) {
	const inst = "filter-cues-test"
	audio.BindInstrumentToRecipe(inst, "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
	audio.SetInstrumentParam(inst, "filter_enabled", 1)
	def := lfoDefFor(t, inst, "filter_cutoff")

	// Big rect: cutoff marker + words present. Small rect: bare curve.
	big := image.Rect(0, 0, 260, 110)
	small := image.Rect(0, 0, 100, 30)
	// Render the small rect scaled up would equal the big only if cues were
	// absent; instead assert directly: moving the cutoff moves INK in the big
	// rect's top band (where the marker lives), and the small render still works.
	fpA := fingerprintFocus(t, func(img *ebiten.Image) { conceptFilter(img, big, inst, def, nil) })
	audio.SetInstrumentParam(inst, "filter_cutoff", 200)
	fpB := fingerprintFocus(t, func(img *ebiten.Image) { conceptFilter(img, big, inst, def, nil) })
	if fpA == fpB {
		t.Fatalf("cutoff move must repaint (marker + curve)")
	}
	_ = fingerprintFocus(t, func(img *ebiten.Image) { conceptFilter(img, small, inst, def, nil) }) // must not panic
}

func TestDrawConceptTimeRuler_GatedTiny(t *testing.T) {
	img := ebiten.NewImage(300, 120)
	tiny := image.Rect(0, 0, 100, 30)
	drawConceptTimeRuler(img, tiny, "1s") // must be a no-op, not a panic
	big := image.Rect(0, 0, 300, 120)
	before := fingerprintFocus(t, func(i *ebiten.Image) {})
	after := fingerprintFocus(t, func(i *ebiten.Image) { drawConceptTimeRuler(i, big, "1s") })
	if before == after {
		t.Fatalf("ruler must draw ink on a big rect")
	}
}
```

- [ ] **Step 2: Run to verify failure.**

- [ ] **Step 3: Implement**

Add the five i18n keys to all three i18n files. Then in `synth_concept_viz.go`:

```go
// ── kid-level story cues ─────────────────────────────────────────────────────

// conceptCueMinW/H gate the in-graph words + markers: the big focus graph gets
// them; short legacy bands stay bare curves.
const (
	conceptCueMinW = 160
	conceptCueMinH = 48
)

func conceptCuesFit(rect image.Rectangle) bool {
	return rect.Dx() >= conceptCueMinW && rect.Dy() >= conceptCueMinH
}

// conceptFilterCutoffX maps a cutoff frequency to its x pixel on the filter
// response's log-Hz axis (20..22000, matching conceptFilter's constants).
func conceptFilterCutoffX(rect image.Rectangle, cutoffHz float64) int {
	const minHz, maxHz = 20.0, 22000.0
	if cutoffHz < minHz {
		cutoffHz = minHz
	}
	if cutoffHz > maxHz {
		cutoffHz = maxHz
	}
	frac := (math.Log10(cutoffHz) - math.Log10(minHz)) / (math.Log10(maxHz) - math.Log10(minHz))
	return rect.Min.X + int(math.Round(frac*float64(rect.Dx()-1)))
}

// drawConceptTimeRuler paints a minimal time axis: baseline ticks at quarter
// intervals plus "0" and endLabel — words a kid can read, no dense numerics.
func drawConceptTimeRuler(dst *ebiten.Image, rect image.Rectangle, endLabel string) {
	if rect.Dx() < 120 || rect.Dy() < 40 {
		return
	}
	baseY := rect.Max.Y - 1
	for i := 0; i <= 4; i++ {
		x := rect.Min.X + i*(rect.Dx()-1)/4
		drawRect(dst, image.Rect(x, baseY-2, x+1, baseY), colConceptGhost, true)
	}
	captionScale := FontSizeCaption / FontSizeBody
	captionH := int(float64(TextHeight()) * captionScale)
	DrawTextColorAtScale(dst, "0", rect.Min.X+1, baseY-captionH-3, TokenTextSecondary(), captionScale)
	endW := int(float64(MeasureTextWidth(endLabel)) * captionScale)
	DrawTextColorAtScale(dst, endLabel, rect.Max.X-endW-1, baseY-captionH-3, TokenTextSecondary(), captionScale)
}
```

(If `MeasureTextWidth` doesn't exist under that name, use the text-measure helper the codebase already uses for right-aligned captions — grep `DrawTextColorAtScale` call sites for the established right-align pattern and copy it.)

**Wire the cues:**
- `conceptFilter` (end of function, gated by `conceptCuesFit(rect)`): draw `DrawIcon(IconChevronDown, image.Rect(cx-4, rect.Min.Y+2, cx+4, rect.Min.Y+10), colAccent)` where `cx = conceptFilterCutoffX(rect, cutoff)` (only when `synthFilterParamsFor` returned ok), plus `i18n.T(i18n.KeyGlossDull)` bottom-left and `i18n.T(i18n.KeyGlossBright)` bottom-right at caption scale, `TokenTextSecondary`. For a high-pass filter (`filter_type == 1`), swap the two words.
- `conceptEnvelope` (end, gated by `conceptCuesFit(r)`): `KeyCapStarts` at `r.Min.X+SpaceSM` and `KeyCapFades` at 65% width, both at `r.Max.Y-captionH-2`.
- `conceptLFO`, `conceptBurst`, `conceptPitchEnvSweep`: call `drawConceptTimeRuler(dst, rect, "1s")` / `"0.25s"` / `"0.4s"` respectively (their window constants). `conceptFMEnvelope`: `"1s"` for decay mode, `"0.4s"` for pitch-env mode.
- Legend in `drawSynthFocusGraph`: when `dv.synthConceptGhost(kIdx) != nil`, right-align `i18n.T(i18n.KeyCapBeforeAfter)` in the caption row (same y as the caption, x from the measured width), color `TokenTextSecondary()`.

- [ ] **Step 4: Run tests** — new cue tests, full sweep, `-run 'TestConcept|TestSynthFocus|TestFocusGraph'` all green. The sweep is fingerprint-based; cues that draw identically at Min/Max (words) don't break it, and the cutoff marker only ADDS responsiveness.

---

### Task 12: Sweep-gate consolidation

**Files:**
- Modify: `src/go/internal/ui/synth_focus_sweep_test.go` (comment blocks + any straggler exemptions)

- [ ] **Step 1:** Run the full sweep with verbose logging:
`cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestFocusGraph_RespondsToEveryKnob' -v 2>&1 | tail -20`
- [ ] **Step 2:** Confirm the log line `focus-graph sweep: N responsive, M exempted` shows M reduced by exactly 2 (`burst4_off`, `lfo_delay` removed) vs the pre-plan 50, and no new failures. Remaining by-name exemptions: `fm_op1_level`, `lfo_target`; pair: `fm-epiano/fm_op3_ratio`.
- [ ] **Step 3:** Update the file-header comment (lines 40-49) to describe the new routing (conceptLFO/conceptBurst/conceptPitchEnvSweep/conceptPostWave) and why each surviving exemption is genuinely unshowable.
- [ ] **Step 4:** Run the FULL ui suite: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -5` — green (or failures provably pre-existing/concurrent-WIP only; verify by `git stash`-free `-run` isolation, never by deleting files).

---

### Task 13: Scenes + screenshot verification (wave-1 gate)

**Files:**
- Modify: `src/go/internal/ui/scene_catalog.go` (new crop scenes near the `crop_synth_focus_*` block at lines ~683-767)
- Modify: `src/go/internal/ui/scene_crop_test.go` expectations run automatically (no edit needed — `crop_*` scenes are auto-exercised)

**Interfaces:**
- New scenes (each with `Setup` + `MobileSetup`, following `crop_synth_focus_motion`'s pattern of selecting a section + knob then calling `renderSynthMirrorNow`): 
  - `crop_synth_full_note` — envelope section selected, `amp_decay` knob selected (shows the full-note card responding to envelope; Subject = the existing synth-panel subject).
  - `crop_synth_focus_burst_hit` — burst section, `burst2_amp` selected.
  - `crop_synth_focus_drive` — post section, `drive` selected, drive at ~70%.

- [ ] **Step 1:** Add the three catalog entries (copy the `crop_synth_focus_burst` entry shape at scene_catalog.go:755, adjusting section/knob/params — use the same `SubjectSynthPanel` subject and the existing knob-selection helper the focus scenes use). Extend `TestSynthFocusSceneSelectsIntendedKnob` (`synth_focus_scene_test.go`) with the two new focus scenes.
- [ ] **Step 2:** Run scene guards: `-run 'TestSceneCrop|TestSynthFocusScene'` — green.
- [ ] **Step 3:** Capture: `cd /home/ymolinar/Repos/beatmo && make screenshots-all SCENES=crop_synth_full_note,crop_synth_focus_burst_hit,crop_synth_focus_drive,crop_synth_focus_osc,crop_synth_focus_env,crop_synth_focus_filter,crop_synth_focus_fm,crop_synth_focus_pitch,crop_synth_mirror,crop_synth_tab_modular,crop_synth_tab_fm MOBILE=1`
- [ ] **Step 4:** READ the PNGs (desktop + mobile) and verify visually: three stacked cards on desktop; "Your sound" shows an enveloped hit; "Up close" caption renamed; FM focus wave smooth (no stair-steps); burst scene shows spikes not wobble; drive scene shows before/after waves; filter shows the cutoff marker + dull/bright; mobile column shows full-note + focus (Up-close collapsed) with everything legible. Fix and re-capture until true.

---

### Task 14 (wave 2): Per-stage thumbnail cache — `synthStageThumbs`

**Files:**
- Create: `src/go/internal/ui/synth_stage_thumbs.go`
- Create: `src/go/internal/ui/synth_stage_thumbs_test.go`

**Interfaces:**
- Produces: `func (dv *DrumView) synthStageDownstreamOverride(chipIdx int) map[string]float64` — for every chip AFTER `chipIdx` in `dv.instEditorChips` with a non-empty `enableParam`, set that param → 0. (Chips already carry `enableParam` — see `drawSynthChipStrip`'s state-dot at `synth_panel_zone.go:2131`.)
- Produces: type `synthStageThumbs` — `mu sync.Mutex; hash string; waves [][]float64`, method `ensure(dv *DrumView, instID, hash string)`: hash-gated; on change renders per-chip `audio.RenderInstrumentPreviewWithOverrides(instID, dv.synthStageDownstreamOverride(i), 150)` and stores each downsampled via `pcmMinMaxColumns` at draw time. Rendering happens SYNCHRONOUSLY but only on hash change (identical gating pattern to `renderLiveFull`; ~10 renders × 150 ms preview per edit — if profiling on WASM shows frame spikes, switch `ensure` to the `request`+pool pattern from `synthMirror`; keep that as a follow-up knob, not a blocker).
- Produces: `func (t *synthStageThumbs) waveFor(i int) []float64` (copy under lock; nil when absent).
- DrumView gains field `stageThumbs *synthStageThumbs` (lazy-init in `ensure` caller).

- [ ] **Step 1: Write the failing tests**

```go
//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestSynthStageDownstreamOverride_DisablesDownstreamOnly(t *testing.T) {
	g := newSynthTabGameForTest(t) // same helper as the chip-strip tests use
	dv := g.drum
	if len(dv.instEditorChips) < 3 {
		t.Skip("modular chip strip expected")
	}
	ov := dv.synthStageDownstreamOverride(1)
	for i, c := range dv.instEditorChips {
		if c.enableParam == "" {
			continue
		}
		_, disabled := ov[c.enableParam]
		if i <= 1 && disabled {
			t.Errorf("chip %d (%s) is upstream/self but disabled", i, c.enableParam)
		}
		if i > 1 && !disabled {
			t.Errorf("chip %d (%s) is downstream but not disabled", i, c.enableParam)
		}
	}
}

func TestSynthStageThumbs_DifferAcrossStagesAndHashGate(t *testing.T) {
	g := newSynthTabGameForTest(t)
	dv := g.drum
	inst := dv.resolveSynthInstrument("")
	// Make at least one gated stage audible so its thumb differs from upstream.
	audio.SetInstrumentParam(inst, "filter_enabled", 1)
	audio.SetInstrumentParam(inst, "filter_cutoff", 300)
	th := &synthStageThumbs{}
	th.ensure(dv, inst, "h1")
	first := th.waveFor(0)
	last := th.waveFor(len(dv.instEditorChips) - 1)
	if len(first) == 0 || len(last) == 0 {
		t.Fatalf("thumbs not rendered")
	}
	if sameWave(first, last) {
		t.Fatalf("first-stage and last-stage thumbs identical — pipeline not depicted")
	}
	// Hash gate: same hash must not re-render (mutate a param w/o changing hash).
	audio.SetInstrumentParam(inst, "filter_cutoff", 8000)
	th.ensure(dv, inst, "h1")
	if !sameWave(th.waveFor(len(dv.instEditorChips)-1), last) {
		t.Fatalf("same hash re-rendered — gate broken")
	}
}
```

- [ ] **Step 2: Run to verify failure.**
- [ ] **Step 3: Implement** per the Interfaces block (the `ensure` loop: `waves = make([][]float64, len(dv.instEditorChips)); for i := range dv.instEditorChips { waves[i] = audio.RenderInstrumentPreviewWithOverrides(instID, dv.synthStageDownstreamOverride(i), 150) }`).
- [ ] **Step 4: Run tests to PASS**; goleak stays clean (no goroutines spawned).

---

### Task 15 (wave 2): Living chip strip — arrows + waveform watermarks

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go:2079-2154` (`drawSynthChipStrip`), `drawSynthTab` (ensure thumbs)
- Test: `src/go/internal/ui/synth_chip_strip_thumbs_test.go` (NEW)

**Interfaces:**
- Arrows: replace the 1px wire (lines 2089-2096) with `DrawIcon(IconChevronRight, arrowR, WithAlpha(colAccent, AlphaMedium))` where `arrowR` is a square of side `min(gapW, 10)` centred in the inter-chip gap; when the gap is `< 4px` keep the old wire (wrapped mobile rows can be tight).
- Watermark: inside each chip, AFTER the pill fill but BEFORE the label, draw the stage's thumb wave as a faint min/max column band across the chip's inner rect (`c.rect` inset by `SpaceSM` horizontally, 2px vertically) using `pcmMinMaxColumns` + `WithAlpha(colWaveTrace, AlphaFaint)`. Disabled stages: a flat 1px centre line in `TokenTextDisabled` instead. **No chip/strip height change** — the wave is a watermark under the label, so no layout or `mobileAudioPanelMinContentH` ripple.
- `drawSynthTab` calls `dv.ensureStageThumbs(resolved)` (lazy-init `dv.stageThumbs`, hash = `dv.synthParamsHash(resolved)`) before `drawSynthChipStrip`.

- [ ] **Step 1: Write the failing test**

```go
//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestSynthChipStrip_WatermarksRespondToParams(t *testing.T) {
	g := newSynthTabGameForTest(t)
	dv := g.drum
	inst := dv.resolveSynthInstrument("")
	render := func() uint64 {
		dv.ensureStageThumbs(inst)
		return fingerprintFocus(t, func(img *ebiten.Image) { dv.drawSynthChipStrip(img) })
	}
	a := render()
	audio.SetInstrumentParam(inst, "osc_type", 1) // sine -> saw: every thumb changes
	b := render()
	if a == b {
		t.Fatalf("chip watermarks did not follow a param change")
	}
}
```

- [ ] **Step 2: Run to verify failure** — `undefined: ensureStageThumbs`.
- [ ] **Step 3: Implement** `ensureStageThumbs` (lazy struct + `ensure` call) and the two draw changes described in Interfaces. Keep the four chip visual states untouched; the watermark uses `AlphaFaint` so labels stay dominant.
- [ ] **Step 4: Run** the new test + `-run 'TestSynthChip'` (existing chip tests) + zone alloc/discipline guards. Update `perTabAllocBudget` for the synth tab with rationale if the watermark pushes it over.
- [ ] **Step 5:** Re-capture `crop_synth_tab_chips_collapsed`, `crop_synth_tab_modular` (desktop+mobile), READ them, verify arrows visible + wave evolving left→right + labels legible.

---

### Task 16: Final verification

- [ ] **Step 1:** Full fast suite: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./... 2>&1 | tail -15` — green (pre-existing/concurrent failures documented explicitly, verified NOT ours via `-run` isolation).
- [ ] **Step 2:** Audio suite native: `... ./internal/audio/... 2>&1 | tail -5`.
- [ ] **Step 3:** WASM compile check: `cd /home/ymolinar/Repos/beatmo && make wasm 2>&1 | tail -5` — builds clean (preview code is tag-free; i18n/token changes must compile under `GOOS=js`).
- [ ] **Step 4:** Browser bridge smoke (mirror exports untouched but re-verify): `GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js` and `GO=... node src/js/synth_knob_viz.browser.test.js`.
- [ ] **Step 5:** Full synth screenshot pass, desktop + mobile (`make screenshots-all SCENES=<all crop_synth_* + mobile_bottom_nav_synth> MOBILE=1`), READ and confirm every acceptance point from the spec's Section 1–3 lists.
- [ ] **Step 6:** Leave everything uncommitted; report results (including the new sweep counts) to the user.

---

## Self-Review Notes

- **Spec coverage:** Section 1 (three cards, adaptive window, mobile collapse) → Tasks 2–5. Section 2 items 1–5 (motion split / post wave / bypass honesty / pitch ghost / resolution) → Tasks 6, 7, 8, 10, 1+9. NOTE: the pitch-knob GHOST (spec §2 item 4) is enabled by Task 1's override form but intentionally deferred out of `conceptPitchWave` unless trivially cheap — the spec's guarantee is responsiveness (already held); implementer may add `RenderInstrumentPreviewWithOverrides`-based ghost to `conceptPitchWave` as an optional step inside Task 8's pattern. Story cues → Task 11. Section 3 → Tasks 14–15. Section 4 (tokens/i18n/tests/scenes) → Tasks 5, 11, 12, 13, 16.
- **Type consistency:** `conceptRenderer` signature reused everywhere; `pcmMinMaxColumns` produced in Task 2, consumed in Tasks 4/14/15; `RenderInstrumentPreviewWithOverrides` produced Task 1, consumed Tasks 14 (and optional pitch ghost); `SynthRightCardMinH` produced Task 5 Step 1, consumed Step 4.
- **Known adaptation points (not placeholders):** test-helper names (`newSynthTabGameForTest`, `fingerprintFocus`, section-selection helpers) must match what the existing test files define — the implementer copies the local pattern; the assertions and code under test are fully specified above.
