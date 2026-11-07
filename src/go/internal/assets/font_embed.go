package assets

import _ "embed"

// SUBSETS of stock Inter (make gen-fonts): main.wasm ships every embedded byte to every visitor, and the gzip budget is
// tight. Only EN and ES ship; a non-Latin locale swaps the whole face through i18n's FontProvider rather than widening
// these. fonts_coverage_test.go pins what they must contain.

//go:embed fonts/Inter-Regular.subset.ttf
var GoRegularTTF []byte

//go:embed fonts/Inter-SemiBold.subset.ttf
var GoBoldTTF []byte
