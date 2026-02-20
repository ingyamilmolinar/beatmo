package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// PortalOverlay is the interface for overlays rendered through the OverlayPortal.
type PortalOverlay interface {
	Layout(anchor, screenBounds image.Rectangle)
	HitAreas() []HitArea
	Draw(screen *ebiten.Image)
	ShouldClose() bool
}

// PortalEntry represents a single overlay in the portal stack.
type PortalEntry struct {
	ID      string
	Owner   Zone
	Overlay PortalOverlay
	Modal   bool
	Anchor  image.Rectangle
	OnClose func() // called when the portal removes this entry
}

// PortalUpdater is an optional interface for overlays that need per-frame
// updates (e.g., momentum, animation). If a PortalOverlay also implements
// PortalUpdater, OverlayPortal.Update() calls it each frame.
type PortalUpdater interface {
	Update()
}

// OverlayPortal manages a stack of overlay entries. The topmost entry
// renders last (on top). Modal overlays restrict input to themselves.
type OverlayPortal struct {
	stack        []PortalEntry
	screenBounds image.Rectangle
	hitIndex     *HitIndex // shared with DrumViewTree
}

// NewOverlayPortal creates a portal with a reference to the shared HitIndex.
func NewOverlayPortal(hitIndex *HitIndex) *OverlayPortal {
	return &OverlayPortal{hitIndex: hitIndex}
}

// Open pushes an overlay onto the stack. If the overlay is modal, it
// becomes the modal filter in the HitIndex.
func (p *OverlayPortal) Open(entry PortalEntry) {
	// Remove existing entry with same ID to avoid duplicates.
	p.closeByID(entry.ID)

	p.stack = append(p.stack, entry)

	// Layout the overlay.
	entry.Overlay.Layout(entry.Anchor, p.screenBounds)

	// Register hit areas with z-index offset by stack position.
	areas := entry.Overlay.HitAreas()
	for i := range areas {
		areas[i].ZIndex = 300 + len(p.stack) - 1
	}
	p.hitIndex.UpdatePortal(entry.ID, areas)

	// If modal, set the modal filter.
	if entry.Modal {
		p.hitIndex.SetModal(entry.ID)
	}
}

// Close removes a specific overlay by ID.
func (p *OverlayPortal) Close(id string) {
	p.closeByID(id)
	p.hitIndex.RemovePortal(id)
	p.updateModalState()
}

// CloseTop closes the topmost overlay and returns true. Returns false if empty.
func (p *OverlayPortal) CloseTop() bool {
	if len(p.stack) == 0 {
		return false
	}
	top := p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	if top.OnClose != nil {
		top.OnClose()
	}
	p.hitIndex.RemovePortal(top.ID)
	p.updateModalState()
	return true
}

// RefreshEntry refreshes a single portal entry's hit areas immediately.
// Use this when the overlay's geometry changes mid-frame (e.g., after
// buildFXPanel() recomputes fxPanelRect) so the hit index stays in sync
// without waiting for the next frame's Update().
func (p *OverlayPortal) RefreshEntry(id string) {
	for si, e := range p.stack {
		if e.ID == id {
			areas := e.Overlay.HitAreas()
			for i := range areas {
				areas[i].ZIndex = 300 + si
			}
			p.hitIndex.UpdatePortal(e.ID, areas)
			return
		}
	}
}

// Has returns true if an overlay with the given ID is in the stack.
func (p *OverlayPortal) Has(id string) bool {
	for _, e := range p.stack {
		if e.ID == id {
			return true
		}
	}
	return false
}

// IsOpen returns true if any overlay is in the stack.
func (p *OverlayPortal) IsOpen() bool {
	return len(p.stack) > 0
}

// HasModal returns true if any overlay in the stack is modal.
func (p *OverlayPortal) HasModal() bool {
	for _, e := range p.stack {
		if e.Modal {
			return true
		}
	}
	return false
}

// TopID returns the ID of the topmost overlay, or "" if empty.
func (p *OverlayPortal) TopID() string {
	if len(p.stack) == 0 {
		return ""
	}
	return p.stack[len(p.stack)-1].ID
}

// SetScreenBounds updates the screen bounds used for overlay layout.
func (p *OverlayPortal) SetScreenBounds(r image.Rectangle) {
	p.screenBounds = r
}

// Layout re-layouts all overlays (called when screen bounds change).
func (p *OverlayPortal) Layout() {
	for si, e := range p.stack {
		e.Overlay.Layout(e.Anchor, p.screenBounds)
		areas := e.Overlay.HitAreas()
		for i := range areas {
			areas[i].ZIndex = 300 + si
		}
		p.hitIndex.UpdatePortal(e.ID, areas)
	}
}

// Update calls PortalUpdater.Update() on all overlays that implement it,
// then refreshes hit areas for all overlays to reflect any layout changes
// (e.g., component switching from categories to instruments mode).
// Overlays that self-close (ShouldClose() returns true) are removed.
// Called once per frame from DrumViewTree.Update().
func (p *OverlayPortal) Update() {
	for _, e := range p.stack {
		if u, ok := e.Overlay.(PortalUpdater); ok {
			u.Update()
		}
	}
	// Refresh hit areas for all overlays. Components may change their
	// InputBounds after processing input (e.g., category navigation),
	// and stale hit areas cause subsequent clicks to miss.
	for si, e := range p.stack {
		areas := e.Overlay.HitAreas()
		for i := range areas {
			areas[i].ZIndex = 300 + si
		}
		p.hitIndex.UpdatePortal(e.ID, areas)
	}
}

// CleanupClosed removes overlays whose ShouldClose() returns true.
// Called after input processing so that components which close themselves
// during handleInput() (e.g., subdiv menu button fires OnSelect → comp.Close())
// have their portal entries removed in the same frame.
func (p *OverlayPortal) CleanupClosed() {
	changed := false
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].Overlay.ShouldClose() {
			id := p.stack[i].ID
			onClose := p.stack[i].OnClose
			// closeByID would also invoke OnClose, but we already captured
			// it to avoid double-invoke — pass a copy with OnClose=nil.
			p.stack[i].OnClose = nil
			p.closeByID(id)
			p.hitIndex.RemovePortal(id)
			if onClose != nil {
				onClose()
			}
			changed = true
		}
	}
	if changed {
		p.updateModalState()
	}
}

// Draw renders all overlays in stack order (bottom to top).
func (p *OverlayPortal) Draw(screen *ebiten.Image) {
	for _, e := range p.stack {
		e.Overlay.Draw(screen)
	}
}

// StackLen returns the number of overlays in the stack.
func (p *OverlayPortal) StackLen() int {
	return len(p.stack)
}

// closeByID removes an overlay by ID without updating the hit index.
func (p *OverlayPortal) closeByID(id string) {
	n := 0
	for _, e := range p.stack {
		if e.ID != id {
			p.stack[n] = e
			n++
		} else if e.OnClose != nil {
			e.OnClose()
		}
	}
	// Clear trailing references.
	for i := n; i < len(p.stack); i++ {
		p.stack[i] = PortalEntry{}
	}
	p.stack = p.stack[:n]
}

// updateModalState sets the HitIndex modal filter to the topmost modal
// overlay, or clears it if no modals remain.
func (p *OverlayPortal) updateModalState() {
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].Modal {
			p.hitIndex.SetModal(p.stack[i].ID)
			return
		}
	}
	p.hitIndex.ClearModal()
}
