package ui

// WheelModel is the value model a ValueWheel scrolls. The wheel owns NO value
// of its own: every label it draws and every step it takes goes through the
// model, so one view serves synth/sampler knobs (knobModel), plain numbers
// (NumberModel: BPM, node values, go-to position/time, clock fields) and any
// future source without the view knowing which.
//
// Offsets and steps follow the barrel's geometry: offset +n is the n-th
// DISTINCT value ABOVE the centre selector (larger), -n below (smaller).
type WheelModel interface {
	// Snapshot / Restore capture and reinstate the value (transactional Cancel).
	Snapshot() float64
	Restore(v float64)

	// CenterLabel is the live value shown in the centre selector.
	CenterLabel() string
	// RowLabel is the label offset distinct rows from the centre; ok == false
	// when that row falls outside the value range (the row is not drawn).
	RowLabel(offset int) (string, bool)
	// SelectRow sets the value to exactly what RowLabel(offset) shows.
	SelectRow(offset int) bool

	// Step moves the value by n detents at the active resolution. Returns
	// whether the value changed (false at a clamp).
	Step(n int) bool
	// StepFine moves by n detents one ladder rung finer than the active one,
	// without persisting the rung (Shift + scroll).
	StepFine(n int) bool

	// HasLadder reports whether a resolution ladder is shown. Ladder indices
	// run fine→coarse (rung 0 = finest); the view draws the coarsest on top.
	HasLadder() bool
	LadderLen() int
	LadderIndex() int
	LadderLabel(i int) string
	SetLadderIndex(i int) bool
	// AdjustLadder moves the rung by d: +1 = FINER (KnobStepBadge semantics).
	AdjustLadder(d int) bool
}
