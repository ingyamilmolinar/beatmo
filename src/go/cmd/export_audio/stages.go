//go:build !test && !js

package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// --stages: each case rendered through desktop's real offline mixer and emitted with everything the browser needs to
// build the SAME case through its graph (documents, insert chains and EQ programs as Go pushes them, master, notes, lane
// stops) plus desktop's signal at every stage tap.

type stageInstrument struct {
	Block    []float64        `json:"block"`
	Seconds  float64          `json:"seconds"`
	Amp      float64          `json:"amp"`
	Melodic  bool             `json:"melodic"`
	GateTail float64          `json:"gateTail"`
	FX       []map[string]any `json:"fx"`
}

type stageChannel struct {
	Chain []audio.InsertEffectInstance `json:"chain"`
	EQ    []float64                    `json:"eq"`
}

type stageNoteJSON struct {
	ID    instrument.ID `json:"id"`
	At    int           `json:"at"`
	Pitch float64       `json:"pitch"`
	Vol   float64       `json:"vol"`
	Dur   float64       `json:"dur"`
	Lane  bool          `json:"lane"`
}

type stageStopJSON struct {
	ID instrument.ID `json:"id"`
	At int           `json:"at"`
}

type stageScene struct {
	Name         string                         `json:"name"`
	Stage        string                         `json:"stage"`
	Frames       int                            `json:"frames"`
	MasterVolume float64                        `json:"masterVolume"`
	MasterEQ     []float64                      `json:"masterEQ"`
	Channels     map[instrument.ID]stageChannel `json:"channels"`
	Notes        []stageNoteJSON                `json:"notes"`
	Stops        []stageStopJSON                `json:"stops"`
	Taps         map[string]string              `json:"taps"` // "<id>/voices|inserts|eq", "out" → float32 LE base64
	// Per tap, the output step of an upstream quantizing effect (bitcrusher mix / 2^bits): inputs a float32 ulp apart
	// (WebAudio sums in float32, desktop in float64) can round to the neighbouring step. The test allows exactly that step.
	Quantum map[string]float64 `json:"quantum,omitempty"`
	// SampleEdits are Sampler edits active during the case, as Go pushes
	// them (audio.SampleEditWire → updateSampleEdit).
	SampleEdits map[instrument.ID]json.RawMessage `json:"sampleEdits,omitempty"`
}

// sceneSpec is a case before rendering.
type sceneSpec struct {
	name, stage  string
	seconds      float64
	masterVolume float64
	masterEQ     []audio.EQBand
	channels     map[instrument.ID]channelSpec
	notes        []audio.StageNote
	stops        []audio.StageStop
	taps         []string // which per-channel taps to emit (voices, inserts, eq); "out" always
	sampleEdits  map[instrument.ID]audio.SampleEdit
}

type channelSpec struct {
	slots []audio.EffectSlot
	eq    []audio.EQBand
}

const (
	kick   = roster.Kick
	snare  = roster.Snare
	hihat  = roster.Hihat
	banjo  = roster.Banjo
	organ  = roster.OrganChurch
	bass   = roster.Bass808
	stepSR = audio.EngineSampleRate
)

// at is a sample index a little off any block boundary.
func at(sec float64) int { return int(sec*stepSR) + 37 }

