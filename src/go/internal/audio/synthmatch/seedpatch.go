package synthmatch

import (
	"fmt"
	"go/format"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// PatchSeedBlock returns src with the `var <varName> = RecipeParams{ … }` block
// updated: existing keys in `tuned` get their values replaced; keys not present
// are inserted before the closing brace. Comments, key order, and untouched keys
// are preserved. The result is gofmt-formatted. Errors if the var block is absent.
func PatchSeedBlock(src []byte, varName string, tuned map[string]float64) ([]byte, error) {
	s := string(src)

	// Locate the opening of the var block.
	opener := "var " + varName + " = RecipeParams{"
	openIdx := strings.Index(s, opener)
	if openIdx < 0 {
		return nil, fmt.Errorf("seedpatch: var block %q not found", varName)
	}

	// Find the position of the opening brace (end of opener string - 1).
	braceStart := openIdx + len(opener) - 1 // points to the '{' character

	// Scan forward to find the matching closing brace, tracking depth.
	depth := 0
	closeIdx := -1
	for i := braceStart; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				closeIdx = i
			}
		}
		if closeIdx >= 0 {
			break
		}
	}
	if closeIdx < 0 {
		return nil, fmt.Errorf("seedpatch: closing brace not found for var block %q", varName)
	}

	// Extract the block contents (between the braces, exclusive).
	blockBody := s[braceStart+1 : closeIdx]

	// For each tuned key that exists in the block, replace its value.
	// Track which keys were found vs need to be inserted.
	inserted := make(map[string]bool)

	for key, val := range tuned {
		// Build a regexp that matches: "<key>": <value> and captures the trailing
		// context (optional whitespace then comma, // comment, or end-of-line) so
		// we can reattach it verbatim. This avoids the old [^,\n/]+ pattern that
		// stopped at the first '/' inside a trailing comment such as "// Hz / sec".
		//
		// Group 1: key + colon + spaces
		// Group 2: the numeric value token (digits, sign, dot, e/E exponent — no slash)
		// Group 3: everything after the value up to (but not including) the newline,
		//          i.e. optional whitespace + optional comma + optional " // …" comment
		pattern := `("` + regexp.QuoteMeta(key) + `"\s*:\s*)([\-+]?[0-9]*\.?[0-9]+(?:[eE][\-+]?[0-9]+)?)([ \t]*,?[ \t]*(?://[^\n]*)?)`
		re := regexp.MustCompile(pattern)

		formatted := strconv.FormatFloat(val, 'g', -1, 64)

		if re.MatchString(blockBody) {
			blockBody = re.ReplaceAllStringFunc(blockBody, func(match string) string {
				submatches := re.FindStringSubmatch(match)
				if len(submatches) < 4 {
					return match
				}
				prefix := submatches[1]   // "filter_cutoff": _
				trailing := submatches[3] // , // bright / open
				return prefix + formatted + trailing
			})
			inserted[key] = true
		}
	}

	// For keys NOT found in the block, insert them before the closing brace.
	// Build the insertions in a deterministic order (sort by key name for stability).
	// We use a simple approach: collect keys to insert, then append them.
	var toInsert []string
	for key := range tuned {
		if !inserted[key] {
			toInsert = append(toInsert, key)
		}
	}
	// Sort for determinism.
	sortStrings(toInsert)

	if len(toInsert) > 0 {
		var sb strings.Builder
		for _, key := range toInsert {
			val := tuned[key]
			formatted := strconv.FormatFloat(val, 'g', -1, 64)
			sb.WriteString("\t\"")
			sb.WriteString(key)
			sb.WriteString("\": ")
			sb.WriteString(formatted)
			sb.WriteString(",\n")
		}
		blockBody = blockBody + sb.String()
	}

	// Reconstruct the full source.
	result := []byte(s[:braceStart+1] + blockBody + s[closeIdx:])

	// Return gofmt-formatted output so a patched instrument_seeds.go stays
	// formatting-clean (the repo has gofmt discipline; an un-aligned insert would
	// show up as formatting noise in the diff / trip gofmt checks).
	formatted, err := format.Source(result)
	if err != nil {
		return nil, fmt.Errorf("seedpatch: go/format failed: %w", err)
	}
	return formatted, nil
}

// sortStrings sorts a string slice in place using a simple insertion sort.
// Avoids importing "sort" package (stdlib is fine, but keeping minimal).
func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		key := ss[i]
		j := i - 1
		for j >= 0 && ss[j] > key {
			ss[j+1] = ss[j]
			j--
		}
		ss[j+1] = key
	}
}

// PatchSeedFile reads path, applies PatchSeedBlock, and writes it back in place.
func PatchSeedFile(path, varName string, tuned map[string]float64) error {
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("seedpatch: read %q: %w", path, err)
	}
	out, err := PatchSeedBlock(src, varName, tuned)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("seedpatch: write %q: %w", path, err)
	}
	return nil
}
