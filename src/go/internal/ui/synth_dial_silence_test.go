//go:build !test && !js

package ui

import (
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
)

// The audibility check cannot see silence (it is as different as a sound gets), so every end of every dial on
// every shipped instrument is rendered through the played voice and must sound.
func TestNoDialEndSilencesItsInstrument(t *testing.T) {
	withPrivateInstrumentLibrary(t)
	audio.ResetInstrumentDocs()
	t.Cleanup(audio.ResetInstrumentDocs)
	type end struct {
		id     instrument.ID
		key    string
		fields map[string]float64
	}
	var ends []end
	for _, id := range roster.All {
		schema := audio.InstrumentParamDefs(id)
		if len(schema) == 0 {
			continue
		}
		merged := audio.MergedInstrumentParamsRO(id)
		voice := 0.0
		if d, ok := audio.InstrumentDoc(id); ok {
			voice = d.Render.Seconds
		}
		for _, cg := range buildMacroGroupsForVoice(schema, merged, voice) {
			for _, d := range cg.Dials {
				start := map[string]float64{}
				for _, tm := range d.Terms {
					if v, ok := merged[tm.Param]; ok {
						start[tm.Param] = v
					}
				}
				key := string(id) + " " + cg.ID + "/" + d.Dial.ID
				ends = append(ends, end{id, key + " at 0", d.Turn(start, 0)}, end{id, key + " at 100", d.Turn(start, 1)})
			}
		}
	}
	if len(ends) < 1000 {
		t.Fatalf("only %d dial ends across the roster: the enumeration is broken", len(ends))
	}
	silent := make([]bool, len(ends))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, e := range ends {
		wg.Add(1)
		go func(i int, e end) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			doc, err := audio.InstrumentDocWithFields(e.id, e.fields)
			if err != nil {
				t.Errorf("%s: %v", e.key, err)
				return
			}
			pcm, _, err := audio.RenderDocForGraph(e.id, doc, 0)
			if err != nil {
				t.Errorf("%s: %v", e.key, err)
				return
			}
			silent[i] = soundview.Silent(pcm)
		}(i, e)
	}
	wg.Wait()
	for i, e := range ends {
		if silent[i] {
			t.Errorf("%s renders SILENCE: a dial end must never mute the instrument (raise the term's from/to in synth_dials.json, or drop a term that is the voice's level)", e.key)
		}
	}
}
