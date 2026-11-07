package audio

import "sync/atomic"

var catalogVersion atomic.Uint64
var instrumentsVersion atomic.Uint64

func bumpCatalogVersion() {
	catalogVersion.Add(1)
}

func bumpInstrumentsVersion() {
	instrumentsVersion.Add(1)
}

func CatalogVersion() uint64 {
	return catalogVersion.Load()
}

func InstrumentsVersion() uint64 {
	return instrumentsVersion.Load()
}
