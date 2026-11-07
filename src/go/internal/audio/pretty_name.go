package audio

import "strings"

var prettyNameReplacer = strings.NewReplacer("_", " ", "-", " ")

// "open_hi_hat" -> "Open Hi Hat". The input is opaque: callers strip a file extension themselves.
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
