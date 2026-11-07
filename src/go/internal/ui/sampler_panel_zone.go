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

// The Sampler tab: a full-width waveform with trim handles above one control area. All sample logic lives
// on samplerState; this file owns layout, drawing and input.

const samplerPreviewID instrument.ID = "preview.sampler"

const samplerTitlePad = SpaceSM

// Captions group the five knobs into three purposeful clusters, as the Synth tab's section cards do.
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

// Sized to the label so no button ever truncates.
func samplerBtnWidth(label string) int {
	return audioTabActionButtonWidth(label)
}

// A var so tests can swap the preview trigger.
var samplerAuditionFn = func(instID instrument.ID) { audio.Play(instID) }

func SwapSamplerAuditionFnForTest(fn func(instrument.ID)) func(instrument.ID) {
	prev := samplerAuditionFn
	if fn != nil {
		samplerAuditionFn = fn
	}
	return prev
}

// Reads the timestamps the engine stamps on every Play; 0 = never triggered.
func samplerTriggerNanos(id instrument.ID) int64 {
	t := audio.LastTriggerAt(id)
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// Runs in the Update/Layout phase so Draw stays a pure reader; triggers come from the preview and from the
// sequencer playing the loaded instrument.
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

// x comes from the live waveform rect and the trim from current state on every call, so nothing cached
// can go stale across a resize or an edit.
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

func (p *samplerPanel) samplerActiveInstrument() instrument.ID {
	dv := p.dv
	return dv.synthTabActiveInstrument()
}

// Call after undo/redo/import: ensureSamplerLoaded keeps an already-loaded instrument's in-progress edit, so
// the knobs would keep pre-undo values and the next gesture would re-apply them. Synth sources only.
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
	// loadEditDescriptor only flips the buffer off→on, so an undo that turns reverse OFF un-flips it here.
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
	// Re-render a synth capture whenever its signal changed, so the waveform follows live synth edits; WAV
	// sources are fixed PCM. hasBuffer makes a failed first load (WASM renders async) retry on the next Layout.
	if s.captureID == instID && s.hasBuffer() {
		if s.source != samplerSourceSynth {
			return
		}
		sig := samplerSourceSignatureFn(instID)
		if sig == s.srcSignature {
			return
		}
		// The RAW renderer: the buffer must stay the unedited source; the edit lives in the knob state.
		capture := samplerCaptureFn
		if audio.HasSampleEdit(instID) {
			capture = samplerRawCaptureFn
		}
		pcm, sr := capture(instID)
		if len(pcm) == 0 {
			// Leave srcSignature stale so the next Layout retries.
			return
		}
		// Preserve the user's in-progress edit — only the source audio changed.
		s.recaptureRaw(pcm, sr)
		s.srcSignature = sig
		return
	}
	// A user sample's PRISTINE PCM is the canonical store entry; the saved edit overlays the knob state, the
	// same non-destructive shape as the synth path. Checked first so a sample's edit never routes through the synth capture.
	if rec, ok := audio.UserSamplePCM(instID); ok && len(rec.PCM) > 0 {
		s.loadFromInstrument(instID, append([]float32(nil), rec.PCM...), rec.SampleRate, samplerSourceWAV)
		if e, hasEdit := audio.SampleEditFor(instID); hasEdit {
			s.loadEditDescriptor(e)
		}
		return
	}
	// An edit on an instrument with a document is a synth with a non-destructive edit: raw render + overlay.
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
	// A file-picker WAV has no document and no user-sample entry: seed its PCM into the store so the live edit
	// path has a source and trim/reverse are audible before Save.
	if !audio.HasInstrumentDoc(instID) {
		audio.PutUserSample(instID, pcm, sr)
		s.loadFromInstrument(instID, pcm, sr, samplerSourceWAV)
		return
	}
	s.loadFromInstrument(instID, pcm, sr, samplerSourceSynth)
	s.srcSignature = samplerSourceSignatureFn(instID)
}

