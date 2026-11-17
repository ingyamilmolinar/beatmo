package ui

import (
	"image"
	"testing"
)

// These tests reproduce the `make test-real` cascade where every test
// after a mobile-scene-setup helper fails with `forceSmallScreenForTest=true
// want false`. They live in a non-`//go:build test` file so they run under
// `make test-real` (which is where the cascade fires).
//
// Before the fix: each test's first sub-test mutates a package global
// (`forceSmallScreenForTest`, `forceAutoSize`) without registering a
// `t.Cleanup`; the second sub-test's `assertDefaultParityState` sees the
// stale value and `t.Fatalf`s.
//
// After the fix: `assertDefaultParityState` registers a `t.Cleanup` that
// resets every global it asserts on, so the second sub-test sees defaults.

// TestAssertDefaultParityState_ResetsForceSmallScreenAfterScene proves
// `forceSmallScreenForTest` is reset between sibling t.Run subtests when
// only `assertDefaultParityState` is used (no explicit per-test cleanup).
func TestAssertDefaultParityState_ResetsForceSmallScreenAfterScene(t *testing.T) {
	t.Run("flips_flag_via_SetForceMobileProfile", func(t *testing.T) {
		assertDefaultParityState(t)
		g := New(testLogger)
		t.Cleanup(g.CloseForTest)
		g.SetForceMobileProfile(true)
		// No explicit cleanup of forceSmallScreenForTest here — we rely on
		// assertDefaultParityState's auto-reset via t.Cleanup.
	})
	t.Run("next_test_sees_default", func(t *testing.T) {
		// Will t.Fatalf today: forceSmallScreenForTest=true want false
		assertDefaultParityState(t)
	})
}

// TestAssertDefaultParityState_ResetsForceAutoSize proves the same
// auto-reset for `forceAutoSize`.
func TestAssertDefaultParityState_ResetsForceAutoSize(t *testing.T) {
	t.Run("flips_force_auto_size", func(t *testing.T) {
		assertDefaultParityState(t)
		g := New(testLogger)
		t.Cleanup(g.CloseForTest)
		g.SetForceAutoSize(true)
	})
	t.Run("next_test_sees_default", func(t *testing.T) {
		assertDefaultParityState(t)
	})
}

// TestSceneCropMobileSetupOptional_DoesNotLeakProfile reproduces the actual
// `make test-real` cascade: a test that exercises a mobile scene leaks
// `forceSmallScreenForTest` to the next sibling test. The 31 scenes in
// scene_catalog.go that call SetForceMobileProfile(true) are the source.
func TestSceneCropMobileSetupOptional_DoesNotLeakProfile(t *testing.T) {
	t.Run("triggers_mobile_scene", func(t *testing.T) {
		assertDefaultParityState(t)
		g := New(testLogger)
		t.Cleanup(g.CloseForTest)
		g.Layout(390, 844)
		// Pick any scene whose Setup or MobileSetup calls
		// SetForceMobileProfile(true). `mobile_default` is the canonical
		// mobile scene and exercises the same global mutation as the
		// crop_eq_tab_* / crop_drum_view_* mobile passes.
		if err := RunSceneMobile(g, "mobile_default"); err != nil {
			// Fall back to RunScene if MobileSetup is nil — the goal is
			// to flip forceSmallScreenForTest=true.
			if err2 := RunScene(g, "mobile_default"); err2 != nil {
				t.Skipf("mobile_default not in catalog (skipping): %v / %v", err, err2)
			}
		}
		// Force a single Update so the scene's profile change settles.
		_ = g.Update()
		// Drop the rect so g.Update doesn't churn — we just need the
		// global to have been flipped.
		_ = image.Rectangle{}
	})
	t.Run("next_test_sees_default", func(t *testing.T) {
		assertDefaultParityState(t)
	})
}
