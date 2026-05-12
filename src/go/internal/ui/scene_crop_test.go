package ui

import (
	"strings"
	"testing"
)

// TestSceneCropEntriesDeclareSubject — every catalog scene whose Name
// starts with the "crop_" prefix must declare a non-empty Subject. This
// is the load-bearing invariant for the screenshot-cropping pipeline:
// without a Subject, the cropped output would silently fall back to a
// full-screen capture and the prefix would be a lie.
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

// TestSceneCropSubjectsResolveToVisibleRect — for every crop_* scene,
// running its Setup followed by a few Update cycles must leave SubjectRect
// returning ok=true with a non-empty rect that fits inside the framebuffer.
// This is the strictly-larger-coverage replacement for visually inspecting
// each crop output.
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
			// Drive a handful of Update frames so caches/layout populate
			// just like the screenshot harness's settle countdown does.
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
			// Bound: a cropped subject should be smaller than the full
			// screen by at least one dimension. If a crop equals the full
			// screen exactly, the subject is mis-configured and the user
			// gets nothing for the prefix.
			if rect.Dx() == 1280 && rect.Dy() == 720 {
				t.Errorf("SubjectRect(%q) = %v fills the entire framebuffer; choose a tighter subject or remove the crop_ prefix",
					s.Subject, rect)
			}
		})
	}
}

// TestSceneCropScopeVariantsProduceDistinctState — each crop_scope_*
// scene must move the scope zone into a state visibly different from
// crop_chain_default. Without this check the scope variants would all
// render pixel-identically (which we observed during validation: 4 of 5
// scope crops were byte-identical because the setters silently no-op'd).
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
		{"crop_chain_auto_gain_on", func(t *testing.T, z *ChainPanelZone) {
			if !z.AutoGain() {
				t.Errorf("scope.AutoGain()=false after crop_chain_auto_gain_on Setup")
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

// TestSceneCropMobileSetupOptional — crop_* scenes with a non-nil
// MobileSetup must still produce a visible Subject when applied via the
// mobile pass. This enforces that any mobile branch we add later doesn't
// silently break the cropped capture.
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
