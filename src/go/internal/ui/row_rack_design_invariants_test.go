//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// row_rack_design_invariants_test.go pins the drum-pane design pass (DESIGN.md
// "Row control", "Cluster alignment", "Groups are spacing, not containers")
// against the REAL crop_row_rack_states scene driven through Game.Update:
//
//   - every control in a rack row shares one vertical band (one row once held
//     three heights: label/volume inset, toggles a smaller centered square);
//   - a row has exactly two gutters: ControlGap inside the toggle group,
//     rowGroupGap between label | volume | toggles (it had 9 / 6 / 3 px);
//   - the rack bed spans the whole rack column (a 10 px background lane used
//     to open between the rack and the timeline on wide windows) and the
//     column is sized to its content, not a share of the screen;
//   - the desktop transport groups are separated by spacing.cluster with no
//     container chrome; the timeline length stepper clears the icon floor;
//   - a running transport is the play key's active state, not a halo.

func rackStatesGame(t *testing.T, mobile bool, w, h int) *Game {
	t.Helper()
	if mobile {
		setupMobileTest(t, true)
	} else {
		assertDefaultParityState(t)
		withDefaultStart(t, false)
	}
	g := New(log.New(testLogOutput(), log.LevelInfo))
	t.Cleanup(g.CloseForTest)
	g.Layout(w, h)
	run := RunScene
	if mobile {
		run = RunSceneMobile
	}
	if err := run(g, "crop_row_rack_states"); err != nil {
		t.Fatalf("scene: %v", err)
	}
	for i := 0; i < 8; i++ {
		_ = g.Update()
	}
	// Row visuals (label swatch, toggle state) are applied at draw time.
	g.Draw(ebiten.NewImage(w, h))
	return g
}

func TestRackRowControlsShareOneBandAndTwoGutters(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		name, w, h := "desktop", 1600, 1000
		if mobile {
			name, w, h = "mobile", 390, 844
		}
		t.Run(name, func(t *testing.T) {
			g := rackStatesGame(t, mobile, w, h)
			z := g.drum.rowRackZone
			gap := Profile().ControlGap
			if gap < SpaceXS {
				gap = SpaceXS
			}
			checked := 0
			for i, e := range z.entries {
				if e.label.Rect().Empty() {
					continue
				}
				checked++
				assertSharedVerticalBounds(t, "rack row", []clusterMember{
					{"label", e.label.Rect},
					{"volume", e.volSlider.Rect},
					{"mute", e.muteBtn.Rect},
					{"solo", e.soloBtn.Rect},
					{"fx", e.fxBtn.Rect},
					{"menu", e.menuBtn.Rect},
				})
				gaps := []struct {
					what string
					l, r image.Rectangle
					want int
				}{
					{"label→volume", e.label.Rect(), e.volSlider.Rect(), rowGroupGap()},
					{"volume→mute", e.volSlider.Rect(), e.muteBtn.Rect(), rowGroupGap()},
					{"mute→solo", e.muteBtn.Rect(), e.soloBtn.Rect(), gap},
					{"solo→fx", e.soloBtn.Rect(), e.fxBtn.Rect(), gap},
					{"fx→menu", e.fxBtn.Rect(), e.menuBtn.Rect(), gap},
				}
				for _, gp := range gaps {
					if got := gp.r.Min.X - gp.l.Max.X; got != gp.want {
						t.Errorf("row %d %s gutter = %d px, want %d", i, gp.what, got, gp.want)
					}
				}
			}
			if checked < 3 {
				t.Fatalf("only %d visible rows — scene precondition unmet", checked)
			}
		})
	}
}

func TestRackBedSpansContentSizedColumn(t *testing.T) {
	for _, w := range []int{1280, 1600, 1919} {
		t.Run("", func(t *testing.T) {
			g := rackStatesGame(t, false, w, 1000)
			dv := g.drum
			col := dv.widgets.ColWidth(0)
			if dv.labelW+dv.controlsW != col {
				t.Errorf("w=%d: rack bed paints %d px but the rack column is %d px — a %d px background lane",
					w, dv.labelW+dv.controlsW, col, col-dv.labelW-dv.controlsW)
			}
			// Content-sized: the column holds the label (names on screen, floor
			// rackLabelMinW) plus the real control cluster — nowhere near the
			// 30%-of-screen floor that made a 505 px bar hold "Kick".
			if want := dv.labelW + dv.rowRackZone.FixedControlsWidth(); col > want+1 && col > dv.widgets.minColW {
				t.Errorf("w=%d: rack column %d px wider than its content %d px", w, col, want)
			}
			if lbl := dv.rowRackZone.entries[0].label.Rect(); lbl.Dx() > w/4 {
				t.Errorf("w=%d: row label %d px wide (> a quarter of the window)", w, lbl.Dx())
			}
		})
	}
}

