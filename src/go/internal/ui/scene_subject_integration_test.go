package ui

import (
	"testing"
)

// TestRunSceneSetsScreenshotSubject verifies that running a catalog scene
// whose Subject is set populates the game's screenshotSubject field. This
// is the bridge between scene declaration and the captureScreen crop step.
func TestRunSceneSetsScreenshotSubject(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	// Inject a temporary scene into the catalog so we don't depend on
	// the canonical crop_* set being added yet.
	const tmpScene = "test_subject_integration_grid"
	prev := sceneCatalog
	t.Cleanup(func() { sceneCatalog = prev })
	sceneCatalog = append(append([]Scene{}, prev...), Scene{
		Name:    tmpScene,
		Setup:   func(*Game) {},
		Subject: SubjectMainGrid,
	})

	if err := RunScene(g, tmpScene); err != nil {
		t.Fatalf("RunScene(%q): %v", tmpScene, err)
	}
	if g.screenshotSubject != SubjectMainGrid {
		t.Errorf("after RunScene, screenshotSubject = %q; want %q", g.screenshotSubject, SubjectMainGrid)
	}
}

// TestRunSceneClearsScreenshotSubjectForLegacyScene — every existing scene
// has Subject="" and must leave screenshotSubject empty so the legacy
// full-screen capture path is preserved.
func TestRunSceneClearsScreenshotSubjectForLegacyScene(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	// Pre-populate to a non-empty subject, then run a legacy scene that
	// must reset it.
	g.SetScreenshotSubject(SubjectMainGrid)
	if err := RunScene(g, "transport_idle"); err != nil {
		t.Fatalf("RunScene(transport_idle): %v", err)
	}
	if g.screenshotSubject != SubjectFullScreen {
		t.Errorf("after legacy scene, screenshotSubject = %q; want SubjectFullScreen", g.screenshotSubject)
	}
}

// TestSceneCatalogLegacyScenesUnchanged — guard rail: every entry in the
// catalog must still default to SubjectFullScreen until intentionally
// migrated. Phase 5 will introduce crop_* scenes; this test will be
// updated then to allow the prefix.
func TestSceneCatalogLegacyScenesUnchanged(t *testing.T) {
	for _, s := range sceneCatalog {
		if s.Subject == SubjectFullScreen {
			continue
		}
		// Once Phase 5 lands, scenes named with the "crop_" prefix may
		// declare a Subject. Anything else is a regression.
		if !hasCropPrefix(s.Name) {
			t.Errorf("scene %q has Subject=%q but is not a crop_* scene", s.Name, s.Subject)
		}
	}
}

func hasCropPrefix(name string) bool {
	const p = "crop_"
	return len(name) >= len(p) && name[:len(p)] == p
}
