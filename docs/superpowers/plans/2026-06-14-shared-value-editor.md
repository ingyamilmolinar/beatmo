# Shared Value Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **REPO RULE — NEVER COMMIT:** This repo's standing instruction is *never `git add`/commit/stage* (applies to subagents too). Every "Checkpoint" step runs tests and **leaves changes uncommitted in the working tree**. Do NOT run git add/commit/restore/stash anywhere in this plan.

**Goal:** Route EQ dB, BPM, and synth/sampler knob value entry through one shared, spec-driven `ValueEditor` that opens pre-filled with the current value, auto-sizes to fit it, commits on Enter/blur, reverts on Escape, flashes on invalid, and (on mobile) shows a native input + numeric soft keyboard — identically everywhere.

**Architecture:** Evolve the existing `ParamValueEditor` (`param_value_editor.go`) into a spec-driven editor: a `ValueSpec` (`Format`/`Parse`/`MinW`/`Style`/`MaxLen`) plugs any numeric surface in; a `ValueOpen` struct carries the anchor, clamp bounds, get/set, and per-open `MobileInputID`. The desktop popup `TextInput` path stays; the editor additionally owns an *open-on-demand* native-input lifecycle (`mobileInputRegister`/`mobileInputPollResult`/`mobileInputClose`) so synth/sampler gain native mobile entry and BPM/EQ consolidate onto it. Migrations delete three bespoke commit paths (`commitDBText`, `commitBPMText`, and the inline EQ focus juggling).

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), `syscall/js` mobile bridge (browser-tested only). Fast test path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`.

---

## File Structure

- **`param_value_editor.go`** (modify) — add `ValueSpec`, `ValueOpen`, `OpenValue`, `paramSpec`, auto-width sizing, Escape handling, native-input lifecycle. Keep `Open(def,…)` as a back-compat adapter so existing tests/synth/sampler call sites compile unchanged until migrated.
- **`param_value_editor_test.go`** (modify) — keep existing tests green; add spec/auto-width/escape/native-id tests.
- **`synth_panel_zone.go`** (modify, `openSynthParamEditor` ~L2219) — call `OpenValue` with `paramSpec(def)`, clamp = content rect, `MobileInputID="synth-param"`.
- **`sampler_panel_zone.go`** (modify, `openSamplerParamEditor` ~L934) — same with `MobileInputID="sampler-param"`.
- **`eq_panel_zone.go`** (modify) — delete `eqDBInputs`/`updateDBInputs`/`commitDBText`/`syncDBInputText`/`syncAllDBInputTexts` + `dbInputFocused`/`dbInputPrev`/`dbInputSyncing`; add `eqDBSpec()` + `openEQDBEditor(band)`; dB-readout hit areas open the shared editor.
- **`transport_zone.go`** (modify) — delete `commitBPMText` clear-on-focus path; BPM box tap opens the shared editor via `bpmSpec()`.
- **`textinput.go`** (modify) — add `FitWidth(text, minW) int` helper used for size-to-fit; text fields reuse it.
- **`drumview_layout.go`** (modify ~L868) — BPM native registration moves into the editor-owned lifecycle (remove the per-frame `mobileInputRegister("bpm",…)`).

New shared helper functions live in `param_value_editor.go` (they change together with the editor).

---

## Task 1: Add `ValueSpec` + `ValueOpen` + `OpenValue`, keep `Open` as adapter

**Files:**
- Modify: `src/go/internal/ui/param_value_editor.go`
- Test: `src/go/internal/ui/param_value_editor_test.go`

- [ ] **Step 1: Write the failing test**

Add to `param_value_editor_test.go`:

```go
func TestOpenValuePrefillsAndCommitsViaSpec(t *testing.T) {
	var wrote float64 = -1
	spec := ValueSpec{
		Format: func(v float64) string { return strconv.Itoa(int(v)) },
		Parse: func(s string) (float64, bool) {
			n, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil || n < 1 || n > 1000 {
				return 0, false
			}
			return float64(n), true
		},
		MinW:   60,
		Style:  BPMBoxStyle,
		MaxLen: 4,
	}
	ed := NewParamValueEditor()
	ed.OpenValue(ValueOpen{
		Spec:   spec,
		Anchor: image.Rect(0, 0, 40, 24),
		Get:    func() float64 { return 120 },
		Set:    func(v float64) { wrote = v },
	})
	if ed.ti.Value() != "120" {
		t.Fatalf("prefill=%q want 120", ed.ti.Value())
	}
	ed.ti.SetText("145")
	ed.commit()
	if wrote != 145 {
		t.Fatalf("wrote=%v want 145", wrote)
	}
	if ed.Active() {
		t.Fatalf("editor should close after commit")
	}
}
```

Add imports `strconv`, `strings` to the test file if missing.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestOpenValuePrefillsAndCommitsViaSpec`
Expected: FAIL — `ValueSpec`/`ValueOpen`/`OpenValue` undefined (build error).

