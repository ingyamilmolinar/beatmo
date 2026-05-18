package ui

import (
	"fmt"
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Synth-tab implementation — redesigned per the plan file
// hey-please-review-the-vectorized-rocket.md. The tab is the canonical
// per-instrument pipeline surface: header strip → section cards (PITCH,
// ENVELOPE, TONE, DRIVE) → OUT-column launchers (FX, EQ, Delay, Reverb)
// → reset footer. Sections render only the knobs the active recipe's
// C renderer actually reads (see WiredParamsForRecipe + recipeWiredParams);
// silent no-op sliders are eliminated at the schema layer.
//
// Why a zone-shaped renderer rather than a portal overlay: the audio
// panel is the canonical place to inspect & adjust an instrument's
// signal chain. A dedicated tab places synth params on the same surface
// users already consult for EQ/Wave/Spectrum/Levels/Chain. The portal
// alternative would re-introduce a stacked-modal UX that the user has
// explicitly rejected (see [[feedback_minimal_ux_changes]]).
//
// Active-row resolution: same as today — `synthTabActiveInstrument()`
// resolves the EQ panel's channel pill (Master falls through to the
// first non-empty row). The header strip renders the resolved name so
// the user always knows which row is being edited; opening the chevron
// reuses the row-picker chrome to switch rows without leaving the tab.

// ---- Section model -----------------------------------------------------

type synthSectionID int

const (
	synthSectionPitch synthSectionID = iota
	synthSectionEnvelope
	synthSectionTone
	synthSectionDrive
)

// synthSectionOrder is the canonical left-to-right (desktop) / top-to-bottom
// (mobile) order. The OUT column is always rendered after the last section.
var synthSectionOrder = []synthSectionID{
	synthSectionPitch,
	synthSectionEnvelope,
	synthSectionTone,
	synthSectionDrive,
}

// sectionLabel returns the display label for the section header.
func sectionLabel(id synthSectionID) string {
	switch id {
	case synthSectionPitch:
		return "PITCH"
	case synthSectionEnvelope:
		return "ENVELOPE"
	case synthSectionTone:
		return "TONE"
	case synthSectionDrive:
		return "DRIVE"
	}
	return ""
}

// sectionForParam returns which section a wired param belongs to. Mirrors
// the table in the plan file (PITCH={pitch}, ENVELOPE={decay},
// TONE={tone,body,brightness}, DRIVE={drive}). Unknown params fall back
// to TONE so they remain visible if the wired-param table picks up a
// new knob before the section assignment is updated.
func sectionForParam(name string) synthSectionID {
	switch name {
	case "pitch":
		return synthSectionPitch
	case "decay":
		return synthSectionEnvelope
	case "drive":
		return synthSectionDrive
	case "tone", "body", "brightness":
		return synthSectionTone
	}
	return synthSectionTone
}

// synthSection is one card in the section row. Empty knobIdxs means the
// section has no wired knobs for the current recipe and the card is
// rendered as a collapsed placeholder.
type synthSection struct {
	id       synthSectionID
	rect     image.Rectangle
	knobIdxs []int // indices into instEditorKnobs / instEditorBindings
}

// ---- OUT-column launchers ----------------------------------------------

type synthOutLinkKind int

const (
	synthOutDelay synthOutLinkKind = iota
	synthOutReverb
)

// synthOutLink is one row in the OUT column. The column now hosts only the
// per-instrument sends (Delay, Reverb) — FX chain editing lives in the
// existing per-row FX overlay (opened from the row rack), and EQ editing
// lives on the audio panel's EQ tab. Surfacing those launchers a second
// time on the Synth tab proved confusing; users have explicit, dedicated
// routes already. The Synth tab now exclusively owns the two surfaces no
// other tab provides: per-recipe knob editing and per-instrument send
// levels.
//
// amount is shown for the active sends as a percent suffix (e.g. "Delay
// 25%"). Tapping a launcher opens the inline sendLevelPopover so the user
// can adjust the send without leaving the tab.
type synthOutLink struct {
	kind   synthOutLinkKind
	rect   image.Rectangle
	amount float64
}

func (l synthOutLink) label() string {
	switch l.kind {
	case synthOutDelay:
		if l.amount > 0 {
			return fmt.Sprintf("Delay %d%%", int(l.amount*100+0.5))
		}
		return "Delay"
	case synthOutReverb:
		if l.amount > 0 {
			return fmt.Sprintf("Reverb %d%%", int(l.amount*100+0.5))
		}
		return "Reverb"
	}
	return ""
}

// synthHeaderLayout records the rects + cached fingerprint for the
// header strip — instrument label + recipe id, waveform thumbnail,
// retrigger button, row-volume readout.
type synthHeaderLayout struct {
	rect          image.Rectangle
	waveformRect  image.Rectangle
	captionRect   image.Rectangle
	retriggerRect image.Rectangle
	volRect       image.Rectangle
	instLabel     string
	recipeID      string
}

// ---- Legacy / compat types ---------------------------------------------

// instParamBinding maps a widget index to a single ParamDef. Lives on
// DrumView state (one set of bindings per row's active recipe).
type instParamBinding struct {
	def audio.ParamDef
}

// synthGroupHeader is a legacy field kept so existing tests that consult
// SynthTabGroupHeaders() continue to read an empty slice (the redesigned
// layout uses section cards instead of inline group dividers).
type synthGroupHeader struct {
	label string
	rect  image.Rectangle
}

// synthResetButtonTag is the sentinel button text that identifies the
// footer reset button without clobbering the user-visible label.
const synthResetButtonTag = "\x00synth-reset"

// ---- Layout constants --------------------------------------------------

const (
	synthHeaderHeightDesktop      = 84
	synthHeaderHeightMobile       = 72
	synthFooterHeightDesktop      = 32
	synthFooterHeightMobile       = 44
	synthSectionPaddingX          = 8
	synthSectionPaddingY          = 8
	synthSectionTitleHeight       = 18
	synthKnobMinDiameter          = 44 // floor: matches touch-target standard
	synthKnobIdealDiameter        = 72 // desktop default — same scale as DAW rotaries
	synthKnobMobileDiameter       = 64
	synthKnobCaptionHeight        = 24
	synthOutColumnDesktopMinWidth = 160
	synthOutLinkHeightDesktop     = 28
	synthOutLinkHeightMobile      = 36
)

// buildSynthTab is the layout entry point invoked by EQPanelZone when
// TabSynth is active. Computes header / section / OUT column / footer
// rects, populates instEditorKnobs + instEditorSliders + instEditorBindings
// with one entry per wired ParamDef, and assigns each entry to its section.
// Called every Layout so re-derivation per [[feedback_runtime_profile_derivation]]
// keeps working when the runtime profile or panel size changes.
//
// Critical: knob and slider INSTANCES are preserved across re-layouts when
// the binding sequence (recipe + param names) is unchanged. Recreating
// them every Layout would destroy in-flight drag state — the knob's
// dragging flag, pressY/pressVal latch, and the hit-area handler's
// captured pointer would all reset mid-drag, manifesting as "I drag the
// knob and nothing happens." Layout is called every frame by the
// drumview tree, so any per-frame allocation here is a correctness bug,
// not just a perf concern.
func (dv *DrumView) buildSynthTab(contentR image.Rectangle, instID string) {
	dv.instEditorBtns = dv.instEditorBtns[:0]
	dv.instEditorGroups = dv.instEditorGroups[:0]
	dv.instEditorSections = dv.instEditorSections[:0]
	dv.instEditorOutLinks = dv.instEditorOutLinks[:0]
	dv.instEditorHeader = synthHeaderLayout{}
	if contentR.Empty() {
		dv.instEditorKnobs = dv.instEditorKnobs[:0]
		dv.instEditorSliders = dv.instEditorSliders[:0]
		dv.instEditorBindings = dv.instEditorBindings[:0]
		return
	}

	resolved := dv.resolveSynthInstrument(instID)
	if resolved == "" {
		dv.instEditorKnobs = dv.instEditorKnobs[:0]
		dv.instEditorSliders = dv.instEditorSliders[:0]
		dv.instEditorBindings = dv.instEditorBindings[:0]
		return
	}
	recipeID := audio.RecipeForInstrument(resolved)
	var schema []audio.ParamDef
	if recipeID != "" {
		if reg, ok := audio.RecipeRegistrations()[recipeID]; ok && reg != nil {
			schema = reg.Params
		}
	}

	p := Profile()
	mobile := p.IsMobile()
	headerH := synthHeaderHeightDesktop
	footerH := synthFooterHeightDesktop
	if mobile {
		headerH = synthHeaderHeightMobile
		footerH = synthFooterHeightMobile
	}
	// Adaptive shrink: when the panel is short (tests use 640×480 which
	// gives the audio panel only ~80 px of content), the section row
	// would otherwise be empty. Shrink header first (most reducible), then
	// footer, before letting sections take over the full content area.
	const minSectionsH = 60
	if contentR.Dy() < headerH+footerH+minSectionsH {
		// Reserve at least minSectionsH for sections; absorb the rest by
		// shrinking header (keeping footer for the reset button).
		spare := contentR.Dy() - minSectionsH - footerH
		if spare < 0 {
			// Even the footer can't fit alongside sections — collapse the
			// header to zero and let sections borrow into the footer band.
			headerH = 0
			if contentR.Dy() < footerH+minSectionsH {
				footerH = contentR.Dy() / 3
				if footerH < 16 {
					footerH = 16
				}
			}
		} else {
			headerH = spare
			if headerH < 0 {
				headerH = 0
			}
		}
	}

	// Header strip occupies the top band. Section row + OUT column fill
	// the middle. Footer hosts the reset button.
	dv.layoutSynthHeader(contentR, headerH, resolved, recipeID)

	sectionsRect := image.Rect(
		contentR.Min.X,
		contentR.Min.Y+headerH,
		contentR.Max.X,
		contentR.Max.Y-footerH,
	)
	if sectionsRect.Empty() {
		return
	}

	// Populate per-section knob index lists from the wired schema. Reuse
	// existing knob+slider instances when the binding sequence is
	// unchanged so in-flight drag state survives the re-layout. A binding
	// sequence is "unchanged" when len(schema) and every param name match
	// the previous bindings.
	sections := make([]synthSection, len(synthSectionOrder))
	for i, id := range synthSectionOrder {
		sections[i] = synthSection{id: id}
	}
	reuseExisting := bindingsMatchSchema(dv.instEditorBindings, schema)
	if !reuseExisting {
		dv.instEditorKnobs = dv.instEditorKnobs[:0]
		dv.instEditorSliders = dv.instEditorSliders[:0]
		dv.instEditorBindings = dv.instEditorBindings[:0]
	}
	for i, def := range schema {
		sec := sectionForParam(def.Name)
		for j := range sections {
			if sections[j].id == sec {
				sections[j].knobIdxs = append(sections[j].knobIdxs, i)
				break
			}
		}
		var initial float64
		if denom := def.Max - def.Min; denom > 0 {
			cur := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(resolved))[def.Name]
			initial = (cur - def.Min) / denom
			if initial < 0 {
				initial = 0
			} else if initial > 1 {
				initial = 1
			}
		}
		if reuseExisting && i < len(dv.instEditorKnobs) {
			// Keep the existing knob/slider — preserve drag state,
			// pressY latch, captured-handler pointer. Only refresh the
			// Value when NO drag is in progress; mid-drag, the user's
			// finger owns the value and we must not clobber it.
			if !dv.instEditorKnobs[i].Capturing() {
				dv.instEditorKnobs[i].Value = initial
				dv.instEditorSliders[i].Value = initial
			}
			dv.instEditorBindings[i] = instParamBinding{def: def}
			continue
		}
		dv.instEditorBindings = append(dv.instEditorBindings, instParamBinding{def: def})
		dv.instEditorKnobs = append(dv.instEditorKnobs, NewKnob(initial))
		dv.instEditorSliders = append(dv.instEditorSliders, NewSlider(initial))
	}

	dv.layoutSynthSectionsAndOutColumn(sectionsRect, sections, resolved, mobile)
	dv.instEditorSections = sections

	// Footer reset button (right-aligned).
	footerY := contentR.Max.Y - footerH
	inset := SpaceMD
	resetW := 72
	if mobile {
		resetW = 96
	}
	resetR := image.Rect(
		contentR.Max.X-inset-resetW,
		footerY+SpaceXS,
		contentR.Max.X-inset,
		contentR.Max.Y-SpaceXS,
	)
	resetBtn := &Button{Text: synthResetButtonTag}
	resetBtn.SetRect(resetR)
	dv.instEditorBtns = append(dv.instEditorBtns, resetBtn)

	// Section-card layout doesn't scroll vertically. Reset scroll fields
	// so the legacy fingerprint test (TestSynthLayoutFingerprintStable)
	// stays deterministic.
	dv.instEditorScrollMax = 0
	dv.instEditorScrollPx = 0
}

