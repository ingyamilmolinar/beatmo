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

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/enum"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

type exportInstrument struct {
	Name    string             `json:"name"`
	ID      instrument.ID      `json:"id"`
	Kind    instrumentKind     `json:"kind"`
	Volume  float64            `json:"volume"`
	Origin  int                `json:"origin"`
	Color   string             `json:"color"`             // #RRGGBBAA
	Path    string             `json:"path,omitempty"`    // local cache path or object URL for custom samples
	EQ      *exportEQ          `json:"eq,omitempty"`      // per-instrument EQ settings
	Effects []audio.EffectSlot `json:"effects,omitempty"` // per-instrument insert effect chain
	// A post-render transform, not baked PCM: the synth stays the source of truth. Present only for a saved edit.
	SampleEdit map[string]float64 `json:"sample_edit,omitempty"`
	// Empty falls back to the audio.RoleForInstrument heuristic.
	Role instrument.Role `json:"role,omitempty"`
	// Supersedes Path when present, so the sound travels inside the project file.
	PCM *exportSamplePCM `json:"pcm,omitempty"`
}

// Little-endian float32 frames, base64.
type exportSamplePCM struct {
	SampleRate int    `json:"sample_rate"`
	Frames     int    `json:"frames"`
	DataB64    string `json:"data_b64"`
}

// Portable little-endian bytes (deterministic across platforms), gzipped so embedded one-shots do not bloat the file.
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

