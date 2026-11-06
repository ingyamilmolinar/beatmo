package ui

// synth_macro_render.go renders the Synth tab's category layer inside the
// content area — the ONE flow every synth instrument gets:
//
//   - ROOT (synthAtMacroRoot): the PIPELINE STRIP (active stages in signal
//     order, synth_pipeline_strip.go) above a grid of CATEGORY cards (Sound,
//     Brightness, …), each a plain-English label + one-liner + a signature
//     graph + up to three MACRO DIALS (synth_dials.go) that move many params
//     at once. Tapping a card (off its dials) or a stage pill DRILLS in.
//   - DRILLED: a header row (Back · title · one-liner · the stage's enable
//     pill when the target maps to exactly one stage) above the target's
//     ESSENTIAL params as ParamCards (tap to arm, barrels, all unchanged), then
//     a "More" divider that expands the remaining fine-tuning params.
//
// It reuses the existing primitives (ControlGrid, Button + buttonHitAdapter,
// drawParamCard/drawParamCardGauge, the FX toggle pill, the Advanced divider,
// growArmedCard) so nothing here is a new component.

import (
	"image"
	"image/color"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// synthBackButtonTag / synthMoreButtonTag are the sentinel Button texts for
// the drilled view's Back button and the More divider's input button (both
// live in instEditorBtns so the existing button loop publishes their hit
// areas). Like the header sentinels, the leading \x00 keeps them off screen.
const (
	synthBackButtonTag = "\x00synth-back"
	synthMoreButtonTag = "\x00synth-more"
)

// macroCardSlot is one laid-out category card at the root. dials is a fixed
// array (no per-layout allocation); nDials is how many are in use, and
// dialRects their screen rects (the hit areas; drawing recomputes the same
// geometry sprite-locally via macroCardDialRects).
type macroCardSlot struct {
	id        string
	spriteID  string
	label     string
	gloss     string
	r         image.Rectangle
	dials     [synthdial.MaxDialsPerCard]macroDialSlot
	nDials    int
	dialRects [synthdial.MaxDialsPerCard]image.Rectangle
}

// macroCardDials fills a card's dial slots: each dial's reading is a pure
// function of the instrument's current values.
func (dv *DrumView) macroCardDials(cg macroGroup, resolved instrument.ID, out *[synthdial.MaxDialsPerCard]macroDialSlot) int {
	n := 0
	for _, d := range cg.Dials {
		if n == synthdial.MaxDialsPerCard {
			break
		}
		out[n] = macroDialSlot{
			id:      d.Dial.ID,
			label:   i18n.T(i18n.Key(d.Dial.Label)),
			reading: dv.synthDialReading(resolved, d),
		}
		n++
	}
	return n
}

// bindingIndexByName maps param name → knob/binding index for the active
// instrument. Cached on the DrumView and rebuilt only when the binding slice
// changed shape (buildSynthTab rebuilds bindings only on a schema swap), so
// the per-frame layout/draw paths allocate nothing here.
func (dv *DrumView) bindingIndexByName() map[string]int {
	if dv.instEditorBindingIdx != nil && len(dv.instEditorBindingIdx) == len(dv.instEditorBindings) &&
		dv.instEditorBindingIdxGen == dv.instEditorBindingsGen {
		return dv.instEditorBindingIdx
	}
	byName := make(map[string]int, len(dv.instEditorBindings))
	for i, b := range dv.instEditorBindings {
		byName[b.def.Name] = i
	}
	dv.instEditorBindingIdx = byName
	dv.instEditorBindingIdxGen = dv.instEditorBindingsGen
	return byName
}

// macroLayerHeaderH is the drilled view's header band: Back button, title +
// one-liner, and the stage pill share one row.
func macroLayerHeaderH() int {
	textH := StyledTextHeight(RoleBody) + 1 + StyledTextHeight(RoleCaption)
	btnH := Profile().DensityValues().SynthHeaderButtonH
	if btnH > textH {
		textH = btnH
	}
	return textH + SpaceSM
}

// macroRootCardH is the root card height: title + one-liner + rows dial rows.
func macroRootCardH(rows int) int {
	return 2*macroCardInset() + macroCardTopH() + rows*(SpaceXS+macroDialRowH())
}

// macroRootCardShortH is the root card height without the dial row (a root
// where no card has a live dial).
func macroRootCardShortH() int {
	return StyledTextHeight(RoleBody) + StyledTextHeight(RoleCaption) + SpaceSM + SpaceXS
}

// layoutSynthMacroLayer positions the category layer inside detailR (the
// content region left of the preview pane). Called from buildSynthTab.
func (dv *DrumView) layoutSynthMacroLayer(detailR image.Rectangle, resolved instrument.ID, mobile bool) {
	p := dv.synp()
	dv.instEditorMacroDetailR = detailR
	dv.instEditorMacroCards = dv.instEditorMacroCards[:0]
	dv.instEditorMacroBackBtn = nil
	dv.instEditorMoreBtn = nil
	dv.instEditorMacroPillRect = image.Rectangle{}
	dv.instEditorMacroPillSec = -1
	dv.instEditorMacroTitle, dv.instEditorMacroGloss = "", ""
	dv.synthAdvDividerRect = image.Rectangle{}
	dv.synthAdvDividerCount = 0
	dv.instEditorPipePills = dv.instEditorPipePills[:0]
	dv.instEditorPipeAddBtn = nil
	dv.instEditorPipeRect = image.Rectangle{}
	dv.instEditorMobileFocusRect = image.Rectangle{}
	// Every knob starts hidden; only the drilled target's knobs get a rect below
	// (the per-knob hit loop + card draw key off a non-empty rect).
	for i := range dv.instEditorControls {
		if dv.instEditorControls[i] != nil {
			dv.instEditorControls[i].SetRect(image.Rectangle{})
		}
		if i < len(dv.instEditorSliders) && dv.instEditorSliders[i] != nil {
			dv.instEditorSliders[i].SetRect(image.Rectangle{})
		}
	}
	if detailR.Empty() {
		return
	}
	grid := dv.synp().sectionGrid(-1)
	inner := image.Rect(
		detailR.Min.X+synthSectionPaddingX,
		detailR.Min.Y+synthSectionPaddingY,
		detailR.Max.X-synthSectionPaddingX,
		detailR.Max.Y-synthSectionPaddingY,
	)
	if inner.Empty() {
		grid.Layout(image.Rectangle{}, 0, 1, 1, 0, 0)
		return
	}
	target := dv.synthDrillTarget(resolved)
	if target.kind == drillNone {
		p.layoutMacroRoot(grid, inner, resolved, mobile)
		return
	}
	p.layoutMacroDrilled(grid, inner, resolved, target, mobile)
}

// layoutMacroRoot: pipeline strip band on top, category cards below.
//
// The production UI (devtools.go) has no pipeline strip and shows only the
// cards with a live dial (productionMacroCards), as compact cards
// (synthCompactCards): title + dials.
func (p *synthPanel) layoutMacroRoot(grid *ControlGrid, inner image.Rectangle, resolved instrument.ID, mobile bool) {
	dv := p.dv
	cardsR := inner
	if devToolsOn() {
		on, off := p.activePipelineStages(resolved)
		nPills := len(on)
		if len(off) > 0 {
			nPills++
		}
		stripH := p.pipelineStripHeight(mobile, nPills, inner.Dx())
		if stripH > inner.Dy()/2 {
			stripH = inner.Dy() / 2
		}
		band := image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, inner.Min.Y+stripH)
		p.layoutPipelineStrip(band, resolved, mobile)
		cardsR = image.Rect(inner.Min.X, band.Max.Y+SpaceSM, inner.Max.X, inner.Max.Y)
	}
	if cardsR.Empty() {
		grid.Layout(image.Rectangle{}, 0, 1, 1, 0, 0)
		return
	}
	cards := dv.instEditorMacros
	if !devToolsOn() {
		if mobile {
			// Phone: the graph is a band above the cards (no side pane).
			bandH := max(120, cardsR.Dy()*38/100)
			if bandH < cardsR.Dy()-60 {
				dv.instEditorMobileFocusRect = image.Rect(cardsR.Min.X, cardsR.Min.Y, cardsR.Max.X, cardsR.Min.Y+bandH)
				cardsR.Min.Y += bandH + SpaceSM
			}
		}
		p.layoutProductionCards(grid, cardsR, resolved, productionMacroCards(cards))
		return
	}
	cols := 3
	if mobile {
		cols = 1
	} else if n := len(cards); n > 0 {
		cols = (n + 1) / 2
		if cols < 3 {
			cols = 3
		}
		if cols > 4 {
			cols = 4
		}
	}
	grid.SetMaxCols(cols)
	idealW := cardsR.Dx()/cols - SpaceSM
	if idealW < 120 {
		idealW = 120
	}
	grid.SetSmoothScroll(true)
	// No peek band: a card row is ~half the phone's cards area, so the band
	// would cost the second card (the scrollbar already shows there is more).
	grid.SetPeekBand(false)
	// One uniform card height: tall enough for the most dial rows any card
	// needs at the card width the grid gives (rows of up to three; a card with
	// four to six dials takes two). The grid scrolls when the cards outgrow
	// the panel. Laid out once at the ideal width, then again if the real cell
	// width asks for a different row count.
	cellHFor := func(cardW int) int {
		rows := 0
		for _, cg := range cards {
			rows = max(rows, macroDialRows(min(len(cg.Dials), synthdial.MaxDialsPerCard), cardW))
		}
		if rows == 0 {
			return macroRootCardShortH()
		}
		return macroRootCardH(rows)
	}
	cellH := cellHFor(idealW)
	grid.Layout(cardsR, len(cards), idealW, cellH, SpaceSM, SpaceSM)
	// The grid's cell width, not CellRect(0): the first cell is not visible
	// once the root scrolls, and the real width (minus the scrollbar) can ask
	// for another row (TestScrolledRootKeepsEveryDial).
	if cw := grid.CellW(); cw > 0 {
		if h := cellHFor(cw); h != cellH {
			cellH = h
			grid.Layout(cardsR, len(cards), idealW, cellH, SpaceSM, SpaceSM)
		}
	}
	for i, cg := range cards {
		slot, vis := grid.CellRect(i)
		if !vis {
			continue
		}
		card := macroCardSlot{id: cg.ID, spriteID: cg.spriteID, label: cg.Label, gloss: cg.Gloss, r: slot}
		card.nDials = dv.macroCardDials(cg, resolved, &card.dials)
		card.dialRects = macroCardDialRects(slot, card.nDials)
		dv.instEditorMacroCards = append(dv.instEditorMacroCards, card)
	}
}

