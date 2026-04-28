package ui

import "image"

// ui_style.go — One-call constructors for the four button categories
// (Primary, Secondary, RowControl, Destructive) and overlay specs
// (PanelSpec, MenuSpec). Follow DESIGN.md §4–7.

// ── Button constructors ───────────────────────────────────────────────────

// IconOnlyButton creates a Button with no text label, only an icon.
// Use for any slot where DESIGN.md §5 specifies icon-mandatory.
func IconOnlyButton(id IconID, style ButtonVisual) *Button {
	b := NewButton("", style, nil)
	b.Icon = string(id)
	b.IconColor = colTextSecondary
	return b
}

// RowControlButton creates a row-control button (mute/solo/fx/origin/delete)
// using InstButtonStyle and the given icon. The caller wires OnClick.
func RowControlButton(id IconID) *Button {
	b := IconOnlyButton(id, InstButtonStyle)
	b.IconColor = colTextSecondary
	return b
}

// ActiveRowControl updates a row-control button's style to reflect its active
// state. Call whenever mute/solo/fx state changes before drawing.
// id must match the icon passed to RowControlButton.
func ActiveRowControl(b *Button, active bool) {
	if b == nil {
		return
	}
	if !active {
		b.Style = InstButtonStyle
		b.IconColor = colTextSecondary
		return
	}
	switch IconID(b.Icon) {
	case IconMute:
		b.Style = MuteActiveStyle
		b.IconColor = colTextPrimary
	case IconSolo:
		b.Style = SoloActiveStyle
		b.IconColor = colAccentBright
	case IconFx:
		b.Style = FXActiveStyle
		b.IconColor = colAccent
	case IconTarget:
		b.Style = ButtonStyle{Fill: colAccentSubtle, Border: colAccent}
		b.IconColor = colAccent
	default:
		b.Style = InstButtonStyle
		b.IconColor = colTextPrimary
	}
}

// DestructiveButton creates a delete-row button: red fill, trash icon,
// ConsumeOnPress = true so rapid clicks don't cascade through layout changes.
func DestructiveButton() *Button {
	b := NewButton("", DeleteButtonStyle, nil)
	b.Icon = string(IconTrash)
	b.IconColor = colStopRed
	b.ConsumeOnPress = true
	return b
}

// AddButton creates an icon-only "+" button using IconPlus and InstButtonStyle.
// Use for "Add row", "Add Effect", etc. — never the raw "+" character.
func AddButton() *Button {
	return IconOnlyButton(IconPlus, InstButtonStyle)
}

// CloseButton creates an icon-only close (×) button.
// Pass into panel headers; the caller sets the OnClick handler.
func CloseButton() *Button {
	b := IconOnlyButton(IconClose, TransportMiscStyle)
	b.IconColor = colTextSecondary
	return b
}

// ── Panel spec ────────────────────────────────────────────────────────────

// PanelSpec describes an overlay panel geometry. Use drawPanel(dst, r) for
// the chrome. Panels follow DESIGN.md §6: rounded corners, shadow, border.
type PanelSpec struct {
	Title     string
	Width     int
	MinHeight int
}

// HeaderRect returns the title-bar rect for a panel anchored at origin.
// Height = Profile().PopupBtnH + 2*SpaceSM.
func (s PanelSpec) HeaderRect(origin image.Point) image.Rectangle {
	h := Profile().PopupBtnH + 2*SpaceSM
	return image.Rect(origin.X, origin.Y, origin.X+s.Width, origin.Y+h)
}

// ContentRect returns the area below the header for the panel body.
func (s PanelSpec) ContentRect(origin image.Point, bodyHeight int) image.Rectangle {
	hdr := s.HeaderRect(origin)
	return image.Rect(hdr.Min.X, hdr.Max.Y, hdr.Min.X+s.Width, hdr.Max.Y+bodyHeight)
}

// ── Menu spec ─────────────────────────────────────────────────────────────

// MenuItemSpec describes one item in a context menu.
type MenuItemSpec struct {
	Label       string
	Icon        IconID
	Destructive bool // true → red background (DeleteButtonStyle)
	OnClick     func()
}

// MenuSpec describes a context menu (desktop floating or mobile bottom sheet).
type MenuSpec struct {
	Title string // optional — shown as header on mobile bottom sheet
	Items []MenuItemSpec
}

// ItemHeight returns the per-item height for the current profile.
func (s MenuSpec) ItemHeight() int {
	if Profile().IsMobile() {
		return BtnHeightLG + SpaceMD // ~48 px touch target
	}
	return BtnHeightMD + SpaceSM // ~40 px desktop
}

// TotalHeight returns the full panel height: header (if Title set) + all items.
func (s MenuSpec) TotalHeight() int {
	itemsH := len(s.Items) * s.ItemHeight()
	if s.Title != "" {
		return Profile().PopupBtnH + itemsH
	}
	return itemsH
}
