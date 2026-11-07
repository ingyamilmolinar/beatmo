package ui

// ROOT: the pipeline strip above category cards, each with up to three macro dials; a tap off the dials or a
// stage pill drills in. DRILLED: Back · title · pill, the essentials, then More. Built only from existing primitives.

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

// Sentinel texts so the existing button loop publishes their hit areas; the leading \x00 keeps them off screen.
const (
	synthBackButtonTag = "\x00synth-back"
	synthMoreButtonTag = "\x00synth-more"
)

// A fixed dial array, so layout allocates nothing; drawing recomputes the same rects sprite-locally.
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

// Rebuilt only when the binding slice changes shape (a schema swap), so per-frame paths allocate nothing.
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

func macroRootCardShortH() int {
	return StyledTextHeight(RoleBody) + StyledTextHeight(RoleCaption) + SpaceSM + SpaceXS
}

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
	// Only the drilled target's knobs get a rect; the hit loop and draw key off a non-empty one.
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

// Production has no pipeline strip and shows only cards with a live dial, compact.
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
	// A card row is about half the phone's card area, so a peek band would cost the second card.
	grid.SetPeekBand(false)
	// One uniform height for the most dial rows any card needs; laid out at the ideal width, then again if the
	// real cell width asks for a different row count.
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
	// The grid's cell width, not CellRect(0): the first cell is not visible once the root scrolls, and the real
	// width (minus the scrollbar) can ask for another row.
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

func (dv *DrumView) macroConceptKnobIdxs(id string) []int {
	e, a := dv.drilledKnobIdxs(synthDrillTarget{kind: drillCategory, cat: id})
	return append(e, a...)
}

// !ok when the target spans several stages or the section has no pill.
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
	if idx, active := dv.armGrowControl(armedScopeSynth, resolved); active {
		dv.growArmedCard(visible, grid, body, idx)
	}
}

// Two passes: the first discovers the column count; the scroll offset is saved around it so the probe's
// smaller row count cannot clamp the window. -1 = phantom divider cell.
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
		// Per-card sprites: only a card whose label, dials or state changed re-rasterises. Readings are re-read at draw
		// time, so an undo that moves a dial repaints it even with no re-layout.
		params := audio.MergedInstrumentParamsRO(resolved)
		focusCard, focusDial, focused := dv.synthFocusedDial(resolved)
		for i := range dv.instEditorMacroCards {
			c := &dv.instEditorMacroCards[i]
			if c.r.Empty() {
				continue
			}
			// By id: cards scrolled out of the window are not laid out, so the slot index drifts from the macro index.
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
		// A bypassed stage's controls stay interactive but read as inert.
		if !stageEnabled && !grid.ContentRect().Empty() {
			drawRoundedRect(dst, grid.ContentRect(), WithAlpha(TokenSurface1(), AlphaStrong), RadiusSM, true)
		}
		grid.Draw(dst)
		drawControlGridEdgeFades(dst, grid, TokenSurface1())
	}
}

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

// The Back/More buttons and the knobs are published by the existing loops in synthTabHitAreas.
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
		// A tap on empty card space drops the graph's focus; a sweep scrolls.
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
			// Above the card's drill area, so a dial press never drills.
			for j := 0; j < c.nDials; j++ {
				dr := c.dialRects[j]
				if dr.Empty() {
					continue
				}
				// The 44 px touch expansion stays inside the dial's card and the visible grid, so it never reaches a sibling.
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

// The drill happens on release through the tap-vs-scroll arbiter: the dials make the root taller than the
// panel, so a sweep starting on a card must scroll.
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
		// Production cards do not drill; a tap focuses the first dial so the graph explains the card.
		h.dv.focusCardFirstDial(inst, h.id)
		h.dv.invalidateSynthPanel()
		return
	}
	h.dv.setSynthMacroOpen(inst, h.id)
	h.dv.invalidateSynthPanel()
}
func (h *macroCardHitAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }

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