- [ ] **Step 3: Implement the spec-driven editor**

In `param_value_editor.go`, replace the struct + `Open` + `commit` block (current lines ~53–106) with:

```go
// ValueSpec describes how a single numeric surface renders, parses, and sizes
// its value in the shared editor. Format produces the box text from the live
// value; Parse validates+clamps editor text (ok==false ⇒ revert + flash).
type ValueSpec struct {
	Format func(float64) string
	Parse  func(string) (float64, bool)
	MinW   int            // comfortable floor width (px); 0 ⇒ use anchor width
	Style  TextInputStyle // box style; zero value ⇒ BPMBoxStyle
	MaxLen int            // rune cap; 0 ⇒ leave editor default (12)
}

// ValueOpen carries the per-open binding for the shared editor.
type ValueOpen struct {
	Spec          ValueSpec
	Anchor        image.Rectangle // readout the editor pops over
	Clamp         image.Rectangle // editor box stays inside this if non-empty
	Get           func() float64
	Set           func(float64)
	MobileInputID string // "" ⇒ desktop popup only; else native mobile input id
}

// ParamValueEditor is the single active numeric-entry box shared across tabs.
type ParamValueEditor struct {
	ti        *TextInput
	spec      ValueSpec
	setValue  func(float64)
	active    bool
	errorAnim float64
	mobileID  string
}

// NewParamValueEditor constructs the shared editor.
func NewParamValueEditor() *ParamValueEditor {
	ti := NewTextInput(image.Rectangle{}, BPMBoxStyle)
	ti.InputMode = "numeric"
	ti.MaxLen = 12
	ti.OnFocusGained = func() { softKeyboardShow("numeric") }
	ti.OnFocusLost = func() { softKeyboardHide() }
	return &ParamValueEditor{ti: ti}
}

// Active reports whether the editor is currently open.
func (e *ParamValueEditor) Active() bool { return e.active }

// OpenValue focuses the editor over o.Anchor, pre-filled with the current
// value (caret at end). The box auto-sizes to fit the value (min o.Spec.MinW)
// and is clamped inside o.Clamp when set.
func (e *ParamValueEditor) OpenValue(o ValueOpen) {
	if e.active {
		e.close()
	}
	e.spec = o.Spec
	e.setValue = o.Set
	e.mobileID = o.MobileInputID
	style := o.Spec.Style
	if style == (TextInputStyle{}) {
		style = BPMBoxStyle
	}
	e.ti.Style = style
	if o.Spec.MaxLen > 0 {
		e.ti.MaxLen = o.Spec.MaxLen
	} else {
		e.ti.MaxLen = 12
	}
	e.ti.MobileInputID = o.MobileInputID
	text := ""
	if o.Spec.Format != nil && o.Get != nil {
		text = o.Spec.Format(o.Get())
	}
	e.ti.Rect = sizeEditorRect(o.Anchor, text, o.Spec.MinW, o.Clamp)
	e.ti.SetText(text)
	e.ti.SetFocus(true)
	e.active = true
}

// commit parses the text; on success writes + closes, on failure reverts
// (no write), flashes the error, and closes.
func (e *ParamValueEditor) commit() {
	if e.spec.Parse != nil {
		if v, ok := e.spec.Parse(e.ti.Value()); ok {
			if e.setValue != nil {
				e.setValue(v)
			}
			e.close()
			return
		}
	}
	e.errorAnim = 1
	e.close()
}

func (e *ParamValueEditor) close() {
	e.active = false
	e.ti.SetFocus(false)
	if e.mobileID != "" {
		mobileInputClose(e.mobileID)
		e.mobileID = ""
	}
}
```

Then add the back-compat adapter + the spec builder + the sizing helper at the end of the file:

```go
// Open keeps the legacy ParamDef entry point (used by existing tests). It
// builds a ValueSpec from def and opens a desktop-only popup (no native input).
func (e *ParamValueEditor) Open(def audio.ParamDef, anchor image.Rectangle, getValue func() float64, setValue func(float64)) {
	e.OpenValue(ValueOpen{
		Spec:   paramSpec(def),
		Anchor: anchor,
		Get:    getValue,
		Set:    setValue,
	})
}

// paramSpec builds a ValueSpec from a synth/sampler ParamDef.
func paramSpec(def audio.ParamDef) ValueSpec {
	d := def
	return ValueSpec{
		Format: func(v float64) string { return formatParamForEntry(d, v) },
		Parse:  func(s string) (float64, bool) { return parseParamEntry(d, s) },
		MinW:   72,
		Style:  BPMBoxStyle,
		MaxLen: 12,
	}
}

// sizeEditorRect returns the editor box rect: width grows to fit text (min
// minW), re-centered horizontally on anchor's center, height = anchor height,
// clamped inside clamp when clamp is non-empty.
func sizeEditorRect(anchor image.Rectangle, text string, minW int, clamp image.Rectangle) image.Rectangle {
	const pad = 4
	const caretMargin = 8
	w := TextWidth(text) + 2*pad + caretMargin
	if w < minW {
		w = minW
	}
	cx := (anchor.Min.X + anchor.Max.X) / 2
	h := anchor.Dy()
	if h <= 0 {
		h = TextHeight() + 2*pad
	}
	r := image.Rect(cx-w/2, anchor.Min.Y, cx-w/2+w, anchor.Min.Y+h)
	if !clamp.Empty() {
		if r.Dx() > clamp.Dx() {
			r = image.Rect(clamp.Min.X, r.Min.Y, clamp.Max.X, r.Max.Y)
		}
		if r.Min.X < clamp.Min.X {
			r = r.Add(image.Pt(clamp.Min.X-r.Min.X, 0))
		}
		if r.Max.X > clamp.Max.X {
			r = r.Add(image.Pt(clamp.Max.X-r.Max.X, 0))
		}
	}
	return r
}
```

Keep the existing `Update` and `Draw` methods unchanged for now (Escape is added in Task 2). Keep `formatParamForEntry`/`parseParamEntry` exactly as they are.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestOpenValue|TestParamValueEditor|TestParseParamEntry|TestFormatParamForEntry'`
Expected: PASS (all old + new).

- [ ] **Step 5: Checkpoint (no commit — repo rule)**

Run the full UI package build to catch signature drift:
`cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/`
Expected: clean. Leave changes uncommitted.

---

## Task 2: Auto-width assertion + Escape-to-revert

**Files:**
- Modify: `src/go/internal/ui/param_value_editor.go` (`Update`)
- Test: `src/go/internal/ui/param_value_editor_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestSizeEditorRectFitsLongValueAndClamps(t *testing.T) {
	anchor := image.Rect(100, 10, 130, 34) // 30px cramped readout
	r := sizeEditorRect(anchor, "-127.5", 72, image.Rect(0, 0, 400, 200))
	if r.Dx() < 72 {
		t.Fatalf("width %d below MinW 72", r.Dx())
	}
	if r.Dx() < TextWidth("-127.5") {
		t.Fatalf("width %d clips value (TextWidth=%d)", r.Dx(), TextWidth("-127.5"))
	}
	// clamps inside the 0..400 bound
	if r.Min.X < 0 || r.Max.X > 400 {
		t.Fatalf("rect %v escapes clamp", r)
	}
}

func TestEditorEscapeReverts(t *testing.T) {
	var wrote float64 = -1
	ed := NewParamValueEditor()
	ed.OpenValue(ValueOpen{
		Spec:   paramSpec(audio.ParamDef{Name: "g", Min: -12, Max: 12}),
		Anchor: image.Rect(0, 0, 40, 24),
		Get:    func() float64 { return 3 },
		Set:    func(v float64) { wrote = v },
	})
	ed.ti.SetText("9")
	ed.cancel() // Escape path
	if wrote != -1 {
		t.Fatalf("escape must not write; wrote=%v", wrote)
	}
	if ed.Active() {
		t.Fatalf("escape should close the editor")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSizeEditorRect|TestEditorEscape'`
Expected: FAIL — `cancel` undefined.

- [ ] **Step 3: Add `cancel` + wire Escape into `Update`**

In `param_value_editor.go`, add:

```go
// cancel closes the editor without writing (Escape / revert).
func (e *ParamValueEditor) cancel() {
	e.close()
}
```

Modify `Update` so that while active it checks Escape before pumping the box:

