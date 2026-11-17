package assets

import _ "embed"

// SynthDialRelevanceJSON is the generated verdict of which macro dials the DSP
// finds inert on each shipped instrument sound (internal/ui
// synth_dial_relevance.go). Regenerate with `make gen-dial-relevance`.
//
//go:embed synth_dial_relevance.gen.json
var SynthDialRelevanceJSON []byte
