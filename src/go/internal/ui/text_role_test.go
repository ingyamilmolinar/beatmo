package ui

import "testing"

// TestTextRoleHierarchy proves the role API exposes a real, distinct type
// scale (panel-title > section-header > body > caption) in every build mode —
// this is what kills the "everything is one chunky size" look.
func TestTextRoleHierarchy(t *testing.T) {
	pt := StyledTextHeight(RolePanelTitle)
	sh := StyledTextHeight(RoleSectionHeader)
	bd := StyledTextHeight(RoleBody)
	cap := StyledTextHeight(RoleCaption)
	if !(pt > sh && sh > bd && bd > cap) {
		t.Fatalf("role heights must strictly decrease: panelTitle=%d sectionHeader=%d body=%d caption=%d", pt, sh, bd, cap)
	}
	if cap < 10 {
		t.Fatalf("caption height %d too small to read", cap)
	}
}

// TestStyledTextWidthScalesWithLength is a sanity check that width grows with
// string length (guards against a constant/0 fallback regression).
func TestStyledTextWidthScalesWithLength(t *testing.T) {
	short := StyledTextWidth("Hi", RoleBody)
	long := StyledTextWidth("Hello world", RoleBody)
	if long <= short {
		t.Fatalf("expected longer string wider: short=%d long=%d", short, long)
	}
}
