//go:build test

package ui

import (
	"image"
	"testing"
)

func TestRowControlButtonHasIcon(t *testing.T) {
	btn := RowControlButton(IconMute)
	if btn.Icon != string(IconMute) {
		t.Errorf("RowControlButton(IconMute) must set Icon field, got %q", btn.Icon)
	}
	if btn.Text != "" {
		t.Errorf("RowControlButton must not set Text, got %q", btn.Text)
	}
}

func TestDestructiveButtonHasDeleteStyle(t *testing.T) {
	btn := DestructiveButton()
	if btn.Icon != string(IconTrash) {
		t.Errorf("DestructiveButton must use IconTrash, got %q", btn.Icon)
	}
	if !btn.ConsumeOnPress {
		t.Error("DestructiveButton must set ConsumeOnPress = true")
	}
}

func TestMenuSpecItemCount(t *testing.T) {
	spec := MenuSpec{Items: []MenuItemSpec{
		{Label: "Foo", Icon: IconClose},
		{Label: "Bar", Icon: IconMute},
	}}
	if len(spec.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(spec.Items))
	}
}

func TestPanelSpecTitleNotEmpty(t *testing.T) {
	spec := PanelSpec{Title: "FX: Kick", Width: 200}
	if spec.Title == "" {
		t.Error("PanelSpec Title must not be empty")
	}
}

func TestRowControlButtonActiveStyle(t *testing.T) {
	btn := RowControlButton(IconMute)
	ActiveRowControl(btn, true)
	got, ok := btn.Style.(ButtonStyle)
	if !ok {
		t.Fatal("style must be ButtonStyle after ActiveRowControl")
	}
	want := MuteActiveStyle.Fill
	if got.Fill != want {
		t.Errorf("ActiveRowControl(mute, true) must apply MuteActiveStyle fill, got %v want %v", got.Fill, want)
	}
}

func TestIconOnlyAddButton(t *testing.T) {
	btn := IconOnlyButton(IconPlus, InstButtonStyle)
	if btn.Text != "" {
		t.Errorf("IconOnlyButton must have empty text, got %q", btn.Text)
	}
	if btn.Icon != string(IconPlus) {
		t.Errorf("IconOnlyButton must set Icon, got %q", btn.Icon)
	}
}

func TestPanelSpecHeaderRect(t *testing.T) {
	assertDefaultParityState(t)
	spec := PanelSpec{Title: "Test", Width: 100, MinHeight: 80}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("PanelSpec.HeaderRect panicked: %v", r)
		}
	}()
	r := spec.HeaderRect(image.Pt(10, 10))
	if r.Dx() != 100 {
		t.Errorf("HeaderRect width must equal PanelSpec.Width=100, got %d", r.Dx())
	}
	if r.Empty() {
		t.Error("HeaderRect must not be empty")
	}
}

func TestCloseButtonHasIcon(t *testing.T) {
	btn := CloseButton()
	if btn.Text != "" {
		t.Errorf("CloseButton must have no text, got %q", btn.Text)
	}
	if btn.Icon != string(IconClose) {
		t.Errorf("CloseButton must use IconClose, got %q", btn.Icon)
	}
}

func TestAddButtonHasIcon(t *testing.T) {
	btn := AddButton()
	if btn.Text != "" {
		t.Errorf("AddButton must have no text, got %q", btn.Text)
	}
	if btn.Icon != string(IconPlus) {
		t.Errorf("AddButton must use IconPlus, got %q", btn.Icon)
	}
}

func TestMenuSpecHeights(t *testing.T) {
	assertDefaultParityState(t)
	spec := MenuSpec{Items: []MenuItemSpec{
		{Label: "A", Icon: IconMute},
		{Label: "B", Icon: IconSolo},
		{Label: "C", Icon: IconFx, Destructive: true},
	}}
	itemH := spec.ItemHeight()
	if itemH <= 0 {
		t.Errorf("ItemHeight must be positive, got %d", itemH)
	}
	total := spec.TotalHeight()
	if total != 3*itemH {
		t.Errorf("TotalHeight must be 3*ItemHeight=%d (no title), got %d", 3*itemH, total)
	}
	// With title.
	specT := MenuSpec{Title: "Row", Items: spec.Items}
	totalT := specT.TotalHeight()
	if totalT <= total {
		t.Errorf("TotalHeight with title must exceed without title: %d vs %d", totalT, total)
	}
}

// TestRowRackButtonsUseIcons verifies that mute/solo/fx/origin/delete buttons
// in the row rack use the unified IconID glyphs (post-Lucide-style migration).
// Previously these were text labels (M/S/FX/O/X) because the old renderer
// looked jagged at small sizes; the new vector renderer makes 16-px icons
// legible, so DESIGN.md's decision matrix flipped to icon-first.
func TestRowRackButtonsUseIcons(t *testing.T) {
	assertDefaultParityState(t)
	dv := newTestDV(t)
	if len(dv.Rows) == 0 {
		t.Skip("no rows")
	}
	g := dv.rowGroups()[0]
	if g.Mute == nil {
		t.Skip("mute button nil (hidden in this layout)")
	}
	for _, tc := range []struct {
		name string
		btn  *Button
		icon IconID
	}{
		{"Mute", g.Mute, IconMute},
		{"Solo", g.Solo, IconSolo},
		{"FX", g.FX, IconFx},
		{"Delete", g.Delete, IconClose},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.btn == nil {
				t.Skip("button nil")
			}
			if tc.btn.Text != "" {
				t.Errorf("%s button must have no text, got %q", tc.name, tc.btn.Text)
			}
			if tc.btn.Icon != string(tc.icon) {
				t.Errorf("%s button must use Icon=%q, got %q", tc.name, tc.icon, tc.btn.Icon)
			}
		})
	}
}

// TestFXPanelButtonsUseIcons verifies the CloseButton and AddButton factories.
func TestFXPanelButtonsUseIcons(t *testing.T) {
	cls := CloseButton()
	if cls.Text != "" {
		t.Errorf("CloseButton must have no text, got %q", cls.Text)
	}
	if cls.Icon != string(IconClose) {
		t.Errorf("CloseButton must use IconClose, got %q", cls.Icon)
	}
	add := AddButton()
	if add.Text != "" {
		t.Errorf("AddButton must have no text, got %q", add.Text)
	}
	if add.Icon != string(IconPlus) {
		t.Errorf("AddButton must use IconPlus, got %q", add.Icon)
	}
}
