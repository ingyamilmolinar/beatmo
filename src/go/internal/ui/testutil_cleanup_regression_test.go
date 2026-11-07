package ui

import (
	"image"
	"testing"
)

// A sub-test that mutates a package global without a cleanup poisons its siblings; assertDefaultParityState
// resets every global it asserts on. Untagged, so `make test-real`, where the mobile-scene leak shows, runs it.

// With only assertDefaultParityState, no explicit per-test cleanup.
func TestAssertDefaultParityState_ResetsForceSmallScreenAfterScene(t *testing.T) {
	t.Run("flips_flag_via_SetForceMobileProfile", func(t *testing.T) {
		assertDefaultParityState(t)
		g := New(testLogger)
		t.Cleanup(g.CloseForTest)
		g.SetForceMobileProfile(true)
	})
	t.Run("next_test_sees_default", func(t *testing.T) {
		// Will t.Fatalf today: forceSmallScreenForTest=true want false
		assertDefaultParityState(t)
	})
}

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

// Mobile scenes call SetForceMobileProfile(true), which would leak to the next sibling test.
func TestSceneCropMobileSetupOptional_DoesNotLeakProfile(t *testing.T) {
	t.Run("triggers_mobile_scene", func(t *testing.T) {
		assertDefaultParityState(t)
		g := New(testLogger)
		t.Cleanup(g.CloseForTest)
		g.Layout(390, 844)
		// mobile_default mutates the same global as the mobile crop passes.
		if err := RunSceneMobile(g, "mobile_default"); err != nil {
			// RunScene when MobileSetup is nil: the point is to flip the global.
			if err2 := RunScene(g, "mobile_default"); err2 != nil {
				t.Skipf("mobile_default not in catalog (skipping): %v / %v", err, err2)
			}
		}
		// Force a single Update so the scene's profile change settles.
		_ = g.Update()
		_ = image.Rectangle{}
	})
	t.Run("next_test_sees_default", func(t *testing.T) {
		assertDefaultParityState(t)
	})
}
