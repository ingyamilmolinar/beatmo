package ui

// seekRequest is a one-shot navigation intent set by any UI entry point
// (ribbon, FF/RW, counter, keyboard) and drained once per frame by
// Game.Update under seqMu. Absolute requests carry steps; delta requests carry
// a beat offset resolved against the live playhead in Game.
type seekRequest struct {
	steps      int
	deltaBeats int
	isDelta    bool
	origin     string
}

// RequestSeekSteps queues an absolute seek to subdivision `steps`.
func (dv *DrumView) RequestSeekSteps(steps int, origin string) {
	dv.pendingSeek = &seekRequest{steps: steps, origin: origin}
}

// RequestSeekDeltaBeats queues a relative seek of `deltaBeats` whole beats.
func (dv *DrumView) RequestSeekDeltaBeats(deltaBeats int, origin string) {
	dv.pendingSeek = &seekRequest{deltaBeats: deltaBeats, isDelta: true, origin: origin}
}

// TakeSeekRequest returns and clears any queued request (last-wins per frame).
func (dv *DrumView) TakeSeekRequest() (steps, deltaBeats int, isDelta bool, origin string, ok bool) {
	r := dv.pendingSeek
	if r == nil {
		return 0, 0, false, "", false
	}
	dv.pendingSeek = nil
	return r.steps, r.deltaBeats, r.isDelta, r.origin, true
}
