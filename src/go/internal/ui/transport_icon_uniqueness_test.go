//go:build test

package ui

import (
	"image"
	"testing"
)

// Two buttons with one icon in one toolbar cannot be told apart. A button that swaps icons shows one at a
// time, and each variant must still be unique.
func TestMobileTransportIconsAreDistinct(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	z, _ := newTestTransportZone()
	z.Layout(image.Rect(0, 0, 390, 96))

	collectIcons := func() []string {
		var got []string
		add := func(name, icon string) {
			if icon == "" {
				return
			}
			got = append(got, icon+" ("+name+")")
		}
		add("play", z.playBtn.Icon)
		add("stop", z.stopBtn.Icon)
		add("record", z.recordBtn.Icon)
		add("bpm-up", z.bpmIncBtn.Icon)
		add("bpm-down", z.bpmDecBtn.Icon)
		if z.viewSwitchBtn != nil {
			add("view", z.viewSwitchBtn.Icon)
		}
		return got
	}

	assertNoIconCollisions := func(label string, icons []string) {
		t.Helper()
		seen := map[string]string{}
		for _, entry := range icons {
			// entry is "icon (name)"; split to compare icon-only.
			icon := entry
			if idx := indexOf(entry, " ("); idx > 0 {
				icon = entry[:idx]
			}
			if prev, dup := seen[icon]; dup {
				t.Errorf("%s: icon %q used by both %q and %q",
					label, icon, prev, entry)
			}
			seen[icon] = entry
		}
	}

	assertNoIconCollisions("default state", collectIcons())

	// Running playback.
	z.playBtn.Icon = string(IconPause)
	assertNoIconCollisions("playing state", collectIcons())

	// The view button shows its destination's icon.
	if z.viewSwitchBtn != nil {
		z.viewSwitchBtn.Icon = string(IconRows)
		assertNoIconCollisions("EQ-mode view button", collectIcons())
	}
}

// indexOf returns the first byte index of sub in s, or -1.
func indexOf(s, sub string) int {
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
