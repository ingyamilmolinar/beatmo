//go:build !test && !js

package audio

import (
	"encoding/binary"
	"fmt"
	"os"
)

// ExportCaptureToWAV writes the captured audio samples to a WAV file.
// The samples should be float64 values in the range [-1, 1].
// The output will be 16-bit mono PCM at the capture sample rate (44100 Hz).
func ExportCaptureToWAV(samples []float64, filename string) error {
	if len(samples) == 0 {
		return fmt.Errorf("no samples to export")
	}

	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	sr := uint32(sampleRate)
	numChannels := uint16(1)
	bitsPerSample := uint16(16)
	bytesPerSample := bitsPerSample / 8
	blockAlign := numChannels * bytesPerSample
	byteRate := sr * uint32(blockAlign)
	dataSize := uint32(len(samples)) * uint32(bytesPerSample)

	// Write RIFF header
	if _, err := f.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(36+dataSize)); err != nil {
		return err
	}
	if _, err := f.Write([]byte("WAVE")); err != nil {
		return err
	}

	// Write fmt chunk
	if _, err := f.Write([]byte("fmt ")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(16)); err != nil { // chunk size
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil { // PCM format
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, numChannels); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, sr); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, byteRate); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, blockAlign); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, bitsPerSample); err != nil {
		return err
	}

	// Write data chunk header
	if _, err := f.Write([]byte("data")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, dataSize); err != nil {
		return err
	}

	// Write samples as 16-bit PCM
	for _, sample := range samples {
		// Clamp to [-1, 1]
		if sample > 1 {
			sample = 1
		} else if sample < -1 {
			sample = -1
		}
		// Convert to 16-bit signed integer
		int16Val := int16(sample * 32767)
		if err := binary.Write(f, binary.LittleEndian, int16Val); err != nil {
			return err
		}
	}

	return nil
}

// ExportCapturedAudioToWAV is a convenience function that stops capture and exports to WAV.
// Returns the number of samples captured.
func ExportCapturedAudioToWAV(filename string) (int, error) {
	samples := StopOutputCapture()
	if len(samples) == 0 {
		return 0, fmt.Errorf("no samples captured")
	}
	if err := ExportCaptureToWAV(samples, filename); err != nil {
		return 0, err
	}
	return len(samples), nil
}
