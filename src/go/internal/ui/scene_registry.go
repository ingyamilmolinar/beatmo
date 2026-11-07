package ui

import (
	"fmt"
	"sort"
)

// Compiled into every build; the catalog DATA is left out of the shipped WASM by -tags noscenes, ~150 KB no
// visitor ever runs.

// Setup runs synchronously before the countdown. MobileSetup replaces Setup on the mobile pass where a panel needs
// another route (EQ renders only after SetMobileEQMode); SettleFrames overrides the default 90.
type Scene struct {
	Name         string
	Description  string
	Mobile       bool
	SettleFrames int
	Setup        func(*Game)
	MobileSetup  func(*Game)

	// Crops the PNG to that surface after Setup + settle; empty means full screen.
	Subject Subject

	// The badge node placement arms is cleared after Setup so it never leaks into a capture; badge scenes opt out.
	KeepCoordBadge bool
}

func RunScene(g *Game, name string) error {
	return runSceneInternal(g, name, false)
}

// Falls back to Setup when MobileSetup is nil; errors for a non-Mobile scene.
func RunSceneMobile(g *Game, name string) error {
	return runSceneInternal(g, name, true)
}

func runSceneInternal(g *Game, name string, mobile bool) error {
	for _, s := range sceneCatalog {
		if s.Name == name {
			// The flag fires before the first Layout, and popups anchor on rects that only exist after one: two queued hops so
			// Setup runs on real geometry, as the browser export always did.
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

func applySceneSetup(g *Game, s Scene, mobile bool) {
	setup := s.Setup
	if mobile && s.MobileSetup != nil {
		setup = s.MobileSetup
	}
	if setup != nil {
		setup(g)
	}
	// It re-arms only on real input, so one clear after Setup holds through the countdown.
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

// mobile-prefixed scenes are mobile only.
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
