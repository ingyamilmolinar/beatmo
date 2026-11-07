//go:build !js

package main

import "strings"

// Everything -sh prints is eval'd by the Makefile, so a single quote in a path (built from a user-chosen
// $BEATMO_WAV_REFS) would end the string and run the rest as shell.

// Close the quote, emit a backslash-escaped quote, reopen: the result is always exactly one shell word.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
