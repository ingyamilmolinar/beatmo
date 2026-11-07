package ui

import (
	"testing"
)

// The bridge between a scene's declaration and the crop step.
func TestRunSceneSetsScreenshotSubject(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	// A temporary scene, so the test does not depend on the catalog's crop_ set.
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

// A scene with no Subject keeps the full-screen capture.
func TestRunSceneClearsScreenshotSubjectForLegacyScene(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	g.SetScreenshotSubject(SubjectMainGrid)
	if err := RunScene(g, "transport_idle"); err != nil {
		t.Fatalf("RunScene(transport_idle): %v", err)
	}
	if g.screenshotSubject != SubjectFullScreen {
		t.Errorf("after legacy scene, screenshotSubject = %q; want SubjectFullScreen", g.screenshotSubject)
	}
}

// Only crop_ scenes may declare a Subject.
func TestSceneCatalogLegacyScenesUnchanged(t *testing.T) {
	for _, s := range sceneCatalog {
		if s.Subject == SubjectFullScreen {
			continue
		}
		if !hasCropPrefix(s.Name) {
			t.Errorf("scene %q has Subject=%q but is not a crop_* scene", s.Name, s.Subject)
		}
	}
}

func hasCropPrefix(name string) bool {
	const p = "crop_"
	return len(name) >= len(p) && name[:len(p)] == p
}
