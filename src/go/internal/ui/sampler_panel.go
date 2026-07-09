package ui

// samplerPanel is the Sampler audio-panel tab, extracted from the DrumView
// God-object (Phase 2a). Rung A groups the sampler methods onto this collaborator
// via a `dv *DrumView` back-reference; the sampler state fields (dv.sampler,
// dv.samplerButtons, dv.samplerWheelPopup) deliberately stay on DrumView for now
// so their ~143 call sites are untouched — a zero-behavior-change cohesion move.
//
// Rung B replaces the back-reference with a narrow SamplerHost interface (~15
// ops) so the panel becomes independently testable; see the Phase-2a plan.
//
// Every relocated method keeps a one-line *DrumView forwarding shim, so all 139
// existing sampler tests continue to drive the panel through DrumView unchanged.
type samplerPanel struct {
	dv *DrumView // back-reference (replaced by a SamplerHost interface in Rung B)
}

// sp returns the DrumView's sampler panel, lazily creating it if absent. The
// ctor sets dv.samplerPanel eagerly; this getter is the self-healing path for
// the ~22 test files that build *DrumView via a bare &DrumView{} literal
// bypassing the ctor. Every sampler forwarding shim calls dv.sp().X() so those
// tests (and any future bare-literal construction) never nil-panic on a
// relocated method. The nil check is a single predictable branch.
func (dv *DrumView) sp() *samplerPanel {
	if dv.samplerPanel == nil {
		dv.samplerPanel = &samplerPanel{dv: dv}
	}
	return dv.samplerPanel
}