// drilledKnobIdxs returns the drilled target's knob indices split into
// essentials and fine-tuning (More), in display order.
func (dv *DrumView) drilledKnobIdxs(target synthDrillTarget) (essential, advanced []int) {
	byName := dv.bindingIndexByName()
	essential = dv.instEditorDrillEss[:0]
	advanced = dv.instEditorDrillAdv[:0]
	defer func() { dv.instEditorDrillEss, dv.instEditorDrillAdv = essential, advanced }()
	switch target.kind {
	case drillCategory:
		cg := dv.macroConceptByID(target.cat)
		if cg == nil {
			return nil, nil
		}
		isEss := make(map[string]bool, len(cg.Essentials))
		for _, n := range cg.Essentials {
			isEss[n] = true
		}
		for _, name := range cg.Params {
			idx, ok := byName[name]
			if !ok {
				continue
			}
			if isEss[name] {
				essential = append(essential, idx)
			} else {
				advanced = append(advanced, idx)
			}
		}
	case drillStage:
		for i := range dv.instEditorSections {
			if dv.instEditorSections[i].id == target.stage {
				return dv.synthSectionTierSplit(dv.instEditorSections[i].knobIdxs)
			}
		}
	}
	return essential, advanced
}

// macroConceptKnobIdxs returns every bound knob index of category id, essentials
// first — the drilled order.
func (dv *DrumView) macroConceptKnobIdxs(id string) []int {
	e, a := dv.drilledKnobIdxs(synthDrillTarget{kind: drillCategory, cat: id})
	return append(e, a...)
}

