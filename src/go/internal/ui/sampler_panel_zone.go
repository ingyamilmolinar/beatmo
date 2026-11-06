package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The Sampler tab. A waveform-prominent editor: a full-width waveform with
// draggable Start/End trim handles on top, and one control area beneath with
// Start/End/Transpose/Detune/Gain knobs plus Reverse/Normalize/Fade toggles
// and a Preview button. The header carries Save / Save As (the dropdown
// auto-loads the selected instrument — the old "From Synth" and "Load WAV"
// buttons are gone; WAV loading now lives only on the master panel's kebab
// Upload action). All sample logic lives on samplerState (sampler_state.go);
// this file owns only layout, drawing, and input.

const samplerPreviewID instrument.ID = "preview.sampler"

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
// Delegates to audioTabActionButtonWidth (shared with the Synth header).
func samplerBtnWidth(label string) int {
	return audioTabActionButtonWidth(label)
}

// samplerAuditionFn triggers playback of the preview instrument. Swappable in
// tests via SwapSamplerAuditionFnForTest, mirroring synthAuditionFn.
var samplerAuditionFn = func(instID instrument.ID) { audio.Play(instID) }

// SwapSamplerAuditionFnForTest installs a test fake for the preview trigger and
// returns the previous value.
func SwapSamplerAuditionFnForTest(fn func(instrument.ID)) func(instrument.ID) {
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
func samplerTriggerNanos(id instrument.ID) int64 {
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
func (p *samplerPanel) updateSamplerPlayheads() {
	dv := p.dv
	s := &dv.sampler
	dur := s.audibleDurationSeconds()
	if !s.hasBuffer() || dur <= 0 {
		s.playheads.reset()
		return
	}
	triggers := map[instrument.ID]int64{samplerPreviewID: samplerTriggerNanos(samplerPreviewID)}
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
func (p *samplerPanel) samplerActivePlayheads() []samplerPlayheadDraw {
	dv := p.dv
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
func (p *samplerPanel) samplerActiveInstrument() instrument.ID {
	dv := p.dv
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
func (p *samplerPanel) resyncSamplerEditFromDocument() {
	dv := p.dv
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

func (p *samplerPanel) ensureSamplerLoaded(instID instrument.ID) {
	dv := p.dv
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
	// A saved sample-edit descriptor on an instrument with a DOCUMENT means it is
	// a SYNTH with a non-destructive edit: load the RAW synth render and overlay
	// the saved edit. (An instrument without one with a descriptor is a user
	// sample, handled above.)
	if e, hasEdit := audio.SampleEditFor(instID); hasEdit && audio.HasInstrumentDoc(instID) {
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
	// An instrument document is what makes an instrument a SYNTH (the
	// descriptor is applied to the fresh synth render at trigger time). A
	// file-picker WAV (audio.RegisterWAV) is a raw PCM Sample with NO document
	// and no user-sample store entry, so it is a WAV source. Seed its pristine PCM into the store so
	// the real-time edit path (audio.SetSampleEdit → reapplyUserSampleEdit) has a
	// source to bake from — this is what makes trim/reverse/etc audible
	// immediately, without waiting for Save.
	if !audio.HasInstrumentDoc(instID) {
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
func (p *samplerPanel) buildSamplerTab(contentR image.Rectangle, instID instrument.ID) {
	dv := p.dv
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
	if mobile {
		// Mobile drops the "SAMPLER" title band (redundant with the highlighted
		// Smpl nav tab + the channel pill) and relocates Save/Save As/Reset to
		// the sticky bar, so the header reserves no height — the waveform rises
		// to the top. An empty headerRect makes drawSamplerTab skip the title.
		headerH = 0
	}
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
func (p *samplerPanel) clearSamplerKnobRects() {
	dv := p.dv
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
func (p *samplerPanel) clearSamplerControlButtonRects() {
	dv := p.dv
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
func (p *samplerPanel) samplerControlMinH(dv2 densityValues) int {
	dv := p.dv
	_ = dv
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
	labelBandH := TextHeight() + SpaceXS // group-label band above the card
	cellH := labelBandH + dv2.ParamCardH // one ParamCard slot per control
	// +SpaceSM slack so the budget lands comfortably above the ideal after the
	// waveform 40% cap and split rounding take their cut. ONE button row now:
	// only the Rev/Norm/Fade transforms row stays in-panel — Preview/Save/⋯
	// moved to the sticky bar (Approach B).
	return cellH + SpaceSM + dv2.SynthHeaderButtonH + SpaceXS + TextHeight() + 2*SpaceXS
}

// layoutSamplerHeaderButtons right-aligns Save / Save As / Reset in the
// header. On mobile the header is too narrow, so Save / Save As / Reset move
// to the action row beneath the knobs (layoutSamplerControlButtons) and their
// header rects are cleared. Every button is sized to fit its label.
func (p *samplerPanel) layoutSamplerHeaderButtons(hdr image.Rectangle, btnH int, mobile bool) {
	dv := p.dv
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
func (p *samplerPanel) layoutSamplerControls(ctrlR image.Rectangle, dv2 densityValues, mobile bool) {
	dv := p.dv
	s := &dv.sampler
	if ctrlR.Empty() {
		dv.clearSamplerKnobRects()
		dv.clearSamplerControlButtonRects()
		s.metaRect = image.Rectangle{}
		return
	}
	labelH := TextHeight()
	btnH := dv2.SynthHeaderButtonH
	metaH := TextHeight()

	// Metadata strip pinned to the bottom, toggle row above it.
	metaR := image.Rect(ctrlR.Min.X, ctrlR.Max.Y-metaH, ctrlR.Max.X, ctrlR.Max.Y)
	toggleY1 := metaR.Min.Y - SpaceXS
	toggleY0 := toggleY1 - btnH
	s.metaRect = metaR

	if mobile {
		// One FIXED full-width row: the Rev/Norm/Fade transforms row pinned
		// above the metadata. The Preview/Save/Save As/Reset actions moved to
		// the sticky bar (Approach B — see MobileActionButtons), so the
		// reclaimed action-row height goes to the knob grid.
		gridBottomM := toggleY0 - SpaceXS
		dv.layoutSamplerKnobsScrolling(image.Rect(ctrlR.Min.X, ctrlR.Min.Y, ctrlR.Max.X, gridBottomM), dv2)
		dv.layoutSamplerMobileToggleRow(image.Rect(ctrlR.Min.X, toggleY0, ctrlR.Max.X, toggleY1))
		return
	}

	// LANGUAGE-INVARIANT knob grid. Every knob is now a ParamCard (name + value +
	// gauge) on ALL platforms — the same universal control the Synth tab uses.
	// Desktop lays all five cards in one side-by-side row; each cell reserves a
	// group-label band above the card. The card is a fixed ParamCardH tall and
	// its width depends only on the cell width + density tokens (never on caption
	// length), so it is identical in every language.
	gridTop := ctrlR.Min.Y

	// Desktop: all five cards in one side-by-side row (the wide control column
	// has room). No scrolling. (mobile is handled above and returns.)
	cellW := ctrlR.Dx() / samplerKnobCount
	cardH := dv2.ParamCardH
	// Cards may stretch a bit in the wide column, but never fill the whole cell
	// (min(cellW-SpaceSM, ParamCardW*3/2)); floored at ParamCardW/2 so a narrow
	// column never collapses the card to an unusable sliver.
	cardW := cellW - SpaceSM
	if maxW := dv2.ParamCardW * 3 / 2; cardW > maxW {
		cardW = maxW
	}
	if minW := dv2.ParamCardW / 2; cardW < minW {
		cardW = minW
	}
	// Hard cap: the card never overhangs its own cell into a neighbour, even
	// when the min-width floor exceeds a degenerate-narrow column.
	if cardW > cellW && cellW > 0 {
		cardW = cellW
	}
	cardTop := gridTop + labelH + SpaceXS
	cellBottom := cardTop + cardH
	for i, k := range s.knobs {
		if k == nil {
			continue
		}
		cellX0 := ctrlR.Min.X + i*cellW
		cardX0 := cellX0 + (cellW-cardW)/2
		// Knob rect = the card rect (label band excluded). The wider CELL is
		// tracked separately so the group label centres in the full cell.
		k.SetRect(image.Rect(cardX0, cardTop, cardX0+cardW, cardTop+cardH))
		s.knobCells[i] = image.Rect(cellX0, gridTop, cellX0+cellW, cellBottom)
		// Capturing() is always false now (the card no longer drives an in-place
		// drag), so this value sync always runs; kept unconditional for clarity.
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
		s.knobGrid = NewControlGrid(controlGridScrollbarStyle())
	}
	return s.knobGrid
}

// layoutSamplerKnobsScrolling lays the five knobs out in a width-adaptive
// grid (ControlGrid's native column selection, capped at 4) inside gridR,
// scrolling when the rows overflow. Each cell reserves a band above the
// control for the group label (TRIM/TUNE/LEVEL), the control band at the
// density ideal, and a caption band below. Off-window knobs get an empty rect
// so drawSamplerKnobs / samplerTabHitAreas skip them. Mirrors the Synth tab's
// ControlGrid usage (same 4-column mobile cap).
func (p *samplerPanel) layoutSamplerKnobsScrolling(gridR image.Rectangle, dv2 densityValues) {
	dv := p.dv
	s := &dv.sampler
	grid := s.ensureKnobGrid()
	s.knobGridRect = gridR

	labelBandH := TextHeight() + SpaceXS // group-label band above the card
	cellH := labelBandH + dv2.ParamCardH + SpaceXS
	if cellH > gridR.Dy() && gridR.Dy() > 0 {
		cellH = gridR.Dy() // degrade gracefully on a tiny panel (one shrunk row)
	}
	if cellH < 1 {
		cellH = 1
	}
	grid.SetMaxCols(4)
	grid.Layout(gridR, samplerKnobCount, dv2.ParamCardW, cellH, SpaceSM, SpaceXS)

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
		// Card fills the slot width, sitting below the group-label band. Fixed
		// ParamCardH tall (clamped to what the slot can hold on a tiny panel).
		cardH := dv2.ParamCardH
		if maxH := slot.Dy() - labelBandH; cardH > maxH && maxH > 1 {
			cardH = maxH
		}
		k.SetRect(image.Rect(slot.Min.X, slot.Min.Y+labelBandH, slot.Max.X, slot.Min.Y+labelBandH+cardH))
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
func (p *samplerPanel) layoutSamplerControlButtons(rowR image.Rectangle) {
	dv := p.dv
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

// layoutSamplerMobileToggleRow lays the sampler's ONE mobile in-panel button
// row — the Rev/Norm/Fade transforms — as a fixed full-width row of equal-width
// cells (last cell claims the remainder so rounding slack never gaps). The
// Preview/Save/Save As/Reset action buttons are relocated to the sticky bar on
// mobile (Approach B — see MobileActionButtons), so their in-panel rects are
// cleared here.
func (p *samplerPanel) layoutSamplerMobileToggleRow(toggleR image.Rectangle) {
	dv := p.dv
	// Actions live on the sticky bar on mobile — clear any stale in-panel rect.
	for _, tag := range []string{"sampler-preview", "sampler-save", "sampler-save-as", "sampler-reset"} {
		if b := dv.samplerButtonByTag(tag); b != nil {
			b.SetRect(image.Rectangle{})
		}
	}
	tags := []string{"sampler-reverse", "sampler-normalize", "sampler-fade"}
	n := len(tags)
	if n == 0 || toggleR.Dx() <= 0 {
		return
	}
	cellW := (toggleR.Dx() - (n-1)*SpaceSM) / n
	if cellW < 1 {
		cellW = 1
	}
	x := toggleR.Min.X
	for i, tag := range tags {
		b := dv.samplerButtonByTag(tag)
		if b == nil {
			continue
		}
		x1 := x + cellW
		if i == n-1 {
			x1 = toggleR.Max.X // last cell claims the remainder
		}
		b.SetRect(image.Rect(x, toggleR.Min.Y, x1, toggleR.Max.Y))
		x = x1 + SpaceSM
	}
}

// samplerHandleRect returns the thin vertical drag-handle rect for a trim
// fraction over the current waveform rect.
func (p *samplerPanel) samplerHandleRect(frac float64) image.Rectangle {
	dv := p.dv
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

// updateSamplerHandleHover refreshes which Start/End trim handle the desktop
// cursor is over (samplerState.hoverHandle: -1 none, 0 start, 1 end), so the
// hovered handle can bloom in its own color — the same desktop-only hover
// language as the EQ handles and grid nodes. Idle on mobile (no pointer hover)
// and when no buffer is loaded. Uses the SAME samplerHandleRect the handle is
// drawn and hit-tested with, so the hover target matches the grab target.
func (p *samplerPanel) updateSamplerHandleHover() {
	s := &p.dv.sampler
	s.hoverHandle = -1
	if Profile().IsMobile() || !s.hasBuffer() || s.waveformRect.Empty() {
		return
	}
	mx, my := cursorPosition()
	pt := image.Pt(mx, my)
	if pt.In(p.samplerHandleRect(s.startFrac)) {
		s.hoverHandle = 0
	} else if pt.In(p.samplerHandleRect(s.endFrac)) {
		s.hoverHandle = 1
	}
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
func (p *samplerPanel) buildSamplerButtons(hasBuf bool) {
	dv := p.dv
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
			dv.sampler.toggleReverse()
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
		{NewSpecButton(i18n.T(i18n.KeySaveAs), actionSpec(true), guard(func() {
			dv.samplerSaveAs()
		})), "sampler-save-as", nil},
	}
	for _, d := range defs {
		d.btn.SetTextRole(RoleBody)
	}
	dv.samplerButtons = defs
	dv.refreshSamplerToggleStates()
}

// refreshSamplerToggleStates syncs each Sampler toggle's latched visual (the
// shared amber engaged keycap drawn by Button.Draw when Toggled) to its live
// on/off predicate, so Reverse / Normalize / Fade read their state identically
// on every platform. Action buttons (active==nil) and the empty-buffer state
// never latch. Called on every (re)build and once per frame from drawSamplerTab.
func (p *samplerPanel) refreshSamplerToggleStates() {
	dv := p.dv
	hasBuf := dv.sampler.hasBuffer()
	for _, b := range dv.samplerButtons {
		b.btn.SetToggled(b.active != nil && hasBuf && b.active())
	}
}

func (p *samplerPanel) samplerButtonByTag(tag string) *Button {
	dv := p.dv
	for _, b := range dv.samplerButtons {
		if b.tag == tag {
			return b.btn
		}
	}
	return nil
}

// drawSamplerTab renders the sampler header, waveform with trim handles, knobs,
// and buttons.
func (p *samplerPanel) drawSamplerTab(dst *ebiten.Image, contentR image.Rectangle) {
	dv := p.dv
	s := &dv.sampler
	hasBuf := s.hasBuffer()

	// Header background + title.
	if !s.headerRect.Empty() {
		drawRoundedRect(dst, s.headerRect, TokenSurface1(), fitRadius(RadiusSM, s.headerRect), true)
		tx := s.headerRect.Min.X + samplerTitlePad
		maxW := s.headerRect.Max.X - tx
		for _, tag := range []string{"sampler-save", "sampler-save-as", "sampler-reset"} {
			if b := dv.samplerButtonByTag(tag); b != nil && !b.Rect().Empty() && b.Rect().Min.X-SpaceMD-tx < maxW {
				maxW = b.Rect().Min.X - SpaceMD - tx
			}
		}
		drawAudioTabHeaderTitle(dst, s.headerRect, tx, maxW, i18n.T(i18n.KeyCapSamplerTitle), audioTabHeaderName(dv.samplerActiveInstrument()))
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
			// Edge fades over the reserved peek band signal more controls below.
			drawControlGridEdgeFades(dst, s.knobGrid, TokenSurface1())
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

	// Save As name-prompt dialog floats above the tab content (shared with the
	// Synth tab; only one can be open at a time).
	dv.drawSaveAsDialog(dst)
}

// drawSamplerPlaceholder paints the empty-state prompt centred in the card,
// falling back to a shorter message (and surfacing s.status) so the text
// never clips the card edges.
func (p *samplerPanel) drawSamplerPlaceholder(dst *ebiten.Image, card image.Rectangle) {
	dv := p.dv
	s := &dv.sampler
	avail := card.Dx() - 2*SpaceMD
	msg := "Pick an instrument from the dropdown"
	if StyledTextWidth(msg, RoleBody) > avail {
		msg = "Capture or load a sample"
	}
	if StyledTextWidth(msg, RoleBody) > avail {
		msg = "No sample"
	}
	bodyH := StyledTextHeight(RoleBody)
	y := card.Min.Y + (card.Dy()-bodyH)/2
	if s.status != "" {
		y -= (bodyH + SpaceXS) / 2
	}
	DrawTextStyled(dst, msg, card.Min.X+(card.Dx()-StyledTextWidth(msg, RoleBody))/2, y, RoleBody, colTextSecondary)
	if s.status != "" {
		st := truncCaptionStyled(s.status, avail, RoleCaption)
		DrawTextStyled(dst, st, card.Min.X+(card.Dx()-StyledTextWidth(st, RoleCaption))/2, y+bodyH+SpaceXS, RoleCaption, colError)
	}
}

// samplerMetaText formats the length / sample-rate readout. The "of"/"de"
// joiner follows the active locale; units (s, kHz) are international.
func samplerMetaText(trimSec, lengthSec, srKHz float64) string {
	return i18n.Tf(i18n.KeySamplerMetaFmt, trimSec, lengthSec, srKHz)
}

// drawSamplerMeta renders the length / sample-rate readout.
func (p *samplerPanel) drawSamplerMeta(dst *ebiten.Image, r image.Rectangle) {
	dv := p.dv
	s := &dv.sampler
	sr := s.rawSampleRate
	txt := samplerMetaText(s.trimSeconds(), s.lengthSeconds(), float64(sr)/1000)
	DrawTextStyled(dst, txt, r.Min.X, r.Min.Y+(r.Dy()-StyledTextHeight(RoleCaption))/2, RoleCaption, WithAlpha(TokenTextSecondary(), AlphaMedium))
}

// drawSamplerKnobs draws each laid-out knob with its group label (above the
// cluster) and its caption (value + plain-English helper) below. It also
// draws the step badge pill beneath the caption and records the caption
// hit-rect so samplerKnobReadoutRect can be used by OnPress and HitAreas.
func (p *samplerPanel) drawSamplerKnobs(dst *ebiten.Image) {
	dv := p.dv
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
			labelY := r.Min.Y - SpaceXS - StyledTextHeight(RoleCaption)
			if labelY < cell.Min.Y {
				labelY = cell.Min.Y
			}
			DrawTextStyled(dst, g, cell.Min.X, labelY, RoleCaption, TokenTextSecondary())
			gloss := samplerKnobPlainEnglish(i)
			glossX := cell.Min.X + StyledTextWidth(g, RoleCaption) + SpaceSM
			glossW := StyledTextWidth(gloss, RoleMicro)
			// Skip the gloss if it would clip past the cell's right edge.
			if gloss != "" && glossX+glossW <= cell.Max.X {
				DrawTextStyled(dst, gloss, glossX, labelY+2, RoleMicro, WithAlpha(TokenTextSecondary(), AlphaMedium))
			}
		}
		// The knob renders as a single ParamCard (name + value + range gauge) on
		// ALL platforms — the same universal control the Synth tab uses. A tap
		// anywhere on it opens the value-wheel popup; on desktop the mouse wheel
		// nudges the value in place. The old mobile value-pill + desktop rotary
		// dial + separate caption + step-badge stack are retired; the card's
		// gauge carries what the step badge used to show.
		drawParamCard(dst, r, samplerKnobNameText(i, s), samplerKnobValueText(i, s),
			paramCardGaugeFor(k, dv.samplerKnobDef(i)), paramCardState{})
		s.readoutRects[i] = image.Rectangle{}
	}
}

// drawSamplerWaveform paints the captured/loaded buffer with the kept (trim)
// region highlighted and the Start/End drag handles.
func (p *samplerPanel) drawSamplerWaveform(dst *ebiten.Image) {
	dv := p.dv
	s := &dv.sampler
	w := s.waveformRect
	// Reuse the cached float64 scratch (no per-frame allocation).
	wave := s.waveFloat64()
	midY := w.Min.Y + w.Dy()/2
	// The sample waveform is a DATA canvas: Visualizers family (viz-wave-a),
	// like the Wave tab trace and the synth graphs — not the chrome accent.
	drawWaveTrace(dst, wave, w, midY, w.Dx(), colWaveTrace, 1.0, TokenSurface3())

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
	// Handles — the hovered one blooms in its OWN color (drawNodeBloom), the
	// same own-color hover language as the EQ handles and grid nodes.
	p.drawSamplerHandle(dst, dv.samplerHandleRect(s.startFrac), s.hoverHandle == 0)
	p.drawSamplerHandle(dst, dv.samplerHandleRect(s.endFrac), s.hoverHandle == 1)

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

// drawSamplerHandle paints one Start/End trim handle. When hovered it adds an
// own-color highlight that CONFORMS to the handle's actual rect via the shared,
// shape-agnostic drawRectHoverGlow — so a tall thin trim line highlights as a
// tall thin line, never a centred node blob. (drawNodeBloom assumes a square
// node and bulged in the middle of the vertical line.)
func (p *samplerPanel) drawSamplerHandle(dst *ebiten.Image, r image.Rectangle, hovered bool) {
	if r.Empty() {
		return
	}
	drawRect(dst, r, colAccent, true)
	if hovered {
		drawRectHoverGlow(dst, r, colAccentBright, 1.0)
	}
}

// samplerKnobCaption returns the short value readout shown beneath each knob.
// Label and value both come from the shared paramFacet registry (sampleEditFacet)
// — the SAME source an undo/redo notification reads — so a Sampler caption and a
// notification describing the same edit can never drift. Captions still read
// coherently with the Synth tab (Pitch "+0 st", Fine "+0 c", Gain "+0 dB")
// because the facet formatters delegate to the shared formatParamValue.
func samplerKnobCaption(idx int, s *samplerState) string {
	name := samplerKnobNameText(idx, s)
	value := samplerKnobValueText(idx, s)
	if name == "" {
		return ""
	}
	return name + " " + value
}

// samplerKnobNameText returns just the parameter name ("Start", "Pitch").
// The mobile caption band uses it: the value pill above already shows the
// value, so a name+value caption rendered the same string twice (the
// "Start 0% / Start 0%" duplicate — A9, 2026-07-04 critique).
func samplerKnobNameText(idx int, s *samplerState) string {
	key, _, ok := samplerKnobEditField(idx, s)
	if !ok {
		return ""
	}
	f, ok := sampleEditFacet(key)
	if !ok {
		return ""
	}
	return f.renderLabel()
}

// samplerKnobValueText returns just the formatted value ("15 %", "+0 st").
// The mobile value pill shows it — matching the Synth tab's pill, which
// also carries the value alone.
func samplerKnobValueText(idx int, s *samplerState) string {
	key, val, ok := samplerKnobEditField(idx, s)
	if !ok {
		return ""
	}
	f, ok := sampleEditFacet(key)
	if !ok {
		return ""
	}
	return f.format(val)
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
func (p *samplerPanel) applySamplerKnob(idx int) {
	dv := p.dv
	if idx < 0 || idx >= len(dv.sampler.knobs) {
		return
	}
	dv.sampler.setKnob(idx, dv.sampler.knobs[idx].Value)
	// Audible NOW, not at the end of the gesture: setKnob writes only the UI's
	// copy of the edit, so without this the knob moved on screen while the audio
	// layer kept the old chop until commitSamplerEdit ran (Enter / tap-away /
	// the 600 ms wheel debounce). The undo step stays one-per-gesture.
	dv.applySamplerEditLive()
}

// samplerKnobReadoutRect returns the caption hit-rect for the given sampler
// knob index. Returns the zero rectangle when the index is out of range or
// the rect has not been populated yet (no Draw pass has occurred).
func (p *samplerPanel) samplerKnobReadoutRect(idx int) image.Rectangle {
	dv := p.dv
	if idx < 0 || idx >= samplerKnobCount {
		return image.Rectangle{}
	}
	return dv.sampler.readoutRects[idx]
}

// samplerKnobDef synthesizes the audio.ParamDef for sampler knob idx from its
// KnobScale — the single source of truth shared by the value-wheel popup, the
// numeric editor, and the ParamCard gauge. The sampler has no per-knob binding
// array (every knob is continuous), so the Def is derived, not stored.
func (dv *DrumView) samplerKnobDef(idx int) audio.ParamDef {
	sc := samplerKnobScale(idx)
	return audio.ParamDef{Name: samplerStepPrefName(idx), Min: sc.Min, Max: sc.Max, Unit: sc.Unit}
}

// samplerTabUpdate is the Sampler tab's per-frame tick (handle hover, the
// debounced wheel-nudge commit, the knob-grid scroll cooldown). Returns true
// when the tab needs a re-layout.
func (p *samplerPanel) samplerTabUpdate() bool {
	dv := p.dv
	// Refresh which trim handle the cursor is hovering (desktop only) so the
	// hovered handle blooms in its own color, next to the wheel-nudge tick.
	p.updateSamplerHandleHover()
	// Advance the debounced desktop wheel-nudge commit once per frame while the
	// Sampler tab is active (mirrors synthWheelNudge.tick in SynthTabUpdate).
	dv.samplerWheelNudge.tick()
	// Tick the mobile knob-scroll cooldown clock (see ControlGrid.WheelStep/Tick).
	if dv.sampler.knobGrid != nil {
		dv.sampler.knobGrid.Tick()
	}
	return false
}

// samplerHandleDrag maps a cursor X position to a trim fraction for the given
// handle (0=start, 1=end) and keeps start <= end.
func (p *samplerPanel) samplerHandleDrag(which, x int) {
	dv := p.dv
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
	// The trim moves in audio WHILE dragging, not on release — same reason as
	// applySamplerKnob. ApplySampleEditLive skips an unchanged descriptor, so a
	// frame that did not move the handle costs nothing.
	dv.applySamplerEditLive()
}

// samplerPreview bakes the current edit, registers it under a transient id, and
// auditions it through the shared playback path.
func (p *samplerPanel) samplerPreview() {
	dv := p.dv
	if !dv.sampler.hasBuffer() {
		return
	}
	baked, sr := dv.sampler.bake()
	audio.RegisterSamplePCM(samplerPreviewID, baked, sr)
	samplerAuditionFn(samplerPreviewID)
}

// samplerSaveAs opens Save As — the same dialog as the Synth tab's — for the
// instrument the Sampler edits. Its Sampler edit is saved as data in the
// instrument's audio bundle; nothing is baked.
func (p *samplerPanel) samplerSaveAs() {
	p.dv.openSaveAsDialogFor(p.dv.sampler.captureID)
}

// samplerTabHitAreas publishes hit areas for the knobs, trim handles, and
// buttons. Per-control areas sit at ZEQPanel+2 so they win over both the panel's
// catch-all at ZEQPanel AND the mobile knob-scroll body catch-all at ZEQPanel+1
// (a press on a knob captures the knob; a press on empty knob-column space
// scrolls). The scrollbar thumb sits a tier above the knobs so it wins on overlap.
func (p *samplerPanel) samplerTabHitAreas() []HitArea {
	dv := p.dv
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
		// The whole card is one tap target: a press anywhere opens the value-wheel
		// popup (OnPress), a desktop wheel notch nudges the value (OnWheel). The
		// old separate readout / step-badge hit areas are retired with the pill.
		out = append(out, HitArea{
			Rect:     k.Rect(),
			ZIndex:   z,
			Handler:  &samplerKnobHitAdapter{dv: dv, idx: i},
			Tag:      fmt.Sprintf("sampler-knob-%d", i),
			Touch:    true,
			ClipRect: clipFor(k.Rect()),
		})
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
	dv  *DrumView
	idx int
	arb dragScrollArbiter // tap-vs-scroll gate for the current gesture
}

func (h *samplerKnobHitAdapter) knob() *Knob {
	if h.dv == nil || h.idx < 0 || h.idx >= len(h.dv.sampler.knobs) {
		return nil
	}
	return h.dv.sampler.knobs[h.idx]
}

// glowRect implements glowTarget: hovering a sampler param card lifts it through
// the shared tree hover overlay like every other clickable. The whole card is
// the hover target (a tap anywhere on it opens the value-wheel popup).
func (h *samplerKnobHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	return hitRect
}

// OnPress opens the value-wheel popup on EVERY platform (the old desktop rotary
// drag + step-badge tap + readout-editor tap are retired; resolution + numeric
// entry now live in the popup). Flush any pending wheel-nudge commit first so
// the popup snapshots the committed value. Mirrors synthControlHitAdapter.OnPress.
func (h *samplerKnobHitAdapter) OnPress(x, y int) InputResult {
	// Capture the gesture and begin tap-vs-scroll arbitration (mirrors the Synth
	// tab): a vertical sweep scrolls the knob grid, a stationary tap opens the
	// value-wheel popup on RELEASE. Capturing (not consuming) is what lets the
	// sweep become a scroll instead of opening the popup on the down frame.
	h.arb.begin(y, h.dv.sampler.knobGrid)
	return InputCaptured
}

// OnDrag advances the knob grid while the finger sweeps; once travel crosses the
// tap slop the gesture is a scroll and the release will NOT open the popup.
func (h *samplerKnobHitAdapter) invalidate() {
	if h.dv != nil && h.dv.eqPanelZone != nil {
		h.dv.eqPanelZone.Invalidate()
	}
}

func (h *samplerKnobHitAdapter) OnDrag(x, y int) {
	if h.arb.drag(y) {
		h.invalidate()
	}
}

// OnRelease opens the value-wheel popup only when the gesture stayed a tap.
func (h *samplerKnobHitAdapter) OnRelease(x, y int) {
	if !h.arb.end() {
		return
	}
	h.dv.openSamplerKnobWheelPopup(h.idx)
}

// OnWheel nudges the value by one StepMul (endless) per notch, holding Shift for
// one-rung-finer. The whole spin collapses into ONE debounced undo step (see
// wheelNudger). Mirrors the Synth tab's card wheel with sampler plumbing.
func (h *samplerKnobHitAdapter) OnWheel(x, y, steps int) InputResult {
	k := h.knob()
	if k == nil || steps == 0 {
		return InputIgnored
	}
	fine := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	var badge *KnobStepBadge
	if h.idx >= 0 && h.idx < len(h.dv.sampler.knobStepBadges) {
		badge = h.dv.sampler.knobStepBadges[h.idx]
	}
	if !nudgeKnobByWheel(k, badge, steps, fine) {
		return InputIgnored
	}
	h.dv.applySamplerKnob(h.idx)
	dv := h.dv
	dv.samplerWheelNudge.bump(func() { dv.commitSamplerEdit() })
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
// Invalidate so the next Layout re-derives the visible knob rects. Mirrors the
// shape of synthSectionScrollAdapter, but the two now DIVERGE in scroll
// behavior: this sampler grid stays row-by-row stepped (no
// SetSmoothScroll(true)), while the synth section grid is smooth
// (continuous + momentum).
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

// ── *DrumView forwarding shims (Rung A relocation onto *samplerPanel) ──────

func (dv *DrumView) updateSamplerPlayheads() { dv.sp().updateSamplerPlayheads() }

func (dv *DrumView) samplerActivePlayheads() []samplerPlayheadDraw {
	return dv.sp().samplerActivePlayheads()
}

func (dv *DrumView) samplerActiveInstrument() instrument.ID { return dv.sp().samplerActiveInstrument() }

func (dv *DrumView) resyncSamplerEditFromDocument() { dv.sp().resyncSamplerEditFromDocument() }

func (dv *DrumView) ensureSamplerLoaded(instID instrument.ID) { dv.sp().ensureSamplerLoaded(instID) }

func (dv *DrumView) buildSamplerTab(contentR image.Rectangle, instID instrument.ID) {
	dv.sp().buildSamplerTab(contentR, instID)
}

func (dv *DrumView) clearSamplerKnobRects() { dv.sp().clearSamplerKnobRects() }

func (dv *DrumView) clearSamplerControlButtonRects() { dv.sp().clearSamplerControlButtonRects() }

func (dv *DrumView) samplerControlMinH(dv2 densityValues) int { return dv.sp().samplerControlMinH(dv2) }

func (dv *DrumView) layoutSamplerHeaderButtons(hdr image.Rectangle, btnH int, mobile bool) {
	dv.sp().layoutSamplerHeaderButtons(hdr, btnH, mobile)
}

func (dv *DrumView) layoutSamplerControls(ctrlR image.Rectangle, dv2 densityValues, mobile bool) {
	dv.sp().layoutSamplerControls(ctrlR, dv2, mobile)
}

func (dv *DrumView) layoutSamplerKnobsScrolling(gridR image.Rectangle, dv2 densityValues) {
	dv.sp().layoutSamplerKnobsScrolling(gridR, dv2)
}

func (dv *DrumView) layoutSamplerControlButtons(rowR image.Rectangle) {
	dv.sp().layoutSamplerControlButtons(rowR)
}

func (dv *DrumView) layoutSamplerMobileToggleRow(toggleR image.Rectangle) {
	dv.sp().layoutSamplerMobileToggleRow(toggleR)
}

// MobileActionButtons returns the Sampler tab's trailing action cluster for the
// sticky bar ([Preview][Save As][⋯]), or nil off-mobile / when no sample
// buffer is loaded (nothing to preview or save). Mirrors the Synth tab.
func (p *samplerPanel) MobileActionButtons() []*Button {
	dv := p.dv
	if !Profile().IsMobile() {
		return nil
	}
	if !dv.sampler.hasBuffer() {
		return nil
	}
	p.ensureMobileActionButtons()
	p.mobilePreviewBtn.Text = i18n.T(i18n.KeyPreview)
	p.mobileSaveAsBtn.Text = i18n.T(i18n.KeySaveAs)
	return []*Button{p.mobilePreviewBtn, p.mobileSaveAsBtn, p.mobileOverflowBtn}
}

// ensureMobileActionButtons builds the persistent sticky-bar action buttons.
// Guarded so a tap while no buffer is loaded is a no-op (belt-and-suspenders
// alongside the nil return above).
func (p *samplerPanel) ensureMobileActionButtons() {
	dv := p.dv
	guard := func(fn func()) func() {
		return func() {
			if dv.sampler.hasBuffer() {
				fn()
			}
		}
	}
	if p.mobilePreviewBtn == nil {
		p.mobilePreviewBtn = NewSpecButton(i18n.T(i18n.KeyPreview), ComponentButtonSecondary, guard(func() { dv.samplerPreview() }))
	}
	if p.mobileSaveAsBtn == nil {
		p.mobileSaveAsBtn = NewSpecButton(i18n.T(i18n.KeySaveAs), ComponentButtonPrimary, guard(func() { dv.samplerSaveAs() }))
	}
	if p.mobileOverflowBtn == nil {
		p.mobileOverflowBtn = NewSpecButton("", ComponentButtonSecondary, func() {
			dv.openSamplerActionMenu(p.mobileOverflowBtn.Rect())
		})
		p.mobileOverflowBtn.Icon = string(IconOverflow)
	}
}

func (dv *DrumView) samplerHandleRect(frac float64) image.Rectangle {
	return dv.sp().samplerHandleRect(frac)
}

func (dv *DrumView) buildSamplerButtons(hasBuf bool) { dv.sp().buildSamplerButtons(hasBuf) }

func (dv *DrumView) refreshSamplerToggleStates() { dv.sp().refreshSamplerToggleStates() }

func (dv *DrumView) samplerButtonByTag(tag string) *Button { return dv.sp().samplerButtonByTag(tag) }

func (dv *DrumView) drawSamplerTab(dst *ebiten.Image, contentR image.Rectangle) {
	dv.sp().drawSamplerTab(dst, contentR)
}

func (dv *DrumView) drawSamplerPlaceholder(dst *ebiten.Image, card image.Rectangle) {
	dv.sp().drawSamplerPlaceholder(dst, card)
}

func (dv *DrumView) drawSamplerMeta(dst *ebiten.Image, r image.Rectangle) {
	dv.sp().drawSamplerMeta(dst, r)
}

func (dv *DrumView) drawSamplerKnobs(dst *ebiten.Image) { dv.sp().drawSamplerKnobs(dst) }

func (dv *DrumView) drawSamplerWaveform(dst *ebiten.Image) { dv.sp().drawSamplerWaveform(dst) }

func (dv *DrumView) applySamplerKnob(idx int) { dv.sp().applySamplerKnob(idx) }

func (dv *DrumView) samplerKnobReadoutRect(idx int) image.Rectangle {
	return dv.sp().samplerKnobReadoutRect(idx)
}

func (dv *DrumView) samplerTabUpdate() bool { return dv.sp().samplerTabUpdate() }

func (dv *DrumView) samplerHandleDrag(which, x int) { dv.sp().samplerHandleDrag(which, x) }

func (dv *DrumView) samplerPreview() { dv.sp().samplerPreview() }

func (dv *DrumView) samplerSaveAs() { dv.sp().samplerSaveAs() }

func (dv *DrumView) samplerTabHitAreas() []HitArea { return dv.sp().samplerTabHitAreas() }
