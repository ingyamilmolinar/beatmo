package ui

import "github.com/ingyamilmolinar/beatmo/internal/hooks"

type seekReqMode int

const (
	seekReqAbs seekReqMode = iota
	seekReqDeltaBeats
	seekReqDeltaSteps
)

// Drained once per frame under seqMu; deltas resolve against the live playhead.
type seekRequest struct {
	value  int // steps (abs) OR delta (beats/steps), per mode
	mode   seekReqMode
	origin hooks.SeekOrigin
}

// RequestSeekSteps queues an absolute seek to subdivision `steps`.
func (dv *DrumView) RequestSeekSteps(steps int, origin hooks.SeekOrigin) {
	dv.pendingSeek = &seekRequest{value: steps, mode: seekReqAbs, origin: origin}
}

// RequestSeekDeltaBeats queues a relative seek of `deltaBeats` whole beats.
func (dv *DrumView) RequestSeekDeltaBeats(deltaBeats int, origin hooks.SeekOrigin) {
	dv.pendingSeek = &seekRequest{value: deltaBeats, mode: seekReqDeltaBeats, origin: origin}
}

func (dv *DrumView) RequestSeekDeltaSteps(deltaSteps int, origin hooks.SeekOrigin) {
	dv.pendingSeek = &seekRequest{value: deltaSteps, mode: seekReqDeltaSteps, origin: origin}
}

// TakeSeekRequest returns and clears any queued request (last-wins per frame).
func (dv *DrumView) TakeSeekRequest() (value int, mode seekReqMode, origin hooks.SeekOrigin, ok bool) {
	r := dv.pendingSeek
	if r == nil {
		return 0, seekReqAbs, 0, false
	}
	dv.pendingSeek = nil
	return r.value, r.mode, r.origin, true
}
