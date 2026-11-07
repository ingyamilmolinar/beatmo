package synthmatch

import (
	"fmt"
	"os"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// A field that cannot land is an error, never a silent skip. An empty patch re-encodes a shipped document byte-identically.
func PatchInstrumentDoc(data []byte, tuned map[string]float64) ([]byte, error) {
	d, err := instrument.ParseFile(data)
	if err != nil {
		return nil, err
	}
	reg := audio.BuiltinModuleRegistry()
	d, lost, err := audio.ApplyInstrumentFields(d, tuned)
	if err != nil {
		return nil, fmt.Errorf("docpatch: %w", err)
	}
	if len(lost) > 0 {
		return nil, fmt.Errorf("docpatch: %s cannot take %v: %w", d.ID, lost, instrument.ErrInactive)
	}
	if err := instrument.Validate(d, reg); err != nil {
		return nil, fmt.Errorf("docpatch: %s: %w", d.ID, err)
	}
	return instrument.MarshalFile(d)
}

// The pack is NOT rebuilt here: run `make asset-pack` after.
func PatchInstrumentDocFile(path string, tuned map[string]float64) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("docpatch: read %q: %w", path, err)
	}
	out, err := PatchInstrumentDoc(data, tuned)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("docpatch: write %q: %w", path, err)
	}
	return nil
}
