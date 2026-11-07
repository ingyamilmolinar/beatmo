package ui

// samplerPanel groups the Sampler tab's methods behind a DrumView back-reference; its state still lives on
// DrumView, and every method keeps a *DrumView forwarding shim, so existing call sites are unchanged.
type samplerPanel struct {
	dv *DrumView // back-reference (replaced by a SamplerHost interface in Rung B)

	// Mobile sticky-bar actions, separate from the in-panel tag buttons so two owners never fight over one rect.
	mobilePreviewBtn  *Button
	mobileSaveAsBtn   *Button
	mobileOverflowBtn *Button
}

// Lazy for DrumViews built as bare literals in tests, which skip the ctor and would nil-panic otherwise.
func (dv *DrumView) sp() *samplerPanel {
	if dv.samplerPanel == nil {
		dv.samplerPanel = &samplerPanel{dv: dv}
	}
	return dv.samplerPanel
}
