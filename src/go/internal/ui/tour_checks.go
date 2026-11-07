//go:build !js

package ui

// Every step declares the state it exists to show, and invariants are checked after EVERY step: the video is the
// product, so a tap on the wrong control must not survive as "the tour finished".

import (
	"fmt"
	"math"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

type tourCheck = func(r *tourRunner, g *Game) error

// A stray tap on the grid would add a node; one on the transport would stop the music.
func (r *tourRunner) tourInvariants(g *Game) error {
	if len(g.nodes) != r.nodes0 || len(g.edges) != r.edges0 {
		return fmt.Errorf("the circuit changed: %d nodes / %d edges, the template has %d / %d", len(g.nodes), len(g.edges), r.nodes0, r.edges0)
	}
	if r.playing && !g.Playing() {
		return fmt.Errorf("the song stopped")
	}
	return nil
}

func reachedPlaying(_ *tourRunner, g *Game) error {
	if !g.Playing() {
		return fmt.Errorf("transport is not playing")
	}
	return nil
}

// nothingOpen: a tap outside closed the open menu and opened nothing else.
func reachedNothingOpen(_ *tourRunner, g *Game) error {
	switch {
	case g.sidebar.IsOpen():
		return fmt.Errorf("the node sidebar is still open")
	case g.drum.IsFXPanelOpen():
		return fmt.Errorf("the FX panel is still open")
	case g.drum.IsEQChannelOpen():
		return fmt.Errorf("the channel dropdown is still open")
	}
	if ov, ok := g.drum.portal().TopOverlay().(*ThemeOverlay); ok && ov != nil {
		return fmt.Errorf("the theme editor is still open")
	}
	return nil
}

func tourTargetNode(r *tourRunner, g *Game) (model.Node, error) {
	n, ok := g.graph.GetNodeByID(r.target)
	if !ok {
		return model.Node{}, fmt.Errorf("the tour's node %v is gone", r.target)
	}
	return n, nil
}

func reachedChord(r *tourRunner, g *Game) error {
	n, err := tourTargetNode(r, g)
	if err != nil {
		return err
	}
	if n.Type != model.NodeTypeChord {
		return fmt.Errorf("the node is %v, not a chord", n.Type)
	}
	return nil
}

func reachedPitch(r *tourRunner, g *Game) error {
	n, err := tourTargetNode(r, g)
	if err != nil {
		return err
	}
	if math.Abs(n.Params.Pitch-r.wantPitch) > 1e-9 {
		return fmt.Errorf("node pitch %v, picked %v", n.Params.Pitch, r.wantPitch)
	}
	return nil
}

// reachedSidebarControl: the state a tap on a node-sidebar control leaves.
func reachedSidebarControl(id sbID) tourCheck {
	return func(r *tourRunner, g *Game) error {
		sb := g.sidebar
		if !sb.IsOpen() {
			return fmt.Errorf("the node sidebar closed")
		}
		switch id {
		case sbSectionHeader(secPit), sbSectionHeader(secAud):
			if sec := sbSection(id.arg); !sb.sectionOpen[sec] {
				return fmt.Errorf("section %q is collapsed", sec)
			}
		case sbPitchNote:
			if !sb.noteMatrixIsOpen() && !sb.chordMatrixIsOpen() {
				return fmt.Errorf("no note / chord picker opened")
			}
		case sbAudible:
			return reachedChord(r, g)
		default:
			return fmt.Errorf("no check for sidebar control %q", id)
		}
		return nil
	}
}

// The pick is kept and the node sidebar stays open.
func reachedPickerConfirmed(pick tourCheck) tourCheck {
	return func(r *tourRunner, g *Game) error {
		if g.sidebar.noteMatrixIsOpen() || g.sidebar.chordMatrixIsOpen() {
			return fmt.Errorf("the picker is still open")
		}
		if !g.sidebar.IsOpen() {
			return fmt.Errorf("the node sidebar closed")
		}
		return pick(r, g)
	}
}

// At least halfway: a layout minimum may stop the drag short.
func (r *tourRunner) markY(key string, y int) {
	if r.mark == nil {
		r.mark = map[string]float64{}
	}
	r.mark[key] = float64(y)
}

func reachedTowardY(key string, cur func(g *Game) (int, bool), frac float64) tourCheck {
	return func(r *tourRunner, g *Game) error {
		y1, ok := cur(g)
		if !ok {
			return fmt.Errorf("%s is not on screen", key)
		}
		y0, target := r.mark[key], frac*float64(g.winH)
		d0, d1 := math.Abs(y0-target), math.Abs(float64(y1)-target)
		if d1 > math.Max(24, d0/2) {
			return fmt.Errorf("%s at y=%d after dragging from %.0f toward %.0f", key, y1, y0, target)
		}
		return nil
	}
}

func eqDividerY(g *Game) (int, bool) {
	dv := g.drum
	idx := dv.eqDividerRowIdx()
	if dv.layoutHandler == nil || idx < 0 {
		return 0, false
	}
	rc := dv.layoutHandler.rowHandleRect(idx)
	return rc.Min.Y + rc.Dy()/2, !rc.Empty()
}

func gridSplitY(g *Game) (int, bool) {
	rc := g.split.HandleRect()
	return rc.Min.Y + rc.Dy()/2, !rc.Empty()
}

func reachedFramed(margin float64, rows []int) tourCheck {
	return func(_ *tourRunner, g *Game) error {
		if k, pan, ok := tourFitDelta(g, margin, rows); !ok {
			return fmt.Errorf("the circuit is not framed (zoom %.3f, pan %v still needed)", k, pan)
		}
		return nil
	}
}

func reachedZoomedIn(r *tourRunner, g *Game) error {
	if g.cam.Scale < 1.2*r.mark["zoom"] {
		return fmt.Errorf("camera scale %.3f, was %.3f: the zoom did not land", g.cam.Scale, r.mark["zoom"])
	}
	return nil
}

func reachedTab(tab PanelTab) tourCheck {
	return func(_ *tourRunner, g *Game) error {
		z := g.drum.eqPanelZone
		if z == nil {
			return fmt.Errorf("no audio panel")
		}
		if got := z.ActiveTab(); got != tab {
			return fmt.Errorf("audio tab %q is active, want %q", PanelTabLabel(got), PanelTabLabel(tab))
		}
		return nil
	}
}

func reachedNav(label string) tourCheck {
	return func(_ *tourRunner, g *Game) error {
		sc := g.drum.viewSwitchSegmented
		if sc == nil {
			return fmt.Errorf("no bottom navigation")
		}
		labels := sc.Labels()
		if a := sc.Active(); a < 0 || a >= len(labels) || labels[a] != label {
			return fmt.Errorf("bottom navigation shows segment %d of %q, want %q", a, labels, label)
		}
		return nil
	}
}

func reachedChannelLead(_ *tourRunner, g *Game) error {
	if g.drum.IsEQChannelOpen() {
		return fmt.Errorf("the channel dropdown is still open")
	}
	if got, want := g.drum.activeEQChannel(), g.drum.Rows[0].Instrument; got != channel.Of(want) {
		return fmt.Errorf("audio panel channel %q, want the lead %q", got, want)
	}
	return nil
}

// tourDocHashes fingerprints every row's instrument document.
func tourDocHashes(g *Game) map[instrument.ID]string {
	out := map[instrument.ID]string{}
	for _, row := range g.drum.Rows {
		if d, ok := audio.InstrumentDoc(row.Instrument); ok {
			out[row.Instrument] = instrument.ContentHash(d)
		}
	}
	return out
}

func reachedSoundChanged(r *tourRunner, g *Game) error {
	now := tourDocHashes(g)
	for id, h := range now {
		if r.docs0[id] != "" && r.docs0[id] != h {
			return nil
		}
	}
	return fmt.Errorf("the dial drag changed no instrument")
}

func reachedEffect(t audio.EffectType) tourCheck {
	return func(_ *tourRunner, g *Game) error {
		inst := g.drum.Rows[0].Instrument
		for _, s := range audio.GetInsertEffects(inst) {
			if s.Type == t {
				return nil
			}
		}
		return fmt.Errorf("no %s effect on the lead (%s): %v", t, inst, audio.GetInsertEffects(inst))
	}
}

func reachedThemeEditorOpen(_ *tourRunner, g *Game) error {
	if ov, ok := g.drum.portal().TopOverlay().(*ThemeOverlay); !ok || ov == nil {
		return fmt.Errorf("the theme editor is not open (top overlay %T)", g.drum.portal().TopOverlay())
	}
	return nil
}

func allOf(cs ...tourCheck) tourCheck {
	return func(r *tourRunner, g *Game) error {
		for _, c := range cs {
			if err := c(r, g); err != nil {
				return err
			}
		}
		return nil
	}
}
