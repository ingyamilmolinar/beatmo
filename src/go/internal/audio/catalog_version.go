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

// CatalogVersion returns a monotonically increasing version that changes
// whenever the catalog contents are replaced.
func CatalogVersion() uint64 {
	return catalogVersion.Load()
}

// InstrumentsVersion returns a monotonically increasing version that changes
// whenever the registered instrument list changes.
func InstrumentsVersion() uint64 {
	return instrumentsVersion.Load()
}
