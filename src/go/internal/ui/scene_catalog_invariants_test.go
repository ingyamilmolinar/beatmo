package ui

import (
	"regexp"
	"strings"
	"testing"
)

// nameRE is the kebab-case-with-digits-and-underscores pattern. Existing
// scenes use underscores liberally (e.g. transport_high_bpm), so we accept
// `[a-z][a-z0-9_]*` rather than strict kebab-case-with-hyphens.
var sceneNameRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// TestSceneCatalogNamesUniqueAndKebabCase guards the most basic catalog
// invariants: names must be unique (the screenshot harness keys on them)
// and conform to a stable lowercase format so external tooling can match
// them without surprise.
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

// TestSceneCatalogCropSubjectInvariant enforces the "subject ↔ crop_"
// rule. Per CLAUDE.md § Screenshot Subjects, every crop_* scene must
// declare a Subject (the harness uses it to clip the PNG), and every
// non-crop scene must NOT declare one (or the harness silently crops a
// full-screen capture).
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

// TestSceneCatalogMobileScenesHaveSetup verifies that every Mobile=true
// scene is actually invokable on the mobile pass — either MobileSetup is
// non-nil, or Setup is non-nil (which RunSceneMobile falls back to).
// Without this guard, a Mobile entry with both nil would no-op silently
// and the screenshot would duplicate the bare mobile_default.
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

// TestSceneCatalogSettleFramesBounded keeps a ceiling on declared
// SettleFrames. The default is 90; any single scene above 300 (5 seconds
// at 60fps) is almost certainly a misconfigured copy-paste. This guard
// fails loudly so the screenshot harness doesn't grind for minutes.
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

// TestSceneCatalogListScenesStable asserts that ListScenes returns the
// catalog sorted by Name (so external tooling that depends on iteration
// order — manifest.json, the screenshots-all SCENES filter — sees stable
// output).
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

// TestSceneCatalogSceneNamesFilters covers the SceneNames(includeMobile)
// branches: mobile-only entries (name prefix "mobile_") are excluded
// when includeMobile=false, but Mobile=true entries that ALSO run on
// desktop (e.g. transport_idle) remain visible. This is the contract
// SceneNames(false) gives `screenshots-all` for the desktop pass.
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

// TestSceneCatalogRunSceneUnknownReturnsError nails the documented
// "RunScene returns an error if the name is not in the catalog" contract
// — important because tooling depends on it (not a panic).
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
