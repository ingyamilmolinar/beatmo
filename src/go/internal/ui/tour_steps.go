//go:build !js

package ui

import (
	"fmt"
	"image"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// tourScripts holds one tour per landing cut. Each cut resolves its targets
// against its own layout; for now both perform the same user actions.
var tourScripts = map[string][]tourStep{
	"desktop": append(tourSetupDesktop(), showcaseSteps(false)...),
	"phone":   append(tourSetupPhone(), showcaseSteps(true)...),
}

// ─── pre-roll (real input, before the recording starts) ───

func tourSetupDesktop() []tourStep {
	return []tourStep{
		// Shrink the audio panel and grow the graph: drag the two dividers.
		{name: "setup-eq-divider", preroll: true, act: func(r *tourRunner, g *Game) error {
			dv := g.drum
			idx := dv.eqDividerRowIdx()
			if dv.layoutHandler == nil || idx < 0 {
				return nil // no divider in this layout
			}
			p, err := needRect("audio-panel divider", dv.layoutHandler.rowHandleRect(idx))
			if err != nil {
				return err
			}
			r.markY("audio-panel divider", p.Y)
			r.drag(p, image.Pt(p.X, int(0.80*float64(g.winH))), 12)
			return nil
		}, reached: func(r *tourRunner, g *Game) error {
			if _, ok := r.mark["audio-panel divider"]; !ok {
				return nil // no divider in this layout: nothing was dragged
			}
			return reachedTowardY("audio-panel divider", eqDividerY, 0.80)(r, g)
		}},
		{name: "setup-grid-split", preroll: true, act: func(r *tourRunner, g *Game) error {
			p, err := needRect("grid splitter", g.split.HandleRect())
			if err != nil {
				return err
			}
			r.markY("grid splitter", p.Y)
			r.drag(p, image.Pt(p.X, int(0.47*float64(g.winH))), 12)
			return nil
		}, reached: reachedTowardY("grid splitter", gridSplitY, 0.47)},
		tourFitStep("setup-fit", 0.06, 0),
	}
}

func tourSetupPhone() []tourStep {
	return []tourStep{tourFitStep("setup-fit", 0.12, 0)}
}

// ─── the recorded tour ───

func showcaseSteps(phone bool) []tourStep {
	steps := []tourStep{
		{name: "play", chapter: "circuit", title: "Signals walk the paths you draw", sub: "every node they reach plays its instrument", side: "left", beats: 6,
			act: func(r *tourRunner, g *Game) error {
				p, err := needRect("play button", g.drum.playBtn().Rect())
				if err != nil {
					return err
				}
				r.tap(p)
				return nil
			},
			reached: reachedPlaying},
		{name: "zoom-in", beats: 4, act: func(r *tourRunner, g *Game) error {
			n := tourLeadNode(g)
			if n == nil {
				return fmt.Errorf("no lead-row node on screen")
			}
			x1, y1, x2, y2 := g.nodeScreenRect(n)
			if r.mark == nil {
				r.mark = map[string]float64{}
			}
			r.mark["zoom"] = g.cam.Scale
			r.wheelSmooth(image.Pt(int((x1+x2)/2), int((y1+y2)/2)), 0.3, 30)
			return nil
		}, reached: reachedZoomedIn},
		{name: "node", chapter: "nodes", title: "Tap a node, change its note", sub: "every node can play its own pitch", side: "right", beats: 2.5,
			act: func(r *tourRunner, g *Game) error {
				n := tourLeadNode(g)
				if n == nil {
					return fmt.Errorf("no lead-row node on screen")
				}
				x1, y1, x2, y2 := g.nodeScreenRect(n)
				r.tap(image.Pt(int((x1+x2)/2), int((y1+y2)/2)))
				r.target = n.ID
				return nil
			},
			reached: func(r *tourRunner, g *Game) error {
				if !g.sidebar.IsOpen() {
					return fmt.Errorf("node sidebar did not open")
				}
				if n := g.sidebar.Node(); n == nil || n.ID != r.target {
					return fmt.Errorf("the sidebar is not editing the tapped node")
				}
				return nil
			}},
		tourSidebarTap("pitch-section", sbSectionHeader(secPit), 1.5),
		tourSidebarTap("pitch-chip", sbPitchNote, 2),
		{name: "pick-note", beats: 2.5, act: func(r *tourRunner, g *Game) error {
			if !g.sidebar.noteMatrixIsOpen() {
				return fmt.Errorf("note matrix is not open")
			}
			n, ok := g.graph.GetNodeByID(r.target)
			if !ok {
				return fmt.Errorf("target node gone")
			}
			m := g.sidebar.noteMatrix
			col, row, ok := m.Model().CellFor(n.Params.Pitch + 4)
			if !ok {
				return fmt.Errorf("no cell for pitch %v", n.Params.Pitch+4)
			}
			rc, vis := m.cellRect(col, row)
			if !vis {
				return fmt.Errorf("note cell not visible")
			}
			r.cell = rc
			r.wantPitch = n.Params.Pitch + 4
			r.tap(tourCenter(rc))
			return nil
		}, reached: reachedPitch},
		tourRetapCell("confirm-note", 1.5, reachedPickerConfirmed(reachedPitch)),
		tourSidebarTap("audible-section", sbSectionHeader(secAud), 1.5).withCaption("chords", "Stack notes into a chord", "pick the chord right on the node", "right"),
		tourSidebarTap("make-chord", sbAudible, 1.5),
		tourSidebarTap("chord-chip", sbPitchNote, 2),
		{name: "pick-chord", beats: 2.5, act: func(r *tourRunner, g *Game) error {
			if !g.sidebar.chordMatrixIsOpen() {
				return fmt.Errorf("chord matrix is not open")
			}
			// A minor chord (column 1) on whichever root row is in view: a real
			// user picks what they can see.
			var vis []image.Rectangle
			for row := 0; row < 128; row++ {
				if rc, ok := g.sidebar.chordMatrix.cellRect(1, row); ok {
					vis = append(vis, rc)
				}
			}
			if len(vis) == 0 {
				return fmt.Errorf("no minor-chord cell in view")
			}
			rc := vis[len(vis)/2]
			r.cell = rc
			r.tap(tourCenter(rc))
			return nil
		}, reached: reachedChord},
		tourRetapCell("confirm-chord", 1.5, reachedPickerConfirmed(reachedChord)),
		tourTapOutside("close-sidebar", 1),
		{name: "fx-open", chapter: "effects", title: "Drop in effects", sub: "delay, reverb, crush… per instrument", side: "left", beats: 2,
			act: func(r *tourRunner, g *Game) error {
				btns := g.drum.rowFXBtns()
				if len(btns) == 0 {
					return fmt.Errorf("no FX buttons")
				}
				p, err := needRect("row FX button", btns[0].Rect())
				if err != nil {
					return err
				}
				r.tap(p)
				return nil
			},
			reached: func(_ *tourRunner, g *Game) error {
				if !g.drum.IsFXPanelOpen() || g.drum.fxPanelRow != 0 {
					return fmt.Errorf("the lead row's FX panel is not open (open=%v row=%d)", g.drum.IsFXPanelOpen(), g.drum.fxPanelRow)
				}
				return nil
			}},
		tourFXButton("fx-add", "+ Add Effect", 2, func(_ *tourRunner, g *Game) error {
			if !g.drum.fxAddMenuOpen {
				return fmt.Errorf("the effect list did not open")
			}
			return nil
		}),
		tourFXButton("fx-delay", "Delay", 4, reachedEffect(audio.EffectDelay)),
		tourTapOutside("fx-close", 1),
	}
	if phone {
		steps = append(steps, tourSoundPhone()...)
	} else {
		steps = append(steps, tourSoundDesktop()...)
	}
	steps = append(steps, tourThemesAndOutro(phone)...)
	return steps
}

// tourSoundDesktop: grow the audio panel by its divider, walk the tab pills,
// pick the lead channel and turn a synth dial.
func tourSoundDesktop() []tourStep {
	return []tourStep{
		tourDragDivider("panel-grow", 0.50, 1.5).withCaption("sound", "Read your mix live", "spectrum, levels and the whole signal chain", "left"),
		tourTab("tab-spectrum", TabSpectrum, 3),
		tourTab("tab-levels", TabMeters, 3),
		tourTab("tab-chain", TabScope, 3),
		tourChannelPill(), tourChannelLead(),
		tourTab("tab-synth", TabSynth, 2.5).withCaption("sound", "Shape every instrument", "turn a dial, hear it on the next note", "left"),
		tourTurnDial(),
		// The Synth tab is the most expensive surface to draw (~10 fps under
		// xvfb's software GL, measured); leave it once the dial has turned.
		tourTab("tab-eq-again", TabEQ, 0.5),
	}
}

// tourSoundPhone: the phone's audio views are bottom-nav segments.
func tourSoundPhone() []tourStep {
	return []tourStep{
		tourNav("nav-spec", "Spec", 3).withCaption("sound", "Read your mix live", "spectrum, levels and the whole signal chain", "left"),
		tourNav("nav-levels", "Lvl", 3),
		tourNav("nav-chain", "Chn", 3),
		tourChannelPill(), tourChannelLead(),
		tourNav("nav-synth", "Syn", 2.5).withCaption("sound", "Shape every instrument", "turn a dial, hear it on the next note", "left"),
		tourTurnDial(),
		tourNav("nav-pads", "Pads", 0.5),
	}
}

// tourThemesAndOutro: repaint through the Theme editor, then return to the
// opening framing so the video loops without a visible seam.
func tourThemesAndOutro(phone bool) []tourStep {
	steps := []tourStep{
		{name: "theme-open", chapter: "themes", title: "Repaint everything in one tap", sub: "six themes, or tune every colour", side: "left", beats: 1.5,
			act: func(r *tourRunner, g *Game) error {
				p, err := needRect("theme button", g.gridThemeButtonRect())
				if err != nil {
					return err
				}
				r.tap(p)
				return nil
			}, reached: reachedThemeEditorOpen},
		tourPickPreset("ocean", 3),
		tourPickPreset("ember", 3),
		tourPickPreset("vice-city", 2),
		tourTapOutside("theme-close", 1),
	}
	if !phone {
		steps = append(steps, tourDragDivider("panel-shrink", 0.80, 1))
	}
	margin := 0.06
	if phone {
		margin = 0.12
	}
	return append(steps, tourFitStep("outro-fit", margin, 0), tourStep{name: "outro-hold", beats: 2, reached: reachedPlaying})
}

// tourTapOutside closes the open menu the way a user does on every platform:
// one tap on the grid outside it ("tap outside any menu = close, and nothing
// else", popup_outside_press.go). On a phone the node card can cover most of
// the grid pane, so any clear grid point outside the open surfaces will do —
// never the splitter's grab strip (it reaches into the grid, passes presses
// through, and a tap there creates a node) and never on or beside a node.
func tourTapOutside(name string, beats float64) tourStep {
	return tourStep{name: name, beats: beats, reached: reachedNothingOpen, act: func(r *tourRunner, g *Game) error {
		p, ok := tourOutsidePoint(g)
		if !ok {
			return fmt.Errorf("no clear grid point outside the open menus")
		}
		r.tap(p)
		return nil
	}}
}

// tourOutsidePoint: a grid point outside every open surface, the corner
// buttons and the splitter strip, and clear of every node — the one nearest
// (0.3, 0.6) of the grid pane.
func tourOutsidePoint(g *Game) (image.Point, bool) {
	covered := []image.Rectangle{g.split.InputBounds().Inset(-12)}
	if g.sidebar.IsOpen() {
		covered = append(covered, g.sidebar.rects[sbPanel].Inset(-12))
	}
	if ov, ok := g.drum.portal().TopOverlay().(*ThemeOverlay); ok && ov != nil {
		covered = append(covered, ov.Rect().Inset(-12))
	}
	for _, b := range []image.Rectangle{g.gridHelpButtonRect(), g.gridFileButtonRect(), g.gridLogsButtonRect(), g.gridThemeButtonRect()} {
		covered = append(covered, b.Inset(-12))
	}
	gr := g.split.GridRect(g.winW, g.winH).Inset(24)
	want := image.Pt(gr.Min.X+int(0.3*float64(gr.Dx())), gr.Min.Y+int(0.6*float64(gr.Dy())))
	best, bestD := image.Point{}, math.Inf(1)
	for y := gr.Min.Y; y < gr.Max.Y; y += 6 {
		for x := gr.Min.X; x < gr.Max.X; x += 6 {
			p := image.Pt(x, y)
			if inAny(p, covered) || !clearOfNodes(g, p, 36) {
				continue
			}
			if d := math.Hypot(float64(x-want.X), float64(y-want.Y)); d < bestD {
				best, bestD = p, d
			}
		}
	}
	return best, !math.IsInf(bestD, 1)
}

func clearOfNodes(g *Game, p image.Point, pad float64) bool {
	for _, n := range g.nodes {
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		if float64(p.X) > x1-pad && float64(p.X) < x2+pad && float64(p.Y) > y1-pad && float64(p.Y) < y2+pad {
			return false
		}
	}
	return true
}

func inAny(p image.Point, rs []image.Rectangle) bool {
	for _, rc := range rs {
		if p.In(rc) {
			return true
		}
	}
	return false
}

// tourDragDivider drags the audio panel's divider to frac of the window height.
func tourDragDivider(name string, frac, beats float64) tourStep {
	return tourStep{name: name, beats: beats, act: func(r *tourRunner, g *Game) error {
		dv := g.drum
		idx := dv.eqDividerRowIdx()
		if dv.layoutHandler == nil || idx < 0 {
			return fmt.Errorf("no audio-panel divider")
		}
		p, err := needRect("audio-panel divider", dv.layoutHandler.rowHandleRect(idx))
		if err != nil {
			return err
		}
		r.markY("audio-panel divider", p.Y)
		r.drag(p, image.Pt(p.X, int(frac*float64(g.winH))), 20)
		return nil
	}, reached: reachedTowardY("audio-panel divider", eqDividerY, frac)}
}

// tourChannelPill opens the audio panel's channel dropdown.
func tourChannelPill() tourStep {
	return tourStep{name: "channel-pill", beats: 1, reached: func(_ *tourRunner, g *Game) error {
		if !g.drum.IsEQChannelOpen() {
			return fmt.Errorf("the channel dropdown did not open")
		}
		return nil
	}, act: func(r *tourRunner, g *Game) error {
		z := g.drum.eqPanelZone
		if z == nil || z.stickyBar == nil {
			return fmt.Errorf("no audio panel")
		}
		p, err := needRect("channel pill", z.stickyBar.ChannelBtn().Rect())
		if err != nil {
			return err
		}
		r.tap(p)
		return nil
	}}
}

// tourChannelLead picks the lead row in the open channel dropdown.
func tourChannelLead() tourStep {
	return tourStep{name: "channel-lead", beats: 1, reached: reachedChannelLead, act: func(r *tourRunner, g *Game) error {
		top := g.drum.portal().TopOverlay()
		ov, ok := top.(*eqChannelDropdownOverlay)
		if !ok || ov == nil {
			return fmt.Errorf("channel dropdown is not open (top overlay %T)", top)
		}
		want := g.drum.Rows[0].Name
		var labels []string
		for _, b := range ov.buttons {
			if b.Text == want && !b.Rect().Empty() {
				r.tap(tourCenter(b.Rect()))
				return nil
			}
			labels = append(labels, b.Text)
		}
		return fmt.Errorf("channel %q not in the dropdown (have %q)", want, labels)
	}}
}

// tourTurnDial drags the first laid-out synth macro dial upward.
func tourTurnDial() tourStep {
	return tourStep{name: "turn-dial", beats: 4, reached: reachedSoundChanged, act: func(r *tourRunner, g *Game) error {
		r.docs0 = tourDocHashes(g)
		for _, c := range g.drum.instEditorMacroCards {
			for j := 0; j < c.nDials; j++ {
				if !c.dialRects[j].Empty() {
					p := tourCenter(c.dialRects[j])
					r.drag(p, p.Add(image.Pt(0, -60)), 18)
					return nil
				}
			}
		}
		return fmt.Errorf("no synth dial on screen")
	}}
}

// tourNav taps a segment of the phone's bottom navigation by label.
func tourNav(name, label string, beats float64) tourStep {
	return tourStep{name: name, beats: beats, reached: reachedNav(label), act: func(r *tourRunner, g *Game) error {
		sc := g.drum.viewSwitchSegmented
		if sc == nil {
			return fmt.Errorf("no bottom navigation")
		}
		labels := sc.Labels()
		for i, l := range labels {
			if l == label {
				p, err := needRect(fmt.Sprintf("nav segment %s (mobile=%v view=%v)", label, Profile().IsMobile(), g.drum.currentViewMode), sc.SegmentRect(i))
				if err != nil {
					return err
				}
				r.tap(p)
				return nil
			}
		}
		return fmt.Errorf("nav segment %q not found (have %q)", label, labels)
	}}
}

// tourRetapCell taps the picker cell just picked again: re-selecting
// confirms and closes the picker.
func tourRetapCell(name string, beats float64, reached tourCheck) tourStep {
	return tourStep{name: name, beats: beats, reached: reached, act: func(r *tourRunner, g *Game) error {
		if r.cell.Empty() {
			return fmt.Errorf("no picked cell to confirm")
		}
		r.tap(tourCenter(r.cell))
		r.cell = image.Rectangle{}
		return nil
	}}
}

func (s tourStep) withCaption(chapter, title, sub, side string) tourStep {
	s.chapter, s.title, s.sub, s.side = chapter, title, sub, side
	return s
}

// tourSidebarTap taps a node-sidebar control by id, first scrolling the
// sidebar with real wheel notches while the control is outside its viewport.
func tourSidebarTap(name string, id sbID, beats float64) tourStep {
	return tourStep{name: name, beats: beats, repeat: 40, reached: reachedSidebarControl(id),
		until: func(r *tourRunner, _ *Game) bool { return r.done },
		act: func(r *tourRunner, g *Game) error {
			sb := g.sidebar
			sb.layout()
			rc, ok := sb.rects[id]
			if !ok || rc.Empty() {
				keys := make([]string, 0, len(sb.rects))
				for k := range sb.rects {
					keys = append(keys, k.String())
				}
				sort.Strings(keys)
				return fmt.Errorf("sidebar control %q not laid out (have %v)", id, keys)
			}
			if sb.inViewport(rc) {
				r.tap(tourCenter(rc))
				r.done = true
				return nil
			}
			view := sb.scroll.VS.View
			dir := -1.0 // content below the viewport: scroll down
			if rc.Max.Y <= view.Min.Y {
				dir = 1
			}
			r.waitUntil = 0 // scrolling is not a dwell
			r.wheel(tourCenter(view), int(dir)*2, 3)
			return nil
		}}
}

func tourFXButton(name, text string, beats float64, reached tourCheck) tourStep {
	return tourStep{name: name, beats: beats, reached: reached, act: func(r *tourRunner, g *Game) error {
		var labels []string
		for _, b := range g.drum.fxPanelBtns {
			if b.Text == text && !b.Rect().Empty() {
				r.tap(tourCenter(b.Rect()))
				return nil
			}
			labels = append(labels, b.Text)
		}
		return fmt.Errorf("FX panel button %q not on screen (have %q)", text, labels)
	}}
}

// tourTab taps an audio-panel tab pill (labels resolve at run time: the tour
// pins English before its first step).
func tourTab(name string, tab PanelTab, beats float64) tourStep {
	return tourStep{name: name, beats: beats, reached: reachedTab(tab), act: func(r *tourRunner, g *Game) error {
		z := g.drum.eqPanelZone
		if z == nil || z.stickyBar == nil {
			return fmt.Errorf("no audio panel")
		}
		label := PanelTabLabel(tab)
		var labels []string
		for _, b := range z.stickyBar.tabBtns {
			if b.Text == label && !b.Rect().Empty() {
				r.tap(tourCenter(b.Rect()))
				return nil
			}
			labels = append(labels, b.Text)
		}
		return fmt.Errorf("audio tab %q not on screen (have %q)", label, labels)
	}}
}

// tourPickPreset taps a preset row in the open Theme editor.
func tourPickPreset(id string, beats float64) tourStep {
	return tourStep{name: "theme-" + id, beats: beats,
		act: func(r *tourRunner, g *Game) error {
			ov, ok := g.drum.portal().TopOverlay().(*ThemeOverlay)
			if !ok || ov == nil {
				return fmt.Errorf("theme editor is not open")
			}
			p, err := needRect("preset row "+id, ov.PresetRowRect(id))
			if err != nil {
				return err
			}
			r.tap(p)
			return nil
		},
		reached: func(r *tourRunner, g *Game) error {
			if got := g.UserTheme().Preset; got != id && (id != defaultThemePresetID || got != "") {
				return fmt.Errorf("theme preset is %q, want %q", got, id)
			}
			return nil
		}}
}

// tourNotchZoom is one native wheel detent's zoom on the grid camera,
// measured: 9 detents took the scale from 0.5736 to 0.8150 (x1.421), i.e.
// x1.0398 per detent.
const tourNotchZoom = 1.0398

// tourFitStep frames the nodes of the given rows (all rows when none) in the
// grid pane with real input: a drag-pan that centres them, then one smooth
// wheel zoom sized from the measured detent gain. It repeats (correcting the
// residue) until the bounding box fills the pane with `margin` to spare.
func tourFitStep(name string, margin float64, rows ...int) tourStep {
	return tourStep{name: name, preroll: name[:5] == "setup", beats: 0, repeat: 12, reached: reachedFramed(margin, rows),
		until: func(_ *tourRunner, g *Game) bool { _, _, ok := tourFitDelta(g, margin, rows); return ok },
		act: func(r *tourRunner, g *Game) error {
			k, pan, ok := tourFitDelta(g, margin, rows)
			if ok {
				return nil
			}
			if pan != (image.Point{}) {
				a, found := emptyGridPoint(g, 0.5, 0.5)
				if !found {
					return fmt.Errorf("no empty grid point to pan from")
				}
				r.drag(a, a.Add(pan), 20)
			}
			if k != 1 {
				gr := g.split.GridRect(g.winW, g.winH)
				notches := math.Log(k) / math.Log(tourNotchZoom)
				frames := int(math.Max(12, math.Min(45, math.Abs(notches)/0.8)))
				r.wheelSmooth(tourCenter(gr), notches/float64(frames), frames)
			}
			return nil
		}}
}

// tourFitDelta: the scale factor and the pan that frame the rows' node
// bounding box in the grid pane; ok when it already fits within tolerance.
func tourFitDelta(g *Game, margin float64, rows []int) (k float64, pan image.Point, ok bool) {
	gr := g.split.GridRect(g.winW, g.winH)
	inner := gr.Inset(int(margin * float64(min(gr.Dx(), gr.Dy()))))
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	found := false
	for _, n := range g.nodes {
		if len(rows) > 0 {
			row, has := g.nodeRows[n.ID]
			if !has || !containsInt(rows, row) {
				continue
			}
		}
		found = true
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		minX, minY, maxX, maxY = math.Min(minX, x1), math.Min(minY, y1), math.Max(maxX, x2), math.Max(maxY, y2)
	}
	if !found {
		return 1, image.Point{}, true
	}
	bw, bh := math.Max(1, maxX-minX), math.Max(1, maxY-minY)
	k = math.Min(float64(inner.Dx())/bw, float64(inner.Dy())/bh)
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	dx, dy := float64(inner.Min.X+inner.Dx()/2)-cx, float64(inner.Min.Y+inner.Dy()/2)-cy
	if math.Abs(dx) > 0.04*float64(inner.Dx()) || math.Abs(dy) > 0.04*float64(inner.Dy()) {
		pan = image.Pt(int(dx), int(dy))
	}
	if k > 0.97 && k < 1.08 {
		k = 1
	}
	return k, pan, k == 1 && pan == (image.Point{})
}

// tourLeadNode picks the lead row's on-screen node nearest the pane centre
// (not the start node), the node the tour edits.
func tourLeadNode(g *Game) *uiNode {
	gr := g.split.GridRect(g.winW, g.winH)
	c := tourCenter(gr)
	var best *uiNode
	bestD := math.Inf(1)
	for _, n := range g.nodes {
		if g.start != nil && n.ID == g.start.ID {
			continue
		}
		if row, ok := g.nodeRows[n.ID]; !ok || row != 0 {
			continue
		}
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		p := image.Pt(int((x1+x2)/2), int((y1+y2)/2))
		if !p.In(gr.Inset(min(70, min(gr.Dx(), gr.Dy())/12))) {
			continue
		}
		// Prefer the loop's LEFT side at mid-height: the sidebar popover then
		// opens into open space to the node's right.
		d := 3*math.Abs(float64(p.Y-c.Y)) + math.Abs(float64(p.X-c.X))
		if p.X > c.X {
			d += float64(gr.Dx())
		}
		if d < bestD {
			best, bestD = n, d
		}
	}
	return best
}

// emptyGridPoint finds a grid-pane point near (fx, fy) (fractions of the grid
// rect) that is far from every node, so a press there pans instead of
// grabbing a node.
func emptyGridPoint(g *Game, fx, fy float64) (image.Point, bool) {
	gr := g.split.GridRect(g.winW, g.winH)
	base := image.Pt(gr.Min.X+int(fx*float64(gr.Dx())), gr.Min.Y+int(fy*float64(gr.Dy())))
	for rad := 0; rad < 400; rad += 12 {
		for a := 0.0; a < 2*math.Pi; a += math.Pi / 8 {
			p := base.Add(image.Pt(int(float64(rad)*math.Cos(a)), int(float64(rad)*math.Sin(a))))
			if !p.In(gr.Inset(40)) {
				continue
			}
			if clearOfNodes(g, p, 36) {
				return p, true
			}
		}
	}
	return image.Point{}, false
}

func containsInt(xs []int, v int) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
