package ui

import (
	"strings"
	"testing"
)

// Without a Subject the crop silently falls back to a full-screen capture.
func TestSceneCropEntriesDeclareSubject(t *testing.T) {
	for _, s := range sceneCatalog {
		if !strings.HasPrefix(s.Name, "crop_") {
			continue
		}
		if s.Subject == SubjectFullScreen {
			t.Errorf("scene %q has crop_ prefix but Subject is empty", s.Name)
		}
	}
}

func TestSceneCropSubjectsResolveToVisibleRect(t *testing.T) {
	for _, s := range sceneCatalog {
		if !strings.HasPrefix(s.Name, "crop_") {
			continue
		}
		s := s // capture
		t.Run(s.Name, func(t *testing.T) {
			assertDefaultParityState(t)
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(1280, 720)

			if err := RunScene(g, s.Name); err != nil {
				t.Fatalf("RunScene(%q): %v", s.Name, err)
			}
			// Like the harness's settle countdown.
			for i := 0; i < 8; i++ {
				_ = g.Update()
			}

			rect, ok := g.SubjectRect(s.Subject)
			if !ok {
				t.Fatalf("SubjectRect(%q) ok=false after Setup; the Subject is not visible",
					s.Subject)
			}
			if rect.Empty() {
				t.Fatalf("SubjectRect(%q) returned empty rect", s.Subject)
			}
			if rect.Min.X < 0 || rect.Min.Y < 0 || rect.Max.X > 1280 || rect.Max.Y > 720 {
				t.Errorf("SubjectRect(%q) = %v escapes 1280x720 framebuffer", s.Subject, rect)
			}
			// A crop equal to the full screen is mis-configured.
			if rect.Dx() == 1280 && rect.Dy() == 720 {
				t.Errorf("SubjectRect(%q) = %v fills the entire framebuffer; choose a tighter subject or remove the crop_ prefix",
					s.Subject, rect)
			}
		})
	}
}

// The bounds check would pass a silently failed selection (chip strip not built yet → default stage), so the
// selected stage is pinned. The forced layouts go through the tree, not a direct eqPanelZone.Layout.
func TestSynthChipSceneSelectsIntendedStage(t *testing.T) {
	cases := []struct {
		scene     string
		wantLabel string
	}{
		{"crop_synth_tab_chips_collapsed", "OSC"},
		{"crop_synth_tab_detail_envelope", "ENVELOPE"},
		{"crop_synth_tab_ghost_chip", "FILTER"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.scene, func(t *testing.T) {
			assertDefaultParityState(t)
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(1280, 720)

			if err := RunScene(g, tc.scene); err != nil {
				t.Fatalf("RunScene(%q): %v", tc.scene, err)
			}
			for i := 0; i < 8; i++ {
				_ = g.Update()
			}

			inst := g.drum.resolveSynthInstrument(g.drum.synthTabActiveInstrument())
			tgt := g.drum.synthDrillTarget(inst)
			if tgt.kind != drillStage {
				t.Fatalf("scene %q: not drilled into a stage (target %+v)", tc.scene, tgt)
			}
			selLabel := sectionLabel(tgt.stage)
			if selLabel != tc.wantLabel {
				t.Errorf("scene %q: selected stage %q, want %q — SelectSynthSectionByLabel did not take (silent fallback to default)",
					tc.scene, selLabel, tc.wantLabel)
			}
		})
	}
}

// Setters that silently no-op would make every scope variant render identically.
func TestSceneCropScopeVariantsProduceDistinctState(t *testing.T) {
	type expect struct {
		scene string
		check func(*testing.T, *ChainPanelZone)
	}
	cases := []expect{
		{"crop_chain_frozen", func(t *testing.T, z *ChainPanelZone) {
			if !z.Frozen() {
				t.Errorf("scope.Frozen()=false after crop_chain_frozen Setup")
			}
		}},
		{"crop_chain_default", func(t *testing.T, z *ChainPanelZone) {
			// Auto-gain is the default, so the DEFAULT scene pins it.
			if !z.AutoGain() {
				t.Errorf("scope.AutoGain()=false in the default Chain scene")
			}
		}},
		{"crop_chain_trace_a_only", func(t *testing.T, z *ChainPanelZone) {
			if z.TraceVisible("B") {
				t.Errorf("scope.TraceVisible(B)=true after crop_chain_trace_a_only Setup")
			}
			if !z.TraceVisible("A") {
				t.Errorf("scope.TraceVisible(A)=false; trace A should remain visible")
			}
		}},
		{"crop_chain_zoomed_in", func(t *testing.T, z *ChainPanelZone) {
			if z.WindowMs() >= 20 {
				t.Errorf("scope.WindowMs()=%v; want <20 (zoomed in)", z.WindowMs())
			}
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.scene, func(t *testing.T) {
			assertDefaultParityState(t)
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(1280, 720)

			if err := RunScene(g, c.scene); err != nil {
				t.Fatalf("RunScene(%q): %v", c.scene, err)
			}
			for i := 0; i < 8; i++ {
				_ = g.Update()
			}
			z := chainZoneOf(g)
			if z == nil {
				t.Fatal("scope zone unreachable after scene Setup")
			}
			c.check(t, z)
		})
	}
}

func TestSceneCropMobileSetupOptional(t *testing.T) {
	for _, s := range sceneCatalog {
		if !strings.HasPrefix(s.Name, "crop_") {
			continue
		}
		if s.MobileSetup == nil {
			continue
		}
		s := s
		t.Run(s.Name, func(t *testing.T) {
			assertDefaultParityState(t)
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(390, 844)

			if err := RunSceneMobile(g, s.Name); err != nil {
				t.Fatalf("RunSceneMobile(%q): %v", s.Name, err)
			}
			for i := 0; i < 8; i++ {
				_ = g.Update()
			}
			if _, ok := g.SubjectRect(s.Subject); !ok {
				t.Errorf("RunSceneMobile(%q): subject %q not visible", s.Name, s.Subject)
			}
		})
	}
}