// bindingsMatchSchema reports whether the existing bindings line up with
// the new schema by length + param name. Used by buildSynthTab to decide
// whether to reuse existing knob/slider instances (preserving drag state)
// or rebuild the slice from scratch (when the active recipe changed).
func bindingsMatchSchema(bindings []instParamBinding, schema []audio.ParamDef) bool {
	if len(bindings) != len(schema) {
		return false
	}
	for i := range schema {
		if bindings[i].def.Name != schema[i].Name {
			return false
		}
	}
	return true
}

// layoutSynthHeader fills dv.instEditorHeader. The header strip is a
// single row containing the instrument label (with row-picker chevron
// affordance), a waveform thumbnail placeholder, and a row-volume readout.
// Concrete widgets (chevron button, thumbnail) are drawn in drawSynthTab —
// the layout only fixes the rects.
func (dv *DrumView) layoutSynthHeader(contentR image.Rectangle, headerH int, instID, recipeID string) {
	r := image.Rect(contentR.Min.X, contentR.Min.Y, contentR.Max.X, contentR.Min.Y+headerH)
	inset := SpaceMD
	innerY0 := r.Min.Y + SpaceSM
	innerY1 := r.Max.Y - SpaceSM
	thumbW := 120
	if Profile().IsMobile() {
		thumbW = 80
	}
	if thumbW > r.Dx()/3 {
		thumbW = r.Dx() / 3
	}
	waveformR := image.Rect(r.Min.X+inset, innerY0, r.Min.X+inset+thumbW, innerY1)
	captionR := image.Rect(waveformR.Max.X+SpaceSM, innerY0, r.Max.X-160, innerY1)
	if captionR.Max.X < captionR.Min.X+40 {
		captionR.Max.X = captionR.Min.X + 40
	}
	volW := 80
	volR := image.Rect(r.Max.X-inset-volW, innerY0, r.Max.X-inset, innerY1)
	retriggerW := 100
	retriggerR := image.Rect(volR.Min.X-SpaceSM-retriggerW, innerY0, volR.Min.X-SpaceSM, innerY1)
	if retriggerR.Min.X < captionR.Max.X+SpaceSM {
		retriggerR.Min.X = captionR.Max.X + SpaceSM
	}
	dv.instEditorHeader = synthHeaderLayout{
		rect:          r,
		waveformRect:  waveformR,
		captionRect:   captionR,
		retriggerRect: retriggerR,
		volRect:       volR,
		instLabel:     instID,
		recipeID:      recipeID,
	}
}

