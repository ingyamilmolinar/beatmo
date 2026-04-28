package audio

import "strings"

var prettyNameReplacer = strings.NewReplacer("_", " ", "-", " ")

// PrettyName converts an instrument id or filename base into a
// human-readable title-cased label. Hyphens and underscores become
// spaces; each whitespace-separated word is capitalized.
//
//	"hi-hat"      -> "Hi Hat"
//	"open_hi_hat" -> "Open Hi Hat"
//	"kick-drum"   -> "Kick Drum"
//	""            -> ""
//
// Callers that pass file paths must strip the extension themselves —
// PrettyName treats its input as an opaque string.
func PrettyName(s string) string {
	s = prettyNameReplacer.Replace(s)
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) == 0 {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}
