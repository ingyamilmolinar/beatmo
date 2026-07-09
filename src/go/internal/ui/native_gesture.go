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
	cands := dv.collectNativeRectCandidates()
	kept := make([]NativeRect, 0, len(cands))
	for _, c := range cands {
		cx := (c.Rect.Min.X + c.Rect.Max.X) / 2
		cy := (c.Rect.Min.Y + c.Rect.Max.Y) / 2
		if dv.rootTree.TopmostOwnerAt(cx, cy) == c.Owner {
			kept = append(kept, NativeRect{Rect: c.Rect, Intent: c.Intent})
		}
	}
	// Documented exception: the shared numeric ParamValueEditor (synth-param /
	// sampler-param / eq-db-N) is NOT a portal, so it has no ownerID to gate on;
	// it arms its own mobile <input> imperatively in ParamValueEditor.OpenValue.
	// Because platformSyncNativeRects clears ALL mobile-input rects before
	// re-arming, an active editor's rect must be re-added here every frame or the
	// clear-cycle would wipe it (mobile param-entry regression). It is armed
	// unconditionally while active (self-isolated by the Go tree as the focused
	// editor). See TestParamEditorNativeRectArmedBySync and the discipline
	// test's param_value_editor.go exemption.
	kept = append(kept, dv.activeParamEditorNativeRects()...)
	if nativeRectsEqual(kept, dv.lastNativeRects) {
		return
	}
	platformSyncNativeRects(kept)
	dv.lastNativeRects = kept
}

// activeParamEditorNativeRects returns the mobile <input> rect(s) for any
// currently-open ParamValueEditor that owns a mobile input id (synth/sampler via
// dv.paramEditor, EQ-dB via eqPanelZone.paramEditor). The transport (BPM) editor
// is intentionally excluded — the BPM box is already armed by
// nativeInputCandidates under id "bpm", and including the editor would duplicate
// that id. Empty on desktop. This is the pragmatic exception to the tree-owned
// projection (the editor is not a portal); it keeps the editor's rect alive
// across the seam's clear-and-re-arm cycle.
func (dv *DrumView) activeParamEditorNativeRects() []NativeRect {
	if dv == nil || !Profile().IsMobile() {
		return nil
	}
	editors := []*ParamValueEditor{dv.paramEditor}
	if dv.eqPanelZone != nil {
		editors = append(editors, dv.eqPanelZone.paramEditor)
	}
	var out []NativeRect
	for _, e := range editors {
		if e == nil || !e.active || e.mobileID == "" || e.ti == nil || e.ti.Rect.Empty() {
			continue
		}
		out = append(out, NativeRect{
			Rect: e.ti.Rect,
			Intent: NativeIntent{
				Channel:   NativeTextInput,
				ID:        e.mobileID,
				Text:      e.ti.Value(),
				MaxLen:    e.ti.MaxLen,
				InputMode: "numeric",
			},
		})
	}
	return out
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
// browser only (on mobile the native-input channel handles text). Mirrors the
// former drumview_layout.go registration block, page/open-state gates intact.
func (dv *DrumView) softKeyboardCandidates() []NativeRectCandidate {
	if dv == nil || Profile().IsMobile() {
		return nil
	}
	var out []NativeRectCandidate
	add := func(owner, id, mode string, r image.Rectangle) {
		if r.Empty() {
			return
		}
		out = append(out, NativeRectCandidate{Owner: owner, Rect: r,
			Intent: NativeIntent{Channel: NativeSoftKeyboard, ID: id, InputMode: mode}})
	}
	if dv.bpmBox() != nil {
		add("transport", "bpm", "numeric", dv.bpmBox().Rect)
	}
	if dv.IsInstMenuOpen() && dv.instSearchBox != nil {
		add("inst-menu", instSearchMobileInputID, "text", dv.instSearchRect)
	}
	if dv.IsNamingOpen() && dv.nameBox != nil {
		add("naming", "wav-name", "text", dv.nameBox.Rect)
	}
	if dv.renameBox != nil {
		add("rename", "rename", "text", dv.renameBox.Rect)
	}
	return out
}

// nativeInputCandidates returns mobile native <input> triggers. Mirrors the
// former drumview_layout.go mobile block: BPM direct rect, rename trigger→label
// glue, inst-search, wav-name. Gates + open-state guards preserved.
func (dv *DrumView) nativeInputCandidates() []NativeRectCandidate {
	if dv == nil || !Profile().IsMobile() {
		return nil
	}
	var out []NativeRectCandidate
	if dv.bpmBox() != nil && !dv.bpmBox().Rect.Empty() {
		out = append(out, NativeRectCandidate{Owner: "transport", Rect: dv.bpmBox().Rect,
			Intent: NativeIntent{Channel: NativeTextInput, ID: "bpm", Text: dv.bpmBox().Text, MaxLen: 4, InputMode: "numeric"}})
	}
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
	if dv.IsInstMenuOpen() && dv.instSearchBox != nil && !dv.instSearchRect.Empty() {
		out = append(out, NativeRectCandidate{Owner: "inst-menu", Rect: dv.instSearchRect,
			Intent: NativeIntent{Channel: NativeTextInput, ID: instSearchMobileInputID, Text: dv.instSearchBox.Text, MaxLen: 40, InputMode: "text"}})
	}
	if dv.IsNamingOpen() && dv.nameBox != nil && !dv.nameBox.Rect.Empty() {
		out = append(out, NativeRectCandidate{Owner: "naming", Rect: dv.nameBox.Rect,
			Intent: NativeIntent{Channel: NativeTextInput, ID: "wav-name", Text: dv.nameBox.Text, MaxLen: 32, InputMode: "text"}})
	}
	return out
}
