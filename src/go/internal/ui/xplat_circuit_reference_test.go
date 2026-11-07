//go:build !test

package ui

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The desktop reference xplat_audio_stages.browser.test.js compares against, rendered fresh every run and never
// committed; runs only when XPLAT_CIRCUIT_OUT names an output. The file carries the project as exported, so the
// browser imports exactly the rendered state, plus the notes and the output after the soft clip.
func TestWriteXplatCircuitReference(t *testing.T) {
	out := os.Getenv("XPLAT_CIRCUIT_OUT")
	if out == "" {
		t.Skip("generator for xplat_audio_stages.browser.test.js (set XPLAT_CIRCUIT_OUT)")
	}
	const seconds = 4.0
	type circuit struct {
		name  string
		bytes []byte
		edit  bool
	}
	circuits := []circuit{
		{"startup-demo", assets.StartupDemoJSON, false},
		{"test-fixture", assets.TestFixtureDemoJSON, false},
	}
	tps := assets.Templates()
	for i, idx := range []int{0, len(tps) / 3, 2 * len(tps) / 3, len(tps) - 1} {
		if idx >= 0 && idx < len(tps) {
			circuits = append(circuits, circuit{"template/" + tps[idx].Genre, tps[idx].Bytes, i == 1})
		}
	}
	type noteJSON struct {
		ID    instrument.ID `json:"id"`
		At    int           `json:"at"`
		Pitch float64       `json:"pitch"`
		Vol   float64       `json:"vol"`
		Dur   float64       `json:"dur"`
	}
	type circuitJSON struct {
		Name    string     `json:"name"`
		Project string     `json:"project"`
		Frames  int        `json:"frames"`
		Notes   []noteJSON `json:"notes"`
		Out     string     `json:"out"`
	}
	res := struct {
		SampleRate int           `json:"sampleRate"`
		Circuits   []circuitJSON `json:"circuits"`
	}{SampleRate: audio.SampleRate()}
	for _, c := range circuits {
		g := New(game_log.New(nil, game_log.LevelError))
		t.Cleanup(g.CloseForTest)
		g.Layout(1280, 720)
		if err := g.Import(c.bytes); err != nil {
			t.Fatalf("%s: import: %v", c.name, err)
		}
		if c.edit {
			editFirstRow(t, g)
			c.name += "+edits"
		}
		project, err := g.drum.exportBytes()
		if err != nil {
			t.Fatalf("%s: export: %v", c.name, err)
		}
		notes, total := liveArrangement(t, g, seconds)
		master := audio.RenderMasterMixOffline(notes, audio.SampleRate(), total)
		cj := circuitJSON{Name: c.name, Project: string(project), Frames: total, Out: pcmB64(master)}
		for _, n := range notes {
			dur := 1.0
			if base := audio.ConfigForInstrument(n.InstID).DurationSec; n.DurSec > 0 && base > 0 {
				dur = n.DurSec / base
			}
			cj.Notes = append(cj.Notes, noteJSON{ID: n.InstID, At: n.AtSample, Pitch: n.Pitch, Vol: n.Gain, Dur: dur})
		}
		if len(cj.Notes) == 0 {
			t.Fatalf("%s: no notes in %v s — the comparison would be vacuous", c.name, seconds)
		}
		res.Circuits = append(res.Circuits, cj)
		t.Logf("%s: %d notes, %d frames", c.name, len(cj.Notes), total)
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := json.NewEncoder(f).Encode(res); err != nil {
		t.Fatal(err)
	}
}

func pcmB64(x []float64) string {
	raw := make([]byte, 4*len(x))
	for i, v := range x {
		binary.LittleEndian.PutUint32(raw[4*i:], math.Float32bits(float32(v)))
	}
	return base64.StdEncoding.EncodeToString(raw)
}
