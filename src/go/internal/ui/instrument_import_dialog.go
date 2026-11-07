package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Esc, outside tap and × all mean Keep both, the safe default. onChoice fires exactly once.
func (dv *DrumView) openImportCollisionDialog(_ []ImportCollision, onChoice func(CollisionChoice)) {
	if dv.portal() == nil {
		onChoice(CollisionKeepBoth)
		return
	}
	dlg := newConfirmDialog(importCollisionConfig(onChoice))
	dv.portal().Open(dlg.portalEntry(func() image.Rectangle { return dv.Bounds }))
}

// Separate so the choice mapping is testable without the portal.
func importCollisionConfig(onChoice func(CollisionChoice)) confirmConfig {
	return confirmConfig{
		id:          "instrument-import-collision",
		titleKey:    i18n.KeyImportCollisionTitle,
		confirmKey:  i18n.KeyCapReplace,
		cancelKey:   i18n.KeyCapKeepBoth,
		destructive: true, // Replace overwrites an existing instrument
		onConfirm:   func() { onChoice(CollisionReplace) },
		onCancel:    func() { onChoice(CollisionKeepBoth) },
	}
}
