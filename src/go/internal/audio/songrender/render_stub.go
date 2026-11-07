//go:build test

package songrender

import (
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Master is the exact element-wise sum of the stems.
func Render(stem string, bars, sampleRate int) (Rendered, error) {
	pt, err := LoadTemplate(stem)
	if err != nil {
		return Rendered{}, err
	}
	arr, err := BuildArrangement(pt, bars, sampleRate)
	if err != nil {
		return Rendered{}, err
	}
	stems := make(map[instrument.ID][]float64)
	for _, in := range arr.Instruments {
		if _, ok := stems[in.ID]; !ok {
			stems[in.ID] = make([]float64, arr.TotalSamples)
		}
	}
	for _, n := range arr.Notes {
		w, err := synthmatch.RenderWithParams(n.InstID, n.PitchSemis, sampleRate, n.DurSec, nil)
		if err != nil {
			continue // stub: skip un-renderable instruments rather than fail the whole song
		}
		dst := stems[n.InstID]
		for i, s := range w.Samples {
			j := n.AtSample + i
			if j >= len(dst) {
				break
			}
			dst[j] += s * n.Volume
		}
	}
	master := make([]float64, arr.TotalSamples)
	keys := make([]instrument.ID, 0, len(stems))
	for k := range stems {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		for i, v := range stems[k] {
			master[i] += v
		}
	}
	return Rendered{Arrangement: arr, Master: master, Stems: stems}, nil
}
