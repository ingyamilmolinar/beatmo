package audio

import "sync"

// eqRecord captures the EQ configuration applied to a channel. Used by
// UI/export tests to assert the correct band values were propagated into the
// audio layer.
type eqRecord struct {
	ID         string
	SampleRate int
	Bands      []EQBand
}

var (
	channelEQMu  sync.RWMutex
	channelEQMap = make(map[string]eqRecord)
)

// recordChannelEQ stores the EQ record for one channel.
// Called from SetChannelEQ on every build path.
func recordChannelEQ(id string, rec eqRecord) {
	channelEQMu.Lock()
	if len(rec.Bands) == 0 {
		delete(channelEQMap, id)
	} else {
		channelEQMap[id] = rec
	}
	channelEQMu.Unlock()
}

// lookupChannelEQ returns the EQ record for a channel, or the zero value if
// no EQ has ever been set for it.
func lookupChannelEQ(id string) eqRecord {
	channelEQMu.RLock()
	defer channelEQMu.RUnlock()
	return channelEQMap[id]
}

// ChannelEQ is the public snapshot type returned by ChannelEQSnapshot.
// Mirrors the internal eqRecord but with exported fields so callers
// outside the audio package can read it.
type ChannelEQ struct {
	ID         string
	SampleRate int
	Bands      []EQBand
}

// ChannelEQSnapshot returns the most recently applied EQ for the named
// channel. Returns the zero value when no EQ has been set for the
// channel. Callers receive an independent copy of the Bands slice so
// later mutations to the internal record do not alias the snapshot.
func ChannelEQSnapshot(id string) ChannelEQ {
	rec := lookupChannelEQ(id)
	out := ChannelEQ{ID: rec.ID, SampleRate: rec.SampleRate}
	if len(rec.Bands) > 0 {
		out.Bands = append([]EQBand(nil), rec.Bands...)
	}
	return out
}

// GetChannelEQResponse returns the magnitude response of the per-
// channel EQ chain at numPoints log-spaced frequencies between
// startHz and endHz. Used by the Synth tab's filter preview to
// render the active channel's biquad cascade alongside the recipe
// knob cards. Returns nil when the channel has no EQ bands set or
// when numPoints is non-positive.
func GetChannelEQResponse(channelID string, numPoints int, startHz, endHz float64) []FreqResponsePoint {
	if numPoints <= 0 {
		return nil
	}
	rec := lookupChannelEQ(channelID)
	if len(rec.Bands) == 0 {
		return nil
	}
	sr := rec.SampleRate
	if sr <= 0 {
		sr = 48000
	}
	return ComputeFreqResponse(sr, rec.Bands, numPoints, startHz, endHz)
}