// layoutSynthSectionsAndOutColumn computes the geometry for the 4 section
// cards + OUT column. Desktop lays them side-by-side; mobile stacks each
// section as a full-width row with OUT as a horizontal pill strip at the
// bottom.
func (dv *DrumView) layoutSynthSectionsAndOutColumn(rowR image.Rectangle, sections []synthSection, instID string, mobile bool) {
	inset := SpaceMD
	rowR.Min.X += inset
	rowR.Max.X -= inset
	rowR.Min.Y += SpaceSM
	rowR.Max.Y -= SpaceSM
	if rowR.Empty() {
		return
	}

	if mobile {
		// Vertical stack: header + each section + OUT pill row.
		stackH := rowR.Dy()
		outH := synthOutLinkHeightMobile + SpaceSM
		sectionsAreaH := stackH - outH
		if sectionsAreaH < 1 {
			sectionsAreaH = stackH
			outH = 0
		}
		perSectionH := sectionsAreaH / len(sections)
		if perSectionH < synthKnobMobileDiameter+synthKnobCaptionHeight+SpaceSM {
			perSectionH = synthKnobMobileDiameter + synthKnobCaptionHeight + SpaceSM
		}
		y := rowR.Min.Y
		for i := range sections {
			rect := image.Rect(rowR.Min.X, y, rowR.Max.X, y+perSectionH)
			if rect.Max.Y > rowR.Min.Y+sectionsAreaH {
				rect.Max.Y = rowR.Min.Y + sectionsAreaH
			}
			sections[i].rect = rect
			dv.placeKnobsInSection(sections[i], mobile)
			y = rect.Max.Y
		}
		if outH > 0 {
			outRect := image.Rect(rowR.Min.X, rowR.Min.Y+sectionsAreaH, rowR.Max.X, rowR.Max.Y)
			dv.layoutOutColumn(outRect, instID, mobile)
		}
		return
	}

	// Desktop: horizontal columns. OUT gets a minimum width; the other 4
	// share the remainder. Each section gets equal width within that pool.
	outW := synthOutColumnDesktopMinWidth
	if outW > rowR.Dx()/3 {
		outW = rowR.Dx() / 3
	}
	sectionsW := rowR.Dx() - outW - SpaceMD
	perSectionW := sectionsW / len(sections)
	if perSectionW < synthKnobMinDiameter+synthSectionPaddingX*2 {
		perSectionW = synthKnobMinDiameter + synthSectionPaddingX*2
	}
	x := rowR.Min.X
	for i := range sections {
		rect := image.Rect(x, rowR.Min.Y, x+perSectionW, rowR.Max.Y)
		sections[i].rect = rect
		dv.placeKnobsInSection(sections[i], mobile)
		x = rect.Max.X
	}
	outRect := image.Rect(rowR.Max.X-outW, rowR.Min.Y, rowR.Max.X, rowR.Max.Y)
	dv.layoutOutColumn(outRect, instID, mobile)
}

// placeKnobsInSection assigns rects to every knob+slider pair in the
// section. Knobs are arranged horizontally (centred) so a single-knob
// section still feels balanced. The slider's rect is set equal to the
// knob's rect so legacy hit-tests + JS export rects continue to land on
// the visible widget.
func (dv *DrumView) placeKnobsInSection(s synthSection, mobile bool) {
	if len(s.knobIdxs) == 0 {
		return
	}
	innerR := image.Rect(
		s.rect.Min.X+synthSectionPaddingX,
		s.rect.Min.Y+synthSectionTitleHeight+synthSectionPaddingY,
		s.rect.Max.X-synthSectionPaddingX,
		s.rect.Max.Y-synthSectionPaddingY,
	)
	if innerR.Empty() {
		return
	}
	knobDiameter := synthKnobIdealDiameter
	if mobile {
		knobDiameter = synthKnobMobileDiameter
	}
	// Constrain to available space per knob.
	maxPerKnobW := innerR.Dx() / len(s.knobIdxs)
	if knobDiameter > maxPerKnobW {
		knobDiameter = maxPerKnobW
	}
	maxKnobH := innerR.Dy() - synthKnobCaptionHeight
	if knobDiameter > maxKnobH {
		knobDiameter = maxKnobH
	}
	if knobDiameter < synthKnobMinDiameter {
		knobDiameter = synthKnobMinDiameter
	}
	totalKnobsW := knobDiameter * len(s.knobIdxs)
	gap := 0
	if len(s.knobIdxs) > 1 {
		remaining := innerR.Dx() - totalKnobsW
		gap = remaining / (len(s.knobIdxs) + 1)
		if gap < 0 {
			gap = 0
		}
	}
	startX := innerR.Min.X + gap
	if len(s.knobIdxs) == 1 {
		startX = innerR.Min.X + (innerR.Dx()-knobDiameter)/2
	}
	knobY := innerR.Min.Y
	for i, kIdx := range s.knobIdxs {
		x0 := startX + i*(knobDiameter+gap)
		rect := image.Rect(x0, knobY, x0+knobDiameter, knobY+knobDiameter+synthKnobCaptionHeight)
		dv.instEditorKnobs[kIdx].SetRect(rect)
		dv.instEditorSliders[kIdx].SetRect(rect)
	}
}

