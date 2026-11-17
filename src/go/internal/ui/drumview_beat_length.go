package ui

// SetBeatLength sets the beat length in the underlying graph.
func (dv *DrumView) SetBeatLength(length int) {
	if dv.Graph != nil {
		dv.Graph.SetBeatLength(length)
		dv.logger.Debugf("[drumview] Graph beat length set to: %d", length)
	}
	// Do not change the visual timeline scale while playing to avoid jumps
	// in the cursor position. The header can be updated after Stop.
	if !dv.isPlaying {
		if length > dv.timelineBeats {
			dv.timelineBeats = length
		}
	}
}
