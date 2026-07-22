package ui

// seekReqMode discriminates the three flavors of navigation intent a
// seekRequest can carry: an absolute subdivision target, a beat-snapped
// delta, or a subdivision-granular delta.
type seekReqMode int

const (
	seekReqAbs seekReqMode = iota
	seekReqDeltaBeats
	seekReqDeltaSteps
)

// seekRequest is a one-shot navigation intent set by any UI entry point
// (ribbon, FF/RW, counter, keyboard) and drained once per frame by
// Game.Update under seqMu. Absolute requests carry steps in value; delta
// requests (beats or subdivision-granular steps) carry an offset in value
// resolved against the live playhead in Game.
type seekRequest struct {
	value  int // steps (abs) OR delta (beats/steps), per mode
	mode   seekReqMode
	origin string
}

// RequestSeekSteps queues an absolute seek to subdivision `steps`.
func (dv *DrumView) RequestSeekSteps(steps int, origin string) {
	dv.pendingSeek = &seekRequest{value: steps, mode: seekReqAbs, origin: origin}
}

// RequestSeekDeltaBeats queues a relative seek of `deltaBeats` whole beats.
func (dv *DrumView) RequestSeekDeltaBeats(deltaBeats int, origin string) {
	dv.pendingSeek = &seekRequest{value: deltaBeats, mode: seekReqDeltaBeats, origin: origin}
}

// RequestSeekDeltaSteps queues a relative seek of `deltaSteps` subdivisions —
// no beat snap, unlike RequestSeekDeltaBeats.
func (dv *DrumView) RequestSeekDeltaSteps(deltaSteps int, origin string) {
	dv.pendingSeek = &seekRequest{value: deltaSteps, mode: seekReqDeltaSteps, origin: origin}
}

// TakeSeekRequest returns and clears any queued request (last-wins per frame).
func (dv *DrumView) TakeSeekRequest() (value int, mode seekReqMode, origin string, ok bool) {
	r := dv.pendingSeek
	if r == nil {
		return 0, seekReqAbs, "", false
	}
	dv.pendingSeek = nil
	return r.value, r.mode, r.origin, true
}
