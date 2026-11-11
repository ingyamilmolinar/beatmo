package ui

import (
	"fmt"
	"image"
)

// NativeChannel identifies which browser-gesture registry a native rect targets.
type NativeChannel int

const (
	NativeFilePicker   NativeChannel = iota // OS file dialog on touchend
	NativeSoftKeyboard                      // focusable soft-keyboard rect (desktop browser)
	NativeTextInput                         // mobile native <input> overlay
)

// NativeIntent describes the browser gesture to arm at a rect.
type NativeIntent struct {
	Channel   NativeChannel
	ID        string
	Accept    string // file picker
	InputMode string // "numeric" | "text"
	Text      string // native text input initial value
	MaxLen    int    // native text input max length
	// InputRect: trigger/input split for NativeTextInput. Zero → the input
	// appears at the (gated) trigger rect. Non-zero → the trigger rect is what
	// the user taps, InputRect is where the <input> is placed (rename glue).
	InputRect image.Rectangle
}

// NativeRectCandidate is a producer's pre-gating output. Rect is the TRIGGER
// rect (what the user taps, the thing gated by z-order). Owner is the ownerID
// TopmostOwnerAt(center) must return for the candidate to survive gating.
type NativeRectCandidate struct {
	Owner  string
	Rect   image.Rectangle
	Intent NativeIntent
}

// NativeRect is a gated, screen-space rect synced to the JS registries.
type NativeRect struct {
	Rect   image.Rectangle
	Intent NativeIntent
}

// collectNativeRectCandidates returns the native-gesture rects the current UI
// state wants armed, PRE-gating. Pure function of current state; no JS side
// effects. Producers are filled in by later tasks.
func (dv *DrumView) collectNativeRectCandidates() []NativeRectCandidate {
	if dv == nil {
		return nil
	}
	var out []NativeRectCandidate
	out = append(out, dv.filePickerCandidates()...)
	out = append(out, dv.softKeyboardCandidates()...)
	out = append(out, dv.nativeInputCandidates()...)
	return out
}

// syncNativeGestures is the single per-frame native-rect projection: collect
// candidates, keep only those whose trigger rect resolves (via the composed
// dispatch precedence) to their own owner, diff against last frame, and sync the
// delta to the JS registries. Called once from (*DrumView).Update after the
// RootTree has ticked and dispatched (indexes final).
func (dv *DrumView) syncNativeGestures() {
	if dv == nil || dv.rootTree == nil {
		return
	}
	dv.textFields.advance() // one tick per native projection (TextField liveness)
	cands := dv.collectNativeRectCandidates()
	kept := make([]NativeRect, 0, len(cands))
	for _, c := range cands {
		cx := (c.Rect.Min.X + c.Rect.Max.X) / 2
		cy := (c.Rect.Min.Y + c.Rect.Max.Y) / 2
		if dv.rootTree.TopmostOwnerAt(cx, cy) == c.Owner {
			kept = append(kept, NativeRect{Rect: c.Rect, Intent: c.Intent})
		}
	}
	if nativeRectsEqual(kept, dv.lastNativeRects) {
		return
	}
	platformSyncNativeRects(kept)
	dv.lastNativeRects = kept
}

func nativeRectsEqual(a, b []NativeRect) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// filePickerCandidates returns the Upload/Import file-picker triggers, owner
// "overflow-menu", ONLY when the overflow menu is open on the File page
// (overflowPage 0). On the Templates page (or closed) it returns nil, so the
// tree-owned sync clears the JS registry and no rect can leak under a template
// row. The close-button crop is kept here (menu-internal; the menu is a single
// hit area, so it is not derivable from z-order).
func (dv *DrumView) filePickerCandidates() []NativeRectCandidate {
	if dv == nil || !Profile().IsMobile() || !dv.IsOverflowMenuOpen() || dv.overflowPage != 0 {
		return nil
	}
	popupRect := dv.overflowPopupRect()
	if popupRect.Empty() {
		return nil
	}
	dv.configureOverflowScroll()
	offset := 0
	if dv.overflowMenuScroll != nil {
		offset = dv.overflowMenuScroll.OffsetPx()
	}
	rowH := touchMinTargetPx
	accept := map[string]string{"Upload": ".wav", "Import": "application/json,.json"}
	idByLabel := map[string]string{"Upload": "upload", "Import": "import"}
	closeR := closeButtonRect(popupRect, SpaceXS)

	var out []NativeRectCandidate
	for i, item := range dv.overflowItems() {
		fileID, ok := idByLabel[item.label]
		if !ok {
			continue
		}
		y0 := popupRect.Min.Y + i*rowH - offset
		r := insetRect(image.Rect(popupRect.Min.X, y0, popupRect.Max.X, y0+rowH), SpaceXS)
		if r.Max.Y > closeR.Min.Y && r.Min.Y < closeR.Max.Y {
			if r.Max.X > closeR.Min.X {
				r.Max.X = closeR.Min.X - SpaceSM
			}
		}
		out = append(out, NativeRectCandidate{
			Owner:  "overflow-menu",
			Rect:   r,
			Intent: NativeIntent{Channel: NativeFilePicker, ID: fileID, Accept: accept[item.label]},
		})
	}
	return out
}

// softKeyboardCandidates returns focusable soft-keyboard rects for DESKTOP
// browser only (on mobile the native-input channel handles text): every live
// TextField (textfield.go).
func (dv *DrumView) softKeyboardCandidates() []NativeRectCandidate {
	if dv == nil || Profile().IsMobile() {
		return nil
	}
	return dv.textFieldSoftKeyboardCandidates()
}

// nativeInputCandidates returns mobile native <input> triggers: the row
// context menu's Rename trigger→label glue (a button, not a text box) and every
// live TextField (textfield.go) — text boxes are never listed here by hand.
func (dv *DrumView) nativeInputCandidates() []NativeRectCandidate {
	if dv == nil || !Profile().IsMobile() {
		return nil
	}
	var out []NativeRectCandidate
	if dv.IsContextMenuOpen() && dv.contextMenuRow >= 0 &&
		dv.contextMenuRow < len(dv.Rows) && dv.contextMenuRow < len(dv.rowLabels()) {
		if renameBtn := dv.contextMenuRenameBtn(); renameBtn != nil {
			trigR := renameBtn.Rect()
			labelR := dv.rowLabels()[dv.contextMenuRow].Rect()
			if !trigR.Empty() && !labelR.Empty() {
				out = append(out, NativeRectCandidate{Owner: "context-menu", Rect: trigR,
					Intent: NativeIntent{Channel: NativeTextInput,
						ID:   fmt.Sprintf("rename-%d", dv.contextMenuRow),
						Text: dv.Rows[dv.contextMenuRow].Name, MaxLen: 32, InputMode: "text",
						InputRect: labelR}})
			}
		}
	}
	out = append(out, dv.textFieldNativeInputCandidates()...)
	return out
}