// !ok for an empty or malformed payload, so import falls back to the path/catalog strategies.
func decodeSamplePCM(p *exportSamplePCM) ([]float32, int, bool) {
	if p == nil || p.DataB64 == "" || p.Frames <= 0 {
		return nil, 0, false
	}
	raw, err := base64.StdEncoding.DecodeString(p.DataB64)
	if err != nil {
		return nil, 0, false
	}
	if len(raw) < 2 || raw[0] != 0x1f || raw[1] != 0x8b {
		return nil, 0, false
	}
	zr, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, false
	}
	defer zr.Close()
	if raw, err = io.ReadAll(zr); err != nil {
		return nil, 0, false
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

// exportGroupRule mirrors model.GroupRule for JSON persistence.
type exportGroupRule struct {
	Param     model.GroupParam `json:"param"`
	Delta     float64          `json:"delta"`
	EveryN    int              `json:"every_n"`
	Min       float64          `json:"min,omitempty"`
	Max       float64          `json:"max,omitempty"`
	AnchorIdx int              `json:"anchor_idx,omitempty"`
}

// RulesEnabled is a *bool: a plain bool + omitempty cannot tell "key absent" from "explicitly false". Export
// always writes it, so only foreign or hand-written files decode nil.
type exportGroup struct {
	ID           int               `json:"id"`
	Name         string            `json:"name,omitempty"`
	NodeIDs      []int             `json:"node_ids"`
	Rules        []exportGroupRule `json:"rules,omitempty"`
	RulesEnabled *bool             `json:"rules_enabled,omitempty"`
}

type exportNode struct {
	ID      int            `json:"id"`
	I       int            `json:"i"`
	J       int            `json:"j"`
	Type    model.NodeType `json:"type"`
	Inputs  []int          `json:"inputs,omitempty"`
	Outputs []int          `json:"outputs,omitempty"`
	// Optional per-node parameters. Omitted when at defaults.
	Volume   float64 `json:"volume,omitempty"`
	Pitch    float64 `json:"pitch,omitempty"`
	Duration float64 `json:"duration,omitempty"`
	// Node logic fields.
	LogicKind model.LogicKind `json:"logic_kind,omitempty"`
	LogicN    int             `json:"logic_n,omitempty"`
	LogicP    float64         `json:"logic_p,omitempty"`
	// Groove: per-node delay/rush and percentage (0..1)
	GrooveKind model.GrooveKind `json:"groove_kind,omitempty"`
	GroovePct  float64          `json:"groove_pct,omitempty"`
	// Per-node effect overrides (V1: model-only, no UI)
	EffectOverrides []model.EffectOverride `json:"effect_overrides,omitempty"`
	// Written for ANY node type carrying one: a chord collapsed to one note keeps its spec, and undo replays the file.
	Chord *exportChord `json:"chord,omitempty"`
}

// exportChord is a node's model.ChordSpec on disk.
type exportChord struct {
	Quality   string `json:"quality"`
	Inversion int    `json:"inversion,omitempty"`
	Spread    int    `json:"spread,omitempty"`
}

func (c *exportChord) spec() model.ChordSpec {
	if c == nil {
		return model.ChordSpec{}
	}
	return model.ChordSpec{Quality: c.Quality, Inversion: c.Inversion, Spread: c.Spread}
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

// Derived on export, checked on import.
type instrumentKind int

const (
	kindBuiltin instrumentKind = iota // the synth plays the instrument's document
	kindSample                        // embedded PCM or a WAV file
)

var instrumentKindNames = enum.Names[instrumentKind]{Kind: "instrument kind", Names: []string{"builtin", "sample"}}

func (k instrumentKind) String() string               { return enum.String(instrumentKindNames, k) }
func (k instrumentKind) MarshalText() ([]byte, error) { return enum.Marshal(instrumentKindNames, k) }
func (k *instrumentKind) UnmarshalText(b []byte) error {
	return enum.Unmarshal(instrumentKindNames, b, k)
}

// carriesSample reports whether the row carries its sound itself.
func (ei exportInstrument) carriesSample() bool { return ei.PCM != nil || ei.Path != "" }

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
	name := stampedFileName("beatmo-export", ".json")
	if err := saveJSON(name, data); err != nil {
		dv.logger.Errorf("[drumview] export failed: %v", err)
		return err
	}
	dv.logger.Debugf("[drumview] export completed: %s (%d bytes)", name, len(data))
	return nil
}

// Save function indirection for platform and tests.
var saveJSON = saveJSONDefault

// Game points it at the live grid's MaxDiv.
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
		en := exportNode{ID: id, I: n.I, J: n.J, Type: n.Type}
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
			if p.LogicKind != model.LogicNone {
				en.LogicKind = p.LogicKind
				if p.LogicN > 0 {
					en.LogicN = p.LogicN
				}
				if p.LogicP > 0 {
					en.LogicP = p.LogicP
				}
			}
			if p.GrooveKind != model.GrooveNone {
				en.GrooveKind = p.GrooveKind
			}
			if p.GroovePct != 0 {
				en.GroovePct = p.GroovePct
			}
			if len(p.EffectOverrides) > 0 {
				en.EffectOverrides = p.EffectOverrides
			}
			if c := model.NormalizeChord(p.Chord); !c.IsZero() {
				en.Chord = &exportChord{Quality: c.Quality, Inversion: c.Inversion, Spread: c.Spread}
			}
		}
		nodes = append(nodes, en)
	}
	// Invisible nodes are not exported, so members are filtered; a group that empties is skipped.
	exportedID := make(map[int]bool, len(nodes))
	for _, en := range nodes {
		exportedID[en.ID] = true
	}
	groups := make([]exportGroup, 0)
	for _, grp := range g.AllGroups() {
		enabled := grp.RulesEnabled
		eg := exportGroup{ID: int(grp.ID), Name: grp.Name, RulesEnabled: &enabled}
		for _, n := range grp.NodeIDs {
			if exportedID[int(n)] {
				eg.NodeIDs = append(eg.NodeIDs, int(n))
			}
		}
		if len(eg.NodeIDs) == 0 {
			continue
		}
		for _, r := range grp.Rules {
			eg.Rules = append(eg.Rules, exportGroupRule{
				Param: r.Param, Delta: r.Delta, EveryN: r.EveryN, Min: r.Min, Max: r.Max,
				AnchorIdx: r.AnchorIdx,
			})
		}
		groups = append(groups, eg)
	}
	insts := make([]exportInstrument, 0, len(dv.Rows))
	var docs map[instrument.ID]projectDoc
	for _, r := range dv.Rows {
		ei := exportInstrument{
			Name:   audio.InstrumentDisplayName(r.Instrument),
			ID:     r.Instrument,
			Volume: r.Volume,
			Origin: int(r.Origin),
			Color:  hexColor(r.Color),
		}
		// Check samplePath for custom uploaded instruments
		if dv.samplePath != nil {
			if p, ok := dv.samplePath[r.Instrument]; ok && p != "" {
				ei.Path = p
			}
		}
		// A catalog WAV without a custom path carries the catalog's path.
		if ei.Path == "" {
			if meta, ok := audio.CatalogLookup(r.Instrument); ok && meta.Source == audio.SourceWAV && meta.Path != "" {
				ei.Path = meta.Path
			}
		}
		// Embedded PCM supersedes any path, so the sound travels inside the file.
		if rec, ok := audio.UserSamplePCM(r.Instrument); ok && len(rec.PCM) > 0 {
			ei.Path = ""
			ei.PCM = encodeSamplePCM(rec)
		}
		ei.Kind = kindBuiltin
		if meta, ok := audio.CatalogLookup(r.Instrument); ei.carriesSample() || (ok && meta.Source == audio.SourceWAV) {
			ei.Kind = kindSample
		}
		// Export per-instrument EQ if any band is non-zero, muted, or filters enabled
		req := rowEQ(r)
		hasNonZeroGain := false
		for _, g := range req.GainsDB {
			if g != 0 {
				hasNonZeroGain = true
				break
			}
		}
		hasMutedBand := false
		for _, m := range req.BandMuted {
			if m {
				hasMutedBand = true
				break
			}
		}
		hasFilters := req.HPF || req.LPF
		// The instrument's insert effects, as the mixer renders them.
		if fx := audio.GetInsertEffects(r.Instrument); len(fx) > 0 {
			ei.Effects = fx
		}
		if hasNonZeroGain || hasMutedBand || hasFilters {
			eq := exportEQ{}
			eq.GainsDB = append(eq.GainsDB, req.GainsDB...)
			for _, b := range eqBandDefs {
				eq.BandsHz = append(eq.BandsHz, [2]float64{b.loHz, b.hiHz})
			}
			if hasMutedBand {
				eq.BandMuted = append(eq.BandMuted, req.BandMuted...)
			}
			eq.HPFEnabled, eq.HPFCutoffHz = req.HPF, req.HPFHz
			eq.LPFEnabled, eq.LPFCutoffHz = req.LPF, req.LPFHz
			ei.EQ = &eq
		}
		if d, ok := audio.InstrumentDoc(r.Instrument); ok {
			if docs == nil {
				docs = map[instrument.ID]projectDoc{}
			}
			docs[r.Instrument] = projectDoc{Doc: d}
		}
		// Travels with the project so the chop reproduces on import while the synth stays editable.
		if e, ok := audio.SampleEditFor(r.Instrument); ok {
			ei.SampleEdit = e.Fields()
		}
		if r.Role != instrument.RoleNone {
			ei.Role = r.Role
		}
		insts = append(insts, ei)
	}
	file := newProjectFile()
	file.Subdiv = currentMaxDiv()
	file.BPM = dv.BPM()
	file.Instruments = insts
	file.Nodes = nodes
	file.Groups = groups
	// The circuit's OWN scheme, never the user's effective theme.
	if g := dv.game; g != nil && !g.circuitTheme.IsEmpty() {
		cp := g.circuitTheme.Clone()
		file.Theme = &cp
	}
	file.InstrumentDocs = docs
	// KitsForExport returns a defensive copy.
	if kits := audio.KitsForExport(); len(kits) > 0 {
		file.Kits = kits
	}
	if pins := dv.exportPinnedInstrumentIDs(); len(pins) > 0 {
		file.PinnedInstruments = pins
	}
	// Always exported: sessions start at 0.5 (headroom), so omitting a "default" bakes a stale value into the file
	// and a 1.0 round-trip hard-clipped the master.
	mv := audio.MainVolume()
	file.MasterVolume = &mv
	// masterEQBandState, NOT eqBandGainsDB: the panel's working copy follows the channel pill, and exporting it gave
	// the master whatever curve the selected instrument had.
	masterGains, masterMuted := dv.masterEQBandState()
	masterHasNonZeroGain := false
	for _, g := range masterGains {
		if g != 0 {
			masterHasNonZeroGain = true
			break
		}
	}
	masterHasMutedBand := false
	for _, m := range masterMuted {
		if m {
			masterHasMutedBand = true
			break
		}
	}
	masterHasFilters := dv.hpfEnabled || dv.lpfEnabled
	if masterHasNonZeroGain || masterHasMutedBand || masterHasFilters {
		eq := exportEQ{}
		eq.GainsDB = append(eq.GainsDB, masterGains...)
		for _, b := range eqBandDefs {
			eq.BandsHz = append(eq.BandsHz, [2]float64{b.loHz, b.hiHz})
		}
		if masterHasMutedBand {
			eq.BandMuted = append(eq.BandMuted, masterMuted...)
		}
		eq.HPFEnabled = dv.hpfEnabled
		eq.HPFCutoffHz = dv.hpfCutoffHz
		eq.LPFEnabled = dv.lpfEnabled
		eq.LPFCutoffHz = dv.lpfCutoffHz
		file.EQ = &eq
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
