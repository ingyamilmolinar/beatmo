package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// The Sampler tab. A waveform-prominent editor: a full-width waveform with
// draggable Start/End trim handles on top, and one control area beneath with
// Start/End/Transpose/Detune/Gain knobs plus Reverse/Normalize/Fade toggles
// and a Preview button. The header carries Save / Save As (the dropdown
// auto-loads the selected instrument — the old "From Synth" and "Load WAV"
// buttons are gone; WAV loading now lives only on the master panel's kebab
// Upload action). All sample logic lives on samplerState (sampler_state.go);
// this file owns only layout, drawing, and input.

const samplerPreviewID = "preview.sampler"

// samplerTitlePad is the left inset of the "SAMPLER" title inside the
// header strip.
const samplerTitlePad = SpaceSM

// samplerGroupLabel returns the functional group a knob belongs to, used
// to draw a small grouping caption above each knob cluster (TRIM / TUNE /
// LEVEL) so the five knobs read as three purposeful groups rather than one
// flat strip — mirroring the Synth tab's section-card grouping.
func samplerGroupLabel(idx int) string {
	switch idx {
	case samplerKnobStart:
		return i18n.T(i18n.KeySamplerGroupTrim)
	case samplerKnobTranspose:
		return i18n.T(i18n.KeySamplerGroupTune)
	case samplerKnobGain:
		return i18n.T(i18n.KeySamplerGroupLevel)
	}
	return ""
}

// samplerKnobPlainEnglish returns a kid-readable gloss for each knob, the
// same convention the Synth/EQ tabs use via PlainEnglish(). Shown as the
// knob caption's helper text.
func samplerKnobPlainEnglish(idx int) string {
	switch idx {
	case samplerKnobStart:
		return i18n.T(i18n.KeySamplerGlossStart)
	case samplerKnobEnd:
		return i18n.T(i18n.KeySamplerGlossEnd)
	case samplerKnobTranspose:
		return i18n.T(i18n.KeySamplerGlossPitch)
	case samplerKnobDetune:
		return i18n.T(i18n.KeySamplerGlossFine)
	case samplerKnobGain:
		return i18n.T(i18n.KeySamplerGlossGain)
	}
	return ""
}

// samplerBtnWidth sizes a button to fit its label with comfortable padding,
// so no header/control button ever truncates ("From Sy…" / "Load …").
// Delegates to the shared labelButtonWidth (also used by the Synth header).
func samplerBtnWidth(label string) int {
	return labelButtonWidth(label)
}

// samplerAuditionFn triggers playback of the preview instrument. Swappable in
// tests via SwapSamplerAuditionFnForTest, mirroring synthAuditionFn.
var samplerAuditionFn = func(instID string) { audio.Play(instID) }

// SwapSamplerAuditionFnForTest installs a test fake for the preview trigger and
// returns the previous value.
func SwapSamplerAuditionFnForTest(fn func(string)) func(string) {
	prev := samplerAuditionFn
	if fn != nil {
		samplerAuditionFn = fn
	}
	return prev
}

