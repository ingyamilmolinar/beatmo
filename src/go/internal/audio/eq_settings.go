package audio

import (
	"math"
	"slices"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// An instrument's EQ as the user set it; its compiled biquad form is what reaches the channel.

// The last capped at 20 kHz.
var EQBandEdges = [EQBandCount][2]float64{
	{22, 44}, {44, 88}, {88, 177}, {177, 354}, {354, 707},
	{707, 1414}, {1414, 2828}, {2828, 5657}, {5657, 11314}, {11314, 20000},
}

const (
	EQBandCount    = 10
	EQHPFDefaultHz = 20.0    // a high-pass at its floor filters nothing
	EQLPFDefaultHz = 20000.0 // a low-pass at its ceiling filters nothing
	eqBandQ        = 1.414   // one-octave graphic bands
	eqFilterQ      = 0.707
)

// EQSettings is one EQ as set: GainsDB and BandMuted have EQBandCount entries.
type EQSettings struct {
	GainsDB   []float64 `json:"gains_db"`
	BandMuted []bool    `json:"band_muted"`
	HPF       bool      `json:"hpf,omitempty"`
	HPFHz     float64   `json:"hpf_hz,omitempty"`
	LPF       bool      `json:"lpf,omitempty"`
	LPFHz     float64   `json:"lpf_hz,omitempty"`
}

// NewEQSettings is a flat EQ.
func NewEQSettings() *EQSettings {
	return &EQSettings{GainsDB: make([]float64, EQBandCount), BandMuted: make([]bool, EQBandCount), HPFHz: EQHPFDefaultHz, LPFHz: EQLPFDefaultHz}
}

// IsFlat: every gain 0, nothing muted, no filter that filters.
func (s *EQSettings) IsFlat() bool {
	if s == nil {
		return true
	}
	for i := range s.GainsDB {
		if s.GainsDB[i] != 0 {
			return false
		}
	}
	return !slices.Contains(s.BandMuted, true) && !s.hpfActive() && !s.lpfActive()
}

// A cutoff of 0 is unset (the filter's default): it filters nothing.
func (s *EQSettings) hpfActive() bool { return s.HPF && s.HPFHz > EQHPFDefaultHz }
func (s *EQSettings) lpfActive() bool { return s.LPF && s.LPFHz > 0 && s.LPFHz < EQLPFDefaultHz }

// Clone is a deep copy.
func (s *EQSettings) Clone() *EQSettings {
	if s == nil {
		return nil
	}
	c := *s
	c.GainsDB = slices.Clone(s.GainsDB)
	c.BandMuted = slices.Clone(s.BandMuted)
	return &c
}

// [high-pass] → low shelf, peaking bands, high shelf → [low-pass].
func (s *EQSettings) Bands() []EQBand {
	bands := make([]EQBand, 0, EQBandCount+2)
	if s.hpfActive() {
		bands = append(bands, EQBand{Kind: EQHighpass, Freq: s.HPFHz, Q: eqFilterQ})
	}
	for i, e := range EQBandEdges {
		kind := EQPeaking
		switch i {
		case 0:
			kind = EQLowShelf
		case EQBandCount - 1:
			kind = EQHighShelf
		}
		b := EQBand{Kind: kind, Freq: math.Sqrt(e[0] * e[1]), Q: eqBandQ}
		if i < len(s.GainsDB) {
			b.GainDB = s.GainsDB[i]
		}
		b.Muted = i < len(s.BandMuted) && s.BandMuted[i]
		bands = append(bands, b)
	}
	if s.lpfActive() {
		bands = append(bands, EQBand{Kind: EQLowpass, Freq: s.LPFHz, Q: eqFilterQ})
	}
	return bands
}

var eqSettings = struct {
	sync.Mutex
	byInst map[instrument.ID]*EQSettings
}{byInst: map[instrument.ID]*EQSettings{}}

// Created flat on first use; the pointer is stable because the UI edits it in place.
func EQSettingsOf(id instrument.ID) *EQSettings {
	eqSettings.Lock()
	defer eqSettings.Unlock()
	s := eqSettings.byInst[id]
	if s == nil {
		s = NewEQSettings()
		eqSettings.byInst[id] = s
	}
	return s
}

// A flat EQ installs none: a unity chain differs from an absent one in the last bits, and export→import must stay
// bit-exact. The analyzers in the same list are re-enabled.
func ApplyEQSettings(id instrument.ID) {
	s := EQSettingsOf(id)
	ch := channel.Of(id)
	if s.IsFlat() {
		ClearChannelProcessors(ch)
	} else {
		SetChannelEQ(ch, SampleRate(), s.Bands()...)
	}
	_ = EnableChannelAnalyzer(ch, 512)
	_ = EnablePreEQAnalyzer(ch, 512)
	_ = EnableSynthAnalyzer(ch, 512)
}

// No channel may keep filtering behind a store that reads flat.
func ResetEQSettings() {
	eqSettings.Lock()
	var had []instrument.ID
	for id, s := range eqSettings.byInst {
		if !s.IsFlat() {
			had = append(had, id)
		}
	}
	eqSettings.byInst = map[instrument.ID]*EQSettings{}
	eqSettings.Unlock()
	for _, id := range had {
		ApplyEQSettings(id)
	}
}
