package ui

import "image"

// input_capture.go — opaque-to-z `HitArea` factory.
//
// **The contract.** The DrumView Zone tree dispatches input by z-priority:
// `HitIndex.At()` returns hits sorted by (exact-rect-containment, then
// z-index descending), and the dispatcher (`drumview_tree.go.handleInput`)
// tries each in order, stopping on `InputCaptured` or `InputConsumed`.
// For that contract to be load-bearing, every Zone that renders an
// opaque visual surface must **register a full-bounds catch-all** at its
// own nominal z so taps in the zone's whitespace can't fall through to
// a lower-z sibling.
//
// Before this helper landed, `EQPanelZone.rebuildHitAreas()` registered
// only per-control hit areas. On the mobile Synth tab, the panel was
// visually opaque but logically transparent: a tap in chrome whitespace
// fell past the knob rects, past the panel z=130, and onto the
// `gridDragHitAdapter` at z=110 underneath — letting "drag a synth
// knob" also pan the drum grid / scrub the timeline. The same shape
// affected `ChainPanelZone` and any future audio tab. The
// `NewInputCaptureHitArea` helper formalises the catch-all so the
// pattern is one line per zone, not a one-off hand-rolled struct each
// time.
//
// **Why InputConsumed (not InputCaptured).** Captured means "I'm
// starting a drag, give me every move until release"; consumed means
// "I handled this press, don't try anyone below me". A zone-level
// catch-all has no drag semantics — it's just a one-shot "stop here".
// Consuming on press lets the next press dispatch normally (the
// tree's `capturedHandler` stays nil), so a drag inside the panel can
// still bring up per-control hit areas via their `OnPress` returning
// `InputCaptured` (knobs / sliders take that path).
//
// **Why wheel returns InputIgnored.** Vertical scroll inside an audio
// panel should bubble to whatever wheel handler owns scrolling (e.g.
// the rack scroller). A catch-all that swallowed wheels would break
// touchpad scrolling over the panel. If a panel wants wheel capture
// it registers its own per-control wheel handlers at higher z.

// inputCaptureHandler implements HitHandler with "consume press, ignore
// the rest". Stateless — safe to share a single zero-value instance
// across every catch-all hit area.
type inputCaptureHandler struct{}

// OnPress unconditionally consumes the press so dispatch stops at this
// hit area without falling through to lower-z siblings.
func (inputCaptureHandler) OnPress(x, y int) InputResult { return InputConsumed }

// OnDrag is a no-op: a consumed (vs captured) press never gets drag
// events from the tree dispatcher.
func (inputCaptureHandler) OnDrag(x, y int) {}

// OnRelease is a no-op for the same reason.
func (inputCaptureHandler) OnRelease(x, y int) {}

// OnWheel ignores wheel events so vertical scroll bubbles through the
// catch-all to whatever zone owns scrolling under this point. See the
// file-level docstring for why this is the right default.
func (inputCaptureHandler) OnWheel(x, y, steps int) InputResult { return InputIgnored }

// sharedInputCaptureHandler is the stateless singleton every
// catch-all `HitArea` references. Lives at the file level so a hot
// `rebuildHitAreas` loop doesn't allocate a fresh struct per frame.
var sharedInputCaptureHandler inputCaptureHandler

// inputCaptureWheelHandler is the catch-all variant that ALSO consumes the
// wheel. It exists because the audio-panel subtree (dv.audioTree) is isolated:
// there is no scroll-owning zone *beneath* the panel inside that subtree, so a
// wheel the panel's own controls don't handle must stop here. If it bubbled
// (the InputIgnored default), the legacy row-scroll fallback in
// drumview_update.go — which lives OUTSIDE the tree and keys off the cursor
// being inside dv.Bounds — would scroll the unrelated drum rows (the canonical
// "wheel over a non-scrollable synth knob scrolls the rows" leak that the
// two-subtree split fixes). Consuming sets the subtree's wheelHandled flag, and
// drumview_update.go gates the legacy fallback on (dv.tree || dv.audioTree)
// WheelHandled.
type inputCaptureWheelHandler struct{ inputCaptureHandler }

// OnWheel consumes the wheel so it does not bubble out of the isolated
// audio-panel subtree to the legacy row-scroll fallback.
func (inputCaptureWheelHandler) OnWheel(x, y, steps int) InputResult { return InputConsumed }

// sharedInputCaptureWheelHandler is the stateless singleton for the
// wheel-consuming catch-all.
var sharedInputCaptureWheelHandler inputCaptureWheelHandler

// NewInputCaptureHitAreaConsumeWheel returns a catch-all `HitArea` that consumes
// both press AND wheel inside `rect` at `zIndex`. Use it for opaque zones living
// in an isolated subtree with no scroll-owning zone beneath them (the audio
// panel). For zones whose subtree owns scrolling beneath the catch-all, use
// NewInputCaptureHitArea (wheel bubbles).
func NewInputCaptureHitAreaConsumeWheel(rect image.Rectangle, zIndex int, tag string) HitArea {
	return HitArea{
		Rect:    rect,
		ZIndex:  zIndex,
		Handler: sharedInputCaptureWheelHandler,
		Tag:     tag,
	}
}

// NewInputCaptureHitArea returns a `HitArea` that consumes any press
// inside `rect` at `zIndex`. Drag/release/wheel pass through.
//
// Usage at a Zone's `rebuildHitAreas`:
//
//	if !z.rect.Empty() {
//	    z.hitAreas = append(z.hitAreas,
//	        NewInputCaptureHitArea(z.rect, ZEQPanel, "eq-panel-capture"))
//	}
//
// The catch-all goes at the Zone's *nominal* z. Per-control hit
// areas inside the same zone sit at `nominal-z + 1` / `+2` and win
// on overlap (per `HitIndex.At`'s z-descending sort), so knobs and
// buttons remain interactive while panel whitespace is opaque.
func NewInputCaptureHitArea(rect image.Rectangle, zIndex int, tag string) HitArea {
	return HitArea{
		Rect:    rect,
		ZIndex:  zIndex,
		Handler: sharedInputCaptureHandler,
		Tag:     tag,
	}
}
