package ui

// TEMPORARY throwaway reproduction test for the beatmo.json import failure.
// Reproduces a brand-new game instance importing a freshly-exported project
// file. Review and convert/delete as needed.

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

const reproBeatmoJSONPath = "/home/ymolinar/Repos/beatmo/beatmo.json"

func TestReproImportBeatmoJSON(t *testing.T) {
	assertDefaultParityState(t)
	// NOTE: withDefaultAudio(t) intentionally OMITTED here. See CONTROL
	// EXPERIMENT 5 / 6 — calling it (ResetInstruments + ResetCatalogForTest)
	// is what made the main-body import appear to drop embedded PCM. The
	// product import is correct; the earlier "failure" was test contamination.

	data, err := os.ReadFile(reproBeatmoJSONPath)
	if err != nil {
		t.Fatalf("read %s: %v", reproBeatmoJSONPath, err)
	}
	t.Logf("read %d bytes from %s", len(data), reproBeatmoJSONPath)

	// Pre-flight: parse the file the same way Import does and probe the PCM
	// decode path directly, so we can tell whether a non-registration is the
	// decoder's fault or a downstream registration/reset issue.
	var f importFile
	{
		if err := json.Unmarshal(data, &f); err != nil {
			t.Fatalf("pre-flight unmarshal failed: %v", err)
		}
		t.Logf("pre-flight: version=%d subdiv=%d bpm=%d insts=%d nodes=%d",
			f.Version, f.Subdiv, f.BPM, len(f.Instruments), len(f.Nodes))

		// DIRECT: after full-file unmarshal into importFile, is f.Instruments[0].PCM nil?
		if len(f.Instruments) > 0 {
			t.Logf("DIRECT f.Instruments[0] ID=%q Kind=%q PCM==nil? %v",
				f.Instruments[0].ID, f.Instruments[0].Kind, f.Instruments[0].PCM == nil)
		}
		// CONTROL: unmarshal full file into a struct using exportFile shape
		// (the EXPORT side type, which also has Instruments []exportInstrument).
		var ef exportFile
		if err := json.Unmarshal(data, &ef); err != nil {
			t.Logf("exportFile unmarshal err=%v", err)
		} else if len(ef.Instruments) > 0 {
			t.Logf("CONTROL exportFile.Instruments[0] ID=%q PCM==nil? %v",
				ef.Instruments[0].ID, ef.Instruments[0].PCM == nil)
		}

		// Raw view: what does the JSON actually carry for instrument[0].pcm?
		var raw struct {
			Instruments []map[string]json.RawMessage `json:"instruments"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			t.Fatalf("raw unmarshal failed: %v", err)
		}
		if len(raw.Instruments) > 0 {
			pcmRaw, has := raw.Instruments[0]["pcm"]
			snippet := string(pcmRaw)
			if len(snippet) > 120 {
				snippet = snippet[:120] + "...(truncated)"
			}
			t.Logf("raw instruments[0] has 'pcm' key=%v rawlen=%d snippet=%s", has, len(pcmRaw), snippet)
			// Round-trip just instruments[0] into a single exportInstrument.
			var one exportInstrument
			if err := json.Unmarshal(data, &struct{ X *int }{}); err != nil {
				_ = err
			}
			b, _ := json.Marshal(map[string]json.RawMessage(raw.Instruments[0]))
			if err := json.Unmarshal(b, &one); err != nil {
				t.Logf("re-unmarshal instruments[0] err=%v", err)
			} else {
				t.Logf("re-unmarshal instruments[0]: ID=%q Kind=%q PCM==nil? %v", one.ID, one.Kind, one.PCM == nil)
				if one.PCM != nil {
					t.Logf("   PCM.sr=%d PCM.frames=%d PCM.b64len=%d", one.PCM.SampleRate, one.PCM.Frames, len(one.PCM.DataB64))
				}
			}
		}
		for _, inst := range f.Instruments {
			if inst.PCM == nil {
				t.Logf("  inst %-12s kind=%q PCM=nil", inst.ID, inst.Kind)
				continue
			}
			pcm, sr, ok := decodeSamplePCM(inst.PCM)
			t.Logf("  inst %-12s kind=%q PCM{sr=%d frames=%d b64len=%d} decodeSamplePCM ok=%v -> %d float32, sr=%d",
				inst.ID, inst.Kind, inst.PCM.SampleRate, inst.PCM.Frames, len(inst.PCM.DataB64), ok, len(pcm), sr)
		}
	}

	// Brand-new instance, mirroring import_validation_test.go setup.
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Sanity probe: does PutUserSample → UserSamplePCM round-trip at all under
	// the current build tag, BEFORE import? Use a throwaway id so we don't
	// pollute the real instrument ids.
	audio.PutUserSample("__probe__", []float32{0.1, 0.2, 0.3}, 48000)
	if rec, ok := audio.UserSamplePCM("__probe__"); ok {
		t.Logf("PRE-IMPORT direct PutUserSample/UserSamplePCM round-trips: frames=%d sr=%d", len(rec.PCM), rec.SampleRate)
	} else {
		t.Logf("PRE-IMPORT direct PutUserSample/UserSamplePCM FAILED to round-trip (build-tag no-op?)")
	}

	start := time.Now()
	importErr := g.Import(data)
	elapsed := time.Since(start)

	// Did the probe id survive the import call?
	if _, ok := audio.UserSamplePCM("__probe__"); ok {
		t.Logf("POST-IMPORT __probe__ still present (import did NOT wipe userSamples)")
	} else {
		t.Logf("POST-IMPORT __probe__ GONE (import wiped userSamples)")
	}

	t.Logf("g.Import took %v", elapsed)
	if importErr != nil {
		t.Errorf("g.Import returned error: %v", importErr)
	} else {
		t.Logf("g.Import returned NIL error")
	}

	// Post-import structural state.
	t.Logf("len(g.drum.Rows) = %d", len(g.drum.Rows))
	t.Logf("len(g.graph.Nodes) = %d", len(g.graph.Nodes))
	t.Logf("len(g.graph.Edges) = %d", len(g.graph.Edges))

	for i, r := range g.drum.Rows {
		t.Logf("  row %d: name=%q inst=%q origin=%d vol=%.2f", i, r.Name, r.Instrument, r.Origin, r.Volume)
	}

	// Sample-instrument PCM registration. The exported file marks kick-1,
	// snare, hihat, fm-epiano-1 as kind=sample; check whether each carried
	// embedded PCM and got registered as a user sample.
	sampleIDs := []string{"kick-1", "snare", "hihat", "fm-epiano-1"}
	for _, id := range sampleIDs {
		rec, ok := audio.UserSamplePCM(id)
		t.Logf("sample %-12s UserSamplePCM ok=%v IsUserSample=%v frames=%d sr=%d avail=%v",
			id, ok, audio.IsUserSample(id), len(rec.PCM), rec.SampleRate, g.drum.IsInstrumentAvailable(id))
	}
	t.Logf("audio.UserSampleIDs() after import = %v", audio.UserSampleIDs())

	// Re-run JUST the registration call the import row-loop makes, to prove
	// PutUserSample DOES land when called directly here (isolating whether the
	// import loop ever reached it).
	if pcm, sr, ok := decodeSamplePCM(f.Instruments[0].PCM); ok {
		audio.PutUserSample("kick-1", pcm, sr)
		_, ok2 := audio.UserSamplePCM("kick-1")
		t.Logf("MANUAL PutUserSample(kick-1) post-import -> UserSamplePCM ok=%v", ok2)
	}

	// Recipe bindings.
	for _, id := range []string{"kick-1", "clap", "cowbell", "fm-epiano-1"} {
		t.Logf("recipe[%s] = %q", id, audio.RecipeForInstrument(id))
	}

	// Try to start playback (does not require real audio device under test tag).
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("playback start panicked: %v", r)
			}
		}()
		g.SetPlaying(true)
		t.Logf("isPlaying after SetPlaying(true) = %v", g.Playing())
		stopPlaybackForTest(g)
	}()

	// CONTROL EXPERIMENT: import a MINIMAL synthetic project that carries ONE
	// embedded-PCM sample instrument, on a fresh game. If THIS registers the
	// PCM, the failure is specific to the 664KB file's content/shape; if it
	// also fails, the embedded-PCM import branch is universally broken.
	t.Run("minimal_embedded_pcm", func(t *testing.T) {
		g2 := New(testLogger)
		t.Cleanup(g2.CloseForTest)
		g2.Layout(800, 600)

		mini := exportFile{
			Version: 1, Subdiv: 8, BPM: 120,
			Instruments: []exportInstrument{{
				Name: "Mini", ID: "mini-sample", Kind: "sample", Volume: 1,
				Origin: 0, Color: "#FF0000FF",
				PCM: encodeSamplePCM(audio.SampleRecord{PCM: []float32{0.5, -0.5, 0.25, -0.25}, SampleRate: 48000}),
			}},
			Nodes: []exportNode{{ID: 0, I: 0, J: 0, Type: "regular"}},
		}
		mb, err := json.Marshal(mini)
		if err != nil {
			t.Fatalf("marshal minimal: %v", err)
		}
		if err := g2.Import(mb); err != nil {
			t.Fatalf("minimal import error: %v", err)
		}
		rec, ok := audio.UserSamplePCM("mini-sample")
		t.Logf("MINIMAL import: UserSamplePCM(mini-sample) ok=%v frames=%d sr=%d ids=%v",
			ok, len(rec.PCM), rec.SampleRate, audio.UserSampleIDs())
	})

	// CONTROL EXPERIMENT 2: same as above but the sample instrument id is a
	// BUILT-IN catalog id (kick-1) WITH embedded PCM — the exact shape the real
	// beatmo.json uses. This isolates whether a builtin id defeats the embedded
	// PCM registration.
	t.Run("builtin_id_embedded_pcm", func(t *testing.T) {
		g3 := New(testLogger)
		t.Cleanup(g3.CloseForTest)
		g3.Layout(800, 600)

		proj := exportFile{
			Version: 1, Subdiv: 8, BPM: 120,
			Instruments: []exportInstrument{{
				Name: "Kick-1", ID: "kick-1", Kind: "sample", Volume: 1,
				Origin: 0, Color: "#FF0000FF",
				Recipe: "drum-kick",
				PCM:    encodeSamplePCM(audio.SampleRecord{PCM: []float32{0.5, -0.5, 0.25, -0.25}, SampleRate: 48000}),
			}},
			Nodes: []exportNode{{ID: 0, I: 0, J: 0, Type: "regular"}},
		}
		pb, err := json.Marshal(proj)
		if err != nil {
			t.Fatalf("marshal builtin-id proj: %v", err)
		}
		if err := g3.Import(pb); err != nil {
			t.Fatalf("builtin-id import error: %v", err)
		}
		rec, ok := audio.UserSamplePCM("kick-1")
		t.Logf("BUILTIN-ID import: UserSamplePCM(kick-1) ok=%v frames=%d sr=%d ids=%v",
			ok, len(rec.PCM), rec.SampleRate, audio.UserSampleIDs())
	})

	// CONTROL EXPERIMENT 3: re-import the REAL file's instruments+nodes only,
	// stripping eq / send_effects / master_volume / pinned_instruments. If PCM
	// now registers, one of those top-level fields is the culprit.
	t.Run("real_instruments_nodes_only", func(t *testing.T) {
		g4 := New(testLogger)
		t.Cleanup(g4.CloseForTest)
		g4.Layout(800, 600)

		stripped := exportFile{
			Version:     f.Version,
			Subdiv:      f.Subdiv,
			BPM:         f.BPM,
			Instruments: f.Instruments,
			Nodes:       f.Nodes,
		}
		sb, err := json.Marshal(stripped)
		if err != nil {
			t.Fatalf("marshal stripped: %v", err)
		}
		if err := g4.Import(sb); err != nil {
			t.Fatalf("stripped import error: %v", err)
		}
		t.Logf("STRIPPED import: UserSampleIDs=%v", audio.UserSampleIDs())
		for _, id := range []string{"kick-1", "snare", "hihat", "fm-epiano-1"} {
			_, ok := audio.UserSamplePCM(id)
			t.Logf("  STRIPPED sample %-12s ok=%v", id, ok)
		}
	})

	// CONTROL EXPERIMENT 4: add each stripped top-level field back, one at a
	// time, to pinpoint which one defeats embedded-PCM registration.
	addBackCase := func(name string, mutate func(*exportFile)) {
		t.Run(name, func(t *testing.T) {
			g5 := New(testLogger)
			t.Cleanup(g5.CloseForTest)
			g5.Layout(800, 600)
			ef := exportFile{
				Version: f.Version, Subdiv: f.Subdiv, BPM: f.BPM,
				Instruments: f.Instruments, Nodes: f.Nodes,
			}
			mutate(&ef)
			b, err := json.Marshal(ef)
			if err != nil {
				t.Fatalf("marshal %s: %v", name, err)
			}
			if err := g5.Import(b); err != nil {
				t.Fatalf("%s import error: %v", name, err)
			}
			t.Logf("ADDBACK[%s]: UserSampleIDs=%v", name, audio.UserSampleIDs())
		})
	}
	addBackCase("with_master_volume", func(ef *exportFile) { ef.MasterVolume = f.MasterVolume })
	addBackCase("with_eq", func(ef *exportFile) { ef.EQ = f.EQ })
	addBackCase("with_send_effects", func(ef *exportFile) { ef.SendEffects = f.SendEffects })
	addBackCase("with_pinned", func(ef *exportFile) { ef.PinnedInstruments = f.PinnedInstruments })

	// CONTROL EXPERIMENT 5: re-import the EXACT original bytes on a fresh game
	// in a subtest (no withDefaultAudio). If this registers PCM but the main
	// test body did not, the failure is test-harness state contamination
	// (withDefaultAudio's ResetInstruments cleanup or similar) rather than a
	// product import bug.
	t.Run("full_real_file_fresh_game", func(t *testing.T) {
		g6 := New(testLogger)
		t.Cleanup(g6.CloseForTest)
		g6.Layout(800, 600)
		if err := g6.Import(data); err != nil {
			t.Fatalf("full real-file import error: %v", err)
		}
		t.Logf("FULL-REAL-FRESH import: UserSampleIDs=%v", audio.UserSampleIDs())
	})
}