// Desktop puts the waveform left of the controls, mobile stacks them (a layout decision; sizing comes from
// density). With no buffer the waveform card fills the body and the edit controls are suppressed.
func (p *samplerPanel) buildSamplerTab(contentR image.Rectangle, instID instrument.ID) {
	dv := p.dv
	s := &dv.sampler
	// The dropdown selection is the single source of what the Sampler edits.
	dv.ensureSamplerLoaded(instID)

	dv.updateSamplerPlayheads()

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
		// Mobile has no title band (the nav tab and channel pill say it) and its actions live on the sticky bar.
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
		// Controls are reserved first so they always fit; the waveform gets the (capped) rest.
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
		// A wider control column so long captions render at full font.
		waveW := body.Dx() * 50 / 100
		waveR = image.Rect(body.Min.X, body.Min.Y, body.Min.X+waveW, body.Max.Y)
		ctrlR = image.Rect(waveR.Max.X+SpaceMD, body.Min.Y, body.Max.X, body.Max.Y)
	}
	s.waveformRect = waveR
	s.startHandleRect = dv.samplerHandleRect(s.startFrac)
	s.endHandleRect = dv.samplerHandleRect(s.endFrac)
	dv.layoutSamplerControls(ctrlR, dv2, mobile)
}

func (p *samplerPanel) clearSamplerKnobRects() {
	dv := p.dv
	for _, k := range dv.sampler.knobs {
		if k != nil {
			k.SetRect(image.Rectangle{})
		}
	}
}

// Includes Save/Save As/Reset, which on mobile live in the control column.
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

func (p *samplerPanel) samplerControlMinH(dv2 densityValues) int {
	dv := p.dv
	_ = dv
	return samplerControlMinHeight(dv2)
}

// Shared with the mobile content-floor calc: one full knob cell, the button row and the metadata strip. The
// other knobs scroll, so only the first dial must fit at full size.
func samplerControlMinHeight(dv2 densityValues) int {
	labelBandH := TextHeight() + SpaceXS // group-label band above the card
	cellH := labelBandH + dv2.ParamCardH // one ParamCard slot per control
	// SpaceSM slack so the budget stays above the ideal after the waveform cap and rounding take their cut.
	return cellH + SpaceSM + dv2.SynthHeaderButtonH + SpaceXS + TextHeight() + 2*SpaceXS
}

// On mobile the header is too narrow, so these move beneath the knobs and their header rects are cleared.
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

// Computed so nothing overflows ctrlR.
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
		// The mobile actions live on the sticky bar, so the reclaimed height goes to the knob grid.
		gridBottomM := toggleY0 - SpaceXS
		dv.layoutSamplerKnobsScrolling(image.Rect(ctrlR.Min.X, ctrlR.Min.Y, ctrlR.Max.X, gridBottomM), dv2)
		dv.layoutSamplerMobileToggleRow(image.Rect(ctrlR.Min.X, toggleY0, ctrlR.Max.X, toggleY1))
		return
	}

	// Language-invariant: a card's size depends only on its cell and density tokens, never on caption length.
	gridTop := ctrlR.Min.Y

	cellW := ctrlR.Dx() / samplerKnobCount
	cardH := dv2.ParamCardH
	// Stretches in a wide column but never fills the cell; floored so a narrow column cannot collapse it.
	cardW := cellW - SpaceSM
	if maxW := dv2.ParamCardW * 3 / 2; cardW > maxW {
		cardW = maxW
	}
	if minW := dv2.ParamCardW / 2; cardW < minW {
		cardW = minW
	}
	// The card never overhangs its cell, even when the floor exceeds a degenerate column.
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
		// The wider cell is tracked separately so the group label centres in it.
		k.SetRect(image.Rect(cardX0, cardTop, cardX0+cardW, cardTop+cardH))
		s.knobCells[i] = image.Rect(cellX0, gridTop, cellX0+cellW, cellBottom)
		if !k.Capturing() {
			k.Value = s.knobValue(i)
		}
	}

	dv.layoutSamplerControlButtons(image.Rect(ctrlR.Min.X, toggleY0, ctrlR.Max.X, toggleY1))
}

