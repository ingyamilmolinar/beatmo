//go:build ignore

// gen_instrument_ids.go reads the instruments map from engine_instruments.go
// (the actual synth implementations — the real source of truth) and generates
// instrument_ids_gen.go with a BuiltinInstrumentIDs slice that every platform
// file can reference.
//
// It also reads startup_demo.json and fails if the demo references any
// instrument ID that doesn't exist in the synth map — catching the class of
// bug where the demo JSON is updated but the synth registry is not.
//
// Usage:
//
//	go run gen_instrument_ids.go
package main

import (
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"regexp"
	"strings"
)

// mapKeyRe matches `"instrument-id":` entries inside the instruments map literal.
var mapKeyRe = regexp.MustCompile(`^\s+"([a-z][a-z0-9-]*)"\s*:`)

// idRe validates that an instrument ID is well-formed.
var idRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func main() {
	ids, err := extractMapKeys("engine_instruments.go")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error extracting map keys: %v\n", err)
		os.Exit(1)
	}
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "error: no instrument IDs found in engine_instruments.go")
		os.Exit(1)
	}

	// The config-first modular instruments (kick family, …) are not literals in
	// engine_instruments.go — they're rows in modular_instruments.go's table,
	// added to the engine map via modularTableVoices. Extract their IDs too so
	// BuiltinInstrumentIDs stays complete. Appended after the literals, matching
	// the modularTableVoices(instruments) call order in ResetInstruments.
	tableIDs, err := extractTableIDs("modular_instruments.go")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error extracting modular-instrument table IDs: %v\n", err)
		os.Exit(1)
	}
	ids = append(ids, tableIDs...)

	// Validate: no duplicates.
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			fmt.Fprintf(os.Stderr, "error: duplicate instrument ID %q\n", id)
			os.Exit(1)
		}
		seen[id] = true
	}

	// Validate: startup demo instruments are all present.
	demoIDs, err := extractDemoInstrumentIDs("../assets/startup_demo.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading startup_demo.json: %v\n", err)
		os.Exit(1)
	}
	for _, id := range demoIDs {
		if !seen[id] {
			fmt.Fprintf(os.Stderr, "error: startup_demo.json references instrument %q which is not in engine_instruments.go\n", id)
			os.Exit(1)
		}
	}

	// Generate output.
	if err := writeGenFile("instrument_ids_gen.go", ids); err != nil {
		fmt.Fprintf(os.Stderr, "error writing generated file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("generated instrument_ids_gen.go with %d instruments (validated against %d startup demo IDs)\n", len(ids), len(demoIDs))
}

// tableIDRe matches a `{ID: "instrument-id",` row inside modularInstrumentDefs.
var tableIDRe = regexp.MustCompile(`^\s*\{ID:\s*"([a-z][a-z0-9-]*)"`)

// extractTableIDs reads modular_instruments.go and returns the instrument IDs
// from the `modularInstrumentDefs = []ModularInstrumentDef{...}` table, in
// declaration order. This is the config-first source for those instruments.
func extractTableIDs(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var ids []string
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inTable {
			if strings.Contains(trimmed, "modularInstrumentDefs = []ModularInstrumentDef{") {
				inTable = true
			}
			continue
		}
		if trimmed == "}" {
			break
		}
		if m := tableIDRe.FindStringSubmatch(line); m != nil {
			id := m[1]
			if !idRe.MatchString(id) {
				return nil, fmt.Errorf("invalid instrument ID %q", id)
			}
			ids = append(ids, id)
		}
	}
	if !inTable {
		return nil, fmt.Errorf("could not find `modularInstrumentDefs = []ModularInstrumentDef{` in %s", filename)
	}
	return ids, nil
}

// extractMapKeys reads engine_instruments.go and returns the instrument IDs
// from the `instruments = map[string]Instrument{...}` block, in declaration order.
func extractMapKeys(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	var ids []string
	inMap := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect start of the instruments map.
		if !inMap && strings.Contains(trimmed, "instruments = map[string]Instrument{") {
			inMap = true
			continue
		}

		if !inMap {
			continue
		}

		// Detect end of map: a line that is just "}" (possibly with whitespace).
		// The map close is indented with a single tab, matching the opening brace level.
		if trimmed == "}" {
			break
		}

		// Extract map key.
		if m := mapKeyRe.FindStringSubmatch(line); m != nil {
			id := m[1]
			if !idRe.MatchString(id) {
				return nil, fmt.Errorf("invalid instrument ID %q", id)
			}
			ids = append(ids, id)
		}
	}

	if !inMap {
		return nil, fmt.Errorf("could not find `instruments = map[string]Instrument{` in %s", filename)
	}

	return ids, nil
}

// demoJSON is the minimal structure needed to extract instrument IDs.
type demoJSON struct {
	Instruments []struct {
		ID string `json:"id"`
	} `json:"instruments"`
}

// extractDemoInstrumentIDs reads startup_demo.json and returns the instrument IDs.
func extractDemoInstrumentIDs(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var demo demoJSON
	if err := json.Unmarshal(data, &demo); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", filename, err)
	}

	ids := make([]string, 0, len(demo.Instruments))
	for _, inst := range demo.Instruments {
		if inst.ID == "" {
			return nil, fmt.Errorf("%s contains instrument with empty ID", filename)
		}
		ids = append(ids, inst.ID)
	}
	return ids, nil
}

func writeGenFile(filename string, ids []string) error {
	var buf strings.Builder
	buf.WriteString("// Code generated by gen_instrument_ids.go from engine_instruments.go; DO NOT EDIT.\n\n")
	buf.WriteString("package audio\n\n")
	buf.WriteString("// BuiltinInstrumentIDs is the canonical ordered list of all built-in\n")
	buf.WriteString("// instrument IDs derived from the synth implementations in\n")
	buf.WriteString("// engine_instruments.go (the single source of truth).\n")
	buf.WriteString("var BuiltinInstrumentIDs = []string{\n")
	for _, id := range ids {
		fmt.Fprintf(&buf, "\t%q,\n", id)
	}
	buf.WriteString("}\n")

	formatted, err := format.Source([]byte(buf.String()))
	if err != nil {
		return fmt.Errorf("gofmt: %w", err)
	}

	return os.WriteFile(filename, formatted, 0644)
}
