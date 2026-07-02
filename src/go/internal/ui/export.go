package ui

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Export schema
type exportFile struct {
	Version      int                `json:"version"`
	Subdiv       int                `json:"subdiv,omitempty"`
	BPM          int                `json:"bpm"`
	MasterVolume float64            `json:"master_volume,omitempty"` // 0-1, omitted when default (1.0)
	Instruments  []exportInstrument `json:"instruments"`
	Nodes        []exportNode       `json:"nodes"`
	EQ           *exportEQ          `json:"eq,omitempty"`
	SendEffects  *SendEffectsConfig `json:"send_effects,omitempty"`
	// PinnedInstruments holds project-scope instrument pins (tier 0 in the
	// menu's PinSource). Distinct from per-user favorites which live in
	// userprefs and never touch this file. omitempty keeps v1 projects with
	// no pins byte-identical to pre-PR exports; old loaders ignore the field.
	PinnedInstruments []string `json:"pinned_instruments,omitempty"`
	// Phase 6 (kit infra): every kit registered in the audio package at
	// export time. Additive — old loaders silently ignore the field, and
	// re-importing a project that didn't carry kits leaves the
	// in-process kit registry untouched. Kit-bus audio routing is
	// future work; the InsertChain field on each kit round-trips through
	// JSON so kits authored today survive the rollout.
	Kits []audio.Kit `json:"kits,omitempty"`
}

type exportInstrument struct {
	Name       string             `json:"name"`
	ID         string             `json:"id"`
	Kind       string             `json:"kind"` // builtin|sample
	Volume     float64            `json:"volume"`
	Origin     int                `json:"origin"`
	Color      string             `json:"color"`                 // #RRGGBBAA
	Path       string             `json:"path,omitempty"`        // local cache path or object URL for custom samples
	EQ         *exportEQ          `json:"eq,omitempty"`          // per-instrument EQ settings
	Effects    []audio.EffectSlot `json:"effects,omitempty"`     // per-instrument insert effect chain
	Pan        float64            `json:"pan,omitempty"`         // stereo pan: -1 (left) to +1 (right)
	DelaySend  float64            `json:"delay_send,omitempty"`  // delay send amount (0-1)
	ReverbSend float64            `json:"reverb_send,omitempty"` // reverb send amount (0-1)
	// Phase 3 synth-recipe fields. Recipe is the SynthRecipe id the
	// instrument resolves through (e.g. "drum-snare"); empty for samples
	// or instruments without a binding. SynthParams is the per-instrument
	// override map; absent when the user has not edited any knobs.
	Recipe      string             `json:"recipe,omitempty"`
	SynthParams map[string]float64 `json:"synth_params,omitempty"`
	// SampleEdit carries the non-destructive Sampler edit applied to the
	// recipe render at trigger time (audio.SampleEdit as a flat field map,
	// bools 0/1). Additive + omitempty: present only when the user saved a
	// Sampler edit over a synth source. The synth stays the source of
	// truth — this is a post-render transform, not baked PCM.
	SampleEdit map[string]float64 `json:"sample_edit,omitempty"`
	// Phase 6: explicit kit-role tag (kick/snare/hat/…). Empty means
	// kit application falls back to the audio.RoleForInstrument
	// heuristic. Additive — old files have no Role and migrate cleanly.
	Role string `json:"role,omitempty"`
	// PCM carries the embedded baked sample for Sampler-tab instruments
	// (synth-capture or WAV-load). Additive + omitempty: old loaders ignore
	// it; when present it supersedes Path (the sound travels inside the
	// project file). Version stays 1.
	PCM *exportSamplePCM `json:"pcm,omitempty"`
}

// exportSamplePCM is the inline PCM payload for a Sampler-created instrument:
// little-endian float32 frames, base64-encoded.
type exportSamplePCM struct {
	SampleRate int    `json:"sample_rate"`
	Frames     int    `json:"frames"`
	DataB64    string `json:"data_b64"`
}

