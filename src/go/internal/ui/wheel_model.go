package ui

// The wheel owns NO value: every label and step goes through the model, so one view serves knobs and plain numbers.
// Offset +n is the n-th DISTINCT value ABOVE the centre (larger).
type WheelModel interface {
	// Snapshot / Restore capture and reinstate the value (transactional Cancel).
	Snapshot() float64
	Restore(v float64)

	// CenterLabel is the live value shown in the centre selector.
	CenterLabel() string
	// !ok outside the range: the row is not drawn.
	RowLabel(offset int) (string, bool)
	// SelectRow sets the value to exactly what RowLabel(offset) shows.
	SelectRow(offset int) bool

	// false at a clamp.
	Step(n int) bool
	// Shift + scroll; the rung is not persisted.
	StepFine(n int) bool

	// Ladder indices run fine→coarse (0 = finest); the view draws the coarsest on top.
	HasLadder() bool
	LadderLen() int
	LadderIndex() int
	LadderLabel(i int) string
	SetLadderIndex(i int) bool
	// AdjustLadder moves the rung by d: +1 = FINER (KnobStepBadge semantics).
	AdjustLadder(d int) bool
}
