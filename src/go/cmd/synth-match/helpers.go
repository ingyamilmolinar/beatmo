//go:build !js

// helpers.go contains pure utility functions for synth-match that have no
// dependency on the audio engine or OS and are therefore testable under
// -tags test (unlike main.go which requires the full non-test audio runtime).

package main

import (
	"strings"
	"unicode"
)

// seedVarName converts an instrument id to a Go variable name in camelCase
// with "Seed" appended.
// Examples:
//
//	"sax"             → "saxSeed"
//	"guitar-electric" → "guitarElectricSeed"
//	"fm-bell"         → "fmBellSeed"
//	"--sax"           → "saxSeed"
//	"foo--bar"        → "fooBarSeed"
func seedVarName(instID string) string {
	parts := strings.FieldsFunc(instID, func(r rune) bool {
		return r == '-' || r == '_' || unicode.IsSpace(r)
	})
	var sb strings.Builder
	first := true
	for _, p := range parts {
		if p == "" {
			continue
		}
		if first {
			sb.WriteString(strings.ToLower(p))
			first = false
		} else {
			runes := []rune(p)
			runes[0] = unicode.ToUpper(runes[0])
			sb.WriteString(string(runes))
		}
	}
	sb.WriteString("Seed")
	return sb.String()
}
