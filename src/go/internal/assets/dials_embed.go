package assets

import _ "embed"

// SynthDialsJSON is the Synth tab's macro-dial metadata: each dial is a vector
// over existing synth params (internal/synthdial). Never written to a project.
//
//go:embed synth_dials.json
var SynthDialsJSON []byte
