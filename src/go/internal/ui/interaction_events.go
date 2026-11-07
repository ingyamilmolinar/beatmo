package ui

// Each captures its caller's source, so src= points at the widget chokepoint's caller.

import (
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// Nothing is published when label and icon are both empty.
func emitUITap(label, icon string) {
	if label == "" && icon == "" {
		return
	}
	hooks.PublishWithSource(hooks.EventUITap, hooks.UITapPayload{Label: label, Icon: icon}, hooks.CaptureSource(1))
}

func emitPopupOpened(id string) {
	if id == "" {
		return
	}
	hooks.PublishWithSource(hooks.EventPopupOpened, hooks.PopupPayload{ID: id}, hooks.CaptureSource(1))
}

func emitPopupClosed(id, reason string) {
	if id == "" {
		return
	}
	hooks.PublishWithSource(hooks.EventPopupClosed, hooks.PopupPayload{ID: id, Reason: reason}, hooks.CaptureSource(1))
}

// Coalesced, so a flick plus momentum yields one line.
func emitScroll(surface string) {
	hooks.PublishWithSource(hooks.EventScroll, hooks.ScrollPayload{Surface: surface}, hooks.CaptureSource(1))
}

// Coalesced, so typing yields one settled-query line; "" means cleared.
func emitSearchChanged(surface, query string) {
	hooks.PublishWithSource(hooks.EventSearchChanged, hooks.SearchPayload{Surface: surface, Query: query}, hooks.CaptureSource(1))
}

// No-op for an empty field: domain editors (rename, BPM, search) leave LogField unset, so they never double-log.
func emitTextCommitted(field, value string) {
	if field == "" {
		return // no field tag ⇒ a domain editor owns this commit; skip generic log
	}
	hooks.PublishWithSource(hooks.EventTextCommitted, hooks.TextPayload{Field: field, Value: value}, hooks.CaptureSource(1))
}

func emitViewMode(mode string) {
	hooks.PublishWithSource(hooks.EventViewModeChanged, hooks.ViewModePayload{Mode: mode}, hooks.CaptureSource(1))
}
