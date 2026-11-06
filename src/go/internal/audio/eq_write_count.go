package audio

import "sync/atomic"

// channelEQWrites counts SetChannelEQ calls on every platform. Each one
// replaces a channel's EQ (desktop: a fresh processor swapped into the chain;
// WASM: the live filters retuned, or rebuilt and rewired when the band layout
// changed), so a control held still must not make any. Perf guards read it.
var channelEQWrites atomic.Int64

// ChannelEQWriteCount reports how many channel-EQ writes reached the audio
// layer since process start.
func ChannelEQWriteCount() int64 { return channelEQWrites.Load() }
