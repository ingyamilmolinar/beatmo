//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestBottomNavRelabelsOnLocaleSwitch guards live re-localization of the mobile
// bottom-nav SegmentedControl, which copies its labels at construction. A
// runtime SetLocale → OnLocaleChanged must push fresh-locale labels into it
// (zone Invalidate does not reach a cached SegmentedControl).
func TestBottomNavRelabelsOnLocaleSwitch(t *testing.T) {
	defer i18n.SetLocale(i18n.LocaleEN)
	i18n.SetLocale(i18n.LocaleEN)

	dv := newTestDrumView(t, 390, 780)
	if dv.viewSwitchSegmented == nil {
		t.Fatal("no bottom-nav segmented control")
	}

	// English baseline: the Levels short label is "Lvl".
	enLabels := dv.viewSwitchSegmented.Labels()
	if !containsStr(enLabels, "Lvl") {
		t.Fatalf("EN bottom-nav labels missing 'Lvl': %v", enLabels)
	}

	// Switch locale and re-localize.
	i18n.SetLocale(i18n.LocaleES)
	dv.OnLocaleChanged()

	esLabels := dv.viewSwitchSegmented.Labels()
	if !containsStr(esLabels, "Niv") {
		t.Fatalf("after ES switch, bottom-nav labels missing Spanish 'Niv': %v", esLabels)
	}
	if containsStr(esLabels, "Lvl") {
		t.Fatalf("bottom-nav still shows English 'Lvl' after ES switch: %v", esLabels)
	}
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
