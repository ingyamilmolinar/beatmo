package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// openImportCollisionDialog shows the shared confirm modal for an import that
// would collide with an existing user instrument, offering Replace (overwrite,
// destructive) vs Keep both (import as a fresh copy). It reuses the standard
// ConfirmDialog infra — Replace = the primary/destructive confirm, Keep both =
// the second choice; Esc / click-outside / close-X map to Keep both (the safe
// non-destructive default). onChoice fires exactly once with the user's pick.
func (dv *DrumView) openImportCollisionDialog(_ []ImportCollision, onChoice func(CollisionChoice)) {
	if dv.portal() == nil {
		onChoice(CollisionKeepBoth)
		return
	}
	dlg := newConfirmDialog(importCollisionConfig(onChoice))
	dv.portal().Open(dlg.portalEntry(func() image.Rectangle { return dv.Bounds }))
}

// importCollisionConfig builds the confirm-dialog config for an import
// collision: Replace = the destructive confirm, Keep both = the second choice.
// Extracted so the choice mapping is unit-testable without the portal.
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
