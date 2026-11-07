package ui

// Only the stages that are ON, in signal order; a pill drills in, and the "+" pill lists the OFF stages - the
// only place a bypassed stage appears at the root.

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

// Value-gated stages at their identity (one voice, body Off) count as off.
func (p *synthPanel) activePipelineStages(instID instrument.ID) (on []int, off []int) {
	dv := p.dv
	on, off = p.pipeOn[:0], p.pipeOff[:0]
	defer func() { p.pipeOn, p.pipeOff = on, off }()
	for i := range dv.instEditorSections {
		sec := &dv.instEditorSections[i]
		st := p.synthStageEnableState(instID, sec)
		switch {
		case sec.id == synthSectionPost:
			// POST carries the gain, so it is always in the signal path.
			on = append(on, i)
		case st.kind == enableNone, st.kind == enableLocked, st.enabled:
			on = append(on, i)
		default:
			off = append(off, i)
		}
	}
	return on, off
}

// The mobile row count comes from a probe Layout of the strip's own grid, so the band fits every wrapped pill.
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
	// Mobile wraps at the minimum chip width so every stage stays visible at 360 px.
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
			// Built on tap, not per Layout.
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

// Persisted across Layouts so hover/press animation survives and the rebuild allocates nothing.
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
