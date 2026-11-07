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
	// Docked tool panels leave it off: the user works around them.
	Scrim bool
	// Display-only (tooltips): never closes on an outside press or makes the grid stand down. Every
	// interactive overlay leaves it false.
	Passive bool
	Anchor  image.Rectangle
	OnClose func() // called when the portal removes this entry
}

type PortalUpdater interface {
	Update()
}

// The topmost entry renders last; modal overlays restrict input to themselves.
type OverlayPortal struct {
	stack        []PortalEntry
	screenBounds image.Rectangle
	hitIndex     *HitIndex // shared with DrumViewTree
}

// NewOverlayPortal creates a portal with a reference to the shared HitIndex.
func NewOverlayPortal(hitIndex *HitIndex) *OverlayPortal {
	return &OverlayPortal{hitIndex: hitIndex}
}

// Nil-safe.
func (p *OverlayPortal) Open(entry PortalEntry) {
	if p == nil {
		return
	}
	// Remove existing entry with same ID to avoid duplicates.
	if p.closeByID(entry.ID) {
		emitPopupClosed(entry.ID, "replaced")
	}

	p.stack = append(p.stack, entry)
	emitPopupOpened(entry.ID)

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

// Close removes a specific overlay by ID. Nil-safe.
func (p *OverlayPortal) Close(id string) {
	if p == nil {
		return
	}
	removed := p.closeByID(id)
	if removed {
		emitPopupClosed(id, "explicit")
	}
	p.hitIndex.RemovePortal(id)
	p.updateModalState()
}

// Nil-safe.
func (p *OverlayPortal) CloseTop() bool {
	if p == nil || len(p.stack) == 0 {
		return false
	}
	top := p.stack[len(p.stack)-1]
	p.stack = p.stack[:len(p.stack)-1]
	emitPopupClosed(top.ID, "top")
	if top.OnClose != nil {
		top.OnClose()
	}
	p.hitIndex.RemovePortal(top.ID)
	p.updateModalState()
	return true
}

// For mid-frame geometry changes, so the hit index stays in sync.
func (p *OverlayPortal) RefreshEntry(id string) {
	if p == nil {
		return
	}
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

// OverlayByID returns the overlay registered under id, or nil. Nil-safe.
func (p *OverlayPortal) OverlayByID(id string) PortalOverlay {
	if p == nil {
		return nil
	}
	for _, e := range p.stack {
		if e.ID == id {
			return e.Overlay
		}
	}
	return nil
}

// Nil-safe.
func (p *OverlayPortal) Has(id string) bool {
	if p == nil {
		return false
	}
	for _, e := range p.stack {
		if e.ID == id {
			return true
		}
	}
	return false
}

// IsOpen returns true if any overlay is in the stack. Nil-safe.
func (p *OverlayPortal) IsOpen() bool {
	return p != nil && len(p.stack) > 0
}

// HasModal returns true if any overlay in the stack is modal. Nil-safe.
func (p *OverlayPortal) HasModal() bool {
	if p == nil {
		return false
	}
	for _, e := range p.stack {
		if e.Modal {
			return true
		}
	}
	return false
}

// Passive and scrim-less docked panels are not blocking: the user works around them.
func (p *OverlayPortal) HasBlocking() bool {
	if p == nil {
		return false
	}
	for _, e := range p.stack {
		if e.Modal || e.Scrim {
			return true
		}
	}
	return false
}

// The single source for "an outside press must close it": every non-Passive overlay. Nil-safe.
func (p *OverlayPortal) OwnsInput() bool {
	if p == nil {
		return false
	}
	for _, e := range p.stack {
		if !e.Passive {
			return true
		}
	}
	return false
}

// TopID returns the ID of the topmost overlay, or "" if empty. Nil-safe.
func (p *OverlayPortal) TopID() string {
	if p == nil || len(p.stack) == 0 {
		return ""
	}
	return p.stack[len(p.stack)-1].ID
}

// TopOverlay returns the topmost overlay on the stack, or nil if empty. Nil-safe.
func (p *OverlayPortal) TopOverlay() PortalOverlay {
	if p == nil || len(p.stack) == 0 {
		return nil
	}
	return p.stack[len(p.stack)-1].Overlay
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

// Hit areas are refreshed every frame for layout changes; self-closed overlays are removed.
func (p *OverlayPortal) Update() {
	for _, e := range p.stack {
		if u, ok := e.Overlay.(PortalUpdater); ok {
			u.Update()
		}
	}
	// Portal overlays only get press/drag/release edges, so hover needs this per-frame seam.
	if !Profile().IsMobile() {
		mx, my := cursorPosition()
		for _, e := range p.stack {
			if hv, ok := e.Overlay.(portalHoverer); ok {
				hv.HoverAt(mx, my)
			}
		}
	}
	// InputBounds can change after input, and stale areas miss clicks.
	for si, e := range p.stack {
		areas := e.Overlay.HitAreas()
		for i := range areas {
			areas[i].ZIndex = 300 + si
		}
		p.hitIndex.UpdatePortal(e.ID, areas)
	}
}

// Desktop only; no press semantics, no capture.
type portalHoverer interface{ HoverAt(mx, my int) }

// After input processing, so self-closing components disappear the same frame.
func (p *OverlayPortal) CleanupClosed() {
	changed := false
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].Overlay.ShouldClose() {
			id := p.stack[i].ID
			onClose := p.stack[i].OnClose
			// Already captured, so closeByID must not invoke it twice.
			p.stack[i].OnClose = nil
			p.closeByID(id)
			p.hitIndex.RemovePortal(id)
			emitPopupClosed(id, "dismiss")
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

// One scrim under the topmost scrim-bearing entry, so stacked popups never double-dim.
func (p *OverlayPortal) Draw(screen *ebiten.Image) {
	scrimIdx := -1
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].Scrim {
			scrimIdx = i
			break
		}
	}
	for i, e := range p.stack {
		if i == scrimIdx {
			drawRect(screen, p.screenBounds, colScrim, true)
		}
		e.Overlay.Draw(screen)
	}
}

// StackLen returns the number of overlays in the stack. Nil-safe.
func (p *OverlayPortal) StackLen() int {
	if p == nil {
		return 0
	}
	return len(p.stack)
}

// Does not update the hit index.
func (p *OverlayPortal) closeByID(id string) bool {
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
	removed := n < len(p.stack)
	p.stack = p.stack[:n]
	return removed
}

func (p *OverlayPortal) updateModalState() {
	for i := len(p.stack) - 1; i >= 0; i-- {
		if p.stack[i].Modal {
			p.hitIndex.SetModal(p.stack[i].ID)
			return
		}
	}
	p.hitIndex.ClearModal()
}
