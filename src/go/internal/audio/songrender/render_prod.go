//go:build !test

package songrender

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Render (production build): faithful offline mix through the real audio engine.
func Render(stem string, bars, sampleRate int) (Rendered, error) {
	pt, err := LoadTemplate(stem)
	if err != nil {
		return Rendered{}, err
	}
	arr, err := BuildArrangement(pt, bars, sampleRate)
	if err != nil {
		return Rendered{}, err
	}
	insts := make([]audio.SongRenderInstrument, len(arr.Instruments))
	for i, in := range arr.Instruments {
		insts[i] = audio.SongRenderInstrument{
			ID: in.ID, Doc: in.Doc, Volume: in.Volume, Effects: in.Effects,
		}
	}
	notes := make([]audio.SongRenderNote, len(arr.Notes))
	for i, n := range arr.Notes {
		notes[i] = audio.SongRenderNote{
			InstID: n.InstID, AtSample: n.AtSample, Pitch: n.PitchSemis,
			Gain: n.Volume, DurSec: n.DurSec,
		}
	}
	master, stems, err := audio.RenderSongOffline(insts, notes, sampleRate, arr.TotalSamples)
	if err != nil {
		return Rendered{}, err
	}
	return Rendered{Arrangement: arr, Master: master, Stems: stems}, nil
}
