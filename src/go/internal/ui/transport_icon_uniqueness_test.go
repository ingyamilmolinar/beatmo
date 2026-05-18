//go:build test

package ui

import (
	"image"
	"testing"
)

// TestMobileTransportIconsAreDistinct ensures every visible mobile-transport
// button carries an icon distinct from every other visible transport
// button. The constraint matters because two buttons with the same icon
// in the same toolbar are indistinguishable to the user; the bug shape
// is silent — the toolbar still lays out, but the user can't tell two
// adjacent controls apart.
//
// Dynamic single-button icons (Play↔Pause, Speaker↔SpeakerOff,
// Audio↔Rows for the view switch) are exempt: at any point in time,
// only one variant is shown. Each variant must still be unique versus
// the rest of the toolbar.
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
		if z.overflowBtn != nil {
			add("overflow", z.overflowBtn.Icon)
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

	// Flip the play button icon to Pause (simulates running playback) and
	// re-check. Done by overwriting the field, mirroring the production
	// site at transport_zone.go:1146 where the icon is selected per state.
	z.playBtn.Icon = string(IconPause)
	assertNoIconCollisions("playing state", collectIcons())

	// The view button uses the destination-icon convention. Flip from
	// IconAudio (Rows mode) to IconRows (EQ mode) and confirm uniqueness
	// holds in both states.
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
