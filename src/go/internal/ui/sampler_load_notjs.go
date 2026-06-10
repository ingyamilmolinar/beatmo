//go:build !test && !js

package ui

import "github.com/ingyamilmolinar/beatmo/internal/audio"

func init() { samplerWAVLoadFn = desktopSamplerLoadWAV }

// desktopSamplerLoadWAV opens the native file picker and decodes the chosen WAV
// to engine-rate PCM via miniaudio.
func desktopSamplerLoadWAV() ([]float32, int, bool) {
	path, err := audio.SelectWAV()
	if err != nil || path == "" {
		return nil, 0, false
	}
	pcm, sr, err := audio.DecodeWAVToPCM(path)
	if err != nil || len(pcm) == 0 {
		return nil, 0, false
	}
	return pcm, sr, true
}