// layoutOutColumn places the Delay and Reverb send launchers in the OUT
// column. Desktop stacks them vertically; mobile lays them out as a
// horizontal pill strip. FX chain + EQ have dedicated entry points
// elsewhere (per-row FX overlay, audio panel's EQ tab) and are
// deliberately omitted here.
func (dv *DrumView) layoutOutColumn(rect image.Rectangle, instID string, mobile bool) {
	if rect.Empty() {
		return
	}
	delayLvl := audio.DelaySend(instID)
	reverbLvl := audio.ReverbSend(instID)
	links := []synthOutLink{
		{kind: synthOutDelay, amount: delayLvl},
		{kind: synthOutReverb, amount: reverbLvl},
	}
	inset := SpaceSM
	innerR := image.Rect(rect.Min.X+inset, rect.Min.Y+synthSectionTitleHeight, rect.Max.X-inset, rect.Max.Y)
	if innerR.Empty() {
		return
	}
	if mobile {
		// Horizontal pill strip.
		count := len(links)
		gap := SpaceSM
		availW := innerR.Dx() - gap*(count-1)
		if availW < 0 {
			availW = 0
		}
		pillW := availW / count
		if pillW < 60 {
			pillW = 60
		}
		x := innerR.Min.X
		for i := range links {
			links[i].rect = image.Rect(x, innerR.Min.Y, x+pillW, innerR.Min.Y+synthOutLinkHeightMobile)
			x += pillW + gap
		}
	} else {
		// Vertical stack.
		linkH := synthOutLinkHeightDesktop
		y := innerR.Min.Y
		for i := range links {
			links[i].rect = image.Rect(innerR.Min.X, y, innerR.Max.X, y+linkH)
			y += linkH + SpaceXS
		}
	}
	dv.instEditorOutLinks = links
}

// drawSynthTab renders the prebuilt section model. Called by EQPanelZone.Draw
// when activeTab == TabSynth.
func (dv *DrumView) drawSynthTab(dst *ebiten.Image, contentR image.Rectangle, instID string) {
	if contentR.Empty() {
		return
	}
	resolved := dv.resolveSynthInstrument(instID)
	if resolved == "" {
		// No active row — draw a hint and return.
		DrawTextColorAt(dst, "no row selected — add a node to begin", contentR.Min.X+SpaceMD, contentR.Min.Y+SpaceMD, TokenTextSecondary())
		return
	}

	dv.drawSynthHeader(dst)
	for _, s := range dv.instEditorSections {
		dv.drawSynthSectionCard(dst, s, resolved)
	}
	dv.drawSynthOutColumn(dst, resolved)
	dv.drawSynthFooter(dst)
	if dv.instEditorSendPopover != nil {
		dv.instEditorSendPopover.draw(dst)
	}
}

// drawSynthHeader renders the instrument label + chevron, the cached
// waveform thumbnail, and the row-volume readout.
func (dv *DrumView) drawSynthHeader(dst *ebiten.Image) {
	h := dv.instEditorHeader
	if h.rect.Empty() {
		return
	}
	// Background card.
	drawRoundedRect(dst, h.rect, TokenSurface1(), 8, true)
	drawRoundedRect(dst, h.rect, TokenBorderSubtle(), 8, false)

	// Caption: "snare  ›  drum-snare"
	captionText := h.instLabel
	if h.recipeID != "" {
		captionText = h.instLabel + "  —  " + h.recipeID
	}
	DrawTextColorAt(dst, captionText, h.captionRect.Min.X, h.captionRect.Min.Y+SpaceSM, TokenTextPrimary())

	// Cached-source mini-waveform — placeholder symmetric envelope until
	// the voice cache exposes a sample-buffer accessor. The placeholder
	// indicates "this is the cached source" and shares the same accent
	// colour as the section knobs.
	drawSynthWaveformPlaceholder(dst, h.waveformRect)

	// Retrigger preview button (placeholder — no audition path; reuses the
	// existing trigger flow in playback rather than a parallel preview, per
	// [[feedback_minimal_ux_changes]]).
	drawRoundedRect(dst, h.retriggerRect, TokenSurface2(), 6, true)
	drawRoundedRect(dst, h.retriggerRect, TokenBorderSubtle(), 6, false)
	DrawTextColorAt(dst, "Preview on play", h.retriggerRect.Min.X+SpaceSM, h.retriggerRect.Min.Y+SpaceSM, TokenTextSecondary())

	// Row volume readout — pulled from the row whose Instrument matches.
	vol := dv.synthHeaderRowVolume(h.instLabel)
	drawRoundedRect(dst, h.volRect, TokenSurface2(), 6, true)
	drawRoundedRect(dst, h.volRect, TokenBorderSubtle(), 6, false)
	DrawTextColorAt(dst, fmt.Sprintf("vol %.2f", vol), h.volRect.Min.X+SpaceSM, h.volRect.Min.Y+SpaceSM, TokenTextSecondary())
}

// synthHeaderRowVolume returns the active row's per-channel volume (0..1).
// Falls back to 1.0 when the row is not found.
func (dv *DrumView) synthHeaderRowVolume(instID string) float64 {
	for _, r := range dv.Rows {
		if r != nil && r.Instrument == instID {
			return r.Volume
		}
	}
	return 1.0
}

// drawSynthWaveformPlaceholder renders a static symmetric envelope as a
// placeholder for the cached waveform. The real cached sample is rendered
// at trigger time and lives in the voice cache; exposing it for paint here
// requires a public accessor on the cache that hasn't been added yet.
// The placeholder communicates "this is the source for the active row"
// without depending on the cache wiring.
func drawSynthWaveformPlaceholder(dst *ebiten.Image, r image.Rectangle) {
	if r.Empty() {
		return
	}
	drawRoundedRect(dst, r, TokenSurface2(), 6, true)
	drawRoundedRect(dst, r, TokenBorderSubtle(), 6, false)
	// Stroke a symmetric exponential decay envelope. The drum-bus
	// inspirations (Drum Bus, Drum Synth) all show a similar gesture in
	// their header.
	mid := (r.Min.Y + r.Max.Y) / 2
	w := r.Dx()
	if w < 8 {
		return
	}
	col := TokenAccentDim()
	for x := 0; x < w; x++ {
		// Exponential decay over the width.
		t := float64(x) / float64(w)
		amp := float64(r.Dy()/2-2) * expDecay(t*4)
		y0 := mid - int(amp)
		y1 := mid + int(amp)
		drawRect(dst, image.Rect(r.Min.X+x, y0, r.Min.X+x+1, y1), col, true)
	}
}

