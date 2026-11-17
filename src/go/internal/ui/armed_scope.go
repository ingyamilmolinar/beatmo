package ui

// armed_scope.go — which panel owns an armed inline control.
//
// The Synth and Sampler tabs both host click-to-arm barrel controls, and both
// address their controls by an INDEX into their own widget slice. A given
// instrument id therefore means "synth control 7" in one tab and "sampler knob
// 2" in the other. The arm bookkeeping and the single grow-animation subject
// are keyed by scope as well as instrument id so the two tabs can never read
// each other's state.
type armedScope int

const (
	armedScopeSynth armedScope = iota
	armedScopeSampler
)