func exportStages() {
	audio.Reset()
	audio.ResetInstruments()
	if audio.SampleRate() != audio.EngineSampleRate {
		fail("engine runs at %d Hz, want %d", audio.SampleRate(), audio.EngineSampleRate)
	}
	for _, id := range []instrument.ID{banjo, organ, bass} {
		if !audio.IsMelodicInstrument(id) {
			fail("%s is no longer melodic: pick another melodic instrument for the stage cases", id)
		}
	}
	out := struct {
		SampleRate  int                               `json:"sampleRate"`
		Instruments map[instrument.ID]stageInstrument `json:"instruments"`
		Scenes      []stageScene                      `json:"scenes"`
	}{SampleRate: audio.EngineSampleRate, Instruments: map[instrument.ID]stageInstrument{}}

	for _, sc := range stageCases() {
		for id := range sc.channels {
			if _, ok := out.Instruments[id]; !ok {
				out.Instruments[id] = instrumentForBrowser(id)
			}
		}
		for _, n := range sc.notes {
			if _, ok := out.Instruments[n.Inst]; !ok {
				out.Instruments[n.Inst] = instrumentForBrowser(n.Inst)
			}
		}
		out.Scenes = append(out.Scenes, renderScene(sc))
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fail("%v", err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "export_audio --stages: "+format+"\n", args...)
	os.Exit(1)
}

func instrumentForBrowser(id instrument.ID) stageInstrument {
	d, ok := audio.ShippedDoc(id)
	if !ok {
		fail("%s: no shipped document", id)
	}
	fx := []map[string]any{}
	for _, c := range audio.VoiceFXCalls(d) {
		fx = append(fx, map[string]any{"type": c.Type, "params": c.Params})
	}
	return stageInstrument{
		Block: instrument.Flatten(d, audio.BuiltinModuleRegistry()), Seconds: d.Render.Seconds,
		Amp: d.Render.Amplitude, Melodic: d.Render.Melodic, GateTail: d.Render.GateTail, FX: fx,
	}
}

// renderScene configures the engine for sc (replace, not merge), renders it
// and packs the browser's input and desktop's taps.
func renderScene(sc sceneSpec) stageScene {
	audio.ClearAllInsertEffectsAndNotify()
	for _, id := range roster.All {
		audio.ClearChannelProcessors(channel.Of(id))
	}
	audio.ClearChannelProcessors(channel.Master)
	audio.SetMainVolume(sc.masterVolume)
	res := stageScene{
		Name: sc.name, Stage: sc.stage, Frames: int(sc.seconds * stepSR), MasterVolume: audio.MainVolume(),
		Channels: map[instrument.ID]stageChannel{}, Taps: map[string]string{},
	}
	if len(sc.masterEQ) > 0 {
		audio.SetChannelEQ(channel.Master, stepSR, sc.masterEQ...)
		res.MasterEQ = audio.BuildEQProgram(stepSR, sc.masterEQ)
	}
	ids := make([]instrument.ID, 0, len(sc.channels))
	for id := range sc.channels {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		cs := sc.channels[id]
		_ = audio.InstrumentChannel(channel.Of(id))
		if len(cs.slots) > 0 {
			audio.SetInsertEffects(id, cs.slots)
		}
		ch := stageChannel{Chain: audio.InsertChainForPlatform(id)}
		if len(cs.eq) > 0 {
			audio.SetChannelEQ(channel.Of(id), stepSR, cs.eq...)
			ch.EQ = audio.BuildEQProgram(stepSR, cs.eq)
		}
		res.Channels[id] = ch
	}
	for _, n := range sc.notes {
		res.Notes = append(res.Notes, stageNoteJSON{ID: n.Inst, At: n.At, Pitch: n.Pitch, Vol: n.Vol, Dur: n.Dur, Lane: n.Lane})
	}
	for _, s := range sc.stops {
		res.Stops = append(res.Stops, stageStopJSON{ID: s.Inst, At: s.At})
	}
	for _, id := range ids {
		q := 0.0
		for _, e := range res.Channels[id].Chain {
			if e.Enabled && e.Type == audio.EffectBitcrusher {
				bits := math.Max(2, math.Min(16, e.Params["bits"]))
				q = math.Max(q, math.Max(0, math.Min(1, e.Params["mix"]))/math.Pow(2, bits))
			}
		}
		if q > 0 {
			if res.Quantum == nil {
				res.Quantum = map[string]float64{}
			}
			res.Quantum[string(id)+"/inserts"], res.Quantum[string(id)+"/eq"] = q, q
			res.Quantum["out"] = math.Max(res.Quantum["out"], q)
		}
	}
	for id, e := range sc.sampleEdits {
		audio.SetSampleEdit(id, e)
		if res.SampleEdits == nil {
			res.SampleEdits = map[instrument.ID]json.RawMessage{}
		}
		res.SampleEdits[id] = audio.SampleEditWire(e)
	}
	master, taps := audio.RenderStagesOffline(sc.notes, sc.stops, res.Frames)
	for id := range sc.sampleEdits {
		audio.ClearSampleEdit(id)
	}
	res.Taps["out"] = f32b64(master)
	for _, id := range ids {
		t := taps[id]
		for _, name := range sc.taps {
			var x []float64
			switch name {
			case "voices":
				x = t.Voices
			case "inserts":
				x = t.Inserts
			case "eq":
				x = t.EQ
			}
			if x == nil {
				x = make([]float64, res.Frames)
			}
			res.Taps[string(id)+"/"+name] = f32b64(x)
		}
	}
	return res
}

func f32b64(x []float64) string {
	raw := make([]byte, 4*len(x))
	for i, v := range x {
		binary.LittleEndian.PutUint32(raw[4*i:], math.Float32bits(float32(v)))
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func note(id instrument.ID, sec, pitch, vol, dur float64) audio.StageNote {
	return audio.StageNote{Inst: id, At: at(sec), Pitch: pitch, Vol: vol, Dur: dur}
}

// stageCases is every case, from the voice outward.
func stageCases() []sceneSpec {
	var cases []sceneSpec
	plain := func(ids ...instrument.ID) map[instrument.ID]channelSpec {
		m := map[instrument.ID]channelSpec{}
		for _, id := range ids {
			m[id] = channelSpec{}
		}
		return m
	}

	// Drums pitched and sized by resampling, melodic notes at exact (fractional) pitch gated by Duration, volumes down
	// below 1/32, starts off every block boundary, overlaps on one channel.
	cases = append(cases, sceneSpec{
		name: "played/drums", stage: "voices", seconds: 2.4, masterVolume: 1,
		channels: plain(kick, snare, hihat),
		notes: []audio.StageNote{
			note(kick, 0.01, 0, 1, 1), note(snare, 0.21, -5, 0.37, 1), note(hihat, 0.42, 7, 0.8, 0.5),
			note(kick, 0.63, 12, 0.025, 2), note(snare, 0.95, 3.3, 0.6, 0.75), note(snare, 1.0, -2.6, 0.5, 1.3),
			note(hihat, 1.3, -12, 1.2, 1), note(kick, 1.7, -7, 0.9, 0.6),
		},
		taps: []string{"voices", "eq"},
	})
	cases = append(cases, sceneSpec{
		name: "played/melodic", stage: "voices", seconds: 3.2, masterVolume: 1,
		channels: plain(banjo, organ, bass),
		notes: []audio.StageNote{
			note(banjo, 0.01, 0, 1, 1), note(banjo, 0.5, -7.3, 0.6, 0.5), note(organ, 0.9, 12, 0.4, 0.25),
			note(organ, 1.2, 5, 0.1, 1.5), note(bass, 1.6, -12, 0.8, 0.3), note(bass, 2.1, 2.5, 0.02, 1),
			note(banjo, 2.4, 3, 0.7, 0.1),
		},
		taps: []string{"voices", "eq"},
	})
	// A Sampler edit (trim, transpose, gain, reverse, normalise, fade) on a
	// drum and on a melodic voice, played pitched and gated.
	cases = append(cases, sceneSpec{
		name: "played/sample-edit", stage: "voices", seconds: 2.0, masterVolume: 1,
		channels: plain(snare, banjo),
		sampleEdits: map[instrument.ID]audio.SampleEdit{
			snare: {StartFrac: 0.05, EndFrac: 0.8, TransposeSemis: 2, DetuneCents: -30, GainDB: -3, Fade: true, Reverse: true, Normalize: true},
			banjo: {StartFrac: 0.1, EndFrac: 0.9, TransposeSemis: -1, GainDB: 2, Fade: true},
		},
		notes: []audio.StageNote{
			note(snare, 0.01, 0, 0.9, 1), note(banjo, 0.3, 4, 0.8, 0.5), note(snare, 0.8, -3, 0.6, 1.4), note(banjo, 1.1, 0, 0.7, 1),
		},
		taps: []string{"voices"},
	})
	// A lane stop (a mute node) lands on its exact sample and fades the voices
	// that started before it; one that starts after it is spared.
	stopAt := at(1.0) + 1234
	cases = append(cases, sceneSpec{
		name: "stop/lane", stage: "voices", seconds: 2.0, masterVolume: 1,
		channels: plain(organ, snare),
		notes: []audio.StageNote{
			{Inst: organ, At: at(0.2), Pitch: 0, Vol: 0.9, Dur: 1, Lane: true},
			{Inst: organ, At: at(0.6), Pitch: 4, Vol: 0.5, Dur: 1, Lane: true},
			{Inst: snare, At: at(0.95), Pitch: 0, Vol: 0.8, Dur: 1, Lane: true},
			{Inst: organ, At: stopAt + 480, Pitch: 7, Vol: 0.7, Dur: 1, Lane: true},
		},
		stops: []audio.StageStop{{Inst: organ, At: stopAt}, {Inst: snare, At: stopAt + 99}},
		taps:  []string{"voices"},
	})

	// Every insert effect at its defaults, at the bottom, middle and top of
	// every parameter's range — each set on its own channel.
	for _, et := range audio.EffectTypes {
		reg := audio.EffectRegistrations()[et]
		sets := map[string]map[string]float64{"default": audio.DefaultParams(et), "min": {}, "mid": {}, "max": {}}
		for _, p := range reg.Params {
			sets["min"][p.Name], sets["max"][p.Name], sets["mid"][p.Name] = p.Min, p.Max, (p.Min+p.Max)/2
		}
		hosts := map[string]instrument.ID{"default": snare, "min": banjo, "mid": kick, "max": organ}
		chans := map[instrument.ID]channelSpec{}
		var notes []audio.StageNote
		for _, set := range []string{"default", "min", "mid", "max"} {
			id := hosts[set]
			chans[id] = channelSpec{slots: []audio.EffectSlot{{Type: et, Enabled: true, Params: sets[set]}}}
			notes = append(notes, note(id, 0.02, 0, 0.9, 1), note(id, 0.4, 3, 0.7, 0.5))
		}
		cases = append(cases, sceneSpec{
			name: "fx/" + string(et), stage: "inserts", seconds: 1.0, masterVolume: 1,
			channels: chans, notes: notes, taps: []string{"voices", "inserts"},
		})
	}
	// A chain of effects with a disabled slot in the middle.
	cases = append(cases, sceneSpec{
		name: "fx/chain", stage: "inserts", seconds: 1.5, masterVolume: 1,
		channels: map[instrument.ID]channelSpec{snare: {slots: []audio.EffectSlot{
			{Type: audio.EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 180, "q": 0.707, "mix": 1}},
			{Type: audio.EffectDistortion, Enabled: false, Params: map[string]float64{"drive": 8}},
			{Type: audio.EffectDelay, Enabled: true, Params: map[string]float64{"time": 120, "feedback": 0.5, "mix": 0.4}},
			{Type: audio.EffectCompressor, Enabled: true, Params: map[string]float64{"threshold": -24, "ratio": 6, "makeup": 6}},
		}}},
		notes: []audio.StageNote{note(snare, 0.02, 0, 1, 1), note(snare, 0.3, -3, 0.8, 1), note(snare, 0.61, 4, 0.5, 0.7)},
		taps:  []string{"inserts"},
	})

	// Every EQ shape the UI makes, on a broadband mix (kick, hats, a melody).
	for _, ec := range eqStageCases() {
		cases = append(cases, sceneSpec{
			name: "eq/" + ec.name, stage: "eq", seconds: 1.4, masterVolume: 1,
			channels: map[instrument.ID]channelSpec{hihat: {eq: ec.bands}, banjo: {eq: ec.bands}, kick: {eq: ec.bands}},
			notes: []audio.StageNote{
				note(kick, 0.02, 0, 1, 1), note(hihat, 0.05, 0, 1, 1), note(banjo, 0.1, 0, 0.9, 1),
				note(hihat, 0.55, 5, 0.8, 1), note(banjo, 0.6, -5, 0.8, 0.5), note(kick, 0.7, -3, 1, 1),
			},
			taps: []string{"voices", "eq"},
		})
	}

	// The master: EQ, volume, and the soft clip (loud enough to reach it).
	loud := []audio.StageNote{
		note(kick, 0.02, 0, 4, 1), note(snare, 0.02, 0, 4, 1), note(bass, 0.03, -12, 4, 1),
		note(organ, 0.25, 0, 3, 1), note(hihat, 0.3, 0, 3, 1), note(kick, 0.5, 0, 0.5, 1),
	}
	cases = append(cases,
		sceneSpec{name: "master/clip", stage: "master", seconds: 1.2, masterVolume: 1, channels: plain(kick, snare, bass, organ, hihat), notes: loud},
		sceneSpec{name: "master/eq", stage: "master", seconds: 1.2, masterVolume: 0.5, masterEQ: eqStageCases()[1].bands, channels: plain(kick, snare, bass, organ, hihat), notes: loud},
	)

	// Everything at once: inserts, a row EQ with a muted band and filters,
	// the master EQ and volume.
	cases = append(cases, sceneSpec{
		name: "chain/everything", stage: "all", seconds: 2.0, masterVolume: 0.8, masterEQ: eqStageCases()[1].bands,
		channels: map[instrument.ID]channelSpec{
			snare: {slots: []audio.EffectSlot{{Type: audio.EffectReverb, Enabled: true, Params: map[string]float64{"room": 0.7, "mix": 0.35}}}, eq: eqStageCases()[4].bands},
			banjo: {slots: []audio.EffectSlot{{Type: audio.EffectChorus, Enabled: true}, {Type: audio.EffectTremolo, Enabled: true, Params: map[string]float64{"shape": 1}}}},
			kick:  {eq: eqStageCases()[2].bands},
		},
		notes: []audio.StageNote{
			note(kick, 0.02, 0, 1, 1), note(snare, 0.25, 0, 0.9, 1), note(banjo, 0.3, 2, 0.8, 0.75),
			note(kick, 0.5, -2, 1, 1), note(snare, 0.75, 1.5, 0.7, 1), note(banjo, 0.9, -5, 0.6, 1),
		},
		taps: []string{"voices", "inserts", "eq"},
	})
	return cases
}

type eqStageCase struct {
	name  string
	bands []audio.EQBand
}

// eqStageCases mirror the UI's band list (DrumView.buildFullEQBands): ten
// ISO bands (low shelf, peaking, high shelf, Q 1.414) with optional high-pass
// first and low-pass last.
func eqStageCases() []eqStageCase {
	defs := [][2]float64{{22, 44}, {44, 88}, {88, 177}, {177, 354}, {354, 707}, {707, 1414}, {1414, 2828}, {2828, 5657}, {5657, 11314}, {11314, 20000}}
	graphic := func(gains []float64, muted map[int]bool) []audio.EQBand {
		var out []audio.EQBand
		for i, d := range defs {
			k := audio.EQPeaking
			if i == 0 {
				k = audio.EQLowShelf
			}
			if i == len(defs)-1 {
				k = audio.EQHighShelf
			}
			out = append(out, audio.EQBand{Kind: k, Freq: math.Sqrt(d[0] * d[1]), Q: 1.414, GainDB: gains[i], Muted: muted[i]})
		}
		return out
	}
	shaped := []float64{12, -6, 3, 0, -12, 6, 0, -3, 9, -12}
	hpf := audio.EQBand{Kind: audio.EQHighpass, Freq: 180, Q: 0.707}
	lpf := audio.EQBand{Kind: audio.EQLowpass, Freq: 9000, Q: 0.707}
	all := map[int]bool{}
	for i := range 10 {
		all[i] = true
	}
	withFilters := func(b []audio.EQBand) []audio.EQBand { return append(append([]audio.EQBand{hpf}, b...), lpf) }
	return []eqStageCase{
		{"flat", graphic(make([]float64, 10), nil)},
		{"shaped", graphic(shaped, nil)},
		{"shaped+filters", withFilters(graphic(shaped, nil))},
		{"muted-band", graphic(shaped, map[int]bool{3: true})},
		{"muted+filters", withFilters(graphic(shaped, map[int]bool{3: true, 7: true}))},
		{"all-muted", graphic(shaped, all)},
	}
}
