package ui

// Layer-B interaction telemetry emit helpers. Each publishes through
// hooks.PublishWithSource with hooks.CaptureSource(1) so the src= column points
// at the widget chokepoint's caller. See
// docs/superpowers/specs/2026-06-27-info-log-audit-and-interaction-coverage-design.md.

import (
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// emitUITap publishes EventUITap from the shared Button press core. Suppressed
// (no publish) when both label and icon are empty.
func emitUITap(label, icon string) {
	if label == "" && icon == "" {
		return
	}
	hooks.PublishWithSource(hooks.EventUITap, hooks.UITapPayload{Label: label, Icon: icon}, hooks.CaptureSource(1))
}

// emitPopupOpened publishes EventPopupOpened from the shared overlay portal
// chokepoint. No-op when id is empty.
func emitPopupOpened(id string) {
	if id == "" {
		return
	}
	hooks.PublishWithSource(hooks.EventPopupOpened, hooks.PopupPayload{ID: id}, hooks.CaptureSource(1))
}

// emitPopupClosed publishes EventPopupClosed from the shared overlay portal
// chokepoint. No-op when id is empty.
func emitPopupClosed(id, reason string) {
	if id == "" {
		return
	}
	hooks.PublishWithSource(hooks.EventPopupClosed, hooks.PopupPayload{ID: id, Reason: reason}, hooks.CaptureSource(1))
}

// emitScroll publishes EventScroll at scroll-gesture end (touch lift /
// scrollbar drag release / stepped-grid drag end). Coalesced by the
// eventlogger so a flick + momentum burst produces one trailing line.
func emitScroll(surface string) {
	hooks.PublishWithSource(hooks.EventScroll, hooks.ScrollPayload{Surface: surface}, hooks.CaptureSource(1))
}

// emitSearchChanged publishes EventSearchChanged when a search field's query
// changes. Coalesced by the eventlogger so rapid per-keystroke emits produce
// one trailing settled-query line. surface names the search context (e.g.
// "inst-menu"); query is the current value ("" means cleared).
func emitSearchChanged(surface, query string) {
	hooks.PublishWithSource(hooks.EventSearchChanged, hooks.SearchPayload{Surface: surface, Query: query}, hooks.CaptureSource(1))
}

// emitTextCommitted publishes EventTextCommitted when a generic text field
// commits its value (Enter key or focus lost). No-op when field is empty —
// this is the guarantee that domain editors (instrument rename → EventInstrumentRenamed,
// BPM entry → EventBPMChange, search → EventSearchChanged) never double-log:
// they leave LogField unset so this helper silently skips them.
func emitTextCommitted(field, value string) {
	if field == "" {
		return // no field tag ⇒ a domain editor owns this commit; skip generic log
	}
	hooks.PublishWithSource(hooks.EventTextCommitted, hooks.TextPayload{Field: field, Value: value}, hooks.CaptureSource(1))
}

// emitViewMode publishes EventViewModeChanged when the mobile bottom-nav
// switches the visible view. mode is the stable lowercase slug (e.g. "pads",
// "eq", "wave", "spectrum", "levels", "chain", "synth", "sampler").
func emitViewMode(mode string) {
	hooks.PublishWithSource(hooks.EventViewModeChanged, hooks.ViewModePayload{Mode: mode}, hooks.CaptureSource(1))
}