// drilledStageForPill resolves the section whose enable pill the drilled header
// shows: the drilled stage itself, or a category's single stage. ok=false when
// the target spans several stages or the section carries no pill.
func (dv *DrumView) drilledStageForPill(target synthDrillTarget) (secIdx int, ok bool) {
	var want synthSectionID = -1
	switch target.kind {
	case drillStage:
		want = target.stage
	case drillCategory:
		if cg := dv.macroConceptByID(target.cat); cg != nil && len(cg.Stages) == 1 {
			want = cg.Stages[0]
		}
	}
	if want < 0 {
		return -1, false
	}
	for i := range dv.instEditorSections {
		s := &dv.instEditorSections[i]
		if s.id == want {
			return i, synthSectionHasPill(s.id, s.enableParam)
		}
	}
	return -1, false
}

// drilledTitle returns the drilled header's title + one-liner.
func (dv *DrumView) drilledTitle(target synthDrillTarget) (title, gloss string) {
	switch target.kind {
	case drillCategory:
		if cg := dv.macroConceptByID(target.cat); cg != nil {
			return cg.Label, cg.Gloss
		}
	case drillStage:
		return sectionLabelLocalized(target.stage), sectionSubtitle(target.stage)
	}
	return "", ""
}

// layoutMacroDrilled: header row (Back · title/gloss · pill), the mobile graph
// band, then essentials + More in the grid.
func (p *synthPanel) layoutMacroDrilled(grid *ControlGrid, inner image.Rectangle, resolved instrument.ID, target synthDrillTarget, mobile bool) {
	dv := p.dv
	dv2 := Profile().DensityValues()
	headerH := macroLayerHeaderH()
	if headerH > inner.Dy() {
		headerH = inner.Dy()
	}
	hdr := image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, inner.Min.Y+headerH)

	// Back button (left).
	btnH := dv2.SynthHeaderButtonH
	if btnH > hdr.Dy() {
		btnH = hdr.Dy()
	}
	// Square icon-only Back button (the label is redundant next to the caret).
	backW := btnH + SpaceSM
	backR := image.Rect(hdr.Min.X, hdr.Min.Y+(hdr.Dy()-btnH)/2, hdr.Min.X+backW, hdr.Min.Y+(hdr.Dy()-btnH)/2+btnH)
	if p.backBtn == nil {
		p.backBtn = NewSpecButton(synthBackButtonTag, ComponentButtonSecondary, nil)
		p.backBtn.Icon = string(IconChevronLeft)
	}
	back := p.backBtn
	back.OnClick = func() {
		dv.setSynthMacroOpen(resolved, "")
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.Invalidate()
		}
	}
	back.SetRect(backR)
	dv.instEditorMacroBackBtn = back
	dv.instEditorBtns = append(dv.instEditorBtns, back)

	// Stage enable pill (right), when the target maps to one pilled stage.
	if secIdx, ok := dv.drilledStageForPill(target); ok {
		pillW := FXToggleTrackW() + 2*SpaceXS
		pillH := FXToggleTrackH() + 2*SpaceXS
		if pillH <= hdr.Dy() && hdr.Max.X-pillW > backR.Max.X+SpaceMD {
			dv.instEditorMacroPillRect = image.Rect(hdr.Max.X-pillW, hdr.Min.Y+(hdr.Dy()-pillH)/2, hdr.Max.X, hdr.Min.Y+(hdr.Dy()-pillH)/2+pillH)
			dv.instEditorMacroPillSec = secIdx
		}
	}
	dv.instEditorMacroTitle, dv.instEditorMacroGloss = dv.drilledTitle(target)

	bodyTop := hdr.Max.Y + SpaceSM
	// Mobile: the signal graph card sits above the controls (no side pane).
	if mobile {
		gh := 3 * dv2.SynthRightCardMinH
		if gh > (inner.Max.Y-bodyTop)/2 {
			gh = (inner.Max.Y - bodyTop) / 2
		}
		if gh > 0 {
			dv.instEditorMobileFocusRect = image.Rect(inner.Min.X, bodyTop, inner.Max.X, bodyTop+gh)
			bodyTop += gh + SpaceSM
		}
	}
	body := image.Rect(inner.Min.X, bodyTop, inner.Max.X, inner.Max.Y)
	if body.Empty() {
		grid.Layout(image.Rectangle{}, 0, 1, 1, 0, 0)
		return
	}
	essential, advanced := dv.drilledKnobIdxs(target)
	dv.instEditorDrilledOrder = append(append(dv.instEditorDrilledOrder[:0], essential...), advanced...)
	moreOpen := dv.synthMoreOpen(resolved)
	visible := dv.layoutTieredGrid(grid, body, essential, advanced, moreOpen)
	for i, kIdx := range visible {
		if kIdx < 0 {
			continue
		}
		slot, vis := grid.CellRect(i)
		if !vis || kIdx >= len(dv.instEditorControls) {
			continue
		}
		dv.instEditorControls[kIdx].SetRect(slot)
		if kIdx < len(dv.instEditorSliders) {
			dv.instEditorSliders[kIdx].SetRect(slot)
		}
	}
	if len(advanced) > 0 && !dv.synthAdvDividerRect.Empty() {
		if p.moreBtn == nil {
			p.moreBtn = NewSpecButton(synthMoreButtonTag, ComponentButtonSecondary, nil)
		}
		more := p.moreBtn
		more.OnClick = func() {
			dv.setSynthMoreOpen(resolved, !dv.synthMoreOpen(resolved))
			if dv.eqPanelZone != nil {
				dv.eqPanelZone.Invalidate()
			}
		}
		more.SetRect(dv.synthAdvDividerRect)
		dv.instEditorMoreBtn = more
		dv.instEditorBtns = append(dv.instEditorBtns, more)
	}
	// An armed control grows its card in place to host the two barrel lanes —
	// the same post-slot extension the stage detail pane applied.
	if idx, active := dv.armGrowControl(armedScopeSynth, resolved); active {
		dv.growArmedCard(visible, grid, body, idx)
	}
}