func (s *samplerState) ensureKnobGrid() *ControlGrid {
	if s.knobGrid == nil {
		s.knobGrid = NewControlGrid(controlGridScrollbarStyle())
	}
	return s.knobGrid
}

// Off-window knobs get an empty rect so draw and hit-test skip them.
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
		// Clamped to what the slot holds on a tiny panel.
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

// Desktop only; mobile uses layoutSamplerMobileToggleRow.
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

// The last cell takes the remainder so rounding never leaves a gap; the actions live on the sticky bar, so
// their in-panel rects are cleared.
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

// Uses the same rect the handle is drawn and grabbed with, so hover matches grab. Desktop only.
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

type samplerButton struct {
	btn    *Button
	tag    string
	active func() bool
}

// Without a buffer the dependent buttons draw disabled and their handlers are gated: no dead affordances.
func (p *samplerPanel) buildSamplerButtons(hasBuf bool) {
	dv := p.dv
	s := &dv.sampler
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
	// A second gate behind the suppressed hit area.
	guard := func(fn func()) func() {
		return func() {
			if dv.sampler.hasBuffer() {
				fn()
			}
		}
	}
	defs := []samplerButton{
		// A latching toggle that flips the working signal in place; its visual reads the net flip parity, so it
		// matches the audio on every platform.
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

// Called on every build and every frame, so the latched visual always matches the live state.
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
		// Mobile only; a no-op when the list fits.
		if Profile().IsMobile() && s.knobGrid != nil {
			s.knobGrid.Draw(dst)
			// Edge fades over the reserved peek band signal more controls below.
			drawControlGridEdgeFades(dst, s.knobGrid, TokenSurface1())
		}
	}

	dv.refreshSamplerToggleStates()
	for _, b := range dv.samplerButtons {
		if b.btn.Rect().Empty() {
			continue
		}
		b.btn.Draw(dst)
	}

	// Shared with the Synth tab; only one can be open.
	dv.drawSaveAsDialog(dst)
}

// Falls back to a shorter message so the text never clips the card.
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

// The joiner follows the locale; the units are international.
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

func (p *samplerPanel) drawSamplerKnobs(dst *ebiten.Image) {
	dv := p.dv
	s := &dv.sampler
	// Cleared first so a stale rect from a prior draw never steals a tap.
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
		// Chrome centres in the full cell, not the card, so a long caption renders at full font without overlap.
		cell := s.knobCells[i]
		if cell.Empty() {
			cell = r
		}
		// The gloss is appended only when it fits, so it never clips into the next cluster.
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
		// The same ParamCard as the Synth tab: a tap opens the wheel popup, the desktop wheel nudges in place.
		drawParamCard(dst, r, samplerKnobNameText(i, s), samplerKnobValueText(i, s),
			paramCardGaugeFor(k, dv.samplerKnobDef(i)), paramCardState{})
		s.readoutRects[i] = image.Rectangle{}
	}
}

