package audio

import "bytes"

func encodeBytes(enc AudioEncoder, samples []float64, sampleRate int) ([]byte, error) {
	var buf bytes.Buffer
	if err := enc.Encode(&buf, samples, sampleRate); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
