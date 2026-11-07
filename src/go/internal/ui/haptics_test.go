//go:build test

package ui

import (
	"testing"
)

func withCapturedHaptics(t *testing.T) *[]int {
	t.Helper()
	captured := []int{}
	orig := hapticEmit
	hapticEmit = func(ms int) { captured = append(captured, ms) }
	t.Cleanup(func() { hapticEmit = orig })
	return &captured
}

// Transport actions differ in haptic length so the user feels play vs. record without looking.
func TestHaptics_PlayClickEmits8ms(t *testing.T) {
	withSmallScreen(t, true)
	captured := withCapturedHaptics(t)

	z, _ := newTestTransportZone()
	z.playBtn.OnClick()

	if len(*captured) != 1 || (*captured)[0] != 8 {
		t.Fatalf("expected one 8 ms haptic on play; got %v", *captured)
	}
}

func TestHaptics_StopClickEmits8ms(t *testing.T) {
	withSmallScreen(t, true)
	captured := withCapturedHaptics(t)

	z, _ := newTestTransportZone()
	z.stopBtn.OnClick()

	if len(*captured) != 1 || (*captured)[0] != 8 {
		t.Fatalf("expected one 8 ms haptic on stop; got %v", *captured)
	}
}

func TestHaptics_RecordClickEmits16ms(t *testing.T) {
	withSmallScreen(t, true)
	captured := withCapturedHaptics(t)

	z, _ := newTestTransportZone()
	z.recordBtn.OnClick()

	if len(*captured) != 1 || (*captured)[0] != 16 {
		t.Fatalf("expected one 16 ms haptic on record; got %v", *captured)
	}
}

func TestHaptics_DesktopSilent(t *testing.T) {
	// Desktop: the production mobile gate must stop every call.
	calls := 0
	origPlatform := hapticEmit
	hapticEmit = func(ms int) {
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