func (p *samplerPanel) drawSamplerWaveform(dst *ebiten.Image) {
	dv := p.dv
	s := &dv.sampler
	w := s.waveformRect
	// Reuse the cached float64 scratch (no per-frame allocation).
	wave := s.waveFloat64()
	midY := w.Min.Y + w.Dy()/2
	// A data canvas: Visualizers family, never the chrome accent.
	drawWaveTrace(dst, wave, w, midY, w.Dx(), colWaveTrace, 1.0, TokenSurface3())

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
	p.drawSamplerHandle(dst, dv.samplerHandleRect(s.startFrac), s.hoverHandle == 0)
	p.drawSamplerHandle(dst, dv.samplerHandleRect(s.endFrac), s.hoverHandle == 1)

	// One line per in-flight trigger, each its own colour so overlapping voices never read as one; drawn last,
	// density-stroked so they never vanish against the trace.
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

// The glow conforms to the handle's rect: drawNodeBloom assumes a square node and bulged mid-line.
func (p *samplerPanel) drawSamplerHandle(dst *ebiten.Image, r image.Rectangle, hovered bool) {
	if r.Empty() {
		return
	}
	drawRect(dst, r, colAccent, true)
	if hovered {
		drawRectHoverGlow(dst, r, colAccentBright, 1.0)
	}
}

// From the paramFacet registry, the same source as undo notifications, so the two never drift.
func samplerKnobCaption(idx int, s *samplerState) string {
	name := samplerKnobNameText(idx, s)
	value := samplerKnobValueText(idx, s)
	if name == "" {
		return ""
	}
	return name + " " + value
}

// Name only: the value shows elsewhere, and name+value printed it twice.
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
	// Audible now, not at gesture end: setKnob writes only the UI's copy. The undo step stays one per gesture.
	dv.applySamplerEditLive()
}

// Zero until the first Draw populates it.
func (p *samplerPanel) samplerKnobReadoutRect(idx int) image.Rectangle {
	dv := p.dv
	if idx < 0 || idx >= samplerKnobCount {
		return image.Rectangle{}
	}
	return dv.sampler.readoutRects[idx]
}

// Derived, not stored: every sampler knob is continuous, so there is no binding array.
func (dv *DrumView) samplerKnobDef(idx int) audio.ParamDef {
	sc := samplerKnobScale(idx)
	return audio.ParamDef{Name: samplerStepPrefName(idx), Min: sc.Min, Max: sc.Max, Unit: sc.Unit}
}

// Returns true when the tab needs a re-layout.
func (p *samplerPanel) samplerTabUpdate() bool {
	dv := p.dv
	p.updateSamplerHandleHover()
	dv.samplerWheelNudge.tick()
	// Tick the mobile knob-scroll cooldown clock (see ControlGrid.WheelStep/Tick).
	if dv.sampler.knobGrid != nil {
		dv.sampler.knobGrid.Tick()
	}
	return false
}

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
	// The trim moves in the audio while dragging; an unchanged descriptor is skipped, so a still frame costs nothing.
	dv.applySamplerEditLive()
}

func (p *samplerPanel) samplerPreview() {
	dv := p.dv
	if !dv.sampler.hasBuffer() {
		return
	}
	baked, sr := dv.sampler.bake()
	audio.RegisterSamplePCM(samplerPreviewID, baked, sr)
	samplerAuditionFn(samplerPreviewID)
}

// The Sampler edit is saved as data in the instrument's audio bundle; nothing is baked.
func (p *samplerPanel) samplerSaveAs() {
	p.dv.openSaveAsDialogFor(p.dv.sampler.captureID)
}

// Controls sit at ZEQPanel+2, above the panel catch-all and the mobile knob-scroll body (+1); the
// scrollbar thumb sits above the knobs.
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

	// Only when the list overflows; the body sits BELOW the knobs, so a knob press captures the knob and an
	// empty-space press scrolls.
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
		// Disabled buttons stay out of the hit index: no dead affordances.
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

func (h *samplerKnobHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	return hitRect
}

// Flushes a pending wheel-nudge commit first so the popup snapshots the committed value.
func (h *samplerKnobHitAdapter) OnPress(x, y int) InputResult {
	// Capturing, not consuming, lets a vertical sweep become a scroll; a still tap opens the popup on RELEASE.
	h.arb.begin(y, h.dv.sampler.knobGrid)
	return InputCaptured
}

// Past the tap slop the gesture is a scroll and the release will not open the popup.
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

// Shift is one rung finer; a whole spin is ONE debounced undo step.
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

// body=true is the empty-space catch-all (content follows the finger); false is the thumb (the thumb follows
// it). Stepped row by row, unlike the smooth synth grid.
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

// nil off-mobile and with no buffer (nothing to preview or save).
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

// Guarded so a tap without a buffer is a no-op, behind the nil return above.
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
