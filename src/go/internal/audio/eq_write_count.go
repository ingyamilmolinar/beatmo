package audio

import "sync/atomic"

// Each write replaces a channel's EQ (or retunes or rewires it on WASM), so a control held still must make none.
var channelEQWrites atomic.Int64

func ChannelEQWriteCount() int64 { return channelEQWrites.Load() }
