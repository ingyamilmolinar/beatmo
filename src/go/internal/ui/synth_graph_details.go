package ui

// Words by default, numbers and value-labelled axes when on; remembered across sessions in prefs.

import (
	"image"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/userprefs"
)

type AudioPanelStateSink interface {
	LoadAudioPanelState() (userprefs.AudioPanelStateDoc, error)
	SaveAudioPanelState(userprefs.AudioPanelStateDoc) error
}

var (
	audioPanelStateSinkMu sync.RWMutex
	audioPanelStateSink   AudioPanelStateSink
)

// nil keeps the state in memory only.
func SetAudioPanelStateSink(s AudioPanelStateSink) {
	audioPanelStateSinkMu.Lock()
	audioPanelStateSink = s
	audioPanelStateSinkMu.Unlock()
}

func activeAudioPanelStateSink() AudioPanelStateSink {
	audioPanelStateSinkMu.RLock()
	defer audioPanelStateSinkMu.RUnlock()
	return audioPanelStateSink
}

type synthGraphDetailsState struct {
	loaded bool
	on     bool
	btn    *Button
}

// synthGraphNumbers reports whether the graph shows its NUMBERS readout.
func (dv *DrumView) synthGraphNumbers() bool {
	st := &dv.graphDetails
	if !st.loaded {
		st.loaded = true
		if s := activeAudioPanelStateSink(); s != nil {
			if doc, err := s.LoadAudioPanelState(); err == nil {
				st.on = doc.SynthGraphDetails
			}
		}
	}
	return st.on
}

func (dv *DrumView) toggleSynthGraphDetails() {
	on := !dv.synthGraphNumbers()
	dv.graphDetails.on = on
	if s := activeAudioPanelStateSink(); s != nil {
		doc, _ := s.LoadAudioPanelState()
		doc.SynthGraphDetails = on
		_ = s.SaveAudioPanelState(doc)
	}
	dv.invalidateSynthPanel()
}

// synthGraphDetailsButton is the toggle's keycap (lazily built).
func (dv *DrumView) synthGraphDetailsButton() *Button {
	st := &dv.graphDetails
	if st.btn == nil {
		st.btn = NewButton(i18n.T(i18n.KeyGraphDetails), InstButtonStyle, nil)
		st.btn.TextColor = colTextSecondary
		st.btn.OnClick = dv.toggleSynthGraphDetails
	}
	st.btn.Text = i18n.T(i18n.KeyGraphDetails)
	return st.btn
}

func detailsToggleReserve() int {
	return TextWidth(i18n.T(i18n.KeyGraphDetails)) + 2*Profile().DensityValues().AudioPillPadX + SpaceSM
}

func synthGraphDetailsRect(pane image.Rectangle) image.Rectangle {
	if pane.Dx() < 160 || pane.Dy() < 60 {
		return image.Rectangle{}
	}
	h := 2*(StyledTextHeight(RoleMicro)+2) - 2
	w := detailsToggleReserve() - SpaceSM
	x1 := pane.Max.X - viewPad
	y0 := pane.Min.Y + viewPad/2
	return image.Rect(x1-w, y0, x1, y0+h)
}

// Outside the graph's sprite: its pressed and hover states are live.
func (dv *DrumView) drawSynthGraphDetails(dst *ebiten.Image, pane image.Rectangle) {
	r := synthGraphDetailsRect(pane)
	btn := dv.synthGraphDetailsButton()
	btn.SetRect(r)
	if r.Empty() {
		return
	}
	drawPillTabAt(dst, btn, dv.synthGraphNumbers())
}

// synthGraphDetailsHitAreas publishes the toggle (touch-skirted on phones).
func (p *synthPanel) synthGraphDetailsHitAreas(z int) []HitArea {
	dv := p.dv
	r := synthGraphDetailsRect(dv.synthGraphPane())
	if r.Empty() {
		return nil
	}
	btn := dv.synthGraphDetailsButton()
	btn.SetRect(r)
	return []HitArea{audioPillHit(btn, z, "synth-graph-details")}
}
