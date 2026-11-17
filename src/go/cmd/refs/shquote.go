//go:build !js

package main

import "strings"

// shquote.go — safe single-quoting for the `-sh` plans.
//
// Everything `-sh` prints is consumed by `eval` in the Makefile recipes, so an
// unescaped value is not a formatting nit: a single quote anywhere in a path
// ends the quoted string and the remainder is executed as shell. The paths are
// built from $BEATMO_WAV_REFS, which the Makefile defaults to $(HOME)/... and
// the user can point anywhere.

// shQuote wraps s in single quotes, escaping any embedded single quote with
// the standard '\” idiom (close, escaped quote, reopen). The result is always
// exactly one shell word.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