```go
func (e *ParamValueEditor) Update() {
	if !e.active {
		if e.errorAnim > 0 {
			e.errorAnim *= 0.85
			if e.errorAnim < 0.01 {
				e.errorAnim = 0
			}
		}
		return
	}
	if isKeyPressed(ebiten.KeyEscape) {
		e.cancel()
		return
	}
	wasFocused := e.ti.Focused()
	e.ti.Update()
	if wasFocused && !e.ti.Focused() {
		e.commit()
	}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSizeEditorRect|TestEditorEscape|TestParamValueEditor'`
Expected: PASS.

- [ ] **Step 5: Checkpoint (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` — clean. Leave uncommitted.

---

## Task 3: Migrate synth + sampler to `OpenValue` (desktop), gain auto-width + Escape

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (`openSynthParamEditor` ~L2219)
- Modify: `src/go/internal/ui/sampler_panel_zone.go` (`openSamplerParamEditor` ~L934)
- Test: `src/go/internal/ui/param_value_editor_test.go` (or existing synth knob test files)

- [ ] **Step 1: Write the failing test**

Add a focused test that the synth editor open path sets the synth mobile id + clamps. Add to `param_value_editor_test.go`:

```go
func TestEditorMobileIDSetPerSurface(t *testing.T) {
	ed := NewParamValueEditor()
	ed.OpenValue(ValueOpen{
		Spec:          paramSpec(audio.ParamDef{Name: "g", Min: 0, Max: 1}),
		Anchor:        image.Rect(0, 0, 40, 24),
		Get:           func() float64 { return 0.5 },
		Set:           func(float64) {},
		MobileInputID: "synth-param",
	})
	if ed.ti.MobileInputID != "synth-param" {
		t.Fatalf("mobile id=%q want synth-param", ed.ti.MobileInputID)
	}
	ed.commit()
}
```

- [ ] **Step 2: Run to verify pass already (id plumbing landed Task 1)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestEditorMobileIDSetPerSurface`
Expected: PASS. (This guards the per-surface id; it locks the contract Task 3 depends on.)

- [ ] **Step 3: Rewrite `openSynthParamEditor`**

In `synth_panel_zone.go`, replace the `dv.paramEditor.Open(def, anchor, …)` call (current ~L2228–2240) with:

```go
	def := dv.instEditorBindings[idx].def
	anchor := dv.synthKnobReadoutRect(idx)
	clamp := dv.synthTabContentRect() // existing content-rect accessor; see note
	dv.paramEditor.OpenValue(ValueOpen{
		Spec:          paramSpec(def),
		Anchor:        anchor,
		Clamp:         clamp,
		MobileInputID: "synth-param",
		Get: func() float64 {
			return def.Min + dv.instEditorKnobs[idx].Value*(def.Max-def.Min)
		},
		Set: func(v float64) {
			span := def.Max - def.Min
			if span <= 0 {
				span = 1
			}
			dv.instEditorKnobs[idx].Value = (v - def.Min) / span
			dv.syncSliderFromKnob(idx)
			dv.propagateSynthSliderValue(idx, instID)
		},
	})
```

Note: use the existing synth content-rect accessor for `clamp`. If none is exported, pass `image.Rectangle{}` (no clamp) — the editor still auto-widths; do NOT invent a new accessor. Verify by grepping `func (dv *DrumView) synthTab` / `contentRect` in `synth_panel_zone.go`; if absent, use `image.Rectangle{}`.

- [ ] **Step 4: Rewrite `openSamplerParamEditor` the same way**

In `sampler_panel_zone.go` (~L934), replace the `paramEditor.Open(def, anchor, …)` call with the `OpenValue` form, `MobileInputID: "sampler-param"`, keeping the sampler's existing `Get`/`Set` closures verbatim (scale knob↔[min,max]).

- [ ] **Step 5: Run tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Synth|Sampler|Knob|ParamValueEditor'`
Expected: PASS. Then `../../.tools/go/bin/go vet -tags test ./internal/ui/` — clean.

- [ ] **Step 6: Checkpoint (no commit).**

---

## Task 4: Migrate EQ dB entry to the shared editor (desktop)

**Files:**
- Modify: `src/go/internal/ui/eq_panel_zone.go`
- Test: `src/go/internal/ui/eq_db_input_test.go`

- [ ] **Step 1: Write the failing test (pre-fill shows value, commit clamps)**

Replace the body of `eq_db_input_test.go`'s commit/focus tests with editor-driven equivalents (read the file first; keep any test asserting `formatDB`). Add:

```go
func TestEQDBEditorPrefillShowsValueAndCommits(t *testing.T) {
	z := newTestEQPanelZone(t) // existing helper in eq_db_input_test.go; reuse it
	z.bandGainsDB[3] = -4.5
	z.openEQDBEditor(3)
	if !z.paramEditor.Active() {
		t.Fatalf("editor should be active")
	}
	if got := z.paramEditor.ti.Value(); got != "-4.5" {
		t.Fatalf("prefill=%q want -4.5 (must show prior value, not blank)", got)
	}
	z.paramEditor.ti.SetText("99") // out of [-12,12]
	z.paramEditor.commit()
	if z.bandGainsDB[3] != 12 {
		t.Fatalf("commit clamp got %v want 12", z.bandGainsDB[3])
	}
}
```

If `newTestEQPanelZone` does not exist, construct the zone the way the existing tests in `eq_db_input_test.go` already do (read the file and mirror its setup). The editor field is `z.paramEditor` (added in Step 3).

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestEQDBEditorPrefill`
Expected: FAIL — `openEQDBEditor` / `z.paramEditor` undefined.

- [ ] **Step 3: Add the shared editor + spec + open method to `EQPanelZone`**

In `eq_panel_zone.go`:

1. Add a field to the `EQPanelZone` struct (near `eqDBInputs`): `paramEditor *ParamValueEditor`. Remove the `eqDBInputs [10]*TextInput`, `dbInputFocused int`, `dbInputPrev float64`, `dbInputSyncing bool` fields.

2. In `initButtons` delete the per-band dB `TextInput` construction loop (current L379–390). Replace with `z.paramEditor = NewParamValueEditor()`.

3. Add the spec + open method:

```go
// eqDBSpec parses/clamps/rounds a band gain to [-12,+12] @ 0.1 dB.
func eqDBSpec() ValueSpec {
	return ValueSpec{
		Format: formatDB,
		Parse: func(s string) (float64, bool) {
			db, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
			if err != nil {
				return 0, false
			}
			db = clampF64(db, -12.0, 12.0)
			return math.Round(db*10) / 10, true
		},
		MinW:   48,
		Style:  EQDBBoxStyle,
		MaxLen: 5,
	}
}

// openEQDBEditor opens the shared editor over band's dB readout cell.
func (z *EQPanelZone) openEQDBEditor(band int) {
	if band < 0 || band >= len(z.bandGainsDB) {
		return
	}
	if z.paramEditor == nil {
		z.paramEditor = NewParamValueEditor()
	}
	z.paramEditor.OpenValue(ValueOpen{
		Spec:          eqDBSpec(),
		Anchor:        z.dbReadoutRect(band),
		Clamp:         z.rect,
		MobileInputID: fmt.Sprintf("eq-db-%d", band),
		Get:           func() float64 { return z.bandGainsDB[band] },
		Set: func(v float64) {
			z.bandGainsDB[band] = v
			z.curveDirty = true
			if z.callbacks.OnGainChange != nil {
				z.callbacks.OnGainChange(band, v)
			}
			if z.callbacks.OnApplyEQ != nil {
				z.callbacks.OnApplyEQ()
			}
			if z.callbacks.OnEQBandCommit != nil {
				z.callbacks.OnEQBandCommit()
			}
		},
	})
}
```

4. Add a `dbReadoutRect(band int) image.Rectangle` accessor that returns the per-band readout rect computed in `layoutMuteAndDBInputs` (store the 10 rects in a new `[10]image.Rectangle` field `dbReadoutRects` populated where the old code set `ti.Rect`). Replace every `z.eqDBInputs[i].Rect = …` assignment in layout with `z.dbReadoutRects[i] = …`.

5. Delete `updateDBInputs`, `commitDBText`, `syncDBInputText`, `syncAllDBInputTexts` (current L2531–2627) and every call to them. In `Update` (L468–510) replace the whole `if z.dbInputFocused < 0 { … } z.updateDBInputs()` block with:

```go
	if z.paramEditor != nil {
		z.paramEditor.Update()
	}
```

6. In `rebuildHitAreas` (L1304–1323) replace the `textInputHitAdapter` registration with a handler that opens the editor. Define an adapter:

```go
// eqDBOpenAdapter opens the shared dB editor for a band on press.
type eqDBOpenAdapter struct {
	z    *EQPanelZone
	band int
}

func (a *eqDBOpenAdapter) OnPress(x, y int) InputResult {
	a.z.openEQDBEditor(a.band)
	return InputConsumed
}
func (a *eqDBOpenAdapter) OnDrag(x, y int)    {}
func (a *eqDBOpenAdapter) OnRelease(x, y int) {}
```

(Match the real `HitHandler` interface in `input_dispatch.go` — read it first and implement exactly its method set.) Register with `Rect: z.dbReadoutRects[i]`, same `ZIndex: zIdx + 3`, `Tag: fmt.Sprintf("eq-db-%s", eqCenterLabels[i])`.

7. Add the editor draw: in `EQPanelZone.Draw`, after the EQ tab content renders, call `if z.paramEditor != nil { z.paramEditor.Draw(screen) }`.

8. Wherever `syncAllDBInputTexts`/`syncDBInputText` were called (e.g. on external EQ changes/import), delete the calls — the editor reads live `bandGainsDB` via `Get` on open, so no text mirroring is needed.

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'EQDB|EQDb|TestEQ'`
Expected: PASS. Then `../../.tools/go/bin/go vet -tags test ./internal/ui/` — clean.

- [ ] **Step 5: Run the EQ + zone isolation guards**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'ZoneInput|EQPanelZoneInput|TestEQPanel'`
Expected: PASS (catch-all isolation intact).

- [ ] **Step 6: Checkpoint (no commit).**

---

## Task 5: Migrate BPM entry to the shared editor (desktop)

**Files:**
- Modify: `src/go/internal/ui/transport_zone.go`
- Test: `src/go/internal/ui/bpm_editor_validation_test.go`

- [ ] **Step 1: Write the failing test**

Add to `bpm_editor_validation_test.go` (mirror its existing zone setup helper):

```go
func TestBPMEditorPrefillShowsValueAndReverts(t *testing.T) {
	z := newTestTransportZone(t) // reuse existing helper / construction
	z.SetBPM(128)
	z.openBPMEditor()
	if z.paramEditor.ti.Value() != "128" {
		t.Fatalf("prefill=%q want 128 (not blank)", z.paramEditor.ti.Value())
	}
	z.paramEditor.ti.SetText("abc")
	z.paramEditor.commit()
	if z.BPM() != 128 {
		t.Fatalf("invalid commit must keep 128, got %d", z.BPM())
	}
	if z.paramEditor.errorAnim <= 0 {
		t.Fatalf("invalid commit must flash")
	}
}
```

Use the real BPM accessor name (`z.BPM()` if present; otherwise the field). Read the file first.

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestBPMEditorPrefill`
Expected: FAIL — `openBPMEditor` / `paramEditor` undefined.

- [ ] **Step 3: Add the editor + spec to TransportZone**

In `transport_zone.go`:

1. Add field `paramEditor *ParamValueEditor` to `TransportZone`; construct it where `bpmBox` is built (L230) with `z.paramEditor = NewParamValueEditor()`. Keep `bpmBox` for its rect/draw, but it no longer handles editing focus.

2. Add:

```go
func bpmSpec() ValueSpec {
	return ValueSpec{
		Format: func(v float64) string { return strconv.Itoa(int(v)) },
		Parse: func(s string) (float64, bool) {
			if v, ok := parseBPM(strings.TrimSpace(s)); ok {
				return float64(v), true
			}
			return 0, false
		},
		MinW:   60,
		Style:  BPMBoxStyle,
		MaxLen: 4,
	}
}

func (z *TransportZone) openBPMEditor() {
	if z.paramEditor == nil {
		z.paramEditor = NewParamValueEditor()
	}
	z.paramEditor.OpenValue(ValueOpen{
		Spec:          bpmSpec(),
		Anchor:        z.bpmBox.Rect,
		MobileInputID: "bpm",
		Get:           func() float64 { return float64(z.bpm) },
		Set: func(v float64) { z.SetBPM(int(v)) },
	})
}
```

3. In the BPM update method (L438–494) replace the legacy `bpmBox.Update()` focus/clear/commit body with:
   - pump `z.paramEditor.Update()`;
   - on invalid, the editor sets `errorAnim` itself, but BPM also wants the `OnNotifyError("Invalid BPM")` toast. Add that to the spec by routing failure through a wrapper: change `Set` to only fire on success (it already does), and detect invalid by checking — simplest: keep `OnNotifyError` by having `bpmSpec().Parse` call the notify callback on failure via a closure built in `openBPMEditor`. Replace the `Parse` in `openBPMEditor` with an inline one capturing `z`:

```go
		Spec: ValueSpec{
			Format: bpmSpec().Format,
			Parse: func(s string) (float64, bool) {
				if v, ok := parseBPM(strings.TrimSpace(s)); ok {
					return float64(v), true
				}
				if z.callbacks.OnNotifyError != nil {
					z.callbacks.OnNotifyError("Invalid BPM")
				}
				return 0, false
			},
			MinW: 60, Style: BPMBoxStyle, MaxLen: 4,
		},
```

4. Replace the BPM box hit handler so a tap calls `z.openBPMEditor()` (mirror the EQ `eqDBOpenAdapter` pattern; reuse the existing BPM hit-area registration site — read it, swap the handler).

5. Delete `commitBPMText` and `forceBlurBPM`'s body that referenced it; if `forceBlurBPM` is still called on overlay-steal, make it `if z.paramEditor != nil && z.paramEditor.Active() { z.paramEditor.cancel() }`.

6. In `TransportZone.Draw`, after the BPM box draws, call `if z.paramEditor != nil { z.paramEditor.Draw(screen) }`.

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'BPM|Transport'`
Expected: PASS. Then `../../.tools/go/bin/go vet -tags test ./internal/ui/` — clean.

- [ ] **Step 5: Checkpoint (no commit).**

---

## Task 6: Mobile native-input lifecycle owned by the editor

**Files:**
- Modify: `src/go/internal/ui/param_value_editor.go` (`Update`, `OpenValue`)
- Modify: `src/go/internal/ui/drumview_layout.go` (remove per-frame `mobileInputRegister("bpm",…)` ~L868)
- Test: `src/js/wasm_bridge_input_sanity.browser.test.js` (or a focused new browser test)

Note: native-input code is `//go:build js && !test` — NOT reachable from the fast path. Cover the Go-side lifecycle structurally + verify behavior in a browser test.

- [ ] **Step 1: Register native input on open; poll + commit in Update**

In `OpenValue`, after computing `e.ti.Rect`, register the native input when an id is set:

```go
	if o.MobileInputID != "" && Profile().IsMobile() {
		r := e.ti.Rect
		mobileInputRegister(o.MobileInputID, r.Min.X, r.Min.Y, r.Dx(), r.Dy(), text, e.ti.MaxLen, "numeric")
	}
```

In `Update`, before the desktop focus pump, poll the native result when a mobile id is active:

```go
	if e.mobileID != "" && Profile().IsMobile() && mobileInputActive(e.mobileID) {
		if val, committed, ok := mobileInputPollResult(e.mobileID); ok {
			if committed {
				e.ti.SetText(val)
				e.commit()
			} else {
				e.cancel()
			}
		}
		return
	}
```

`mobileInputActive`/`mobileInputPollResult`/`mobileInputRegister`/`mobileInputClose` already have desktop stubs (`mobile_input_stub.go`) returning false/no-op, so the fast path compiles and skips this branch. `Profile().IsMobile()` is false on desktop tests.

- [ ] **Step 2: Remove the per-frame BPM native registration**

In `drumview_layout.go` ~L868 delete the `mobileInputRegister("bpm", …)` block (the editor now registers on open). Verify nothing else depends on the always-registered "bpm" input by grepping `mobileInputActive("bpm")` / `mobileInputPollResult("bpm")` and removing the now-dead transport poll at `transport_zone.go:444-461` (superseded by the editor poll).

- [ ] **Step 3: Fast-path build + vet**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS (desktop unaffected).
Run the WASM build tag compile: `cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -tags "" ./internal/ui/` (confirm js path compiles; if the repo's js build needs the non-test tag set, use the Makefile `make wasm` instead).

- [ ] **Step 4: Browser test — synth knob native entry on mobile**

Add a minimal assertion to an existing mobile input browser suite (`mobile_native_input.browser.test.js` if present) that tapping a synth knob readout on a mobile viewport surfaces a native `<input inputmode="numeric">` and that committing it changes the param. Run:
`GO=$(pwd)/.tools/go/bin/go node src/js/mobile_native_input.browser.test.js`
Expected: PASS. (If the suite name differs, add the case to the closest mobile-input suite; do NOT create a brand-new file just for this per the JS Test Charter.)

- [ ] **Step 5: Checkpoint (no commit).**

---

## Task 7: Text-field consistency pass (rename / search / jump / save-as)

**Files:**
- Modify: `src/go/internal/ui/textinput.go` (add `FitWidth`)
- Modify: call sites that size these boxes (`drumview_overlay_rename_comp.go`, `drumview_overlay_inst_comp.go`, `synth_panel_save_as_dialog.go`)
- Test: `src/go/internal/ui/textinput_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestFitWidthGrowsToValueWithMin(t *testing.T) {
	if got := FitWidth("", 80); got != 80 {
		t.Fatalf("empty got %d want min 80", got)
	}
	long := "A very long instrument name"
	if got := FitWidth(long, 80); got < TextWidth(long) {
		t.Fatalf("got %d clips value (TextWidth=%d)", got, TextWidth(long))
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFitWidth`
Expected: FAIL — `FitWidth` undefined.

- [ ] **Step 3: Add `FitWidth`**

In `textinput.go`:

```go
// FitWidth returns a box width that fits text (plus padding + caret margin),
// never below minW. Shared sizing for inline editable boxes.
func FitWidth(text string, minW int) int {
	const pad = 4
	const caretMargin = 8
	w := TextWidth(text) + 2*pad + caretMargin
	if w < minW {
		w = minW
	}
	return w
}
```

Refactor `sizeEditorRect` (Task 1) to call `FitWidth(text, minW)` instead of inlining the math (DRY).

- [ ] **Step 4: Apply to text-field call sites + confirm no clear-on-focus**

For rename (`drumview_overlay_rename_comp.go`), instrument search/jump (`drumview_overlay_inst_comp.go`), and save-as (`synth_panel_save_as_dialog.go`): where each box's width is set, use `FitWidth(currentText, existingMinWidth)` so long names aren't cramped. Read each call site; only widen if the box was a fixed width narrower than the content. Confirm none of them clear text on focus (rename/save-as pre-fill the name; search is intentionally empty — leave it). Do not restructure dialog layout.

- [ ] **Step 5: Run tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TextInput|Rename|Search|SaveAs'`
Expected: PASS. Then `../../.tools/go/bin/go vet -tags test ./internal/ui/` — clean.

- [ ] **Step 6: Checkpoint (no commit).**

---

## Task 8: Full verification sweep

**Files:** none (verification only)

- [ ] **Step 1: Full fast UI + timeline + audio suite**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui ./internal/timeline ./internal/audio/...
```
Expected: PASS. Note: per project memory the full `internal/ui` suite has some *pre-existing* flaky/ordering-dependent failures unrelated to this change — if any fail, re-run the named test in isolation and compare against a baseline `git stash`-free run of the same test on the untouched tree (do NOT stash; instead run the single test by name and confirm it fails identically without our edits by reasoning from the memory notes). Only failures in `param_value_editor`, `eq_db_input`, `bpm_editor_validation`, `textinput`, zone-isolation, and the synth/sampler knob suites are in-scope.

- [ ] **Step 2: Discipline guards**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestDesignMDDrift|TestTokenDiscipline|ZoneInput|GridLayer' ./internal/ui/
```
Expected: PASS (no new color literals; zone input isolation intact).

- [ ] **Step 3: Bridge smoke (exports unaffected)**

Run: `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`
Expected: PASS.

- [ ] **Step 4: Report**

Summarize: tests run + outcomes, files touched, and the three deleted bespoke paths (`commitDBText`, `commitBPMText`, EQ focus juggling). Leave everything uncommitted in the working tree (repo rule).

---

## Self-Review Notes (author)

- **Spec coverage:** §1 ValueEditor → Task 1; §2 behaviors (prefill/auto-width/escape/error) → Tasks 1–2; §3 migrations EQ/BPM/synth/sampler → Tasks 3–5; native mobile for all → Task 6; §4 text fields → Task 7; testing → every task + Task 8.
- **Type consistency:** `ValueSpec`{Format,Parse,MinW,Style,MaxLen}, `ValueOpen`{Spec,Anchor,Clamp,Get,Set,MobileInputID}, `OpenValue`, `paramSpec`, `sizeEditorRect`, `FitWidth`, `cancel`, `openEQDBEditor`, `eqDBSpec`, `dbReadoutRect`/`dbReadoutRects`, `openBPMEditor`, `bpmSpec`, `eqDBOpenAdapter` — used consistently across tasks.
- **Known unknowns the implementer must resolve by reading the file first (flagged inline):** exact `HitHandler` interface method set (`input_dispatch.go`), presence of a synth content-rect accessor for `Clamp`, exact EQ/BPM zone test-construction helpers, exact BPM accessor name. Each task says "read the file first / mirror existing setup" rather than inventing names.