// samplerTriggerNanos returns the latest trigger timestamp for id in unix-nanos,
// or 0 when the instrument has never been triggered. It reads the same
// per-instrument timestamps the engine stamps on every Play — no parallel audio
// path.
func samplerTriggerNanos(id string) int64 {
	t := audio.LastTriggerAt(id)
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// updateSamplerPlayheads advances the playhead tracker once per frame: it spawns
// a line for every new trigger of the preview instrument or the loaded
// instrument (the latter fires from the running sequencer), and prunes finished
// lines. Called from buildSamplerTab (the Update/Layout phase) so Draw stays a
// pure reader. When no buffer is loaded the tracker is cleared.
func (dv *DrumView) updateSamplerPlayheads() {
	s := &dv.sampler
	dur := s.audibleDurationSeconds()
	if !s.hasBuffer() || dur <= 0 {
		s.playheads.reset()
		return
	}
	triggers := map[string]int64{samplerPreviewID: samplerTriggerNanos(samplerPreviewID)}
	if s.captureID != "" && s.captureID != samplerPreviewID {
		triggers[s.captureID] = samplerTriggerNanos(s.captureID)
	}
	s.playheads.observe(samplerNowNanos(), triggers, dur)
}

// samplerPlayheadDraw is one resolved playhead line ready to blit.
type samplerPlayheadDraw struct {
	x   int
	col color.Color
}

// samplerActivePlayheads projects the in-flight lines to (x pixel, colour) for
// the current waveform rect. The x is derived from the live waveformRect every
// call (correct across resizes with nothing cached); the duration and trim
// region come from current state (self-corrects when the sample or edit
// changes). Returns nil when nothing is playing or no buffer is loaded.
func (dv *DrumView) samplerActivePlayheads() []samplerPlayheadDraw {
	s := &dv.sampler
	if !s.hasBuffer() || s.waveformRect.Empty() {
		return nil
	}
	lo, hi := s.startFrac, s.endFrac
	if lo > hi {
		lo, hi = hi, lo
	}
	views := s.playheads.active(samplerNowNanos(), s.audibleDurationSeconds(), lo, hi)
	if len(views) == 0 {
		return nil
	}
	w := s.waveformRect
	out := make([]samplerPlayheadDraw, 0, len(views))
	for _, v := range views {
		x := w.Min.X + int(v.frac*float64(w.Dx()))
		if x < w.Min.X {
			x = w.Min.X
		}
		if x > w.Max.X {
			x = w.Max.X
		}
		out = append(out, samplerPlayheadDraw{x: x, col: samplerPlayheadColor(v.colorIdx)})
	}
	return out
}

// samplerActiveInstrument resolves the default load/override target — the
// active EQ channel, else the first row's instrument.
func (dv *DrumView) samplerActiveInstrument() string {
	return dv.synthTabActiveInstrument()
}

// ensureSamplerLoaded loads the dropdown-selected instrument into the working
// buffer when the selection differs from what is currently loaded. User samples
// (WAV imports / previously-saved chops) come straight from the canonical Go
// PCM store so they load on every platform without a JS round-trip; everything
// else is rendered as a synth one-shot. Idempotent: it returns immediately when
// the selection is already loaded, so the per-Layout call never re-renders or
// thrashes a failed capture. instID == "" (no rows / nothing selected) is a
// no-op that leaves the panel in its empty state.
// resyncSamplerEditFromDocument re-seeds the Sampler tab's editing state from the
// instrument's CURRENT saved edit (audio.SampleEditFor). Call after the document
// changes underneath the tab (undo/redo/import): ensureSamplerLoaded preserves
// an already-loaded instrument's in-progress edit, so without this the knobs /
// trim / reverse would keep the pre-undo values — the gesture wouldn't visibly
// undo, and the next gesture would re-apply the stale edit via commitSamplerEdit.
// Synth-source only (WAV edits aren't descriptors). No-op without a buffer.
func (dv *DrumView) resyncSamplerEditFromDocument() {
	s := &dv.sampler
	if s.captureID == "" || !s.hasBuffer() || s.source != samplerSourceSynth {
		return
	}
	e, ok := audio.SampleEditFor(s.captureID)
	if !ok {
		e = audio.SampleEdit{EndFrac: 1} // identity: no saved edit ⇒ revert to defaults
	}
	// loadEditDescriptor only flips the working buffer one way (off→on); reconcile
	// the reverse parity here so an undo that turns reverse OFF un-flips the buffer.
	if e.Reverse != s.reverse {
		s.reverseBuffer()
	}
	s.loadEditDescriptor(e)
}

func (dv *DrumView) ensureSamplerLoaded(instID string) {
	s := &dv.sampler
	if instID == "" {
		return
	}
	// Same instrument already loaded (and non-empty). For synth-rendered
	// captures, re-render the one-shot whenever the instrument's synth signal
	// has changed since the last capture, so the waveform tracks live synth
	// edits in real time without a manual Preview or tab switch. WAV sources are
	// fixed PCM — never re-render them. Guarding on hasBuffer means a selection
	// whose first load failed (WASM: the synth one-shot hasn't rendered yet —
	// the capture bridge kicks off a render and a later call succeeds) is
	// retried on the next Layout until it yields audio. Desktop renders
	// synchronously, so it never retries.
	if s.captureID == instID && s.hasBuffer() {
		if s.source != samplerSourceSynth {
			return
		}
		sig := samplerSourceSignatureFn(instID)
		if sig == s.srcSignature {
			return
		}
		// A descriptor-bearing instrument re-captures through the RAW renderer:
		// the editor's buffer must stay the un-edited source (the saved edit is
		// represented by the knob/handle state, not the buffer).
		capture := samplerCaptureFn
		if audio.HasSampleEdit(instID) {
			capture = samplerRawCaptureFn
		}
		pcm, sr := capture(instID)
		if len(pcm) == 0 {
			// Renderer not ready yet (WASM async). Keep the current buffer and
			// leave srcSignature stale so the next Layout retries the re-render.
			return
		}
		// Preserve the user's in-progress edit — only the source audio changed.
		s.recaptureRaw(pcm, sr)
		s.srcSignature = sig
		return
	}
	// A user sample (WAV import, saved chop, or a file-picker WAV already seeded
	// below) is a WAV source: its PRISTINE PCM is the canonical store entry. Load
	// that and overlay any saved sample-edit descriptor onto the knob state, so
	// the editor shows the un-edited waveform with the edit applied declaratively
	// — the same non-destructive shape the synth path uses. Checked before the
	// descriptor branch so a user sample's edit never routes through the (recipe-
	// only) raw synth capture.
	if rec, ok := audio.UserSamplePCM(instID); ok && len(rec.PCM) > 0 {
		s.loadFromInstrument(instID, append([]float32(nil), rec.PCM...), rec.SampleRate, samplerSourceWAV)
		if e, hasEdit := audio.SampleEditFor(instID); hasEdit {
			s.loadEditDescriptor(e)
		}
		return
	}
	// A saved sample-edit descriptor on a RECIPE-bound instrument means it is a
	// SYNTH with a non-destructive edit: load the RAW recipe render and overlay
	// the saved edit. (A non-recipe instrument with a descriptor is a user sample,
	// handled above.)
	if e, hasEdit := audio.SampleEditFor(instID); hasEdit && audio.RecipeForInstrument(instID) != "" {
		pcm, sr := samplerRawCaptureFn(instID)
		if len(pcm) == 0 {
			s.captureID = instID
			s.raw = nil
			s.status = "Loading — play the sound once if it doesn't appear"
			return
		}
		s.loadFromInstrument(instID, pcm, sr, samplerSourceSynth)
		s.loadEditDescriptor(e)
		s.srcSignature = samplerSourceSignatureFn(instID)
		return
	}
	pcm, sr := samplerCaptureFn(instID)
	if len(pcm) == 0 {
		s.captureID = instID
		s.raw = nil
		s.status = "Loading — play the sound once if it doesn't appear"
		return
	}
	// A recipe binding is what makes an instrument a SYNTH (the descriptor is
	// applied to the fresh recipe render at trigger time). A file-picker WAV
	// (audio.RegisterWAV) is a raw PCM Sample with NO recipe and no user-sample
	// store entry, so it is a WAV source. Seed its pristine PCM into the store so
	// the real-time edit path (audio.SetSampleEdit → reapplyUserSampleEdit) has a
	// source to bake from — this is what makes trim/reverse/etc audible
	// immediately, without waiting for Save.
	if audio.RecipeForInstrument(instID) == "" {
		audio.PutUserSample(instID, pcm, sr)
		s.loadFromInstrument(instID, pcm, sr, samplerSourceWAV)
		return
	}
	s.loadFromInstrument(instID, pcm, sr, samplerSourceSynth)
	s.srcSignature = samplerSourceSignatureFn(instID)
}

// buildSamplerTab lays out the sampler header, waveform, knobs, and buttons
// inside contentR. Called every Layout while TabSampler is active.
//
// Layout arrangement is chosen by screen class (the one IsMobile() branch
// that is legitimately a *layout* decision, not sizing): desktop puts a big
// waveform on the left and the control column on the right so the wide panel
// is actually used; mobile stacks the waveform above the controls. All
// control *sizing* comes from the density tokens, never from IsMobile.
//
// Empty state (no buffer): the waveform card fills the whole body as a
// load/capture placeholder and the knobs + Rev/Norm/Fade/Preview/Save are
// suppressed so the panel never presents dead affordances.
func (dv *DrumView) buildSamplerTab(contentR image.Rectangle, instID string) {
	s := &dv.sampler
	// The dropdown instrument selector is the single source of truth for what
	// the Sampler edits: load the selected instrument (synth-rendered one-shot
	// or a user sample's PCM) whenever the selection changes. No-op when the
	// selection is already loaded.
	dv.ensureSamplerLoaded(instID)

	// Advance the playhead tracker once per frame (spawn lines for new triggers,
	// prune finished ones) so Draw stays a pure reader of the active lines.
	dv.updateSamplerPlayheads()

	// Knob instances are reused across Layout to preserve in-flight drags;
	// allocate once.
	if len(s.knobs) != samplerKnobCount {
		s.knobs = make([]*Knob, samplerKnobCount)
		s.knobStepBadges = make([]*KnobStepBadge, samplerKnobCount)
		for i := range s.knobs {
			s.knobs[i] = NewKnob(s.knobValue(i))
			s.knobs[i].ZeroFrac, s.knobs[i].Bipolar = samplerKnobBipolar(i)
			sc := samplerKnobScale(i)
			s.knobs[i].Scale = sc
			s.knobs[i].Endless = true
			badge := NewKnobStepBadge(audio.ParamDef{Name: samplerStepPrefName(i), Min: sc.Min, Max: sc.Max, Unit: sc.Unit})
			if persisted, ok := dv.knobStepPref(badge.ParamName()); ok {
				badge.SetStep(persisted)
			}
			s.knobs[i].StepMul = badge.Step()
			s.knobStepBadges[i] = badge
		}
	}

	hasBuf := s.hasBuffer()
	dv.buildSamplerButtons(hasBuf)

	if contentR.Empty() {
		s.headerRect = image.Rectangle{}
		s.waveformRect = image.Rectangle{}
		s.metaRect = image.Rectangle{}
		dv.clearSamplerKnobRects()
		dv.clearSamplerControlButtonRects()
		return
	}

	dv2 := Profile().DensityValues()
	mobile := Profile().IsMobile()

	// ── Header band ───────────────────────────────────────────────
	btnH := dv2.SynthHeaderButtonH
	headerH := btnH + 2*SpaceXS
	minBodyH := Profile().DensityValues().SamplerMinBodyH
	if contentR.Dy() < headerH+minBodyH {
		headerH = contentR.Dy() - minBodyH
		if headerH < 0 {
			headerH = 0
		}
	}
	s.headerRect = image.Rect(contentR.Min.X+SpaceSM, contentR.Min.Y+SpaceXS,
		contentR.Max.X-SpaceSM, contentR.Min.Y+SpaceXS+headerH)
	dv.layoutSamplerHeaderButtons(s.headerRect, btnH, mobile)

	// ── Body ──────────────────────────────────────────────────────
	body := image.Rect(contentR.Min.X+SpaceSM, s.headerRect.Max.Y+SpaceXS,
		contentR.Max.X-SpaceSM, contentR.Max.Y-SpaceSM)
	if body.Empty() {
		s.waveformRect = image.Rectangle{}
		s.metaRect = image.Rectangle{}
		dv.clearSamplerKnobRects()
		dv.clearSamplerControlButtonRects()
		return
	}

	if !hasBuf {
		// Placeholder fills the whole body; no knobs / handles / control row.
		s.waveformRect = body
		s.metaRect = image.Rectangle{}
		s.startHandleRect = image.Rectangle{}
		s.endHandleRect = image.Rectangle{}
		dv.clearSamplerKnobRects()
		dv.clearSamplerControlButtonRects()
		return
	}

	var waveR, ctrlR image.Rectangle
	if mobile {
		// Stacked: reserve controls first so they always fit, give the rest
		// (capped) to the waveform.
		minCtrlH := dv.samplerControlMinH(dv2)
		waveH := body.Dy() - minCtrlH - SpaceSM
		// Cap the graph a bit smaller (40%) so the two-row knob grid has room.
		if maxWaveH := body.Dy() * 40 / 100; waveH > maxWaveH {
			waveH = maxWaveH
		}
		if waveH < 36 {
			waveH = Profile().DensityValues().SamplerWaveMinH
		}
		waveR = image.Rect(body.Min.X, body.Min.Y, body.Max.X, body.Min.Y+waveH)
		ctrlR = image.Rect(body.Min.X, waveR.Max.Y+SpaceSM, body.Max.X, body.Max.Y)
	} else {
		// Side-by-side: waveform left (~50%), wider control column right so the
		// knob captions ("Ganancia +0 dB") render at full font without crowding.
		waveW := body.Dx() * 50 / 100
		waveR = image.Rect(body.Min.X, body.Min.Y, body.Min.X+waveW, body.Max.Y)
		ctrlR = image.Rect(waveR.Max.X+SpaceMD, body.Min.Y, body.Max.X, body.Max.Y)
	}
	s.waveformRect = waveR
	s.startHandleRect = dv.samplerHandleRect(s.startFrac)
	s.endHandleRect = dv.samplerHandleRect(s.endFrac)
	dv.layoutSamplerControls(ctrlR, dv2, mobile)
}

// clearSamplerKnobRects collapses every knob to an empty rect so the
// empty-state body has no interactive knobs to draw or hit-test.
func (dv *DrumView) clearSamplerKnobRects() {
	for _, k := range dv.sampler.knobs {
		if k != nil {
			k.SetRect(image.Rectangle{})
		}
	}
}

// clearSamplerControlButtonRects collapses the edit + action buttons so they
// are neither drawn nor hit-tested when no buffer is loaded. Includes the
// Save/Save As/Reset actions because on mobile they live in the control column
// (the two-row action layout) rather than the header.
func (dv *DrumView) clearSamplerControlButtonRects() {
	for _, tag := range []string{
		"sampler-reverse", "sampler-normalize", "sampler-fade", "sampler-preview",
		"sampler-save", "sampler-save-as", "sampler-reset",
	} {
		if b := dv.samplerButtonByTag(tag); b != nil {
			b.SetRect(image.Rectangle{})
		}
	}
}

// samplerControlMinH is the smallest control-column height that fits the
// group-label row, a minimum-diameter knob + caption, the toggle row, and
// the metadata strip — used to reserve space before sizing the waveform.
func (dv *DrumView) samplerControlMinH(dv2 densityValues) int {
	return samplerControlMinHeight(dv2)
}

// samplerControlMinHeight is the package-level form shared with the mobile
// audio-panel content-floor calc so the panel reserves exactly the height the
// scrolling sampler control column needs: ONE full knob cell (group-label band +
// an ideal-diameter dial + caption band) + the TWO mobile button rows
// (Rev/Norm/Fade toggles, then the Preview/Save/Save As/Reset action row) +
// the metadata strip. The remaining knobs scroll, so the floor only has to
// guarantee the first dial renders at its full size.
func samplerControlMinHeight(dv2 densityValues) int {
	labelBandH := TextHeight() + SpaceXS            // group-label band above the dial
	captionCellH := SpaceXS + dv2.SynthKnobCaptionH // caption band below the dial
	cellH := labelBandH + dv2.SynthKnobIdeal + captionCellH
	// +SpaceSM slack so the budget lands comfortably above the ideal after the
	// waveform 40% cap and split rounding take their cut. Two button rows so
	// every edit + action button stays visible (mirrors the Synth tab).
	return cellH + SpaceSM + 2*dv2.SynthHeaderButtonH + SpaceXS + TextHeight() + 2*SpaceXS
}

// layoutSamplerHeaderButtons right-aligns Save / Save As / Reset in the
// header. On mobile the header is too narrow, so Save / Save As / Reset move
// to the action row beneath the knobs (layoutSamplerControlButtons) and their
// header rects are cleared. Every button is sized to fit its label.
func (dv *DrumView) layoutSamplerHeaderButtons(hdr image.Rectangle, btnH int, mobile bool) {
	by := hdr.Min.Y + (hdr.Dy()-btnH)/2
	if by < hdr.Min.Y {
		by = hdr.Min.Y
	}
	if mobile {
		// Save / Save As / Reset live in the action row on mobile.
		for _, tag := range []string{"sampler-save", "sampler-save-as", "sampler-reset"} {
			if b := dv.samplerButtonByTag(tag); b != nil {
				b.SetRect(image.Rectangle{})
			}
		}
		return
	}
	rx := hdr.Max.X
	for _, tag := range []string{"sampler-reset", "sampler-save-as", "sampler-save"} {
		b := dv.samplerButtonByTag(tag)
		if b == nil {
			continue
		}
		w := samplerBtnWidth(b.Text)
		b.SetRect(image.Rect(rx-w, by, rx, by+btnH))
		rx -= w + SpaceSM
	}
}

// layoutSamplerControls places the knob row (5 knobs grouped TRIM/TUNE/
// LEVEL), the Rev/Norm/Fade/Preview button row, and the metadata strip
// inside the control column. Sizing is density-driven; the layout is
// computed so nothing overflows ctrlR (the regression guard for the
// off-panel control row).
func (dv *DrumView) layoutSamplerControls(ctrlR image.Rectangle, dv2 densityValues, mobile bool) {
	s := &dv.sampler
	if ctrlR.Empty() {
		dv.clearSamplerKnobRects()
		dv.clearSamplerControlButtonRects()
		s.metaRect = image.Rectangle{}
		return
	}
	labelH := TextHeight()
	captionH := dv2.SynthKnobCaptionH
	badgeH := Profile().DensityValues().KnobStepBadgeH
	// Two cell heights below the dial:
	//   captionCellH — caption line only (gap + caption, NO trailing pad — it is
	//                  the last element in the tight two-row cell, so the saved
	//                  pad goes to a bigger dial)
	//   fullCellH    — caption + step-badge pill (the roomy 1-row layout)
	// The badge is reserved only when there is room; on the tight two-row mobile
	// layout it is dropped (the badge draw guard auto-hides it) so the full-font
	// captions fit — readable captions outrank the advanced step-badge affordance.
	captionCellH := SpaceXS + captionH
	fullCellH := captionCellH + SpaceXS + badgeH
	btnH := dv2.SynthHeaderButtonH
	metaH := TextHeight()

	// Metadata strip pinned to the bottom, toggle row above it.
	metaR := image.Rect(ctrlR.Min.X, ctrlR.Max.Y-metaH, ctrlR.Max.X, ctrlR.Max.Y)
	toggleY1 := metaR.Min.Y - SpaceXS
	toggleY0 := toggleY1 - btnH
	s.metaRect = metaR

	if mobile {
		// Two FIXED full-width button rows so every edit + action button is
		// directly visible and untruncated (mirrors the Synth tab's fixed
		// action row): Rev/Norm/Fade toggles on top, then the
		// Preview/Save/Save As/Reset action row pinned above the metadata.
		actionY1 := metaR.Min.Y - SpaceXS
		actionY0 := actionY1 - btnH
		mToggleY1 := actionY0 - SpaceXS
		mToggleY0 := mToggleY1 - btnH
		gridBottomM := mToggleY0 - SpaceXS
		dv.layoutSamplerKnobsScrolling(image.Rect(ctrlR.Min.X, ctrlR.Min.Y, ctrlR.Max.X, gridBottomM), dv2)
		dv.layoutSamplerMobileButtonRows(
			image.Rect(ctrlR.Min.X, mToggleY0, ctrlR.Max.X, mToggleY1),
			image.Rect(ctrlR.Min.X, actionY0, ctrlR.Max.X, actionY1),
		)
		return
	}

	// LANGUAGE-INVARIANT knob grid. The row count depends ONLY on screen class
	// (never on caption length), so the dial diameter is identical in every
	// language. The desktop control column is wide enough for five full-font
	// captions in one row; the narrower mobile column wraps to two rows (3 + 2,
	// dropping the step-badge band) so the longest caption ("Ganancia +0 dB")
	// fits at full font. Captions centre in the full cell, decoupled from the
	// dial, so a longer Spanish caption widens nothing and shrinks no dial.
	gridTop := ctrlR.Min.Y
	gridBottom := toggleY0 - SpaceXS

	// Desktop: all five knobs in one side-by-side row (the wide control column
	// has room). No scrolling. (mobile is handled above and returns.)
	perRow, rows, cellH := samplerKnobCount, 1, fullCellH
	rowOverhead := labelH + SpaceXS + cellH

	// Dial diameter = the chosen grid's vertical budget (capped at the density
	// ideal and the cell width). It is NEVER clamped up past the budget — that
	// would overflow the control column. Instead the panel floor
	// (samplerControlMinHeight / mobileAudioPanelMinContentH) reserves enough
	// height that the budget lands at or above the density minimum, so the dial
	// is usable on every real panel. The size depends only on the rect + density
	// (never on caption length), so it is identical in every language.
	vb := (gridBottom - gridTop) - (rows-1)*SpaceSM - rows*rowOverhead
	knobD := vb / rows
	cellW := ctrlR.Dx() / perRow
	if knobD > dv2.SynthKnobIdeal {
		knobD = dv2.SynthKnobIdeal
	}
	if knobD > cellW {
		knobD = cellW
	}
	if knobD < 1 {
		knobD = 1 // degenerate (panel far too small); never zero/negative
	}

	rowStride := rowOverhead + knobD + SpaceSM
	for i, k := range s.knobs {
		if k == nil {
			continue
		}
		col := i % perRow
		row := i / perRow
		cellX0 := ctrlR.Min.X + col*cellW
		cellTop := gridTop + row*rowStride
		dialTop := cellTop + labelH + SpaceXS
		dialX0 := cellX0 + (cellW-knobD)/2
		// Knob rect = the dial square (+ caption/badge band) so Knob.geom sizes
		// the dial to knobD. The wider CELL is tracked separately so the caption,
		// group label, and step badge centre in the full cell, not the dial.
		k.SetRect(image.Rect(dialX0, dialTop, dialX0+knobD, dialTop+knobD+cellH))
		s.knobCells[i] = image.Rect(cellX0, cellTop, cellX0+cellW, dialTop+knobD+cellH)
		if !k.Capturing() {
			k.Value = s.knobValue(i)
		}
	}

	dv.layoutSamplerControlButtons(image.Rect(ctrlR.Min.X, toggleY0, ctrlR.Max.X, toggleY1))
}

// ensureKnobGrid lazily creates the mobile single-column scroll grid, preserving
// the scroll position across Layout calls.
func (s *samplerState) ensureKnobGrid() *ControlGrid {
	if s.knobGrid == nil {
		s.knobGrid = NewControlGrid(ScrollbarStyleForPlatform())
	}
	return s.knobGrid
}

// layoutSamplerKnobsScrolling lays the five knobs out in a single vertical
// column inside gridR, scrolling when they overflow. Each cell reserves a band
// above the dial for the group label (TRIM/TUNE/LEVEL), the dial itself at the
// density ideal, and a caption band below. Off-window knobs get an empty rect so
// drawSamplerKnobs / samplerTabHitAreas skip them. Mirrors the Synth tab's
// ControlGrid usage (SetMaxCols(1) on mobile).
func (dv *DrumView) layoutSamplerKnobsScrolling(gridR image.Rectangle, dv2 densityValues) {
	s := &dv.sampler
	grid := s.ensureKnobGrid()
	s.knobGridRect = gridR

	labelBandH := TextHeight() + SpaceXS            // group-label band above the dial
	captionCellH := SpaceXS + dv2.SynthKnobCaptionH // caption band below the dial
	cellH := labelBandH + dv2.SynthKnobIdeal + captionCellH
	if cellH > gridR.Dy() && gridR.Dy() > 0 {
		cellH = gridR.Dy() // degrade gracefully on a tiny panel (one shrunk row)
	}
	if cellH < 1 {
		cellH = 1
	}
	grid.SetMaxCols(1)
	grid.Layout(gridR, samplerKnobCount, dv2.SynthKnobIdeal, cellH, SpaceSM, SpaceXS)

	for i, k := range s.knobs {
		if k == nil {
			continue
		}
		slot, vis := grid.CellRect(i)
		if !vis {
			k.SetRect(image.Rectangle{})
			s.knobCells[i] = image.Rectangle{}
			continue
		}
		// Dial below the label band, centred in the cell, capped to the ideal and
		// what the slot can hold (label band + dial + caption must fit).
		d := dv2.SynthKnobIdeal
		if maxH := slot.Dy() - labelBandH - captionCellH; d > maxH {
			d = maxH
		}
		if d > slot.Dx() {
			d = slot.Dx()
		}
		if d < 1 {
			d = 1
		}
		dialX0 := slot.Min.X + (slot.Dx()-d)/2
		dialTop := slot.Min.Y + labelBandH
		// Knob rect = dial square + caption band (drawSamplerKnobs reads r.Dx() as
		// the dial diameter and places the caption at r.Min.Y + r.Dx() + SpaceXS).
		k.SetRect(image.Rect(dialX0, dialTop, dialX0+d, dialTop+d+captionCellH))
		s.knobCells[i] = slot
		if !k.Capturing() {
			k.Value = s.knobValue(i)
		}
	}
}

// layoutSamplerControlButtons lays the DESKTOP Rev/Norm/Fade/Preview row out
// left-to-right, each sized to its label, clamped to the row's right edge.
// (Save / Save As / Reset live in the desktop header; mobile uses the
// two-row layoutSamplerMobileButtonRows instead.)
func (dv *DrumView) layoutSamplerControlButtons(rowR image.Rectangle) {
	x := rowR.Min.X
	for _, tag := range []string{"sampler-reverse", "sampler-normalize", "sampler-fade", "sampler-preview"} {
		b := dv.samplerButtonByTag(tag)
		if b == nil {
			continue
		}
		w := samplerBtnWidth(b.Text)
		if x+w > rowR.Max.X {
			w = rowR.Max.X - x
		}
		if w <= 0 {
			b.SetRect(image.Rectangle{})
			continue
		}
		b.SetRect(image.Rect(x, rowR.Min.Y, x+w, rowR.Max.Y))
		x += w + SpaceSM
	}
}

// layoutSamplerMobileButtonRows lays the sampler's mobile edit controls out as
// TWO FIXED full-width rows of equal-width cells — Rev/Norm/Fade toggles on
// top, Preview/Save/Save As/Reset actions below — so every button stays
// directly visible and untruncated on a narrow phone, matching the Synth tab's
// fixed action row. The last cell in each row claims the remainder so rounding
// slack never leaves a gap.
func (dv *DrumView) layoutSamplerMobileButtonRows(toggleR, actionR image.Rectangle) {
	place := func(rowR image.Rectangle, tags []string) {
		n := len(tags)
		if n == 0 || rowR.Dx() <= 0 {
			return
		}
		cellW := (rowR.Dx() - (n-1)*SpaceSM) / n
		if cellW < 1 {
			cellW = 1
		}
		x := rowR.Min.X
		for i, tag := range tags {
			b := dv.samplerButtonByTag(tag)
			if b == nil {
				continue
			}
			x1 := x + cellW
			if i == n-1 {
				x1 = rowR.Max.X // last cell claims the remainder
			}
			b.SetRect(image.Rect(x, rowR.Min.Y, x1, rowR.Max.Y))
			x = x1 + SpaceSM
		}
	}
	place(toggleR, []string{"sampler-reverse", "sampler-normalize", "sampler-fade"})
	place(actionR, []string{"sampler-preview", "sampler-save", "sampler-save-as", "sampler-reset"})
}

// samplerHandleRect returns the thin vertical drag-handle rect for a trim
// fraction over the current waveform rect.
func (dv *DrumView) samplerHandleRect(frac float64) image.Rectangle {
	w := dv.sampler.waveformRect
	if w.Empty() {
		return image.Rectangle{}
	}
	x := w.Min.X + int(frac*float64(w.Dx()))
	if x < w.Min.X {
		x = w.Min.X
	}
	if x > w.Max.X {
		x = w.Max.X
	}
	const half = 5
	return image.Rect(x-half, w.Min.Y, x+half, w.Max.Y)
}

// samplerButton describes one header/control button (label + click action +
// active predicate for toggles).
type samplerButton struct {
	btn    *Button
	tag    string
	active func() bool
}

// buildSamplerButtons (re)creates the header + control buttons with their
// click handlers. When hasBuf is false the buffer-dependent buttons (Rev /
// Norm / Fade / Preview / Save / Save As) render with the Disabled spec and
// their handlers are gated, so the empty panel presents no dead affordances.
// Toggles reflect their on/off state via Primary/Secondary when enabled.
func (dv *DrumView) buildSamplerButtons(hasBuf bool) {
	s := &dv.sampler
	// toggleSpec selects the spec for an on/off toggle, downgrading to
	// Disabled when no buffer is loaded.
	toggleSpec := func(on bool) ComponentID {
		if !hasBuf {
			return ComponentButtonDisabled
		}
		if on {
			return ComponentButtonPrimary
		}
		return ComponentButtonSecondary
	}
	// actionSpec is the spec for a buffer-dependent action button.
	actionSpec := func(primary bool) ComponentID {
		if !hasBuf {
			return ComponentButtonDisabled
		}
		if primary {
			return ComponentButtonPrimary
		}
		return ComponentButtonSecondary
	}
	// guard wraps a buffer-dependent handler so a click while disabled is a
	// no-op (belt-and-suspenders alongside the suppressed hit area).
	guard := func(fn func()) func() {
		return func() {
			if dv.sampler.hasBuffer() {
				fn()
			}
		}
	}
	defs := []samplerButton{
		// Reverse is a stateful TOGGLE: one click reverses the working signal in
		// place (visible waveform + baked audio both flip) AND latches the button
		// ON; a second click un-reverses and un-latches. The latched visual reads
		// off samplerState.reverse (the net flip parity), so it reflects the live
		// reversed state identically on every platform.
		{NewSpecButton(i18n.T(i18n.KeyReverse), toggleSpec(s.reverse), guard(func() {
			dv.sampler.reverseBuffer()
			dv.commitSamplerEdit()
		})), "sampler-reverse", func() bool { return dv.sampler.reverse }},
		{NewSpecButton(i18n.T(i18n.KeyNormalize), toggleSpec(s.normalize), guard(func() {
			dv.sampler.normalize = !dv.sampler.normalize
			dv.commitSamplerEdit()
		})), "sampler-normalize", func() bool { return dv.sampler.normalize }},
		{NewSpecButton(i18n.T(i18n.KeyFade), toggleSpec(s.fadeOn), guard(func() {
			dv.sampler.fadeOn = !dv.sampler.fadeOn
			dv.commitSamplerEdit()
		})), "sampler-fade", func() bool { return dv.sampler.fadeOn }},
		{NewSpecButton(i18n.T(i18n.KeyPreview), actionSpec(false), guard(func() {
			dv.samplerPreview()
		})), "sampler-preview", nil},
		{NewSpecButton(i18n.T(i18n.KeySave), actionSpec(true), guard(func() {
			dv.samplerSave()
		})), "sampler-save", nil},
		{NewSpecButton(i18n.T(i18n.KeySaveAs), actionSpec(false), guard(func() {
			dv.samplerSaveAs()
		})), "sampler-save-as", nil},
		{NewSpecButton(i18n.T(i18n.KeyReset), actionSpec(false), guard(func() {
			dv.samplerReset()
		})), "sampler-reset", nil},
	}
	dv.samplerButtons = defs
	dv.refreshSamplerToggleStates()
}

// refreshSamplerToggleStates syncs each Sampler toggle's latched visual (the
// shared amber engaged keycap drawn by Button.Draw when Toggled) to its live
// on/off predicate, so Reverse / Normalize / Fade read their state identically
// on every platform. Action buttons (active==nil) and the empty-buffer state
// never latch. Called on every (re)build and once per frame from drawSamplerTab.
func (dv *DrumView) refreshSamplerToggleStates() {
	hasBuf := dv.sampler.hasBuffer()
	for _, b := range dv.samplerButtons {
		b.btn.SetToggled(b.active != nil && hasBuf && b.active())
	}
}

func (dv *DrumView) samplerButtonByTag(tag string) *Button {
	for _, b := range dv.samplerButtons {
		if b.tag == tag {
			return b.btn
		}
	}
	return nil
}

// drawSamplerTab renders the sampler header, waveform with trim handles, knobs,
// and buttons.
func (dv *DrumView) drawSamplerTab(dst *ebiten.Image, contentR image.Rectangle) {
	s := &dv.sampler
	hasBuf := s.hasBuffer()

	// Header background + title.
	if !s.headerRect.Empty() {
		drawRoundedRect(dst, s.headerRect, TokenSurface1(), RadiusSM, true)
		DrawTextStyled(dst, i18n.T(i18n.KeyCapSamplerTitle), s.headerRect.Min.X+samplerTitlePad, s.headerRect.Min.Y+(s.headerRect.Dy()-StyledTextHeight(RoleSectionHeader))/2, RoleSectionHeader, TokenTextPrimary())
	}

	// Waveform card.
	if !s.waveformRect.Empty() {
		drawRoundedRect(dst, s.waveformRect, TokenSurface2(), RadiusXS, true)
		if hasBuf {
			dv.drawSamplerWaveform(dst)
		} else {
			dv.drawSamplerPlaceholder(dst, s.waveformRect)
		}
	}

	// Metadata strip: kept duration / total duration · sample rate.
	if hasBuf && !s.metaRect.Empty() {
		dv.drawSamplerMeta(dst, s.metaRect)
	}

	// Knobs + group labels + captions (only when laid out).
	if hasBuf {
		dv.drawSamplerKnobs(dst)
		// Mobile single-column knob list scrollbar (no-op when it fits without
		// scrolling; never created/used on desktop).
		if Profile().IsMobile() && s.knobGrid != nil {
			s.knobGrid.Draw(dst)
		}
	}

	// Buttons. Sync every toggle's latched (amber engaged keycap) visual to its
	// live state so Reverse/Normalize/Fade read identically on all platforms.
	dv.refreshSamplerToggleStates()
	for _, b := range dv.samplerButtons {
		if b.btn.Rect().Empty() {
			continue
		}
		b.btn.Draw(dst)
	}

	// Numeric param editor floats above the knob captions (shared with the
	// Synth tab's editor; only one can be open at a time).
	if dv.paramEditor != nil {
		dv.paramEditor.Draw(dst)
	}

	// Save As name-prompt dialog floats above the tab content (shared with the
	// Synth tab; only one can be open at a time).
	dv.drawSaveAsDialog(dst)
}

// drawSamplerPlaceholder paints the empty-state prompt centred in the card,
// falling back to a shorter message (and surfacing s.status) so the text
// never clips the card edges.
func (dv *DrumView) drawSamplerPlaceholder(dst *ebiten.Image, card image.Rectangle) {
	s := &dv.sampler
	avail := card.Dx() - 2*SpaceMD
	msg := "Pick an instrument from the dropdown"
	if TextWidth(msg) > avail {
		msg = "Capture or load a sample"
	}
	if TextWidth(msg) > avail {
		msg = "No sample"
	}
	y := card.Min.Y + (card.Dy()-TextHeight())/2
	if s.status != "" {
		y -= (TextHeight() + SpaceXS) / 2
	}
	DrawTextColorAt(dst, msg, card.Min.X+(card.Dx()-TextWidth(msg))/2, y, colTextSecondary)
	if s.status != "" {
		st := s.status
		for TextWidth(st) > avail && len(st) > 4 {
			st = st[:len(st)-4] + "…"
		}
		DrawTextColorAt(dst, st, card.Min.X+(card.Dx()-TextWidth(st))/2, y+TextHeight()+SpaceXS, colError)
	}
}

// samplerMetaText formats the length / sample-rate readout. The "of"/"de"
// joiner follows the active locale; units (s, kHz) are international.
func samplerMetaText(trimSec, lengthSec, srKHz float64) string {
	return i18n.Tf(i18n.KeySamplerMetaFmt, trimSec, lengthSec, srKHz)
}

// drawSamplerMeta renders the length / sample-rate readout.
func (dv *DrumView) drawSamplerMeta(dst *ebiten.Image, r image.Rectangle) {
	s := &dv.sampler
	sr := s.rawSampleRate
	txt := samplerMetaText(s.trimSeconds(), s.lengthSeconds(), float64(sr)/1000)
	DrawTextColorAt(dst, txt, r.Min.X, r.Min.Y+(r.Dy()-TextHeight())/2, WithAlpha(TokenTextSecondary(), AlphaMedium))
}

// drawSamplerKnobs draws each laid-out knob with its group label (above the
// cluster) and its caption (value + plain-English helper) below. It also
// draws the step badge pill beneath the caption and records the caption
// hit-rect so samplerKnobReadoutRect can be used by OnPress and HitAreas.
func (dv *DrumView) drawSamplerKnobs(dst *ebiten.Image) {
	s := &dv.sampler
	// Clear all badge rects and readout rects before populating the visible
	// knobs so stale rects from a prior draw never steal taps.
	for i := range s.knobStepBadges {
		if s.knobStepBadges[i] != nil {
			s.knobStepBadges[i].SetRect(image.Rectangle{})
		}
		s.readoutRects[i] = image.Rectangle{}
	}
	for i, k := range s.knobs {
		if k == nil || k.Rect().Empty() {
			continue
		}
		r := k.Rect()
		// All chrome (group label, caption, badge) centres in the full CELL,
		// not the narrower dial rect — that decoupling is what lets a long
		// Spanish caption render at full font without overlapping a neighbour.
		cell := s.knobCells[i]
		if cell.Empty() {
			cell = r
		}
		// Group label + plain-English gloss above the cluster's first knob,
		// mirroring the Synth tab's section title + subtitle convention. The
		// gloss is appended on the same row at caption scale only when it
		// fits, so it never clips into the neighbouring cluster.
		if g := samplerGroupLabel(i); g != "" {
			labelY := r.Min.Y - SpaceXS - TextHeight()
			if labelY < cell.Min.Y {
				labelY = cell.Min.Y
			}
			DrawTextColorAt(dst, g, cell.Min.X, labelY, TokenTextSecondary())
			gloss := samplerKnobPlainEnglish(i)
			captionScale := FontSizeCaption / FontSizeBody
			glossX := cell.Min.X + TextWidth(g) + SpaceSM
			glossW := int(float64(TextWidth(gloss)) * captionScale)
			// Skip the gloss if it would clip past the cell's right edge.
			if gloss != "" && glossX+glossW <= cell.Max.X {
				DrawTextColorAtScale(dst, gloss, glossX, labelY+2, WithAlpha(TokenTextSecondary(), AlphaMedium), captionScale)
			}
		}
		// Mobile: render the tap-to-open value-pill button (the value caption is
		// drawn below); desktop keeps the rotary dial. Mirrors the Synth tab.
		if Profile().IsMobile() {
			drawKnobValuePillRect(dst, dv.samplerMobileKnobButtonRect(i), samplerKnobCaption(i, s))
		} else {
			k.Draw(dst)
		}
		cap := samplerKnobCaption(i, s)
		// Caption sits just below the dial. The dial is a knobD square at the
		// top of the knob rect, so its bottom = r.Min.Y + r.Dx() (== knobD).
		dialD := r.Dx()
		captionY := r.Min.Y + dialD + SpaceXS
		// Full font, centred in the CELL width — fits because the layout widened
		// the cell (and/or wrapped to two rows) until the caption fits.
		capX := cell.Min.X + (cell.Dx()-TextWidth(cap))/2
		if capX < cell.Min.X {
			capX = cell.Min.X
		}
		DrawTextColorAt(dst, cap, capX, captionY, colTextSecondary)

		// Record the caption line as the tappable readout rect (full cell width).
		readoutR := image.Rect(cell.Min.X, captionY, cell.Max.X, captionY+TextHeight())
		s.readoutRects[i] = readoutR

		// Step badge pill: a narrow pill below the caption, centered in the
		// knob cell and clamped so it never extends outside the cell.
		if i < len(s.knobStepBadges) {
			if badge := s.knobStepBadges[i]; badge != nil {
				bw, bh := Profile().DensityValues().KnobStepBadgeW, Profile().DensityValues().KnobStepBadgeH
				bx := cell.Min.X + (cell.Dx()-bw)/2
				by := captionY + TextHeight() + SpaceXS
				// Only draw if the badge fits within the knob cell below.
				if by >= r.Min.Y && by+bh <= r.Max.Y {
					badge.SetRect(image.Rect(bx, by, bx+bw, by+bh))
					badge.Draw(dst)
				}
				// If it doesn't fit, rect stays empty (cleared above).
			}
		}
	}
}

// drawSamplerWaveform paints the captured/loaded buffer with the kept (trim)
// region highlighted and the Start/End drag handles.
func (dv *DrumView) drawSamplerWaveform(dst *ebiten.Image) {
	s := &dv.sampler
	w := s.waveformRect
	// Reuse the cached float64 scratch (no per-frame allocation).
	wave := s.waveFloat64()
	midY := w.Min.Y + w.Dy()/2
	drawWaveTrace(dst, wave, w, midY, w.Dx(), colAccent, 1.0, TokenSurface3())

	// Shade the trimmed-away regions (outside [start,end]) so the kept area
	// reads as the bright part.
	lo, hi := s.startFrac, s.endFrac
	if lo > hi {
		lo, hi = hi, lo
	}
	x0 := w.Min.X + int(lo*float64(w.Dx()))
	x1 := w.Min.X + int(hi*float64(w.Dx()))
	shade := WithAlpha(colSurface1, AlphaOverlay)
	if x0 > w.Min.X {
		drawRect(dst, image.Rect(w.Min.X, w.Min.Y, x0, w.Max.Y), shade, true)
	}
	if x1 < w.Max.X {
		drawRect(dst, image.Rect(x1, w.Min.Y, w.Max.X, w.Max.Y), shade, true)
	}
	// Handles.
	drawRect(dst, dv.samplerHandleRect(s.startFrac), colAccent, true)
	drawRect(dst, dv.samplerHandleRect(s.endFrac), colAccent, true)

	// Playheads: one bright vertical line per in-flight trigger, sweeping the
	// kept region as the audio progresses. Drawn last so they sit above the
	// trace, shading, and handles; each overlapping voice gets its own colour so
	// they never read as one marker. Stroke is density-driven so the lines stay
	// clearly visible (a 1–2 px line disappeared against the trace).
	stroke := Profile().DensityValues().SamplerPlayheadStroke
	if stroke < 1 {
		stroke = 1
	}
	half := stroke / 2
	for _, ph := range dv.samplerActivePlayheads() {
		x0 := ph.x - half
		drawRect(dst, image.Rect(x0, w.Min.Y, x0+stroke, w.Max.Y), ph.col, true)
	}
}

// samplerKnobCaption returns the short value readout shown beneath each knob.
// Label and value both come from the shared paramFacet registry (sampleEditFacet)
// — the SAME source an undo/redo notification reads — so a Sampler caption and a
// notification describing the same edit can never drift. Captions still read
// coherently with the Synth tab (Pitch "+0 st", Fine "+0 c", Gain "+0 dB")
// because the facet formatters delegate to the shared formatParamValue.
func samplerKnobCaption(idx int, s *samplerState) string {
	key, val, ok := samplerKnobEditField(idx, s)
	if !ok {
		return ""
	}
	f, ok := sampleEditFacet(key)
	if !ok {
		return ""
	}
	return f.renderLabel() + " " + f.format(val)
}

// samplerKnobEditField maps a Sampler knob index to its sample_edit map key and
// the live value — the bridge between the index-keyed UI and the key-keyed
// paramFacet registry.
func samplerKnobEditField(idx int, s *samplerState) (string, float64, bool) {
	switch idx {
	case samplerKnobStart:
		return "start_frac", s.startFrac, true
	case samplerKnobEnd:
		return "end_frac", s.endFrac, true
	case samplerKnobTranspose:
		return "transpose_semis", s.transposeSemis, true
	case samplerKnobDetune:
		return "detune_cents", s.detuneCents, true
	case samplerKnobGain:
		return "gain_db", s.gainDB, true
	}
	return "", 0, false
}

// applySamplerKnob pushes a knob's current normalized value into the state.
func (dv *DrumView) applySamplerKnob(idx int) {
	if idx < 0 || idx >= len(dv.sampler.knobs) {
		return
	}
	dv.sampler.setKnob(idx, dv.sampler.knobs[idx].Value)
}

// samplerKnobReadoutRect returns the caption hit-rect for the given sampler
// knob index. Returns the zero rectangle when the index is out of range or
// the rect has not been populated yet (no Draw pass has occurred).
func (dv *DrumView) samplerKnobReadoutRect(idx int) image.Rectangle {
	if idx < 0 || idx >= samplerKnobCount {
		return image.Rectangle{}
	}
	return dv.sampler.readoutRects[idx]
}

// cycleSamplerKnobStep advances the step badge for sampler knob idx and
// syncs the knob's StepMul, persisting the choice. Analogous to
// cycleKnobStep for the synth tab but operates on s.knobStepBadges.
func (dv *DrumView) cycleSamplerKnobStep(idx int) {
	s := &dv.sampler
	if idx < 0 || idx >= len(s.knobStepBadges) {
		return
	}
	badge := s.knobStepBadges[idx]
	if badge == nil {
		return
	}
	badge.Cycle()
	if idx < len(s.knobs) && s.knobs[idx] != nil {
		s.knobs[idx].StepMul = badge.Step()
	}
	dv.persistKnobStep(badge)
}

// stepSamplerKnobResolution shifts sampler knob idx's step-badge rung by
// `steps` (mouse-wheel delta; positive = scroll up = finer), syncing StepMul
// and persisting. Returns true if the rung moved. Used by the wheel handler
// when the cursor is over the resolution badge.
func (dv *DrumView) stepSamplerKnobResolution(idx, steps int) bool {
	s := &dv.sampler
	if idx < 0 || idx >= len(s.knobStepBadges) {
		return false
	}
	badge := s.knobStepBadges[idx]
	if badge == nil || !badge.WheelResolution(steps) {
		return false
	}
	if idx < len(s.knobs) && s.knobs[idx] != nil {
		s.knobs[idx].StepMul = badge.Step()
	}
	dv.persistKnobStep(badge)
	return true
}

// openSamplerParamEditor opens the shared numeric editor anchored over the
// caption (readout) of sampler knob at idx. The setValue callback writes
// back through setKnob so the state stays consistent with the knob.
// Numeric entry must NOT audition (sampler edits apply on Save/Preview).
func (dv *DrumView) openSamplerParamEditor(idx int) {
	if idx < 0 || idx >= samplerKnobCount {
		return
	}
	if dv.paramEditor == nil {
		dv.paramEditor = NewParamValueEditor()
	}
	sc := samplerKnobScale(idx)
	def := audio.ParamDef{Name: samplerStepPrefName(idx), Min: sc.Min, Max: sc.Max, Unit: sc.Unit}
	anchor := dv.samplerKnobReadoutRect(idx)
	span := sc.Max - sc.Min
	if span <= 0 {
		span = 1
	}
	dv.paramEditor.OpenValue(ValueOpen{
		Spec:          paramSpec(def),
		Anchor:        anchor,
		MobileInputID: "sampler-param",
		Get: func() float64 {
			return sc.Min + dv.sampler.knobValue(idx)*span
		},
		Set: func(v float64) {
			dv.sampler.knobs[idx].Value = (v - sc.Min) / span
			dv.applySamplerKnob(idx)
			// A typed value is one discrete commit → one undo step.
			dv.commitSamplerEdit()
		},
	})
}

// samplerTabUpdate pumps the shared numeric editor so blur-to-commit works
// in production. Returns true when the editor just closed, signalling
// EQPanelZone.Update to request a re-layout so the caption refreshes.
func (dv *DrumView) samplerTabUpdate() bool {
	// Tick the mobile knob-scroll cooldown clock (see ControlGrid.WheelStep/Tick).
	if dv.sampler.knobGrid != nil {
		dv.sampler.knobGrid.Tick()
	}
	if dv.paramEditor != nil {
		wasActive := dv.paramEditor.Active()
		dv.paramEditor.Update()
		if wasActive && !dv.paramEditor.Active() {
			return true
		}
	}
	return false
}

// samplerHandleDrag maps a cursor X position to a trim fraction for the given
// handle (0=start, 1=end) and keeps start <= end.
func (dv *DrumView) samplerHandleDrag(which, x int) {
	w := dv.sampler.waveformRect
	if w.Dx() <= 0 {
		return
	}
	frac := clampUnit(float64(x-w.Min.X) / float64(w.Dx()))
	s := &dv.sampler
	if which == 0 {
		if frac > s.endFrac {
			frac = s.endFrac
		}
		s.startFrac = frac
		if len(s.knobs) > samplerKnobStart && !s.knobs[samplerKnobStart].Capturing() {
			s.knobs[samplerKnobStart].Value = frac
		}
	} else {
		if frac < s.startFrac {
			frac = s.startFrac
		}
		s.endFrac = frac
		if len(s.knobs) > samplerKnobEnd && !s.knobs[samplerKnobEnd].Capturing() {
			s.knobs[samplerKnobEnd].Value = frac
		}
	}
}

// samplerPreview bakes the current edit, registers it under a transient id, and
// auditions it through the shared playback path.
func (dv *DrumView) samplerPreview() {
	if !dv.sampler.hasBuffer() {
		return
	}
	baked, sr := dv.sampler.bake()
	audio.RegisterSamplePCM(samplerPreviewID, baked, sr)
	samplerAuditionFn(samplerPreviewID)
}

// samplerSave overrides the dropdown-selected instrument in place (converting a
// synth instrument into the chopped sample) and refreshes the instrument
// picker. Mirrors the Synth tab's Save (overwrite current, keep editing it).
func (dv *DrumView) samplerSave() {
	dv.sampler.save()
	// Record an undo step at the user-action site. The sample-edit descriptor's
	// hooks event is published from the audio package (which Import also drives),
	// so it never taps recordUndo on its own. Without this, a Save changed the
	// exported document but left the undo baseline stale — undoing a later,
	// unrelated action would silently revert the sample edit (baseline drift).
	dv.recordUndoStep(hooks.EventSampleEditChanged)
	dv.refreshInstruments()
	dv.markAllRowsDirty()
}

// samplerReset reverts the loaded instrument to its factory state (built-in →
// original synth recipe; user WAV → original buffer) and refreshes the picker.
// Clearing captureID forces ensureSamplerLoaded to re-pull the reverted buffer
// (or synth one-shot) on the next Layout — without it the early-return keeps the
// stale chopped buffer in view.
func (dv *DrumView) samplerReset() {
	dv.ResetActiveSampler()
	dv.sampler.captureID = ""
	dv.refreshInstruments()
	dv.markAllRowsDirty()
}

// samplerSaveAs opens the name-prompt dialog. Mirrors the Synth tab's Save As,
// which prompts for a display name before cloning. The actual save runs in
// samplerSaveAsConfirmed once the user confirms a name.
func (dv *DrumView) samplerSaveAs() {
	dv.openSamplerSaveAsDialog()
}

// samplerSaveAsConfirmed bakes the current edit into a brand-new sample
// instrument named displayName, points the row that played the source
// instrument at the new sample (so it is immediately playable and selectable in
// the dropdown — mirroring the Synth tab's Save As, which rebinds the active
// row to the clone), selects it, and refreshes the picker. displayName arrives
// already trimmed from the dialog.
func (dv *DrumView) samplerSaveAsConfirmed(displayName string) {
	if displayName == "" {
		displayName = samplerSaveAsSuggestion(dv, dv.sampler.captureID)
	}
	old := dv.sampler.captureID
	id := dv.sampler.saveAs(displayName)
	if id == "" {
		return
	}
	// Rebind the row that played the source instrument to the new sample.
	for i, r := range dv.Rows {
		if r != nil && r.Instrument == old {
			dv.selRow = i
			dv.SetInstrument(id)
			dv.Rows[i].Name = displayName
			if i < len(dv.rowLabels()) {
				dv.rowLabels()[i].Text = displayName
			}
			break
		}
	}
	dv.selectAudioChannel(id)
	dv.refreshInstruments()
	dv.markAllRowsDirty()
}

// samplerSaveAsSuggestion proposes a default Save As name: the source row's
// display name with a " chop" suffix, falling back to a pretty-printed id.
func samplerSaveAsSuggestion(dv *DrumView, base string) string {
	label := ""
	for _, r := range dv.Rows {
		if r != nil && r.Instrument == base && r.Name != "" {
			label = r.Name
			break
		}
	}
	if label == "" {
		label = audio.PrettyName(base)
	}
	if label == "" {
		return "Sample"
	}
	return label + " chop"
}

// samplerTabHitAreas publishes hit areas for the knobs, trim handles, and
// buttons. Per-control areas sit at ZEQPanel+2 so they win over both the panel's
// catch-all at ZEQPanel AND the mobile knob-scroll body catch-all at ZEQPanel+1
// (a press on a knob captures the knob; a press on empty knob-column space
// scrolls). The scrollbar thumb sits a tier above the knobs so it wins on overlap.
func (dv *DrumView) samplerTabHitAreas() []HitArea {
	s := &dv.sampler
	scrollBodyZ := ZEQPanel + 1
	z := ZEQPanel + 2
	out := make([]HitArea, 0, len(s.knobs)+len(dv.samplerButtons)+4)

	touchMin := TouchMinTarget()
	clipFor := func(r image.Rectangle) image.Rectangle {
		clip := r.Inset(-8)
		if touchMin > 0 {
			if w := clip.Dx(); w < touchMin {
				grow := (touchMin - w + 1) / 2
				clip.Min.X -= grow
				clip.Max.X += grow
			}
			if h := clip.Dy(); h < touchMin {
				grow := (touchMin - h + 1) / 2
				clip.Min.Y -= grow
				clip.Max.Y += grow
			}
		}
		return clip
	}

	// Mobile knob-scroll: a body catch-all (wheel + touch/drag scroll) over the
	// knob column, plus a scrollbar thumb-drag handle. Only when the single-column
	// list actually overflows. The body sits BELOW the knobs in z so a press on a
	// knob still captures the knob; a press on empty column space scrolls.
	if Profile().IsMobile() && s.knobGrid != nil && s.knobGrid.HasScroll() && !s.knobGridRect.Empty() {
		out = append(out, HitArea{
			Rect:    s.knobGridRect,
			ZIndex:  scrollBodyZ,
			Handler: &samplerKnobScrollAdapter{dv: dv, body: true},
			Tag:     "sampler-scrollbody",
		})
		if thumb := s.knobGrid.Scroll().BarRect(); !thumb.Empty() {
			out = append(out, HitArea{
				Rect:    thumb,
				ZIndex:  z + 1,
				Handler: &samplerKnobScrollAdapter{dv: dv},
				Tag:     "sampler-scroll",
			})
		}
	}

	for i, k := range s.knobs {
		if k == nil || k.Rect().Empty() {
			continue
		}
		out = append(out, HitArea{
			Rect:     k.Rect(),
			ZIndex:   z,
			Handler:  &samplerKnobHitAdapter{dv: dv, idx: i},
			Tag:      fmt.Sprintf("sampler-knob-%d", i),
			Touch:    true,
			ClipRect: clipFor(k.Rect()),
		})
		// Readout (caption) hit area — sits below the dial, outside k.Rect().
		// OnPress checks for readout/badge before starting a drag, so tapping
		// here opens the numeric editor rather than capturing a knob drag.
		if rr := dv.samplerKnobReadoutRect(i); !rr.Empty() {
			out = append(out, HitArea{
				Rect:     rr,
				ZIndex:   z,
				Handler:  &samplerKnobHitAdapter{dv: dv, idx: i},
				Tag:      fmt.Sprintf("sampler-readout-%d", i),
				Touch:    true,
				ClipRect: clipFor(rr),
			})
		}
		// Badge hit area — sits below the caption, outside k.Rect().
		// OnPress handles the cycle; this HitArea makes the badge reachable
		// via the real tree dispatch path.
		if i < len(s.knobStepBadges) {
			if b := s.knobStepBadges[i]; b != nil && !b.Rect().Empty() {
				out = append(out, HitArea{
					Rect:     b.Rect(),
					ZIndex:   z + 1, // above readout so badge wins over readout on overlap
					Handler:  &samplerKnobHitAdapter{dv: dv, idx: i},
					Tag:      fmt.Sprintf("sampler-badge-%d", i),
					Touch:    true,
					ClipRect: clipFor(b.Rect()),
				})
			}
		}
	}

	// Trim handles (only when a buffer is loaded).
	if s.hasBuffer() && !s.waveformRect.Empty() {
		out = append(out, HitArea{
			Rect:     s.startHandleRect,
			ZIndex:   z,
			Handler:  &samplerHandleHitAdapter{dv: dv, which: 0},
			Tag:      "sampler-handle-start",
			Touch:    true,
			ClipRect: clipFor(s.startHandleRect),
		})
		out = append(out, HitArea{
			Rect:     s.endHandleRect,
			ZIndex:   z,
			Handler:  &samplerHandleHitAdapter{dv: dv, which: 1},
			Tag:      "sampler-handle-end",
			Touch:    true,
			ClipRect: clipFor(s.endHandleRect),
		})
	}

	for _, b := range dv.samplerButtons {
		if b.btn == nil || b.btn.Rect().Empty() {
			continue
		}
		// Disabled buttons are inert: keep them out of the hit index so a
		// tap on a greyed-out Save/Preview does nothing (matches the empty-
		// state contract that the panel presents no dead affordances).
		if b.btn.SpecID == ComponentButtonDisabled {
			continue
		}
		out = append(out, HitArea{
			Rect:    b.btn.Rect(),
			ZIndex:  z,
			Handler: &buttonHitAdapter{btn: b.btn},
			Tag:     b.tag,
			Touch:   true,
		})
	}

	// Save As name dialog hit areas (shared synthSaveAsDialog). The OK/Cancel
	// adapters route through Confirm/CancelSaveAsDialog, which dispatch to the
	// dialog's confirm closure — set to the sampler save in openSamplerSaveAsDialog.
	out = append(out, dv.saveAsDialogHitAreas(z+1)...)
	return out
}

// ── Input adapters ───────────────────────────────────────────────────────────

type samplerKnobHitAdapter struct {
	dv     *DrumView
	idx    int
	active bool
}

func (h *samplerKnobHitAdapter) knob() *Knob {
	if h.dv == nil || h.idx < 0 || h.idx >= len(h.dv.sampler.knobs) {
		return nil
	}
	return h.dv.sampler.knobs[h.idx]
}

func (h *samplerKnobHitAdapter) OnPress(x, y int) InputResult {
	// Numeric editor open: swallow the tap so a knob drag doesn't start
	// beneath the open editor.
	if h.dv.paramEditor != nil && h.dv.paramEditor.Active() {
		return InputConsumed
	}
	// Mobile: a tap on the knob cell opens the vertical scroll-wheel popup,
	// which owns value + resolution + numeric entry. Desktop keeps the in-place
	// rotary drag (the code below). Mirrors synthKnobHitAdapter.OnPress.
	if Profile().IsMobile() {
		h.dv.openSamplerKnobWheelPopup(h.idx)
		return InputConsumed
	}
	// Step-badge pill: a tap cycles the resolution step instead of dragging.
	if h.idx < len(h.dv.sampler.knobStepBadges) {
		if b := h.dv.sampler.knobStepBadges[h.idx]; b != nil && !b.Rect().Empty() && image.Pt(x, y).In(b.Rect()) {
			h.dv.cycleSamplerKnobStep(h.idx)
			return InputConsumed
		}
	}
	// Value readout (caption line): a tap opens the numeric editor.
	if rr := h.dv.samplerKnobReadoutRect(h.idx); !rr.Empty() && image.Pt(x, y).In(rr) {
		h.dv.openSamplerParamEditor(h.idx)
		return InputConsumed
	}
	k := h.knob()
	if k == nil {
		return InputIgnored
	}
	if k.HandleInputResult(x, y, true) != InputIgnored {
		h.active = true
		h.dv.applySamplerKnob(h.idx)
		return InputCaptured
	}
	return InputIgnored
}

func (h *samplerKnobHitAdapter) OnDrag(x, y int) {
	if !h.active {
		return
	}
	k := h.knob()
	if k == nil {
		h.active = false
		return
	}
	k.HandleInputResult(x, y, true)
	h.dv.applySamplerKnob(h.idx)
}

func (h *samplerKnobHitAdapter) OnRelease(x, y int) {
	if !h.active {
		return
	}
	if k := h.knob(); k != nil {
		k.HandleInputResult(x, y, false)
		h.dv.applySamplerKnob(h.idx)
	}
	h.active = false
	// One undo step per gesture, committed here on release (the per-frame drag
	// only staged the value).
	h.dv.commitSamplerEdit()
}

func (h *samplerKnobHitAdapter) OnWheel(x, y, steps int) InputResult {
	k := h.knob()
	if k == nil {
		return InputIgnored
	}
	// Scrolling while hovering the step-RESOLUTION badge shifts its rung
	// (scroll up = finer, down = coarser). The knob itself is left untouched.
	s := &h.dv.sampler
	if h.idx >= 0 && h.idx < len(s.knobStepBadges) {
		if b := s.knobStepBadges[h.idx]; b != nil && !b.Rect().Empty() && image.Pt(x, y).In(b.Rect()) {
			h.dv.stepSamplerKnobResolution(h.idx, steps)
			return InputConsumed
		}
	}
	// Anywhere else (the knob dial): ORIGINAL behavior — wheel turns the knob.
	res := k.HandleWheel(x, y, steps)
	if res == InputConsumed {
		h.dv.applySamplerKnob(h.idx)
	}
	return res
}

// OnWheel2D handles a two-finger trackpad drag with the raw axes. Over the
// step-resolution badge the scroll shifts the rung. Over the knob body a
// LEFT/RIGHT (horizontal-dominant) scroll changes the value — mirroring a
// left/right mouse drag. An UP/DOWN scroll is reserved for overflow rows; the
// sampler has none, so vertical scroll is a no-op (it never nudges the value).
func (h *samplerKnobHitAdapter) OnWheel2D(x, y, dx, dy int) InputResult {
	k := h.knob()
	if k == nil {
		return InputIgnored
	}
	s := &h.dv.sampler
	if h.idx >= 0 && h.idx < len(s.knobStepBadges) {
		if b := s.knobStepBadges[h.idx]; b != nil && !b.Rect().Empty() && image.Pt(x, y).In(b.Rect()) {
			steps := dy
			if steps == 0 {
				steps = dx
			}
			h.dv.stepSamplerKnobResolution(h.idx, steps)
			return InputConsumed
		}
	}
	if wheelAxisHorizontal(dx, dy) {
		// Debounced + magnitude-insensitive (slow, human-paced); per-step amount
		// is StepMul (the user's chosen resolution).
		if k.StepValueByWheel(dx) {
			h.dv.applySamplerKnob(h.idx)
		}
		return InputConsumed
	}
	// Vertical: no overflow rows to scroll on the sampler — consume as a no-op
	// so it neither turns the knob nor surprises with a page scroll.
	return InputConsumed
}

type samplerHandleHitAdapter struct {
	dv     *DrumView
	which  int
	active bool
}

func (h *samplerHandleHitAdapter) OnPress(x, y int) InputResult {
	h.active = true
	h.dv.samplerHandleDrag(h.which, x)
	return InputCaptured
}

func (h *samplerHandleHitAdapter) OnDrag(x, y int) {
	if h.active {
		h.dv.samplerHandleDrag(h.which, x)
	}
}

func (h *samplerHandleHitAdapter) OnRelease(x, y int) {
	h.active = false
	// One undo step for the whole trim-handle drag, committed on release.
	h.dv.commitSamplerEdit()
}
func (h *samplerHandleHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// samplerKnobScrollAdapter routes scroll input for the mobile single-column knob
// list to the sampler's ControlGrid. With body == true it is the card-body
// catch-all (a grab on empty column space) and scrolls naturally (content follows
// the finger, BeginContentDrag); with body == false it is the scrollbar thumb and
// scrolls directly (thumb follows the finger, BeginDrag). A scroll change calls
// Invalidate so the next Layout re-derives the visible knob rects. Mirrors
// synthSectionScrollAdapter.
type samplerKnobScrollAdapter struct {
	dv   *DrumView
	body bool
}

func (h *samplerKnobScrollAdapter) grid() *ControlGrid {
	if h.dv == nil {
		return nil
	}
	return h.dv.sampler.knobGrid
}

func (h *samplerKnobScrollAdapter) invalidate() {
	if h.dv != nil && h.dv.eqPanelZone != nil {
		h.dv.eqPanelZone.Invalidate()
	}
}

func (h *samplerKnobScrollAdapter) OnPress(x, y int) InputResult {
	g := h.grid()
	if g == nil {
		return InputIgnored
	}
	began := false
	if h.body {
		began = g.BeginContentDrag(y)
	} else {
		began = g.BeginDrag(y)
	}
	if began {
		return InputCaptured
	}
	return InputIgnored
}

func (h *samplerKnobScrollAdapter) OnDrag(x, y int) {
	if g := h.grid(); g != nil && g.DragTo(y) {
		h.invalidate()
	}
}

func (h *samplerKnobScrollAdapter) OnRelease(x, y int) {
	if g := h.grid(); g != nil {
		g.EndDrag()
	}
}

func (h *samplerKnobScrollAdapter) OnWheel(x, y, steps int) InputResult {
	g := h.grid()
	if g == nil {
		return InputIgnored
	}
	if g.WheelStep(steps) {
		h.invalidate()
	}
	return InputConsumed
}

// Sampler footer buttons route through the shared buttonHitAdapter (see
// eq_panel_zone.go) like every other action button — no bespoke adapter.