func TestRowTogglesAreNeutralAtRestAndLettered(t *testing.T) {
	g := rackStatesGame(t, false, 1600, 1000)
	z := g.drum.rowRackZone
	rows := g.drum.Rows
	neutral := InstButtonStyle.fill()
	for i, e := range z.entries {
		if e.label.Rect().Empty() {
			continue
		}
		if e.label.LeadSwatch == nil || !colorsEqual(e.label.LeadSwatch, rows[i].Color) {
			t.Errorf("row %d label swatch %v, want the instrument color %v", i, e.label.LeadSwatch, rows[i].Color)
		}
		for _, tc := range []struct {
			b      *Button
			letter string
			on     bool
		}{
			{e.muteBtn, rowMuteLabel, rows[i].Muted},
			{e.soloBtn, rowSoloLabel, rows[i].Solo},
			{e.fxBtn, rowFXLabel, hasActiveEffects(audio.GetInsertEffects(rows[i].Instrument))},
		} {
			if tc.b.Text != tc.letter || tc.b.Icon != "" {
				t.Errorf("row %d toggle shows text %q icon %q, want the letter %q", i, tc.b.Text, tc.b.Icon, tc.letter)
			}
			v, ok := tc.b.Style.(rowToggleVisual)
			if !ok {
				t.Fatalf("row %d %q style %T", i, tc.letter, tc.b.Style)
			}
			if !tc.on && !colorsEqual(v.Fill, neutral) {
				t.Errorf("row %d idle %q fill %v, want the neutral body %v", i, tc.letter, v.Fill, neutral)
			}
			if tc.on && colorsEqual(v.Fill, neutral) {
				t.Errorf("row %d active %q still wears the neutral body", i, tc.letter)
			}
		}
	}
}

func TestDesktopTransportGroupsAreSpacingNotContainers(t *testing.T) {
	g := rackStatesGame(t, false, 1600, 1000)
	z := g.drum.transportZone
	if z.groupChrome {
		t.Error("desktop transport paints group containers — every key already has its own body")
	}
	rec, bpm := z.recordBtn.Rect(), z.bpmBox.Rect
	subdiv, vol, undo := z.subdivBtn.Rect(), z.mainVolIconRect, z.undoBtn.Rect()
	inGroup := z.stopBtn.Rect().Min.X - z.playBtn.Rect().Max.X
	for _, b := range []struct {
		what string
		got  int
	}{
		{"record→BPM", bpm.Min.X - rec.Max.X},
		{"subdiv→volume", vol.Min.X - subdiv.Max.X},
		{"volume→undo", undo.Min.X - vol.Max.X},
	} {
		if b.got != SpaceCluster {
			t.Errorf("%s gutter = %d px, want spacing.cluster %d (in-group gap %d)", b.what, b.got, SpaceCluster, inGroup)
		}
	}

	// The length stepper: two side-by-side square keys as tall as the beat
	// counter keycap they share the band with (DESIGN.md icon-vs-text matrix,
	// "Timeline band keycaps").
	inc, dec := g.drum.lenIncBtn.Rect(), g.drum.lenDecBtn.Rect()
	bc := g.drum.beatCounterRect
	keyH := TextHeight() + 2*beatCounterPillPadY + genGeomKeycapWallDepth
	if keyH > bc.Dy() {
		keyH = bc.Dy()
	}
	for _, r := range []image.Rectangle{inc, dec} {
		if r.Dx() != r.Dy() || r.Dy() != keyH || r.Max.Y > bc.Max.Y || r.Min.Y < bc.Min.Y {
			t.Errorf("length stepper key %v is not a counter-height square in the band %v", r, bc)
		}
	}
	if dec.Max.X > inc.Min.X || dec.Min.Y != inc.Min.Y {
		t.Errorf("length stepper keys not side by side (− then +): dec %v inc %v", dec, inc)
	}
}

