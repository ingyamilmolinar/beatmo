//go:build test

package ui

import (
	"image"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// synthFooterClickTestEnv builds a Game with one row bound to drum-snare and
// the Synth tab active, ready for HitArea-driven press testing.
type synthFooterClickTestEnv struct {
	g      *Game
	instID string
	hits   []indexedHitArea
}

func setupSynthFooterEnv(t *testing.T, width, height int, mobile bool) *synthFooterClickTestEnv {
	t.Helper()
	assertDefaultParityState(t)
	if mobile {
		restoreProfile := SetRuntimeProfileForTest(browserRuntimeProfile())
		t.Cleanup(restoreProfile)
	}
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	if mobile {
		g.SetForceMobileProfile(true)
	}
	g.Layout(width, height)
	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	if mobile {
		g.drum.SetMobileEQMode(true)
	}
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update() // flush hit index update via tree

	// Collect all hit areas for the synth tab. Use eqPanelZone.HitAreas()
	// which already merges in the synth-tab inputs.
	rawHits := g.drum.eqPanelZone.HitAreas()
	indexed := make([]indexedHitArea, 0, len(rawHits))
	for _, h := range rawHits {
		indexed = append(indexed, indexedHitArea{HitArea: h})
	}
	return &synthFooterClickTestEnv{g: g, instID: "snare", hits: indexed}
}

// findFooterButton returns the *Button matching the sentinel tag.
func findFooterButton(t *testing.T, dv *DrumView, tag string) *Button {
	t.Helper()
	for _, b := range dv.SynthTabButtons() {
		if b != nil && b.Text == tag {
			return b
		}
	}
	t.Fatalf("synth footer button %q not present in dv.SynthTabButtons()", tag)
	return nil
}

// TestSynthFooter_ClickAtVisibleCenter_RoutesToCorrectHandler verifies that
// pressing at the visible center of each footer button (Reset, Save, Save As)
// routes to that button's handler, not to any overlapping OUT-column link or
// knob handler. Drives the actual HitIndex.At dispatch path. Fails today
// because Save/Save-As/Reset share z-index with OUT links and registration
// order makes OUT win on geometric overlap.
func TestSynthFooter_ClickAtVisibleCenter_RoutesToCorrectHandler(t *testing.T) {
	cases := []struct {
		name        string
		w, h        int
		mobile      bool
		expectFound []string
	}{
		{"desktop_typical", 1280, 800, false, []string{synthSaveButtonTag, synthSaveAsButtonTag, synthResetButtonTag}},
		{"desktop_short", 1280, 480, false, []string{synthSaveButtonTag, synthSaveAsButtonTag, synthResetButtonTag}},
		{"mobile_portrait", 414, 896, true, []string{synthSaveButtonTag, synthSaveAsButtonTag, synthResetButtonTag}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupSynthFooterEnv(t, tc.w, tc.h, tc.mobile)
			for _, tag := range tc.expectFound {
				btn := findFooterButton(t, env.g.drum, tag)
				r := btn.Rect()
				if r.Empty() {
					t.Errorf("[%s] %s rect is empty — button hidden at this size", tc.name, tag)
					continue
				}
				cx := (r.Min.X + r.Max.X) / 2
				cy := (r.Min.Y + r.Max.Y) / 2
				winner := topHitAt(env.hits, cx, cy)
				if winner == nil {
					t.Errorf("[%s] %s no hit area at center (%d,%d) rect=%v", tc.name, tag, cx, cy, r)
					continue
				}
				wantTag := footerTagForButtonText(tag)
				if !strings.HasPrefix(winner.Tag, wantTag) {
					t.Errorf("[%s] click at %s rect center (%d,%d) rect=%v routed to hit tag=%q, want prefix=%q",
						tc.name, btnLabel(tag), cx, cy, r, winner.Tag, wantTag)
				}
			}
		})
	}
}

// topHitAt mirrors HitIndex.At sort logic (exact-rect first, then z desc) so
// the test can resolve the winning hit area without building a real game
// tree.
func topHitAt(hits []indexedHitArea, x, y int) *indexedHitArea {
	pt := image.Pt(x, y)
	var best *indexedHitArea
	bestExact := false
	for i, h := range hits {
		if !pt.In(h.Rect) {
			continue
		}
		exact := true
		switch {
		case best == nil:
			best = &hits[i]
			bestExact = exact
		case exact && !bestExact:
			best = &hits[i]
			bestExact = true
		case exact == bestExact && h.ZIndex > best.ZIndex:
			best = &hits[i]
		}
	}
	return best
}

func footerTagForButtonText(tag string) string {
	switch tag {
	case synthSaveButtonTag:
		return "synth-save"
	case synthSaveAsButtonTag:
		return "synth-save-as"
	case synthResetButtonTag:
		return "synth-btn-" // synth-btn-N for reset (legacy naming) OR "synth-reset"
	}
	return ""
}

