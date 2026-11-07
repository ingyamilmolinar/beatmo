package ui

import (
	"regexp"
	"strings"
	"testing"
)

// Existing scenes use underscores (transport_high_bpm), so strict kebab-case is not required.
var sceneNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// The screenshot harness keys on names; a stable lowercase format lets tooling match them.
func TestSceneCatalogNamesUniqueAndKebabCase(t *testing.T) {
	seen := map[string]int{}
	for i, s := range sceneCatalog {
		if !sceneNameRE.MatchString(s.Name) {
			t.Errorf("scene[%d] %q does not match %s", i, s.Name, sceneNameRE)
		}
		if prev, dup := seen[s.Name]; dup {
			t.Errorf("scene name %q appears at indices %d and %d", s.Name, prev, i)
		}
		seen[s.Name] = i
	}
}

// A crop_ scene needs a Subject to clip the PNG; any other scene must not have one, or a full-screen capture is
// cropped silently.
func TestSceneCatalogCropSubjectInvariant(t *testing.T) {
	for _, s := range sceneCatalog {
		isCrop := strings.HasPrefix(s.Name, "crop_")
		hasSubject := s.Subject != ""
		switch {
		case isCrop && !hasSubject:
			t.Errorf("crop scene %q missing Subject (harness would emit full-screen PNG)", s.Name)
		case !isCrop && hasSubject:
			t.Errorf("non-crop scene %q declares Subject %q (use the crop_ prefix or remove Subject)", s.Name, s.Subject)
		}
	}
}

// A Mobile entry with neither setup would no-op and duplicate mobile_default.
func TestSceneCatalogMobileScenesHaveSetup(t *testing.T) {
	for _, s := range sceneCatalog {
		if !s.Mobile {
			continue
		}
		if s.Setup == nil && s.MobileSetup == nil {
			t.Errorf("Mobile scene %q has neither Setup nor MobileSetup; mobile pass would no-op", s.Name)
		}
	}
}

// A scene above the cap is almost certainly a copy-paste error, and the harness would grind for minutes.
func TestSceneCatalogSettleFramesBounded(t *testing.T) {
	const ceiling = 300
	for _, s := range sceneCatalog {
		if s.SettleFrames < 0 {
			t.Errorf("scene %q has negative SettleFrames=%d", s.Name, s.SettleFrames)
		}
		if s.SettleFrames > ceiling {
			t.Errorf("scene %q has SettleFrames=%d > %d (5s @ 60fps); probable misconfig", s.Name, s.SettleFrames, ceiling)
		}
	}
}

// manifest.json and the SCENES filter depend on the order.
func TestSceneCatalogListScenesStable(t *testing.T) {
	scenes := ListScenes()
	for i := 1; i < len(scenes); i++ {
		if scenes[i-1].Name >= scenes[i].Name {
			t.Errorf("ListScenes not strictly sorted at i=%d: %q >= %q", i, scenes[i-1].Name, scenes[i].Name)
		}
	}
	if len(scenes) != len(sceneCatalog) {
		t.Errorf("ListScenes returned %d entries, catalog has %d", len(scenes), len(sceneCatalog))
	}
}

// Mobile-only entries ("mobile_") drop out of the desktop pass; Mobile=true entries that also run on desktop stay.
func TestSceneCatalogSceneNamesFilters(t *testing.T) {
	full := SceneNames(true)
	desktopOnly := SceneNames(false)
	if len(desktopOnly) > len(full) {
		t.Fatalf("desktopOnly (%d) > full (%d) — filter inverted", len(desktopOnly), len(full))
	}
	for _, name := range desktopOnly {
		if strings.HasPrefix(name, "mobile_") {
			t.Errorf("SceneNames(false) returned mobile-only name %q", name)
		}
	}
	// Sanity: every name in desktopOnly is in full.
	fullSet := make(map[string]struct{}, len(full))
	for _, n := range full {
		fullSet[n] = struct{}{}
	}
	for _, n := range desktopOnly {
		if _, ok := fullSet[n]; !ok {
			t.Errorf("SceneNames(false) returned %q which is not in SceneNames(true)", n)
		}
	}
}

// Tooling depends on an error, not a panic.
func TestSceneCatalogRunSceneUnknownReturnsError(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	if err := RunScene(g, "this_scene_does_not_exist"); err == nil {
		t.Fatal("RunScene with unknown name returned nil, want error")
	}
	if err := RunSceneMobile(g, "this_scene_does_not_exist"); err == nil {
		t.Fatal("RunSceneMobile with unknown name returned nil, want error")
	}
}
