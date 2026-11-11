package ui

// synth_pipeline_strip.go — the Synth tab's PIPELINE STRIP: one row of stage
// pills in signal order showing only the stages that are ON and relevant
// (OSC → ENV → FILTER → POST for a plain modular voice), joined by chevrons.
// Each pill carries the stage's short label (text only: the old per-stage
// thumbnails were removed with the modelled graphs, 2026-10-25). Tapping
// a pill DRILLS into the stage (same drilled screen as a category). A trailing
// "+" pill lists the OFF stages in the shared action menu: picking one turns
// it on and drills into it — the only place a bypassed stage appears at the
// root.
//
// Built from existing components only: the pills and the "+" are Buttons
// (published through buttonHitAdapter), the layout is a ControlGrid, the
// menu is openActionMenu.

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// synthPipePill is one laid-out stage pill.
type synthPipePill struct {
	id     synthSectionID
	secIdx int // index into dv.instEditorSections
	btn    *Button
	rect   image.Rectangle
}

// activePipelineStages splits dv.instEditorSections into the stages that are
// ON (drawn in the strip) and OFF (listed under "+"), both as section indices
// in pipeline order. A stage is on when it has no enable pill, is locked on
// (VOICE / sole-source OSC), or its pill reads enabled; value-gated stages
// (ENSEMBLE at one voice, RESONATOR at body "Off") count as off.
func (p *synthPanel) activePipelineStages(instID instrument.ID) (on []int, off []int) {
	dv := p.dv
	on, off = p.pipeOn[:0], p.pipeOff[:0]
	defer func() { p.pipeOn, p.pipeOff = on, off }()
	for i := range dv.instEditorSections {
		sec := &dv.instEditorSections[i]
		st := p.synthStageEnableState(instID, sec)
		switch {
		case sec.id == synthSectionPost:
			// POST is the output stage (gain lives there): always part of the
			// signal path even when its drive/post toggle is off.
			on = append(on, i)
		case st.kind == enableNone, st.kind == enableLocked, st.enabled:
			on = append(on, i)
		default:
			off = append(off, i)
		}
	}
	return on, off
}

// pipelineStripHeight is the band height the strip claims: one chip-strip row
// on desktop; on mobile as many rows as the pills wrap into. The mobile row
// count comes from a probe Layout of the strip's own grid (its adaptive
// column rule, not a re-derivation), so the band always fits every pill —
// including the trailing "+" — instead of clipping a wrapped row.
func (p *synthPanel) pipelineStripHeight(mobile bool, n, availW int) int {
	dv2 := Profile().DensityValues()
	if !mobile || n <= 0 {
		return dv2.SynthChipStripH
	}
	dv := p.dv
	if dv.instEditorPipeGrid == nil {
		dv.instEditorPipeGrid = NewControlGrid(controlGridScrollbarStyle())
	}
	grid := dv.instEditorPipeGrid
	grid.SetMaxCols(0)
	probe := image.Rect(0, 0, availW, 4096)
	savedFirst := grid.Scroll().VS.First
	grid.Layout(probe, n, dv2.SynthChipMinW+pipelineChipGap, dv2.SynthChipH, pipelineChipGap, SpaceXS)
	grid.Scroll().VS.First = savedFirst
	cols := grid.Cols()
	if cols < 1 {
		cols = 1
	}
	rows := (n + cols - 1) / cols
	return rows*(dv2.SynthChipH+SpaceXS) + SpaceXS
}

// pipelineChipGap is the inter-pill gap: room for the accent chevron.
const pipelineChipGap = SpaceSM + 10

