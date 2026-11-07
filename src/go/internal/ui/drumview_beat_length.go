package ui

// SetBeatLength sets the beat length in the underlying graph.
func (dv *DrumView) SetBeatLength(length int) {
	if dv.Graph != nil {
		dv.Graph.SetBeatLength(length)
		dv.logger.Debugf("[drumview] Graph beat length set to: %d", length)
	}
	// The timeline scale holds still while playing, or the cursor would jump.
	if !dv.isPlaying {
		if length > dv.timelineBeats {
			dv.timelineBeats = length
		}
	}
}
