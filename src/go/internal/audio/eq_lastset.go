//go:build !test

package audio

// eqRecord captures the last EQ configuration applied to a channel. Used by
// UI/export tests to assert the correct band values were propagated into the
// audio layer in non-test builds.
type eqRecord struct {
	ID         string
	SampleRate int
	Bands      []EQBand
}

var lastSetEQ eqRecord

// LastSetEQ returns the most recent EQ settings applied via SetChannelEQ.
func LastSetEQ() eqRecord { return lastSetEQ }
