package ui

import (
	"image"
	"sort"
)

// indexedHitArea pairs a HitArea with its owner for removal and filtering.
type indexedHitArea struct {
	HitArea
	ownerID  string // zone ID or overlay ID
	isPortal bool   // true for overlay entries
}

// HitIndex maintains a spatial index of all interactive areas from zones
// and portal overlays. It supports z-sorted queries, modal filtering,
// and touch target expansion.
type HitIndex struct {
	areas []indexedHitArea
	// modalID, when non-empty, restricts At() to return only areas
	// belonging to this overlay (the topmost modal).
	modalID string
}

// Update replaces all hit areas for a given zone ID.
func (idx *HitIndex) Update(zoneID string, areas []HitArea) {
	idx.removeOwner(zoneID)
	for _, a := range areas {
		idx.areas = append(idx.areas, indexedHitArea{
			HitArea:  a,
			ownerID:  zoneID,
			isPortal: false,
		})
	}
}

// UpdatePortal replaces all hit areas for a given overlay ID.
func (idx *HitIndex) UpdatePortal(overlayID string, areas []HitArea) {
	idx.removeOwner(overlayID)
	for _, a := range areas {
		idx.areas = append(idx.areas, indexedHitArea{
			HitArea:  a,
			ownerID:  overlayID,
			isPortal: true,
		})
	}
}

// RemovePortal removes all hit areas for a given overlay ID.
func (idx *HitIndex) RemovePortal(overlayID string) {
	idx.removeOwner(overlayID)
}

// SetModal sets the modal overlay ID. When non-empty, At() returns only
// areas belonging to this overlay.
func (idx *HitIndex) SetModal(overlayID string) {
	idx.modalID = overlayID
}

// ClearModal clears the modal filter.
func (idx *HitIndex) ClearModal() {
	idx.modalID = ""
}

// At returns all hit areas containing the point (x, y), sorted by z-index
// descending (highest first). When a modal is active, only the modal
// overlay's areas are returned. Touch-flagged areas are expanded by
// TouchMinTarget() before testing.
func (idx *HitIndex) At(x, y int) []indexedHitArea {
	pt := image.Pt(x, y)
	expand := TouchMinTarget()

	var result []indexedHitArea
	for _, a := range idx.areas {
		// Modal filtering: when modal is active, only return modal areas.
		if idx.modalID != "" && a.ownerID != idx.modalID {
			continue
		}

		r := a.Rect
		if a.Touch && expand > 0 {
			r = expandRect(r, expand)
			if !a.ClipRect.Empty() {
				r = r.Intersect(a.ClipRect)
			}
		}
		if pt.In(r) {
			result = append(result, a)
		}
	}

	// Sort by (1) exact-rect hits before touch-expanded-only hits,
	// (2) z-index descending within each group. This prevents expanded
	// touch targets of adjacent buttons from stealing taps from buttons
	// whose exact rect contains the point, even across z-indexes.
	sort.SliceStable(result, func(i, j int) bool {
		iExact := pt.In(result[i].Rect)
		jExact := pt.In(result[j].Rect)
		if iExact != jExact {
			return iExact
		}
		if result[i].ZIndex != result[j].ZIndex {
			return result[i].ZIndex > result[j].ZIndex
		}
		return false
	})
	return result
}

// removeOwner removes all areas with the given owner ID.
func (idx *HitIndex) removeOwner(id string) {
	n := 0
	for _, a := range idx.areas {
		if a.ownerID != id {
			idx.areas[n] = a
			n++
		}
	}
	// Clear trailing references to allow GC.
	for i := n; i < len(idx.areas); i++ {
		idx.areas[i] = indexedHitArea{}
	}
	idx.areas = idx.areas[:n]
}

// expandRect expands a rectangle by px pixels on each side for touch targets.
func expandRect(r image.Rectangle, px int) image.Rectangle {
	return image.Rect(r.Min.X-px, r.Min.Y-px, r.Max.X+px, r.Max.Y+px)
}