// layoutTieredGrid lays essentials, then (when there are fine-tuning params) a
// full phantom divider row, then — only when moreOpen — the advanced params.
// Returns the laid-out index list (-1 = phantom cell) and stores the divider
// band in dv.synthAdvDividerRect / dv.synthAdvDividerCount. Two-pass: the
// first Layout discovers the column count for this width, the second lays the
// padded list for real; the scroll offset is saved around the probe so the
// probe's smaller row count cannot clamp the window down.
func (dv *DrumView) layoutTieredGrid(grid *ControlGrid, innerR image.Rectangle, essential, advanced []int, moreOpen bool) []int {
	dv2 := Profile().DensityValues()
	hGap, vGap := SpaceSM, SpaceXS
	cellH := synthControlCellHeight()
	if availH := innerR.Dy(); cellH > availH {
		cellH = availH
	}
	if cellH < 1 {
		cellH = 1
	}
	grid.SetMaxCols(synthMaxGridCols)
	grid.SetPeekBand(true) // shared with the root, which opts out
	dv.synthAdvDividerRect = image.Rectangle{}
	dv.synthAdvDividerCount = len(advanced)
	visible := essential
	dividerStart, dividerEnd := -1, -1
	if len(advanced) > 0 {
		savedFirst := grid.Scroll().VS.First
		probeCount := len(essential) + len(advanced)
		if probeCount < synthMaxGridCols {
			probeCount = synthMaxGridCols
		}
		grid.Layout(innerR, probeCount, dv2.ParamCardW, cellH, hGap, vGap)
		cols := grid.Cols()
		if cols < 1 {
			cols = 1
		}
		grid.Scroll().VS.First = savedFirst
		padded := append([]int{}, essential...)
		for len(padded)%cols != 0 {
			padded = append(padded, -1)
		}
		dividerStart = len(padded)
		for i := 0; i < cols; i++ {
			padded = append(padded, -1)
		}
		dividerEnd = len(padded)
		visible = padded
		if moreOpen {
			visible = append(padded, advanced...)
		}
	}
	grid.Layout(innerR, len(visible), dv2.ParamCardW, cellH, hGap, vGap)
	if dividerStart >= 0 {
		var dr image.Rectangle
		for i := dividerStart; i < dividerEnd; i++ {
			if c, vis := grid.CellRect(i); vis {
				dr = dr.Union(c)
			}
		}
		dv.synthAdvDividerRect = dr
	}
	// Hidden (collapsed) advanced knobs keep empty rects.
	if !moreOpen {
		for _, kIdx := range advanced {
			if kIdx >= 0 && kIdx < len(dv.instEditorControls) {
				dv.instEditorControls[kIdx].SetRect(image.Rectangle{})
				if kIdx < len(dv.instEditorSliders) {
					dv.instEditorSliders[kIdx].SetRect(image.Rectangle{})
				}
			}
		}
	}
	return visible
}

