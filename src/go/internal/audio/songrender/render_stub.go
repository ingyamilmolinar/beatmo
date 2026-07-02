//go:build test

package songrender

import (
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
)

// Render (test build): deterministic. Each note is rendered by the synthmatch
// stub synth, scaled by note.Volume, summed into its instrument's stem at the
// note's sample offset. Master is the exact element-wise sum of all stems.
func Render(stem string, bars, sampleRate int) (Rendered, error) {
	pt, err := LoadTemplate(stem)
	if err != nil {
		return Rendered{}, err
	}
	arr, err := BuildArrangement(pt, bars, sampleRate)
	if err != nil {
		return Rendered{}, err
	}
	stems := make(map[string][]float64)
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
	keys := make([]string, 0, len(stems))
	for k := range stems {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for i, v := range stems[k] {
			master[i] += v
		}
	}
	return Rendered{Arrangement: arr, Master: master, Stems: stems}, nil
}