// expDecay is a small inline exp(-x) shim that avoids dragging in math
// inside the placeholder draw path (which runs every frame the synth tab
// is visible).
func expDecay(x float64) float64 {
	// 8-term Taylor expansion is plenty for x∈[0,4] visual precision.
	sum, term := 1.0, 1.0
	for i := 1; i < 12; i++ {
		term *= -x / float64(i)
		sum += term
	}
	if sum < 0 {
		return 0
	}
	if sum > 1 {
		return 1
	}
	return sum
}

// drawSynthSectionCard renders one section card. Empty sections collapse
// to a placeholder showing the section name + dash, so the layout grid
// stays visually consistent across recipes with very different wired sets.
func (dv *DrumView) drawSynthSectionCard(dst *ebiten.Image, s synthSection, instID string) {
	if s.rect.Empty() {
		return
	}
	innerR := image.Rect(s.rect.Min.X+2, s.rect.Min.Y+2, s.rect.Max.X-2, s.rect.Max.Y-2)
	drawRoundedRect(dst, innerR, TokenSurface1(), 8, true)
	drawRoundedRect(dst, innerR, TokenBorderSubtle(), 8, false)
	// Section title (uppercase, secondary text colour).
	titleR := image.Rect(innerR.Min.X+synthSectionPaddingX, innerR.Min.Y+SpaceXS, innerR.Max.X, innerR.Min.Y+synthSectionTitleHeight)
	DrawTextColorAt(dst, sectionLabel(s.id), titleR.Min.X, titleR.Min.Y, TokenTextSecondary())

	if len(s.knobIdxs) == 0 {
		// Collapsed: section has no wired knobs for the current recipe.
		dashR := image.Rect(innerR.Min.X, innerR.Min.Y+innerR.Dy()/2-1, innerR.Max.X, innerR.Min.Y+innerR.Dy()/2+1)
		drawRect(dst, dashR, TokenSurface3(), true)
		return
	}
	for _, kIdx := range s.knobIdxs {
		if kIdx < 0 || kIdx >= len(dv.instEditorKnobs) {
			continue
		}
		k := dv.instEditorKnobs[kIdx]
		k.Draw(dst)
		// Caption under the knob: name + formatted value + unit.
		b := dv.instEditorBindings[kIdx]
		actual := b.def.Min + k.Value*(b.def.Max-b.def.Min)
		caption := synthKnobCaption(b.def, actual)
		captionR := k.Rect()
		captionY := captionR.Min.Y + (captionR.Dy() - synthKnobCaptionHeight) + SpaceXS
		DrawTextColorAt(dst, caption, captionR.Min.X+2, captionY, TokenTextSecondary())
	}
}

// drawSynthOutColumn renders the OUT column launchers. The column itself
// carries a section title ("OUT") so its visual weight matches the
// adjacent knob sections.
func (dv *DrumView) drawSynthOutColumn(dst *ebiten.Image, instID string) {
	if len(dv.instEditorOutLinks) == 0 {
		return
	}
	// Title bounds derive from the first link's rect.
	first := dv.instEditorOutLinks[0].rect
	last := dv.instEditorOutLinks[len(dv.instEditorOutLinks)-1].rect
	cardR := image.Rect(first.Min.X-SpaceSM, first.Min.Y-synthSectionTitleHeight-SpaceXS, last.Max.X+SpaceSM, last.Max.Y+SpaceXS)
	drawRoundedRect(dst, cardR, TokenSurface1(), 8, true)
	drawRoundedRect(dst, cardR, TokenBorderSubtle(), 8, false)
	DrawTextColorAt(dst, "OUT", cardR.Min.X+synthSectionPaddingX, cardR.Min.Y+SpaceXS, TokenTextSecondary())
	for _, link := range dv.instEditorOutLinks {
		if link.rect.Empty() {
			continue
		}
		drawRoundedRect(dst, link.rect, TokenSurface2(), 6, true)
		drawRoundedRect(dst, link.rect, TokenBorderSubtle(), 6, false)
		DrawTextColorAt(dst, link.label(), link.rect.Min.X+SpaceSM, link.rect.Min.Y+SpaceXS, TokenTextPrimary())
		// Chevron-style affordance: draw a small filled triangle on the
		// right edge using drawRect (the icon system already forbids raw
		// `›`; reusing IconChevronRight would add an alloc per link).
		ch := link.rect.Max.Y - link.rect.Min.Y - 6
		if ch < 4 {
			ch = 4
		}
		cx := link.rect.Max.X - SpaceSM - ch/2
		cy := (link.rect.Min.Y + link.rect.Max.Y) / 2
		drawRect(dst, image.Rect(cx, cy-1, cx+ch/2, cy+1), TokenAccent(), true)
	}
}

// drawSynthFooter renders the reset button.
func (dv *DrumView) drawSynthFooter(dst *ebiten.Image) {
	for _, btn := range dv.instEditorBtns {
		drawRoundedRect(dst, btn.Rect(), TokenSurface1(), 6, true)
		drawRoundedRect(dst, btn.Rect(), TokenBorderSubtle(), 6, false)
		text := "Reset"
		if btn.Text != synthResetButtonTag {
			text = btn.Text
		}
		DrawTextColorAt(dst, text, btn.Rect().Min.X+SpaceSM, btn.Rect().Min.Y+SpaceXS, TokenTextPrimary())
	}
}

// synthKnobCaption formats one knob's caption as "name  value[ unit]"
// with a unit-aware suffix for the params whose C transform has a known
// unit (st for semitones, % for normalised 0..1, ×N for multipliers).
func synthKnobCaption(def audio.ParamDef, value float64) string {
	switch def.Name {
	case "pitch":
		return fmt.Sprintf("tune  %+.0f st", value)
	case "decay":
		return fmt.Sprintf("decay  %.2f×", value)
	case "tone":
		return fmt.Sprintf("tone  %+.2f", value)
	case "drive":
		return fmt.Sprintf("drive  %d%%", int(value*100+0.5))
	case "body":
		return fmt.Sprintf("body  %d%%", int(value*100+0.5))
	case "brightness":
		return fmt.Sprintf("bright  %d%%", int(value*100+0.5))
	}
	if def.Unit == "" {
		return fmt.Sprintf("%s  %.2f", def.Name, value)
	}
	return fmt.Sprintf("%s  %.2f %s", def.Name, value, def.Unit)
}