// drawSynthMacroLayer paints the category layer (root: strip + cards; drilled:
// header + param cards + More).
func (p *synthPanel) drawSynthMacroLayer(dst *ebiten.Image, resolved instrument.ID) {
	dv := p.dv
	detail := dv.instEditorMacroDetailR
	if detail.Empty() {
		return
	}
	drawRoundedRect(dst, detail, TokenSurface1(), RadiusSM, true)
	drawRoundedRect(dst, detail, TokenBorderSubtle(), RadiusSM, false)

	target := dv.synthDrillTarget(resolved)
	if target.kind == drillNone {
		p.sprites.beginCardFrame()
		p.drawPipelineStrip(dst)
		// Root cards render through the per-card sprite cache: a steady frame
		// blits; only a card whose label, dials or state changed re-rasterises.
		// Dial readings are re-read at draw time, so a raw edit / undo that
		// moves a dial's params repaints it even when nothing re-laid the
		// panel out.
		params := audio.MergedInstrumentParamsRO(resolved)
		focusCard, focusDial, focused := dv.synthFocusedDial(resolved)
		for i := range dv.instEditorMacroCards {
			c := &dv.instEditorMacroCards[i]
			if c.r.Empty() {
				continue
			}
			// By id, not by index: cards scrolled out of the grid's window are
			// not laid out, so the slot index drifts from the macro index.
			cg := dv.macroConceptByID(c.id)
			if cg != nil {
				for j := 0; j < c.nDials && j < len(cg.Dials); j++ {
					c.dials[j].reading = synthDialPercent(cg.Dials[j].Reading(params))
					c.dials[j].focused = focused && c.id == focusCard && c.dials[j].id == focusDial
				}
			}
			st := paramCardState{Selected: focused && c.id == focusCard}
			key := struct {
				label, gloss string
				dials        [synthdial.MaxDialsPerCard]macroDialSlot
				n, w, h      int
				compact      bool
				st           paramCardState
			}{c.label, c.gloss, c.dials, c.nDials, c.r.Dx(), c.r.Dy(), synthCompactCards(), st}
			dials := c.dials[:c.nDials]
			label, gloss := c.label, c.gloss
			p.sprites.card(c.spriteID).Draw(dst, c.r, key, func(img *ebiten.Image, rr image.Rectangle) {
				drawMacroCard(img, rr, label, gloss, dials, st)
			})
		}
		p.sprites.endCardFrame()
		if grid := dv.instEditorMacroGrid; grid != nil {
			grid.Draw(dst)
			drawControlGridEdgeFades(dst, grid, TokenSurface1())
		}
		return
	}

	// Drilled header: Back (drawn with the other instEditorBtns by
	// drawSynthFooter) · title · gloss · stage pill.
	tx := detail.Min.X + synthSectionPaddingX
	if b := dv.instEditorMacroBackBtn; b != nil && !b.Rect().Empty() {
		tx = b.Rect().Max.X + SpaceMD
	}
	ty := detail.Min.Y + synthSectionPaddingY
	stageEnabled := true
	if dv.instEditorMacroPillSec >= 0 && dv.instEditorMacroPillSec < len(dv.instEditorSections) {
		st := p.synthStageEnableState(resolved, &dv.instEditorSections[dv.instEditorMacroPillSec])
		stageEnabled = st.kind == enableNone || st.enabled
		if pr := dv.instEditorMacroPillRect; !pr.Empty() {
			if st.interactive {
				drawFXTogglePill(dst, pr, st.enabled)
			} else {
				drawFXTogglePillDisabled(dst, pr)
			}
		}
	}
	titleCol := TokenTextPrimary()
	glossCol := WithAlpha(TokenTextSecondary(), AlphaMedium)
	if !stageEnabled {
		titleCol = TokenTextDisabled()
		glossCol = WithAlpha(TokenTextDisabled(), AlphaMedium)
	}
	availW := detail.Max.X - synthSectionPaddingX - tx
	if !dv.instEditorMacroPillRect.Empty() {
		availW = dv.instEditorMacroPillRect.Min.X - SpaceSM - tx
	}
	if availW > 0 {
		title := truncCaptionStyled(dv.instEditorMacroTitle, availW, RoleBody)
		DrawTextStyled(dst, title, tx, ty, RoleBody, titleCol)
		if !stageEnabled {
			tagX := tx + StyledTextWidth(title, RoleBody) + SpaceSM
			DrawTextStyled(dst, i18n.T(i18n.KeyCapBypassed), tagX, ty+2, RoleMicro, TokenTextDisabled())
		}
		if g := dv.instEditorMacroGloss; g != "" {
			DrawTextStyled(dst, truncCaptionStyled(g, availW, RoleCaption), tx, ty+StyledTextHeight(RoleBody)+1, RoleCaption, glossCol)
		}
	}
	// More divider (its input Button is invisible; the divider IS the affordance).
	p.drawSynthMoreDivider(dst, dv.synthMoreOpen(resolved))

	// Param cards (reuse the ParamCard + sprite cache path).
	armedKIdx, armed := dv.synthControlArmed(resolved)
	p.sprites.beginCardFrame()
	for _, kIdx := range dv.instEditorDrilledOrder {
		if kIdx < 0 || kIdx >= len(dv.instEditorControls) {
			continue
		}
		k := dv.instEditorControls[kIdx]
		r := k.Rect()
		if r.Empty() {
			continue
		}
		b := dv.instEditorBindings[kIdx]
		if armed && armedKIdx == kIdx && dv.instEditorArmedEditor != nil {
			p.drawSynthArmedControlCard(dst, r, b, resolved, kIdx)
			continue
		}
		name := synthParamDisplayName(b.def.Name)
		value := knobPillLabel(k, b.def)
		gauge := paramCardGaugeFor(k, b.def)
		st := paramCardState{Dim: !stageEnabled}
		key := struct {
			name, value string
			gauge       paramCardGauge
			st          paramCardState
		}{name, value, gauge, st}
		p.sprites.card(b.def.Name).Draw(dst, r, key, func(img *ebiten.Image, rr image.Rectangle) {
			drawParamCard(img, rr, name, value, gauge, st)
		})
	}
	p.sprites.endCardFrame()
	if grid := dv.instEditorMacroGrid; grid != nil {
		// Honesty scrim: a bypassed stage's controls stay interactive but read
		// as inert (dimmed cards under a translucent wash).
		if !stageEnabled && !grid.ContentRect().Empty() {
			drawRoundedRect(dst, grid.ContentRect(), WithAlpha(TokenSurface1(), AlphaStrong), RadiusSM, true)
		}
		grid.Draw(dst)
		drawControlGridEdgeFades(dst, grid, TokenSurface1())
	}
}