func TestPlayingIsThePlayKeysActiveStateNotAHalo(t *testing.T) {
	g := rackStatesGame(t, false, 1600, 1000)
	g.SetPlaying(true)
	_ = g.Update()
	if !g.drum.playBtn().Toggled() {
		t.Error("a running transport must latch the play key (accent tint + bottom bar)")
	}
	if newTransportPulseLayer(g.drum).Visible() {
		t.Error("the pulse layer is visible while only playing — the play halo is retired")
	}
	g.SetPlaying(false)
	_ = g.Update()
	if g.drum.playBtn().Toggled() {
		t.Error("stopped transport left the play key latched")
	}
}

// TestIdleRowChipsShareOneBodyAfterRuntimeTheme: after a runtime theme moves
// the surfaces (the Theme editor, or a stored theme applied once the rows
// exist), every idle chip — M, S, FX AND the ⋯ kebab — wears the label's
// body color. The kebab's style used to be captured once at construction
// while the toggles were re-derived each draw, so after a surfaces pick the
// kebab alone kept the old body color.
func TestIdleRowChipsShareOneBodyAfterRuntimeTheme(t *testing.T) {
	defer SetThemeSink(nil)
	SetThemeSink(&fakeThemeSink{})
	g := rackStatesGame(t, false, 1600, 1000)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	before := InstButtonStyle.fill()
	g.setUserThemeCategory("surfaces", "cyan-500")
	if colorsEqual(InstButtonStyle.fill(), before) {
		t.Fatal("precondition: the surfaces pick did not move the row-control body")
	}
	_ = g.Update()
	g.Draw(ebiten.NewImage(1600, 1000))

	z := g.drum.rowRackZone
	want := InstButtonStyle.fill()
	for i, e := range z.entries {
		if e.label.Rect().Empty() || g.drum.Rows[i].Muted || g.drum.Rows[i].Solo || hasActiveEffects(audio.GetInsertEffects(g.drum.Rows[i].Instrument)) {
			continue
		}
		for _, b := range []*Button{e.muteBtn, e.soloBtn, e.fxBtn, e.menuBtn} {
			v, ok := b.Style.(rowToggleVisual)
			if !ok {
				t.Fatalf("row %d chip style %T", i, b.Style)
			}
			if got := v.fill(); !colorsEqual(got, want) {
				t.Errorf("row %d chip %q/%q body %v after the theme change, want the row-control body %v",
					i, b.Text, b.Icon, got, want)
			}
		}
	}
}

// TestHeaderRowSharesTheRackBed: the header row — transport strip and the
// timeline header band right of the beat counter — is painted with the same
// rack bed as the rack and the area under the rows. It used to take the base
// background, a black band across the whole pane above the lighter rack.
func TestHeaderRowSharesTheRackBed(t *testing.T) {
	g := rackStatesGame(t, false, 1600, 1000)
	dv := g.drum
	want := map[WidgetKind]image.Rectangle{
		WidgetTransport: dv.widgetRects[WidgetTransport],
		WidgetTimeline:  dv.widgetRects[WidgetTimeline],
	}
	painted := map[WidgetKind]bool{}
	orig := drawRect
	drawRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		for k, wr := range want {
			if filled && r == wr && !wr.Empty() {
				painted[k] = true
				if !colorsEqual(c, colRackSurface) {
					t.Errorf("widget %v bed painted %v, want the rack bed %v", k, c, colRackSurface)
				}
			}
		}
		orig(d, r, c, filled)
	}
	t.Cleanup(func() { drawRect = orig })
	newBackgroundLayer(dv).Draw(ebiten.NewImage(1600, 1000))
	for k := range want {
		if !painted[k] {
			t.Errorf("widget %v bed was not painted", k)
		}
	}
}

// TestFollowLockIsActiveNotHeldDown: follow-on is the lock key's active state
// (Toggled), never the `pressed` flag — ButtonStyle renders pressed as the −20
// press dim, which left a latched lock as a near-black square in the header.
func TestFollowLockIsActiveNotHeldDown(t *testing.T) {
	g := rackStatesGame(t, false, 1600, 1000)
	z := g.drum.transportZone
	for _, on := range []bool{true, false} {
		z.SetFollow(on)
		if z.trackBtn.Toggled() != on {
			t.Errorf("follow=%v: lock Toggled()=%v", on, z.trackBtn.Toggled())
		}
		if z.trackBtn.pressed {
			t.Errorf("follow=%v: lock carries the pressed flag (renders held down)", on)
		}
	}
}
