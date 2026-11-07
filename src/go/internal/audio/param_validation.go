package audio

import (
	"fmt"
	"math"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// modularSchemaNames is every param the modular engine has (its block layout).
var modularSchemaNames = func() map[string]bool {
	m := make(map[string]bool, len(modularParamSchema))
	for _, n := range modularParamSchema {
		m[n] = true
	}
	return m
}()

// A runtime plugin recipe may define its own names.
func knownParamName(instrumentID instrument.ID, name string) bool {
	return modularSchemaNames[name] || lookupParamDef(instrumentID, name).Name != ""
}

// A name the block does not have, a non-finite value, or a family voice in a slot its family has no fields for. The
// error names the first offender, sorted so the message is stable. Every boundary that accepts params from outside
// the running app goes through this.
func ValidateRecipeParams(p map[string]float64) error {
	if err := validateParamNames(p); err != nil {
		return err
	}
	for _, k := range sortedKeys(p) {
		if v := p[k]; math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("synth param %q is not a finite number", k)
		}
	}
	return nil
}

// For boundaries that already strip non-finite values themselves.
func validateParamNames(p map[string]float64) error {
	for _, k := range sortedKeys(p) {
		if !modularSchemaNames[k] {
			return fmt.Errorf("unknown synth param %q", k)
		}
	}
	return validateSlotCapacity(RecipeParams(p))
}

func sortedKeys(p map[string]float64) []string {
	names := make([]string, 0, len(p))
	for k := range p {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// isFiniteParam reports whether v is a usable param value (not NaN or ±Inf).
func isFiniteParam(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
