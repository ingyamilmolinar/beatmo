package audio

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

type eqRecord struct {
	ID         channel.ID
	SampleRate int
	Bands      []EQBand
	// nil where EQ runs outside Go. Reused by insert-chain edits, so editing effects never resets the EQ filters.
	proc Processor
}

var (
	channelEQMu  sync.RWMutex
	channelEQMap = make(map[channel.ID]eqRecord)
)

// On every build path.
func recordChannelEQ(id channel.ID, rec eqRecord) {
	channelEQMu.Lock()
	if len(rec.Bands) == 0 {
		delete(channelEQMap, id)
	} else {
		channelEQMap[id] = rec
	}
	channelEQMu.Unlock()
}

// The zero value when never set.
func lookupChannelEQ(id channel.ID) eqRecord {
	channelEQMu.RLock()
	defer channelEQMu.RUnlock()
	return channelEQMap[id]
}

type ChannelEQ struct {
	ID         string
	SampleRate int
	Bands      []EQBand
}

// Bands is an independent copy.
func ChannelEQSnapshot(id channel.ID) ChannelEQ {
	rec := lookupChannelEQ(id)
	out := ChannelEQ{ID: rec.ID.String(), SampleRate: rec.SampleRate}
	if len(rec.Bands) > 0 {
		out.Bands = append([]EQBand(nil), rec.Bands...)
	}
	return out
}