// drawSynthMoreDivider paints the "More · n" divider band with a chevron that
// points down when collapsed and up when expanded. Reuses the Advanced
// divider's rect/count (synthAdvDividerRect / synthAdvDividerCount).
func (p *synthPanel) drawSynthMoreDivider(dst *ebiten.Image, open bool) {
	dv := p.dv
	r := dv.synthAdvDividerRect
	if r.Empty() {
		return
	}
	lbl := i18n.T(i18n.KeySynthMore)
	if dv.synthAdvDividerCount > 0 {
		lbl += " · " + strconv.Itoa(dv.synthAdvDividerCount)
	}
	lh := StyledTextHeight(RoleMicro)
	lw := StyledTextWidth(lbl, RoleMicro) + SpaceXS + lh
	cx := r.Min.X + (r.Dx()-lw)/2
	cy := r.Min.Y + (r.Dy()-lh)/2
	hairY := r.Min.Y + r.Dy()/2
	hairCol := WithAlphaFromColor(TokenBorderSubtle(), AlphaMedium)
	if leftEnd := cx - SpaceSM; leftEnd > r.Min.X {
		drawRect(dst, image.Rect(r.Min.X, hairY, leftEnd, hairY+1), hairCol, true)
	}
	if rightStart := cx + lw + SpaceSM; rightStart < r.Max.X {
		drawRect(dst, image.Rect(rightStart, hairY, r.Max.X, hairY+1), hairCol, true)
	}
	DrawTextStyled(dst, lbl, cx, cy, RoleMicro, TokenTextSecondary())
	icon := IconChevronDown
	if open {
		icon = IconChevronUp
	}
	DrawIcon(dst, icon, image.Rect(cx+lw-lh, cy, cx+lw, cy+lh), TokenTextSecondary())
}

