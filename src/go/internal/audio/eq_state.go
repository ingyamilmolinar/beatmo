package audio

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// eqRecord captures the EQ configuration applied to a channel. Used by
// UI/export tests to assert the correct band values were propagated into the
// audio layer.
type eqRecord struct {
	ID         channel.ID
	SampleRate int
	Bands      []EQBand
	// proc is the channel's live EQ processor (nil on platforms whose EQ runs
	// outside Go). An insert-chain edit reuses it, so editing a row's effects
	// never resets its EQ filters — the browser keeps them too.
	proc Processor
}

var (
	channelEQMu  sync.RWMutex
	channelEQMap = make(map[channel.ID]eqRecord)
)

// recordChannelEQ stores the EQ record for one channel.
// Called from SetChannelEQ on every build path.
func recordChannelEQ(id channel.ID, rec eqRecord) {
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
func lookupChannelEQ(id channel.ID) eqRecord {
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
func ChannelEQSnapshot(id channel.ID) ChannelEQ {
	rec := lookupChannelEQ(id)
	out := ChannelEQ{ID: rec.ID.String(), SampleRate: rec.SampleRate}
	if len(rec.Bands) > 0 {
		out.Bands = append([]EQBand(nil), rec.Bands...)
	}
	return out
}
