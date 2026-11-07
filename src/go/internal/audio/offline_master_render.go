//go:build !test && !js

package audio

// Unlike RenderSongOffline, which installs its own configuration, this touches NONE: whatever the engine holds (volumes,
// insert chains, channel and master EQ) is rendered, so an export/import identity test reads live state. Every pass
// starts with ResetDSPState, so the same inputs are byte-identical.
func RenderMasterMixOffline(notes []SongRenderNote, sampleRate, totalSamples int) []float64 {
	return renderOnePass(notes, sampleRate, totalSamples, "")
}
