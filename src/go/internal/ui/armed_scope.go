package ui

// Both tabs address armed controls by an index into their own widget slice, so the bookkeeping is keyed
// by scope too, or one tab could read the other's state.
type armedScope int

const (
	armedScopeSynth armedScope = iota
	armedScopeSampler
)
