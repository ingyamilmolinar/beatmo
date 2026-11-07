package synthmatch

import (
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Shared by the production and test renderers so param resolution is identical in both builds.
func resolveRender(instID instrument.ID, pitch float64, sr int, durSec float64, overrides map[string]float64) (fields map[string]float64, merged map[string]float64, samples int, err error) {
	fields = make(map[string]float64, len(overrides)+1)
	for k, v := range overrides {
		fields[k] = v
	}
	fields["pitch"] = pitch
	d, err := audio.InstrumentDocWithFields(instID, fields)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("synthmatch: %w", err)
	}
	merged = instrument.FieldValues(d, audio.BuiltinModuleRegistry())
	if durSec <= 0 {
		durSec = audio.ConfigForInstrument(instID).DurationSec
	}
	samples = int(float64(sr) * durSec)
	if samples < 1 {
		return nil, nil, 0, fmt.Errorf("synthmatch: 0 samples for dur=%v sr=%d", durSec, sr)
	}
	return fields, merged, samples, nil
}