// layoutPipelineStrip lays the ON stages + the "+" pill into band (root only).
func (p *synthPanel) layoutPipelineStrip(band image.Rectangle, instID instrument.ID, mobile bool) {
	dv := p.dv
	dv.instEditorPipePills = dv.instEditorPipePills[:0]
	dv.instEditorPipeAddBtn = nil
	dv.instEditorPipeRect = band
	if band.Empty() {
		return
	}
	on, off := p.activePipelineStages(instID)
	n := len(on)
	if len(off) > 0 {
		n++
	}
	if n == 0 {
		return
	}
	if dv.instEditorPipeGrid == nil {
		dv.instEditorPipeGrid = NewControlGrid(controlGridScrollbarStyle())
	}
	grid := dv.instEditorPipeGrid
	dv2 := Profile().DensityValues()
	chipH := dv2.SynthChipH
	if chipH > band.Dy() {
		chipH = band.Dy()
	}
	// Desktop: a single row, every pill sharing the width. Mobile: greedy wrap
	// at the minimum chip width so every stage stays visible at 360 px.
	if mobile {
		grid.SetMaxCols(0)
	} else {
		grid.SetMaxCols(n)
	}
	inner := image.Rect(band.Min.X, band.Min.Y+(band.Dy()-chipH)/2, band.Max.X, band.Min.Y+(band.Dy()-chipH)/2+chipH)
	if mobile {
		inner = band
	}
	grid.Layout(inner, n, dv2.SynthChipMinW+pipelineChipGap, chipH, pipelineChipGap, SpaceXS)
	resolved := dv.resolveSynthInstrument(instID)
	drilled := dv.synthDrillTarget(resolved)
	for i, secIdx := range on {
		cell, vis := grid.CellRect(i)
		if !vis {
			continue
		}
		id := dv.instEditorSections[secIdx].id
		stageID := id
		btn := p.pipeButton(id)
		btn.Text = sectionLabelChipFitted(id, cell.Dx()-2*SpaceSM)
		btn.OnClick = func() {
			dv.setSynthStageOpen(resolved, stageID)
			if dv.eqPanelZone != nil {
				dv.eqPanelZone.Invalidate()
			}
		}
		btn.SetToggled(drilled.kind == drillStage && drilled.stage == id)
		btn.SetRect(cell)
		dv.instEditorPipePills = append(dv.instEditorPipePills, synthPipePill{id: id, secIdx: secIdx, btn: btn, rect: cell})
	}
	if len(off) > 0 {
		cell, vis := grid.CellRect(len(on))
		if vis {
			add := p.pipeButton(-1)
			add.Text = "+ " + i18n.T(i18n.KeySynthAddStage)
			// The menu items are built on tap (not per Layout): one entry per
			// OFF stage, turning it on and drilling into it.
			add.OnClick = func() {
				_, offNow := p.activePipelineStages(instID)
				items := make([]actionMenuItem, 0, len(offNow))
				for _, secIdx := range offNow {
					id := dv.instEditorSections[secIdx].id
					stageID := id
					items = append(items, actionMenuItem{
						Label: sectionLabelLocalized(id),
						OnSelect: func() {
							p.applySynthStageToggleByID(instID, stageID)
							dv.setSynthStageOpen(resolved, stageID)
							if dv.eqPanelZone != nil {
								dv.eqPanelZone.Invalidate()
							}
						},
					})
				}
				dv.openActionMenu("synth-add-stage", cell, items)
			}
			add.SetRect(cell)
			dv.instEditorPipeAddBtn = add
		}
	}
}

// drawPipelineStrip paints the pills (Button chrome), the chevron connectors
// between same-row neighbours, and the "+" pill.
func (p *synthPanel) drawPipelineStrip(dst *ebiten.Image) {
	dv := p.dv
	var prev image.Rectangle
	for i, pill := range dv.instEditorPipePills {
		if pill.rect.Empty() || pill.btn == nil {
			continue
		}
		if i > 0 && !prev.Empty() && prev.Min.Y == pill.rect.Min.Y && prev.Max.X < pill.rect.Min.X {
			drawPipelineChevron(dst, prev, pill.rect)
		}
		pill.btn.Draw(dst)
		prev = pill.rect
	}
	if add := dv.instEditorPipeAddBtn; add != nil && !add.Rect().Empty() {
		if !prev.Empty() && prev.Min.Y == add.Rect().Min.Y && prev.Max.X < add.Rect().Min.X {
			drawPipelineChevron(dst, prev, add.Rect())
		}
		add.Draw(dst)
	}
}

// drawPipelineChevron draws the accent "signal flows this way" chevron centred
// in the gap between two same-row pills (the old chip strip's connector).
func drawPipelineChevron(dst *ebiten.Image, a, b image.Rectangle) {
	gapW := b.Min.X - a.Max.X
	cy := (b.Min.Y + b.Max.Y) / 2
	if gapW < 4 {
		drawRect(dst, image.Rect(a.Max.X, cy, b.Min.X, cy+1), WithAlpha(genColorBorder, AlphaFaint), true)
		return
	}
	side := gapW
	if side > 10 {
		side = 10
	}
	cx := (a.Max.X + b.Min.X) / 2
	DrawIcon(dst, IconChevronRight, image.Rect(cx-side/2, cy-side/2, cx-side/2+side, cy-side/2+side), WithAlpha(colAccent, AlphaMedium))
}

// pipeButton returns the cached Button for stage id (-1 = the "+" pill). Buttons
// persist across Layouts so their hover/press animation state survives and
// the per-frame rebuild allocates nothing.
func (p *synthPanel) pipeButton(id synthSectionID) *Button {
	if p.pipeBtns == nil {
		p.pipeBtns = map[synthSectionID]*Button{}
	}
	b := p.pipeBtns[id]
	if b == nil {
		b = NewSpecButton("", ComponentButtonSecondary, nil)
		p.pipeBtns[id] = b
	}
	return b
}

// pipelineHitAreas publishes the pills + "+" through the shared button adapter.
func (p *synthPanel) pipelineHitAreas(z int) []HitArea {
	dv := p.dv
	var out []HitArea
	for _, pill := range dv.instEditorPipePills {
		if pill.rect.Empty() || pill.btn == nil {
			continue
		}
		out = append(out, HitArea{
			Rect:     pill.rect,
			ZIndex:   z + 1,
			Handler:  &buttonHitAdapter{btn: pill.btn},
			Tag:      "synth-pipe-" + sectionLabel(pill.id),
			Touch:    true,
			ClipRect: pill.rect.Inset(-4),
		})
	}
	if add := dv.instEditorPipeAddBtn; add != nil && !add.Rect().Empty() {
		out = append(out, HitArea{
			Rect:     add.Rect(),
			ZIndex:   z + 1,
			Handler:  &buttonHitAdapter{btn: add},
			Tag:      "synth-pipe-add",
			Touch:    true,
			ClipRect: add.Rect().Inset(-4),
		})
	}
	return out
}