// handleSynthTabInput dispatches a single mouse/touch event into the
// synth tab. Returns true when the event was consumed. Called by
// EQPanelZone.handleInput when activeTab == TabSynth.
func (dv *DrumView) handleSynthTabInput(x, y int, pressed bool, instID string) bool {
	instID = dv.resolveSynthInstrument(instID)
	// Send popover gets first chance — it floats above everything else.
	if dv.instEditorSendPopover != nil {
		if dv.instEditorSendPopover.handleInput(x, y, pressed) {
			return true
		}
		// Tap outside the popover dismisses it.
		if pressed && !image.Pt(x, y).In(dv.instEditorSendPopover.rect) {
			dv.instEditorSendPopover = nil
		}
	}
	// Active knob drag has priority.
	if dv.instEditorDragging {
		if dv.instEditorDragIdx < 0 || dv.instEditorDragIdx >= len(dv.instEditorKnobs) {
			dv.instEditorDragging = false
			return false
		}
		k := dv.instEditorKnobs[dv.instEditorDragIdx]
		result := k.HandleInputResult(x, y, pressed)
		if result == InputCaptured {
			dv.syncSliderFromKnob(dv.instEditorDragIdx)
			dv.propagateSynthSliderValue(dv.instEditorDragIdx, instID)
			return true
		}
		if result == InputConsumed {
			dv.syncSliderFromKnob(dv.instEditorDragIdx)
			dv.propagateSynthSliderValue(dv.instEditorDragIdx, instID)
			dv.instEditorDragging = false
			dv.auditionInstrumentAfterDrag(instID)
			return true
		}
		dv.instEditorDragging = false
	}
	if !pressed {
		return false
	}
	// Knob hit-test.
	for i, k := range dv.instEditorKnobs {
		if image.Pt(x, y).In(k.Rect()) {
			k.HandleInputResult(x, y, pressed)
			dv.instEditorDragging = true
			dv.instEditorDragIdx = i
			dv.syncSliderFromKnob(i)
			dv.propagateSynthSliderValue(i, instID)
			return true
		}
	}
	// Out-column launcher hit-test.
	for _, link := range dv.instEditorOutLinks {
		if image.Pt(x, y).In(link.rect) {
			dv.handleSynthOutLinkPress(link, instID)
			return true
		}
	}
	// Reset button.
	for _, btn := range dv.instEditorBtns {
		if !image.Pt(x, y).In(btn.Rect()) {
			continue
		}
		if btn.Text == synthResetButtonTag && instID != "" {
			audio.ResetInstrumentParams(instID)
			return true
		}
	}
	return false
}

// syncSliderFromKnob copies the Knob's value into the paired Slider so the
// existing propagate/coalesce path (which reads Slider.Value) continues to
// observe the live drag value.
func (dv *DrumView) syncSliderFromKnob(idx int) {
	if idx < 0 || idx >= len(dv.instEditorKnobs) || idx >= len(dv.instEditorSliders) {
		return
	}
	dv.instEditorSliders[idx].Value = dv.instEditorKnobs[idx].Value
}

// handleSynthOutLinkPress dispatches an OUT-column launcher tap. Both
// launchers (Delay, Reverb) open the inline sendLevelPopover so the user
// can adjust the per-instrument send without leaving the tab.
func (dv *DrumView) handleSynthOutLinkPress(link synthOutLink, instID string) {
	switch link.kind {
	case synthOutDelay:
		dv.openSendPopover(link, instID, synthSendDelay)
	case synthOutReverb:
		dv.openSendPopover(link, instID, synthSendReverb)
	}
}

// ---- Send-level popover -------------------------------------------------

type synthSendKind int

const (
	synthSendDelay synthSendKind = iota
	synthSendReverb
)

// sendLevelPopover is the floating slider that drives audio.SetDelaySend
// / audio.SetReverbSend in place. Tap-outside dismisses (handled in
// handleSynthTabInput). The popover keeps the user on the Synth tab so
// adjusting a send level doesn't require a navigation hop.
type sendLevelPopover struct {
	kind     synthSendKind
	instID   string
	rect     image.Rectangle
	slider   *Slider
	dragging bool
}

func (dv *DrumView) openSendPopover(link synthOutLink, instID string, kind synthSendKind) {
	if instID == "" {
		return
	}
	var current float64
	switch kind {
	case synthSendDelay:
		current = audio.DelaySend(instID)
	case synthSendReverb:
		current = audio.ReverbSend(instID)
	}
	// Place popover to the left of the link rect, vertically centred on it.
	popW, popH := 200, 36
	rect := image.Rect(link.rect.Min.X-popW-SpaceSM, link.rect.Min.Y-(popH-link.rect.Dy())/2, link.rect.Min.X-SpaceSM, link.rect.Max.Y+(popH-link.rect.Dy())/2)
	if rect.Min.X < 0 {
		// Flip to the right if there's no space on the left.
		rect = image.Rect(link.rect.Max.X+SpaceSM, link.rect.Min.Y-(popH-link.rect.Dy())/2, link.rect.Max.X+SpaceSM+popW, link.rect.Max.Y+(popH-link.rect.Dy())/2)
	}
	sl := NewSlider(current)
	sl.SetRect(image.Rect(rect.Min.X+SpaceSM, rect.Min.Y+SpaceXS, rect.Max.X-SpaceSM, rect.Max.Y-SpaceXS))
	dv.instEditorSendPopover = &sendLevelPopover{
		kind:   kind,
		instID: instID,
		rect:   rect,
		slider: sl,
	}
}

func (p *sendLevelPopover) handleInput(x, y int, pressed bool) bool {
	if p == nil {
		return false
	}
	if !p.dragging && !image.Pt(x, y).In(p.rect) {
		return false
	}
	result := p.slider.HandleInputResult(x, y, pressed)
	if result == InputIgnored {
		// Click inside popover frame but outside the slider track — keep
		// the popover open and consume the event so tap-outside doesn't
		// fire on the same press.
		if pressed {
			return true
		}
		return false
	}
	if result == InputCaptured {
		p.dragging = true
		p.commit()
		return true
	}
	if result == InputConsumed {
		p.dragging = false
		p.commit()
		return true
	}
	return false
}

func (p *sendLevelPopover) commit() {
	switch p.kind {
	case synthSendDelay:
		audio.SetDelaySend(p.instID, p.slider.Value)
	case synthSendReverb:
		audio.SetReverbSend(p.instID, p.slider.Value)
	}
}

func (p *sendLevelPopover) draw(dst *ebiten.Image) {
	if p == nil || p.rect.Empty() {
		return
	}
	drawRoundedRect(dst, p.rect, TokenSurface2(), 8, true)
	drawRoundedRect(dst, p.rect, TokenBorderMedium(), 8, false)
	p.slider.Draw(dst)
}

// ---- Hit areas + propagate ---------------------------------------------