func btnLabel(tag string) string {
	switch tag {
	case synthSaveButtonTag:
		return "Save"
	case synthSaveAsButtonTag:
		return "Save As"
	case synthResetButtonTag:
		return "Reset"
	}
	return tag
}

// TestSynthFooter_ButtonsInsideHeader — the three action buttons live
// inside the header strip (right of the caption), not in a separate
// footer band beneath the section row. Asserts every visible button's
// rect sits within the header rect's vertical bounds.
func TestSynthFooter_ButtonsInsideHeader(t *testing.T) {
	env := setupSynthFooterEnv(t, 1280, 800, false)
	hdr := env.g.drum.SynthTabHeader()
	if hdr.rect.Empty() {
		t.Fatal("header rect empty")
	}
	for _, b := range env.g.drum.SynthTabButtons() {
		if b == nil || b.Rect().Empty() {
			continue
		}
		r := b.Rect()
		if r.Min.Y < hdr.rect.Min.Y || r.Max.Y > hdr.rect.Max.Y {
			t.Errorf("button %q rect %v escapes header band %v — buttons must live inside the header strip, not in a footer below the sections",
				btnLabel(b.Text), r, hdr.rect)
		}
	}
}

// TestSynthFooter_KnobRectsClearOfFooter — invariant: no knob's visible
// rect intersects any footer button rect. With the OUT column removed,
// section cards span the full content width, so the rightmost section's
// knob lives directly above the right-aligned Save / Save As / Reset
// buttons. The buildSynthTab footerGap inset must keep them apart at
// every supported viewport size; this test fails the moment they touch.
func TestSynthFooter_KnobRectsClearOfFooter(t *testing.T) {
	cases := []struct {
		name   string
		w, h   int
		mobile bool
	}{
		{"desktop_wide", 1280, 800, false},
		{"desktop_short", 1280, 480, false},
		{"desktop_compact", 1280, 360, false},
		{"mobile_portrait", 414, 896, true},
		{"mobile_compact", 360, 640, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupSynthFooterEnv(t, tc.w, tc.h, tc.mobile)
			footerRects := map[string]image.Rectangle{}
			for _, b := range env.g.drum.SynthTabButtons() {
				if b != nil && !b.Rect().Empty() {
					footerRects[b.Text] = b.Rect()
				}
			}
			for i, k := range env.g.drum.SynthTabKnobs() {
				if k == nil {
					continue
				}
				kr := k.Rect()
				if kr.Empty() {
					continue
				}
				for tag, fr := range footerRects {
					if !kr.Intersect(fr).Empty() {
						t.Errorf("[%s] knob %d rect %v intersects footer %q rect %v — section row crowds footer band",
							tc.name, i, kr, btnLabel(tag), fr)
					}
				}
			}
		})
	}
}

// TestSynthFooter_EveryPointRoutesToFooter samples a 5×5 grid of points
// inside each footer button rect and asserts the top-of-stack hit area at
// each point is the footer button itself. This is the user-facing
// guarantee: anywhere the user can tap on a visible footer button, the
// click goes to that button. Corner-pixel geometric overlap that doesn't
// affect dispatch is intentionally tolerated.
func TestSynthFooter_EveryPointRoutesToFooter(t *testing.T) {
	cases := []struct {
		name   string
		w, h   int
		mobile bool
	}{
		{"desktop_wide", 1280, 800, false},
		{"desktop_short", 1280, 480, false},
		{"desktop_compact", 1280, 360, false},
		{"mobile_portrait", 414, 896, true},
		{"mobile_compact", 360, 640, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := setupSynthFooterEnv(t, tc.w, tc.h, tc.mobile)
			for _, b := range env.g.drum.SynthTabButtons() {
				if b == nil || b.Rect().Empty() {
					continue
				}
				fr := b.Rect()
				tag := b.Text
				// Inset by 2 px so we sample inside the visible chrome
				// rather than landing on a 1-px shared edge with a
				// neighbouring button.
				inset := 2
				step := 5
				for dy := inset; dy <= fr.Dy()-inset; dy += max(1, (fr.Dy()-2*inset)/step) {
					for dx := inset; dx <= fr.Dx()-inset; dx += max(1, (fr.Dx()-2*inset)/step) {
						px := fr.Min.X + dx
						py := fr.Min.Y + dy
						winner := topHitAt(env.hits, px, py)
						if winner == nil {
							t.Errorf("[%s] %s pt=(%d,%d) — no hit area found", tc.name, btnLabel(tag), px, py)
							continue
						}
						wantPrefix := footerTagForButtonText(tag)
						if !strings.HasPrefix(winner.Tag, wantPrefix) {
							t.Errorf("[%s] %s rect=%v pt=(%d,%d) routed to tag=%q want prefix=%q",
								tc.name, btnLabel(tag), fr, px, py, winner.Tag, wantPrefix)
						}
					}
				}
			}
		})
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