// macroConceptByID looks up a built concept group by id.
func (dv *DrumView) macroConceptByID(id string) *macroGroup {
	for i := range dv.instEditorMacros {
		if dv.instEditorMacros[i].ID == id {
			return &dv.instEditorMacros[i]
		}
	}
	return nil
}

// synthMacroHitAreas publishes the category layer's own hit areas: at the root
// the pipeline strip + the card drill handlers + the card grid's scroll body;
// drilled, the stage pill + the param grid's scroll body. The Back / More
// Buttons and the param knobs are published by the existing instEditorBtns /
// per-knob loops in synthTabHitAreas.
func (p *synthPanel) synthMacroHitAreas(instID instrument.ID) []HitArea {
	dv := p.dv
	z := ZEQPanel + 2
	resolved := dv.resolveSynthInstrument(instID)
	var out []HitArea
	out = append(out, p.synthGraphPickHitAreas(resolved, z+3)...)
	out = append(out, p.synthGraphDetailsHitAreas(z+3)...)
	grid := dv.instEditorMacroGrid
	atRoot := dv.synthAtMacroRoot(resolved)
	if atRoot && !dv.instEditorMacroDetailR.Empty() {
		// The root's empty card space: a tap drops the graph's focus, a sweep
		// scrolls the cards (synthFocusClearAdapter replaces the scroll body).
		out = append(out, HitArea{
			Rect:    dv.instEditorMacroDetailR,
			ZIndex:  ZEQPanel + 1,
			Handler: &synthFocusClearAdapter{dv: dv},
			Tag:     "synth-macro-focus-clear",
		})
	}
	if grid != nil && grid.HasScroll() && !grid.ContentRect().Empty() {
		if !atRoot {
			out = append(out, HitArea{
				Rect:    grid.ContentRect(),
				ZIndex:  ZEQPanel + 1,
				Handler: &synthSectionScrollAdapter{dv: dv, grid: grid, body: true},
				Tag:     "synth-macro-scrollbody",
			})
		}
		if thumb := grid.Scroll().BarRect(); !thumb.Empty() {
			out = append(out, HitArea{
				Rect:    thumb,
				ZIndex:  z + 1,
				Handler: &synthSectionScrollAdapter{dv: dv, grid: grid},
				Tag:     "synth-macro-scroll",
			})
		}
	}
	if dv.synthAtMacroRoot(resolved) {
		out = append(out, p.pipelineHitAreas(z)...)
		for _, c := range dv.instEditorMacroCards {
			if c.r.Empty() {
				continue
			}
			out = append(out, HitArea{
				Rect:     c.r,
				ZIndex:   z + 1,
				Handler:  &macroCardHitAdapter{dv: dv, id: c.id, instID: instID},
				Tag:      "synth-macro-" + c.id,
				Touch:    true,
				ClipRect: c.r.Inset(-8),
			})
			// Dials sit above their card's drill area: a press on a dial
			// adjusts it and never drills.
			for j := 0; j < c.nDials; j++ {
				dr := c.dialRects[j]
				if dr.Empty() {
					continue
				}
				// Touch expansion (44 px minimum) stays inside the dial's own
				// card and the visible grid, so it can never reach a sibling.
				clip := c.r
				if grid != nil && !grid.ContentRect().Empty() {
					dr = dr.Intersect(grid.ContentRect())
					clip = clip.Intersect(grid.ContentRect())
					if dr.Empty() {
						continue
					}
				}
				out = append(out, HitArea{
					Rect:     dr,
					ZIndex:   z + 2,
					Handler:  &synthDialHitAdapter{dv: dv, instID: instID, card: c.id, dial: c.dials[j].id, rect: c.dialRects[j]},
					Tag:      "synth-dial-" + c.dials[j].id,
					Touch:    true,
					ClipRect: clip,
				})
			}
		}
		return out
	}
	if pr := dv.instEditorMacroPillRect; !pr.Empty() && dv.instEditorMacroPillSec >= 0 && dv.instEditorMacroPillSec < len(dv.instEditorSections) {
		sec := &dv.instEditorSections[dv.instEditorMacroPillSec]
		if st := p.synthStageEnableState(resolved, sec); st.interactive {
			out = append(out, HitArea{
				Rect:     pr,
				ZIndex:   z + 1,
				Handler:  &synthToggleHitAdapter{dv: dv, sec: sec.id, instID: instID},
				Tag:      "synth-toggle-" + st.param,
				Touch:    true,
				ClipRect: pr.Inset(-8),
			})
		}
	}
	return out
}