// propagateSynthSliderValue rescales a 0..1 slider value into the
// ParamDef's [Min, Max] range and pushes it through audio.SetInstrumentParam.
// Coalescing matches the pre-redesign behaviour: stationary drag bursts
// don't flood the JS bridge — see
// instrument_params_playback_stability.browser.test.js.
func (dv *DrumView) propagateSynthSliderValue(idx int, instID string) {
	if idx < 0 || idx >= len(dv.instEditorSliders) || idx >= len(dv.instEditorBindings) {
		return
	}
	if instID == "" {
		return
	}
	sl := dv.instEditorSliders[idx]
	b := dv.instEditorBindings[idx]
	actual := b.def.Min + sl.Value*(b.def.Max-b.def.Min)
	current := audio.GetInstrumentParams(instID)
	if prev, ok := current[b.def.Name]; ok {
		span := b.def.Max - b.def.Min
		if span <= 0 {
			span = 1
		}
		epsilon := span * 1e-4
		if abs64(prev-actual) < epsilon {
			return
		}
	}
	audio.SetInstrumentParam(instID, b.def.Name, actual)
}

// abs64 is a small inline absolute value helper.
func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// synthAuditionFn is the hook the Synth tab uses to play a one-shot of the
// edited instrument when the user releases a knob. Production code points
// at audio.Play; tests swap it via SwapSynthAuditionFnForTest to assert the
// audition fires the expected number of times without producing real sound.
//
// This is the entirety of Phase 10 — there is no parallel "preview voice"
// path. Reusing audio.Play means the audition flows through the same
// trigger plumbing as a sequencer hit, so the effect chain, voice cache
// invalidation, send-FX, and mixer behave identically.
var synthAuditionFn = func(instID string) {
	audio.Play(instID)
}

// SwapSynthAuditionFnForTest installs a test fake for the audition trigger
// and returns the previous value. Mirrors the SwapPlatformInstrumentParamsChangedForTest
// pattern in internal/audio so cross-package tests can observe the audition
// without making real audio.
func SwapSynthAuditionFnForTest(fn func(string)) func(string) {
	prev := synthAuditionFn
	if fn != nil {
		synthAuditionFn = fn
	}
	return prev
}

// auditionInstrumentAfterDrag is the single call site for the post-release
// audition. The caller must invoke this only on a genuine drag release (not
// at drag-start or drag-continue) so the user hears exactly one playback per
// knob movement. instID is the already-resolved instrument id; the empty
// string is a no-op so callers don't have to gate on resolveSynthInstrument
// returning a value.
func (dv *DrumView) auditionInstrumentAfterDrag(instID string) {
	if instID == "" {
		return
	}
	synthAuditionFn(instID)
}

// resolveSynthInstrument resolves the editor's target row instrument id.
// Mirrors synthTabActiveInstrument but accepts a forced id (used by the
// callback bridge from EQPanelZone).
func (dv *DrumView) resolveSynthInstrument(instID string) string {
	if instID != "" {
		return instID
	}
	return dv.synthTabActiveInstrument()
}

// synthTabActiveInstrument resolves "which instrument's params is the
// synth tab editing right now?" Mirrors the channel-resolution logic in
// other audio tabs: prefer the EQ panel's active channel, fall back to
// the first audible/non-empty row.
func (dv *DrumView) synthTabActiveInstrument() string {
	if dv.eqPanelZone != nil {
		ch := dv.eqPanelZone.ActiveChannel()
		if ch != "" && ch != "main" {
			return ch
		}
	}
	for _, r := range dv.Rows {
		if r != nil && r.Instrument != "" {
			return r.Instrument
		}
	}
	return ""
}

// synthTabHitAreas constructs HitArea entries for every knob, OUT-column
// link, send popover slider, and reset button. Z-index sits one tier
// above the EQ panel chrome.
func (dv *DrumView) synthTabHitAreas() []HitArea {
	instID := dv.synthTabActiveInstrument()
	z := ZEQPanel + 1
	out := make([]HitArea, 0, len(dv.instEditorKnobs)+len(dv.instEditorOutLinks)+len(dv.instEditorBtns)+1)
	for i, k := range dv.instEditorKnobs {
		if k == nil {
			continue
		}
		r := k.Rect()
		if r.Empty() {
			continue
		}
		out = append(out, HitArea{
			Rect:     r,
			ZIndex:   z,
			Handler:  &synthKnobHitAdapter{dv: dv, idx: i, instID: instID},
			Tag:      fmt.Sprintf("synth-knob-%d", i),
			Touch:    true,
			ClipRect: r.Inset(-8),
		})
		// Also publish under the legacy "synth-slider-%d" tag so existing
		// browser tests / hit-area tests that target sliders still resolve.
		out = append(out, HitArea{
			Rect:     r,
			ZIndex:   z,
			Handler:  &synthKnobHitAdapter{dv: dv, idx: i, instID: instID},
			Tag:      fmt.Sprintf("synth-slider-%d", i),
			Touch:    true,
			ClipRect: r.Inset(-8),
		})
	}
	for i, link := range dv.instEditorOutLinks {
		if link.rect.Empty() {
			continue
		}
		linkCopy := link
		out = append(out, HitArea{
			Rect:    link.rect,
			ZIndex:  z,
			Handler: &synthOutLinkHitAdapter{dv: dv, link: linkCopy, instID: instID},
			Tag:     fmt.Sprintf("synth-out-%d", i),
			Touch:   true,
		})
	}
	if dv.instEditorSendPopover != nil {
		p := dv.instEditorSendPopover
		out = append(out, HitArea{
			Rect:    p.rect,
			ZIndex:  z + 1,
			Handler: &synthSendPopoverHitAdapter{dv: dv, popover: p},
			Tag:     "synth-send-popover",
		})
	}
	for i, btn := range dv.instEditorBtns {
		if btn == nil {
			continue
		}
		r := btn.Rect()
		if r.Empty() {
			continue
		}
		out = append(out, HitArea{
			Rect:    r,
			ZIndex:  z,
			Handler: &synthResetHitAdapter{instID: instID},
			Tag:     fmt.Sprintf("synth-btn-%d", i),
			Touch:   true,
		})
	}
	return out
}

// synthKnobHitAdapter wraps a per-knob hit area with the synth-tab
// propagate callback. The adapter does NOT cache the *Knob pointer —
// buildSynthTab can rebuild instEditorKnobs mid-drag (analyzer state
// changes, channel switches, …), which would leave a cached pointer
// dangling against a now-orphan widget. Looking up by index every call
// stays correct across re-layouts. The captured pressY / pressVal stay
// inside the live Knob, so a re-layout preserves drag state because the
// new knob slot inherits the same Value from audio.MergeRecipeDefaults.
//
// The fresh-instance lookup also fixes a long-standing knob-drag bug:
// before, the captured *Knob would receive HandleInputResult calls that
// updated its own Value, but the visible widget (instEditorKnobs[idx])
// was a different object whose Value never changed — making the drag
// look unresponsive.
type synthKnobHitAdapter struct {
	dv     *DrumView
	idx    int
	instID string
	active bool
}

func (h *synthKnobHitAdapter) currentKnob() *Knob {
	if h.dv == nil || h.idx < 0 || h.idx >= len(h.dv.instEditorKnobs) {
		return nil
	}
	return h.dv.instEditorKnobs[h.idx]
}

