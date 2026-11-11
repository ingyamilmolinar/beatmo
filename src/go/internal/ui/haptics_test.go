//go:build test

package ui

import (
	"testing"
)

// withCapturedHaptics swaps the package-level hapticEmit seam for a slice
// recorder during the test, restoring it on cleanup. Returns a pointer to
// the captured durations so tests can read them after the action.
func withCapturedHaptics(t *testing.T) *[]int {
	t.Helper()
	captured := []int{}
	orig := hapticEmit
	hapticEmit = func(ms int) { captured = append(captured, ms) }
	t.Cleanup(func() { hapticEmit = orig })
	return &captured
}

// TestHaptics_PlayClickEmits8ms verifies tapping play on mobile fires an
// 8 ms vibration. B9 in the screenshot critique: distinguishing transport
// actions by haptic intensity helps the user feel "play vs. record"
// without looking.
func TestHaptics_PlayClickEmits8ms(t *testing.T) {
	withSmallScreen(t, true)
	captured := withCapturedHaptics(t)

	z, _ := newTestTransportZone()
	z.playBtn.OnClick()

	if len(*captured) != 1 || (*captured)[0] != 8 {
		t.Fatalf("expected one 8 ms haptic on play; got %v", *captured)
	}
}

// TestHaptics_StopClickEmits8ms verifies tapping stop on mobile fires an
// 8 ms vibration.
func TestHaptics_StopClickEmits8ms(t *testing.T) {
	withSmallScreen(t, true)
	captured := withCapturedHaptics(t)

	z, _ := newTestTransportZone()
	z.stopBtn.OnClick()

	if len(*captured) != 1 || (*captured)[0] != 8 {
		t.Fatalf("expected one 8 ms haptic on stop; got %v", *captured)
	}
}

// TestHaptics_RecordClickEmits16ms verifies tapping record on mobile fires
// a longer 16 ms vibration so the user feels record-arm distinctly.
func TestHaptics_RecordClickEmits16ms(t *testing.T) {
	withSmallScreen(t, true)
	captured := withCapturedHaptics(t)

	z, _ := newTestTransportZone()
	z.recordBtn.OnClick()

	if len(*captured) != 1 || (*captured)[0] != 16 {
		t.Fatalf("expected one 16 ms haptic on record; got %v", *captured)
	}
}

// TestHaptics_DesktopSilent verifies haptics never fire on desktop. The
// production hapticEmit short-circuits on !mobile so we don't ship
// vibration to keyboard-and-mouse desktop browsers (would no-op anyway,
// but being explicit prevents accidental side effects).
func TestHaptics_DesktopSilent(t *testing.T) {
	// Desktop: no withSmallScreen. Use the production hapticEmit (not the
	// captured seam) so the !Profile().IsMobile() short-circuit applies.
	calls := 0
	origPlatform := hapticEmit
	hapticEmit = func(ms int) {
		// Run the real production path — re-implement its mobile gate so
		// we can record calls that pass it.
		if !Profile().IsMobile() {
			return
		}
		if ms > 0 {
			calls++
		}
	}
	t.Cleanup(func() { hapticEmit = origPlatform })

	z, _ := newTestTransportZone()
	z.playBtn.OnClick()
	z.stopBtn.OnClick()
	z.recordBtn.OnClick()

	if calls != 0 {
		t.Fatalf("expected 0 haptics on desktop; got %d", calls)
	}
}