// encodeSamplePCM serializes a SampleRecord to the embedded export shape using
// portable little-endian float32 bytes (deterministic across platforms),
// gzip-compressed before base64 so embedded one-shots don't bloat beatmo.json
// (the bytes stay opaque; the file Version stays 1).
func encodeSamplePCM(rec audio.SampleRecord) *exportSamplePCM {
	buf := make([]byte, len(rec.PCM)*4)
	for i, v := range rec.PCM {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	_, _ = w.Write(buf)
	_ = w.Close()
	return &exportSamplePCM{
		SampleRate: rec.SampleRate,
		Frames:     len(rec.PCM),
		DataB64:    base64.StdEncoding.EncodeToString(gz.Bytes()),
	}
}

// decodeSamplePCM reverses encodeSamplePCM. Returns ok=false for a nil/empty or
// malformed payload so import can fall back to the path/catalog strategies.
// Gzipped payloads (current encoder) carry the 0x1f 0x8b magic; a payload
// without it is treated as legacy raw little-endian float32 bytes.
func decodeSamplePCM(p *exportSamplePCM) ([]float32, int, bool) {
	if p == nil || p.DataB64 == "" || p.Frames <= 0 {
		return nil, 0, false
	}
	raw, err := base64.StdEncoding.DecodeString(p.DataB64)
	if err != nil {
		return nil, 0, false
	}
	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		zr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, 0, false
		}
		defer zr.Close()
		if raw, err = io.ReadAll(zr); err != nil {
			return nil, 0, false
		}
	}
	if len(raw) < p.Frames*4 {
		return nil, 0, false
	}
	pcm := make([]float32, p.Frames)
	for i := range pcm {
		pcm[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return pcm, p.SampleRate, true
}

type exportNode struct {
	ID      int    `json:"id"`
	I       int    `json:"i"`
	J       int    `json:"j"`
	Type    string `json:"type"` // regular|invisible|silent|mute
	Inputs  []int  `json:"inputs,omitempty"`
	Outputs []int  `json:"outputs,omitempty"`
	// Optional per-node parameters. Omitted when at defaults.
	Volume   float64 `json:"volume,omitempty"`
	Pitch    float64 `json:"pitch,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	// Node logic fields.
	LogicKind string  `json:"logic_kind,omitempty"`
	LogicN    int     `json:"logic_n,omitempty"`
	LogicP    float64 `json:"logic_p,omitempty"`
	// Groove: per-node rule (none|delay|rush) and percentage (0..1)
	GrooveKind string  `json:"groove_kind,omitempty"`
	GroovePct  float64 `json:"groove_pct,omitempty"`
	// Per-node effect overrides (V1: model-only, no UI)
	EffectOverrides []model.EffectOverride `json:"effect_overrides,omitempty"`
	// Per-node synth override map (canonical synth-param shape).
	SynthOverrides map[string]float64 `json:"synth_overrides,omitempty"`
	// Legacy v1 individual synth fields — read-only on import for backward
	// compatibility with files written before the synth_overrides map.
	// Export never writes these (writeNode emits SynthOverrides); import
	// folds any non-zero value into the same NodeParams slots.
	LegacySynthDecay      float64 `json:"synth_decay,omitempty"`
	LegacySynthTone       float64 `json:"synth_tone,omitempty"`
	LegacySynthAttack     float64 `json:"synth_attack,omitempty"`
	LegacySynthDrive      float64 `json:"synth_drive,omitempty"`
	LegacySynthBody       float64 `json:"synth_body,omitempty"`
	LegacySynthColor      float64 `json:"synth_color,omitempty"`
	LegacySynthBrightness float64 `json:"synth_brightness,omitempty"`
}

// legacySynthOverrides folds the legacy v1 individual synth_* node fields
// into the canonical override-map shape. Returns nil when no legacy field
// is set so callers can cheaply skip the merge.
func (n exportNode) legacySynthOverrides() map[string]float64 {
	out := map[string]float64{}
	if n.LegacySynthDecay != 0 {
		out["decay"] = n.LegacySynthDecay
	}
	if n.LegacySynthTone != 0 {
		out["tone"] = n.LegacySynthTone
	}
	if n.LegacySynthAttack != 0 {
		out["attack"] = n.LegacySynthAttack
	}
	if n.LegacySynthDrive != 0 {
		out["drive"] = n.LegacySynthDrive
	}
	if n.LegacySynthBody != 0 {
		out["body"] = n.LegacySynthBody
	}
	if n.LegacySynthColor != 0 {
		out["color"] = n.LegacySynthColor
	}
	if n.LegacySynthBrightness != 0 {
		out["brightness"] = n.LegacySynthBrightness
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SendEffectsConfig encodes global send effect parameters. Optional; absent for legacy files.
type SendEffectsConfig struct {
	Delay  *SendDelayConfig  `json:"delay,omitempty"`
	Reverb *SendReverbConfig `json:"reverb,omitempty"`
}

// SendDelayConfig encodes delay send bus parameters.
type SendDelayConfig struct {
	TimeMs    float64 `json:"time_ms"`
	Feedback  float64 `json:"feedback"`
	DampingHz float64 `json:"damping_hz"`
}

// SendReverbConfig encodes reverb send bus parameters.
type SendReverbConfig struct {
	Room    float64 `json:"room"`
	Damping float64 `json:"damping"`
	Wet     float64 `json:"wet"`
}

// exportEQ encodes the master EQ settings. Optional; absent for legacy files.
type exportEQ struct {
	GainsDB     []float64    `json:"gains_db,omitempty"` // per band gain in dB
	BandsHz     [][2]float64 `json:"bands_hz,omitempty"`
	BandMuted   []bool       `json:"band_muted,omitempty"`    // per band mute state
	HPFEnabled  bool         `json:"hpf_enabled,omitempty"`   // high-pass filter on/off
	HPFCutoffHz float64      `json:"hpf_cutoff_hz,omitempty"` // HPF cutoff frequency
	LPFEnabled  bool         `json:"lpf_enabled,omitempty"`   // low-pass filter on/off
	LPFCutoffHz float64      `json:"lpf_cutoff_hz,omitempty"` // LPF cutoff frequency
}

// kindForID determines the instrument kind by checking catalog metadata.
// WAV-based and embedded samples return "sample", synthesized instruments return "builtin".
func kindForID(id string) string {
	// Legacy "sample-" prefix check for backwards compatibility
	if strings.HasPrefix(id, "sample-") {
		return "sample"
	}
	// Look up in catalog to check Source field
	if meta, ok := audio.CatalogLookup(id); ok {
		switch meta.Source {
		case "wav", "embedded":
			return "sample"
		}
	}
	return "builtin"
}

// synthOverridesFromNodeParams collects the 7 individual NodeParams.Synth*
// fields into a map for emission as the v1+synth "synth_overrides" JSON
// shape. Zero values are omitted so a node with all-default params yields
// an empty map (and the json:"omitempty" tag drops the key entirely).
func synthOverridesFromNodeParams(p model.NodeParams) map[string]float64 {
	out := map[string]float64{}
	if p.SynthDecay != 0 {
		out["decay"] = p.SynthDecay
	}
	if p.SynthTone != 0 {
		out["tone"] = p.SynthTone
	}
	if p.SynthAttack != 0 {
		out["attack"] = p.SynthAttack
	}
	if p.SynthDrive != 0 {
		out["drive"] = p.SynthDrive
	}
	if p.SynthBody != 0 {
		out["body"] = p.SynthBody
	}
	if p.SynthColor != 0 {
		out["color"] = p.SynthColor
	}
	if p.SynthBrightness != 0 {
		out["brightness"] = p.SynthBrightness
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// applySynthOverridesToNodeParams writes back a SynthOverrides map onto
// NodeParams. Used by the import path so both the v1 legacy shape (7
// individual fields) and the v1+synth shape (single map) populate the same
// in-memory slots.
func applySynthOverridesToNodeParams(p *model.NodeParams, over map[string]float64) {
	if v, ok := over["decay"]; ok {
		p.SynthDecay = v
	}
	if v, ok := over["tone"]; ok {
		p.SynthTone = v
	}
	if v, ok := over["attack"]; ok {
		p.SynthAttack = v
	}
	if v, ok := over["drive"]; ok {
		p.SynthDrive = v
	}
	if v, ok := over["body"]; ok {
		p.SynthBody = v
	}
	if v, ok := over["color"]; ok {
		p.SynthColor = v
	}
	if v, ok := over["brightness"]; ok {
		p.SynthBrightness = v
	}
}

func hexColor(c color.Color) string {
	r, g, b, a := c.RGBA()
	// r,g,b,a are 0..65535; convert to 0..255
	return fmt.Sprintf("#%02X%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
}

// Export builds the export JSON and triggers a download/save.
func (dv *DrumView) Export() error {
	dv.logger.Debugf("[drumview] export requested")
	data, err := dv.exportBytes()
	if err != nil {
		return err
	}
	name := "beatmo-export.json"
	if err := saveJSON(name, data); err != nil {
		dv.logger.Errorf("[drumview] export failed: %v", err)
		return err
	}
	dv.logger.Debugf("[drumview] export completed: %s (%d bytes)", name, len(data))
	return nil
}

// Save function indirection for platform and tests.
var saveJSON = saveJSONDefault

// currentMaxDiv returns the smallest subdivision per beat for export. Game sets
// this to the live grid's MaxDiv; default to 32.
var currentMaxDiv = func() int { return 32 }

// exportBytes builds the export JSON without saving to disk.
func (dv *DrumView) exportBytes() ([]byte, error) {
	if dv == nil || dv.Graph == nil {
		return nil, fmt.Errorf("no graph")
	}
	g := dv.Graph
	ids := make([]int, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)

	in := map[int][]int{}
	out := map[int][]int{}
	for e := range g.Edges {
		a := int(e[0])
		b := int(e[1])
		out[a] = append(out[a], b)
		in[b] = append(in[b], a)
	}
	nodes := make([]exportNode, 0, len(ids))
	for _, id := range ids {
		n := g.Nodes[model.NodeID(id)]
		if n.Type == model.NodeTypeInvisible {
			continue
		}
		typ := "regular"
		switch n.Type {
		case model.NodeTypeSilent:
			typ = "silent"
		case model.NodeTypeMute:
			typ = "mute"
		}
		en := exportNode{ID: id, I: n.I, J: n.J, Type: typ}
		if v := in[id]; len(v) > 0 {
			sort.Ints(v)
			en.Inputs = v
		}
		if v := out[id]; len(v) > 0 {
			sort.Ints(v)
			en.Outputs = v
		}
		// Include params only when not defaults to keep JSON compact.
		if p := n.Params; true {
			if p.Volume != 0 && p.Volume != 1 {
				en.Volume = p.Volume
			}
			if p.Pitch != 0 {
				en.Pitch = p.Pitch
			}
			if p.Duration != 0 && p.Duration != 1 {
				en.Duration = p.Duration
			}
			// New logic fields
			if p.LogicKind != "" {
				en.LogicKind = p.LogicKind
				if p.LogicN > 0 {
					en.LogicN = p.LogicN
				}
				if p.LogicP > 0 {
					en.LogicP = p.LogicP
				}
			}
			if p.GrooveKind != "" {
				en.GrooveKind = p.GrooveKind
			}
			if p.GroovePct != 0 {
				en.GroovePct = p.GroovePct
			}
			if len(p.EffectOverrides) > 0 {
				en.EffectOverrides = p.EffectOverrides
			}
			// Phase 3: emit SynthOverrides as a single map; never write the
			// legacy individual fields. Old loaders that look for synth_decay
			// etc. simply see them absent and treat the node as "no synth
			// override", which is the same observable result as the legacy
			// "field is zero" semantics.
			if over := synthOverridesFromNodeParams(p); len(over) > 0 {
				en.SynthOverrides = over
			}
		}
		nodes = append(nodes, en)
	}
	insts := make([]exportInstrument, 0, len(dv.Rows))
	for _, r := range dv.Rows {
		kind := kindForID(r.Instrument)
		ei := exportInstrument{
			Name:   audio.InstrumentDisplayName(r.Instrument),
			ID:     r.Instrument,
			Kind:   kind,
			Volume: r.Volume,
			Origin: int(r.Origin),
			Color:  hexColor(r.Color),
		}
		// Check samplePath for custom uploaded instruments
		if dv.samplePath != nil {
			if p, ok := dv.samplePath[r.Instrument]; ok && p != "" {
				ei.Path = p
				// Custom uploads are samples even if kindForID says builtin
				ei.Kind = "sample"
			}
		}
		// For catalog-based samples without a custom path, try catalog lookup
		if ei.Path == "" && (kind == "sample" || ei.Kind == "sample") {
			if meta, ok := audio.CatalogLookup(r.Instrument); ok && meta.Path != "" {
				ei.Path = meta.Path
			}
		}
		// Sampler-created instruments embed their baked PCM directly; this
		// supersedes any path so the sound travels inside the project file.
		if rec, ok := audio.UserSamplePCM(r.Instrument); ok && len(rec.PCM) > 0 {
			ei.Kind = "sample"
			ei.Path = ""
			ei.PCM = encodeSamplePCM(rec)
		}
		// Export pan and send levels when non-default.
		if r.Pan != 0 {
			ei.Pan = r.Pan
		}
		if r.DelaySend != 0 {
			ei.DelaySend = r.DelaySend
		}
		if r.ReverbSend != 0 {
			ei.ReverbSend = r.ReverbSend
		}
		// Export per-instrument EQ if any band is non-zero, muted, or filters enabled
		hasNonZeroGain := false
		for _, g := range r.EQGainsDB {
			if g != 0 {
				hasNonZeroGain = true
				break
			}
		}
		hasMutedBand := false
		for _, m := range r.EQBandMuted {
			if m {
				hasMutedBand = true
				break
			}
		}
		hasFilters := r.HPFEnabled || r.LPFEnabled
		// Export per-instrument insert effects if any. Read the AUDIO layer,
		// not r.Effects: the FX panel's param slider writes only to the audio
		// chain (propagateFXSliderValue → audio.SetInsertEffectParam, no row
		// re-sync), so the row copy can hold pre-edit values. The audio layer
		// is what the mixer renders — it is the tone the file must reproduce.
		if fx := audio.GetInsertEffects(r.Instrument); len(fx) > 0 {
			ei.Effects = fx
		}
		if hasNonZeroGain || hasMutedBand || hasFilters {
			eq := exportEQ{}
			eq.GainsDB = append(eq.GainsDB, r.EQGainsDB...)
			for _, b := range eqBandDefs {
				eq.BandsHz = append(eq.BandsHz, [2]float64{b.loHz, b.hiHz})
			}
			if hasMutedBand {
				eq.BandMuted = append(eq.BandMuted, r.EQBandMuted...)
			}
			eq.HPFEnabled = r.HPFEnabled
			eq.HPFCutoffHz = r.HPFCutoffHz
			eq.LPFEnabled = r.LPFEnabled
			eq.LPFCutoffHz = r.LPFCutoffHz
			ei.EQ = &eq
		}
		// Phase 3 synth-recipe metadata: the recipe binding + the instrument's
		// effective synth tone. Recipe is included even when no params have been
		// set so a reload preserves the binding.
		//
		// SynthParams captures the EFFECTIVE tone as a delta from the recipe's
		// SHIPPED defaults — not just the per-instrument overlay. A Synth-tab
		// "Save" bakes the tone into the recipe's registered defaults (persisted
		// in userprefs, not the project); after an app restart the overlay is
		// empty while the recipe default stays customized, so exporting the raw
		// overlay would silently drop the Saved tone. Folding the recipe-default
		// delta into SynthParams makes the project self-describing: importing it
		// onto a shipped-default recipe (fresh instance / other machine) applies
		// the delta as an overlay and reproduces the exact sound. For an
		// uncustomized recipe the delta equals the raw overlay, so default
		// projects stay byte-identical.
		if recipe := audio.RecipeForInstrument(r.Instrument); recipe != "" {
			ei.Recipe = recipe
			effective := audio.MergeRecipeDefaults(recipe, audio.GetInstrumentParams(r.Instrument))
			shipped := audio.RecipeShippedDefaults(recipe)
			delta := map[string]float64{}
			for k, v := range effective {
				if s, ok := shipped[k]; !ok || math.Abs(v-s) > 1e-9 {
					delta[k] = v
				}
			}
			if len(delta) > 0 {
				ei.SynthParams = delta
			}
		} else if params := audio.GetInstrumentParams(r.Instrument); len(params) > 0 {
			// No recipe binding: fall back to the raw overlay.
			ei.SynthParams = params
		}
		// Non-destructive Sampler edit: travels with the project so the saved
		// chop reproduces on import while the recipe binding above keeps the
		// synth editable.
		if e, ok := audio.SampleEditFor(r.Instrument); ok {
			ei.SampleEdit = e.Fields()
		}
		// Phase 6: per-row explicit kit role tag. Omitted when empty so
		// pre-Phase-6 projects stay byte-identical to their old export.
		if r.Role != "" {
			ei.Role = r.Role
		}
		insts = append(insts, ei)
	}
	file := exportFile{Version: 1, Subdiv: currentMaxDiv(), BPM: dv.BPM(), Instruments: insts, Nodes: nodes}
	// Phase 6: embed every registered kit. KitsForExport returns a
	// defensive copy; the field stays omitempty so projects with no
	// kits round-trip identically to the pre-Phase-6 shape.
	if kits := audio.KitsForExport(); len(kits) > 0 {
		file.Kits = kits
	}
	if pins := dv.exportPinnedInstrumentIDs(); len(pins) > 0 {
		file.PinnedInstruments = pins
	}
	// Export master volume when not at default (1.0).
	mv := audio.MainVolume()
	if mv != 1 {
		file.MasterVolume = mv
	}
	// Export master EQ if any band has non-zero gain, is muted, or filters enabled.
	masterHasNonZeroGain := false
	for _, g := range dv.eqBandGainsDB() {
		if g != 0 {
			masterHasNonZeroGain = true
			break
		}
	}
	masterHasMutedBand := false
	for _, m := range dv.eqBandMuted() {
		if m {
			masterHasMutedBand = true
			break
		}
	}
	masterHasFilters := dv.hpfEnabled || dv.lpfEnabled
	if masterHasNonZeroGain || masterHasMutedBand || masterHasFilters {
		eq := exportEQ{}
		eq.GainsDB = append(eq.GainsDB, dv.eqBandGainsDB()...)
		for _, b := range eqBandDefs {
			eq.BandsHz = append(eq.BandsHz, [2]float64{b.loHz, b.hiHz})
		}
		if masterHasMutedBand {
			eq.BandMuted = append(eq.BandMuted, dv.eqBandMuted()...)
		}
		eq.HPFEnabled = dv.hpfEnabled
		eq.HPFCutoffHz = dv.hpfCutoffHz
		eq.LPFEnabled = dv.lpfEnabled
		eq.LPFCutoffHz = dv.lpfCutoffHz
		file.EQ = &eq
	}
	// Export send effects configuration.
	dlyTime, dlyFb, dlyDamp := audio.SendDelayParams()
	revRoom, revDamp, revWet := audio.SendReverbParams()
	file.SendEffects = &SendEffectsConfig{
		Delay: &SendDelayConfig{
			TimeMs:    dlyTime,
			Feedback:  dlyFb,
			DampingHz: dlyDamp,
		},
		Reverb: &SendReverbConfig{
			Room:    revRoom,
			Damping: revDamp,
			Wet:     revWet,
		},
	}
	return json.MarshalIndent(file, "", "  ")
}

// Allow tests to override save sink.
func SetSaveJSONForTest(fn func(name string, data []byte) error) (restore func()) {
	prev := saveJSON
	saveJSON = fn
	return func() { saveJSON = prev }
}

// saveJSONDefault is defined in platform-specific files.
