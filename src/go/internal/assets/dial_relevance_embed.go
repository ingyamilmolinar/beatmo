package assets

import _ "embed"

// Which macro dials the DSP finds inert on each shipped sound. Regenerate with make gen-dial-relevance.
//
//go:embed synth_dial_relevance.gen.json
var SynthDialRelevanceJSON []byte
