//go:build test

package audio

import "fmt"

// SaveRecording is a stub for test builds.
func SaveRecording(result *RecordingResult) (string, error) {
	if result == nil || len(result.Channels) == 0 {
		return "", fmt.Errorf("no channels to save")
	}
	return "test-recordings/" + result.Metadata.Timestamp, nil
}
