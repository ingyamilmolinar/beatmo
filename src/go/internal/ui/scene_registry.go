package ui

import (
	"fmt"
	"sort"
)

// scene_registry.go — the scene TYPE and the lookups the screenshot harness,
// the -scene / -list-scenes flags and the runScene/listScenes JS exports go
// through. It is compiled into every build; the catalog DATA it reads
// (sceneCatalog, scene_catalog*.go) is dev tooling and is left out of the
// shipped WASM by `-tags noscenes` (`make wasm`) -- about 150 KB of code that
// no visitor ever runs. Without it sceneCatalog is empty (scene_catalog_off.go)
// and every lookup reports that the catalog is not compiled in.

// Scene captures a reproducible UI state for the screenshot harness.
// Setup runs synchronously before the screenshot countdown starts.
//
// Mobile=true marks scenes that are also captured under MOBILE=1.
// MobileSetup, if non-nil, replaces Setup on the mobile capture pass —
// use it when a panel/overlay needs a different invocation path on
// mobile (e.g. EQ panel only renders after SetMobileEQMode(true)).
// SettleFrames overrides the default 90-frame screenshot wait when the
// scene's overlays/caches need extra time to stabilize.
type Scene struct {
	Name         string
	Description  string
	Mobile       bool
	SettleFrames int
	Setup        func(*Game)
	MobileSetup  func(*Game)

	// Subject, when non-empty, instructs the screenshot harness to crop
	// the captured PNG to that surface's on-screen bounds (resolved via
	// (*Game).SubjectRect after Setup + settle). The empty value preserves
	// the legacy full-screen behavior — every existing scene leaves this
	// unset and is unchanged.
	Subject Subject

	// KeepCoordBadge opts a scene out of the post-Setup coordinate-badge
	// clear. By default applySceneSetup nils g.coordBadgeNode so the
	// "(i, j)" pill that node placement transiently arms never leaks into
	// an unrelated capture. Scenes that exist to VERIFY the badge itself
	// (e.g. coord_badge_fraction) set this true so their armed badge
	// survives the settle countdown and renders.
	KeepCoordBadge bool
}

// RunScene applies the named scene's Setup to g. Returns an error if the
// name is not in the catalog.
func RunScene(g *Game, name string) error {
	return runSceneInternal(g, name, false)
}

// RunSceneMobile applies the named scene's MobileSetup to g (or Setup if
// MobileSetup is nil). Returns an error if the name is not in the
// catalog or the scene is not marked Mobile.
func RunSceneMobile(g *Game, name string) error {
	return runSceneInternal(g, name, true)
}

func runSceneInternal(g *Game, name string, mobile bool) error {
	for _, s := range sceneCatalog {
		if s.Name == name {
			// The native -scene flag fires before Ebiten's first
			// Layout/Update. Popup-opening setups anchor against control
			// rects that only exist after a layout pass, so applying the
			// scene pre-layout mis-anchors every popup (context menu
			// clipped in a corner, instrument menu fully offscreen). Defer
			// to the update loop — two queued hops so frame 1 performs the
			// first Layout + recalc and frame 2 runs Setup against real
			// geometry (mirrors the browser runScene export, which always
			// deferred via QueueAction and never mis-anchored).
			if g.winW == 0 {
				scene := s
				g.QueueAction(func(g *Game) {
					g.QueueAction(func(g *Game) { applySceneSetup(g, scene, mobile) })
				})
				return nil
			}
			applySceneSetup(g, s, mobile)
			return nil
		}
	}
	if len(sceneCatalog) == 0 {
		return fmt.Errorf("unknown scene %q: the scene catalog is not compiled into this build (-tags noscenes; build with `make wasm-dev`)", name)
	}
	return fmt.Errorf("unknown scene %q", name)
}

// applySceneSetup performs the scene's Setup against a laid-out game and
// arms the screenshot settle/subject state.
func applySceneSetup(g *Game, s Scene, mobile bool) {
	setup := s.Setup
	if mobile && s.MobileSetup != nil {
		setup = s.MobileSetup
	}
	if setup != nil {
		setup(g)
	}
	// Suppress the coordinate badge ("(i, j)" pill) that node
	// placement arms: tryAddNode selects the new node and sets
	// g.coordBadgeNode, which would otherwise leak a transient
	// debug-ish hover affordance into the capture (observed as a
	// stray "(4, 2)" pill in graph_complex_3_nodes_4_edges). It
	// re-arms only on real input, so a single clear after Setup
	// holds through the settle countdown. Scenes verifying the badge
	// itself opt out via KeepCoordBadge.
	if !s.KeepCoordBadge {
		g.coordBadgeNode = nil
	}
	if s.SettleFrames > 0 {
		g.SetScreenshotSettleFrames(s.SettleFrames)
	}
	g.SetScreenshotSubject(s.Subject)
	emitSceneApplied(s.Name)
}

// ListScenes returns the catalog sorted by name.
func ListScenes() []Scene {
	out := make([]Scene, len(sceneCatalog))
	copy(out, sceneCatalog)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// SceneNames returns just the scene names sorted alphabetically.
func SceneNames(includeMobile bool) []string {
	var names []string
	for _, s := range sceneCatalog {
		if !includeMobile && s.Mobile && !sceneRunsOnDesktop(s) {
			continue
		}
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return names
}

// sceneRunsOnDesktop returns true if the scene is appropriate for the
// desktop pass. Mobile-prefixed scenes are mobile-only.
func sceneRunsOnDesktop(s Scene) bool {
	if len(s.Name) >= 7 && s.Name[:7] == "mobile_" {
		return false
	}
	return true
}

// MobileSceneNames returns the names of scenes captured under MOBILE=1.
func MobileSceneNames() []string {
	var names []string
	for _, s := range sceneCatalog {
		if s.Mobile {
			names = append(names, s.Name)
		}
	}
	sort.Strings(names)
	return names
}