// macroCardHitAdapter drills into a category on a TAP. A vertical sweep that
// starts on a card scrolls the root grid instead (the dials make the root
// taller than the panel), so the drill happens on release, through the same
// tap-vs-scroll arbiter the synth controls use.
type macroCardHitAdapter struct {
	dv     *DrumView
	id     string
	instID instrument.ID
	arb    dragScrollArbiter
}

func (h *macroCardHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle {
	return hitRect
}
func (h *macroCardHitAdapter) OnPress(x, y int) InputResult {
	if h.dv == nil {
		return InputIgnored
	}
	h.arb.begin(y, h.dv.instEditorMacroGrid)
	return InputCaptured
}
func (h *macroCardHitAdapter) OnDrag(x, y int) {
	if h.arb.drag(y) {
		h.dv.invalidateSynthPanel()
	}
}
func (h *macroCardHitAdapter) OnRelease(x, y int) {
	if !h.arb.end() {
		return
	}
	inst := h.dv.resolveSynthInstrument(h.instID)
	if !devToolsOn() {
		// Production: a card does not drill (its knobs are developer tools);
		// a tap focuses its first dial, so the graph explains the card.
		h.dv.focusCardFirstDial(inst, h.id)
		h.dv.invalidateSynthPanel()
		return
	}
	h.dv.setSynthMacroOpen(inst, h.id)
	h.dv.invalidateSynthPanel()
}
func (h *macroCardHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// drawMacroCard renders one category card: the flat-tonal card body with a
// plain-English LABEL + one-liner GLOSS and a drill caret, and — when the card
// has dials — the dial row across the card's full width underneath.
func drawMacroCard(dst *ebiten.Image, r image.Rectangle, label, gloss string, dials []macroDialSlot, st paramCardState) {
	if r.Empty() {
		return
	}
	drawFlatButtonBody(dst, r, TokenSurface3(), RadiusControl)
	if st.Selected {
		drawFlatSelectionBar(dst, r, colConceptStroke, selectionEdgeBottom, 1)
	}
	inner := r.Inset(macroCardInset())
	if inner.Empty() {
		return
	}
	var labelCol color.Color = colTextPrimary
	var glossCol color.Color = WithAlpha(TokenTextSecondary(), AlphaMedium)
	if st.Dim {
		labelCol = TokenTextDisabled()
		glossCol = TokenTextDisabled()
	}
	var dialR [synthdial.MaxDialsPerCard]image.Rectangle
	if len(dials) > 0 {
		dialR = macroCardDialRects(r, len(dials))
	}
	hasDials := !dialR[0].Empty()
	top := inner
	if hasDials {
		top.Max.Y = inner.Min.Y + macroCardTopH()
	}

	compact := synthCompactCards()
	const caretW = 9
	textR := top

	if compact {
		DrawTextStyled(dst, truncCaptionStyled(label, textR.Dx(), RoleBody), textR.Min.X, textR.Min.Y, RoleBody, labelCol)
	} else {
		nm := truncCaptionStyled(label, textR.Dx()-caretW-SpaceXS, RoleBody)
		DrawTextStyled(dst, nm, textR.Min.X, textR.Min.Y, RoleBody, labelCol)
		cx := textR.Max.X - caretW
		cy := textR.Min.Y + StyledTextHeight(RoleBody)/2
		DrawIcon(dst, IconChevronRight, image.Rect(cx, cy-caretW/2, cx+caretW, cy+caretW/2+1), WithAlphaFromColor(genColorPrimary, AlphaStrong))
		gy := textR.Min.Y + StyledTextHeight(RoleBody) + 2
		DrawTextStyled(dst, truncCaptionStyled(gloss, textR.Dx(), RoleCaption), textR.Min.X, gy, RoleCaption, glossCol)
	}

	if !hasDials {
		return
	}
	for i, d := range dials {
		if i >= len(dialR) {
			break
		}
		if st.Dim {
			d.dim = true
		}
		drawSynthMacroDial(dst, dialR[i], d)
	}
}