func (h *synthKnobHitAdapter) OnPress(x, y int) InputResult {
	k := h.currentKnob()
	if k == nil {
		return InputIgnored
	}
	result := k.HandleInputResult(x, y, true)
	if result != InputIgnored {
		h.active = true
		h.dv.syncSliderFromKnob(h.idx)
		h.dv.propagateSynthSliderValue(h.idx, h.instID)
		return InputCaptured
	}
	return InputIgnored
}

func (h *synthKnobHitAdapter) OnDrag(x, y int) {
	if !h.active {
		return
	}
	k := h.currentKnob()
	if k == nil {
		h.active = false
		return
	}
	k.HandleInputResult(x, y, true)
	h.dv.syncSliderFromKnob(h.idx)
	h.dv.propagateSynthSliderValue(h.idx, h.instID)
}

func (h *synthKnobHitAdapter) OnRelease(x, y int) {
	if !h.active {
		return
	}
	k := h.currentKnob()
	if k != nil {
		k.HandleInputResult(x, y, false)
		h.dv.syncSliderFromKnob(h.idx)
		h.dv.propagateSynthSliderValue(h.idx, h.instID)
	}
	h.active = false
	h.dv.auditionInstrumentAfterDrag(h.instID)
}

func (h *synthKnobHitAdapter) OnWheel(x, y, steps int) InputResult {
	k := h.currentKnob()
	if k == nil {
		return InputIgnored
	}
	result := k.HandleWheel(x, y, steps)
	if result == InputConsumed {
		h.dv.syncSliderFromKnob(h.idx)
		h.dv.propagateSynthSliderValue(h.idx, h.instID)
	}
	return result
}

// synthSliderHitAdapter is the legacy name kept so any external callers
// (or older tests) that reference the symbol continue to compile. It
// wraps the new knob adapter so behaviour is identical.
type synthSliderHitAdapter = synthKnobHitAdapter

// synthOutLinkHitAdapter dispatches OUT-column launcher taps.
type synthOutLinkHitAdapter struct {
	dv     *DrumView
	link   synthOutLink
	instID string
}

func (h *synthOutLinkHitAdapter) OnPress(x, y int) InputResult {
	h.dv.handleSynthOutLinkPress(h.link, h.instID)
	return InputCaptured
}

func (h *synthOutLinkHitAdapter) OnDrag(x, y int)                     {}
func (h *synthOutLinkHitAdapter) OnRelease(x, y int)                  {}
func (h *synthOutLinkHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// synthSendPopoverHitAdapter routes pointer events into the send-level
// popover slider.
type synthSendPopoverHitAdapter struct {
	dv      *DrumView
	popover *sendLevelPopover
}

func (h *synthSendPopoverHitAdapter) OnPress(x, y int) InputResult {
	if h.popover.handleInput(x, y, true) {
		return InputCaptured
	}
	return InputIgnored
}

func (h *synthSendPopoverHitAdapter) OnDrag(x, y int) { h.popover.handleInput(x, y, true) }

func (h *synthSendPopoverHitAdapter) OnRelease(x, y int) { h.popover.handleInput(x, y, false) }

func (h *synthSendPopoverHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// synthResetHitAdapter clears the per-instrument param map when the reset
// button is clicked.
type synthResetHitAdapter struct {
	instID string
}

func (h *synthResetHitAdapter) OnPress(x, y int) InputResult {
	if h.instID != "" {
		audio.ResetInstrumentParams(h.instID)
	}
	return InputCaptured
}

func (h *synthResetHitAdapter) OnDrag(x, y int)                     {}
func (h *synthResetHitAdapter) OnRelease(x, y int)                  {}
func (h *synthResetHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// ---- Test accessors ----------------------------------------------------

// SynthTabSliders exposes the slider value-store for tests.
func (dv *DrumView) SynthTabSliders() []*Slider { return dv.instEditorSliders }

// SynthTabKnobs exposes the visible Knob widgets for tests.
func (dv *DrumView) SynthTabKnobs() []*Knob { return dv.instEditorKnobs }

// SynthTabBindings exposes the widget→param map for tests.
func (dv *DrumView) SynthTabBindings() []instParamBinding { return dv.instEditorBindings }

// SynthTabButtons exposes the editor buttons (currently just reset).
func (dv *DrumView) SynthTabButtons() []*Button { return dv.instEditorBtns }

// SynthTabGroupHeaders is retained for legacy tests. The redesigned panel
// uses section cards instead of inline group dividers, so the returned
// slice is always empty.
func (dv *DrumView) SynthTabGroupHeaders() []synthGroupHeader { return dv.instEditorGroups }

// SynthTabSections exposes the section card layout for tests.
func (dv *DrumView) SynthTabSections() []synthSection { return dv.instEditorSections }

// SynthTabOutLinks exposes the OUT-column launchers for tests.
func (dv *DrumView) SynthTabOutLinks() []synthOutLink { return dv.instEditorOutLinks }

// SynthTabHeader exposes the header layout state for tests.
func (dv *DrumView) SynthTabHeader() synthHeaderLayout { return dv.instEditorHeader }

// SynthTabSendPopover exposes the send-level popover for tests (nil when
// no popover is open).
func (dv *DrumView) SynthTabSendPopover() *sendLevelPopover { return dv.instEditorSendPopover }

// SectionID returns the section's enum id (test-only accessor).
func (s synthSection) SectionID() synthSectionID { return s.id }

// Label returns the human-readable section label (test-only accessor).
func (s synthSection) Label() string { return sectionLabel(s.id) }

// KnobCount returns the number of knobs assigned to the section
// (test-only accessor).
func (s synthSection) KnobCount() int { return len(s.knobIdxs) }

// Rect returns the section card's bounding rectangle (test-only).
func (s synthSection) Rect() image.Rectangle { return s.rect }

// Kind returns the OUT-column launcher kind (test-only accessor).
func (l synthOutLink) Kind() synthOutLinkKind { return l.kind }

// Label returns the user-visible label for the launcher (test-only).
func (l synthOutLink) DisplayLabel() string { return l.label() }

// Rect returns the launcher's bounding rectangle (test-only).
func (l synthOutLink) Rect() image.Rectangle { return l.rect }

// Amount exposes the Delay/Reverb send amount for tests.
func (l synthOutLink) Amount() float64 { return l.amount }

// Kind exposes the popover's send kind for tests.
func (p *sendLevelPopover) Kind() synthSendKind { return p.kind }

// Slider exposes the popover slider for tests.
func (p *sendLevelPopover) Slider() *Slider { return p.slider }

// Rect exposes the popover frame for tests.
func (p *sendLevelPopover) PopoverRect() image.Rectangle { return p.rect }

// EnsureNoTrailingNewline is a tiny helper kept around so a fmt+strings
// import set is not silently dropped when the file is edited above.
func ensureSynthImports() string { return strings.ToUpper("") }
