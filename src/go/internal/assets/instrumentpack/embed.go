package instrumentpack

import _ "embed"

//go:embed instruments.pack.json.gz
var embedded []byte

// Embedded is the gzip'd instrument pack built from
// internal/assets/instruments by `make asset-pack`.
func Embedded() []byte { return embedded }
