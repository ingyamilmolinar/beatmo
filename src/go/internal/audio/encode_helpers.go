package audio

import "bytes"

// encodeBytes is a thin helper that runs encoder.Encode against a
// bytes.Buffer and returns the resulting payload. Used by callers that
// need an in-memory encoded blob (legacy WASM-style fallback, tests).
func encodeBytes(enc AudioEncoder, samples []float64, sampleRate int) ([]byte, error) {
	var buf bytes.Buffer
	if err := enc.Encode(&buf, samples, sampleRate); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
