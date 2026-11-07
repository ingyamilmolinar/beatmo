package audio

// WavResolveStatus is the outcome of resolving a WavRef against its sources.
type WavResolveStatus int

const (
	// No source produced bytes.
	WavResolveMissing WavResolveStatus = iota
	// Bytes matched the pinned Sha256; PCM/SampleRate are set.
	WavResolveMatched
	// Bytes were found but none matched; what was found is reported so the caller can offer "use current version anyway".
	WavResolveMismatch
)

// WavResolveResult reports what ResolveWavRef found.
type WavResolveResult struct {
	Status WavResolveStatus
	Data   []byte    // raw WAV bytes the matching/first source produced
	Source WavSource // the source that produced bytes (Matched/Mismatch)
	GotSha string    // content hash of the produced bytes (Matched/Mismatch)
}

// Raw bytes, not PCM, so the pin is a decoder-independent file version: decoding happens only after the hash matches.
type WavLocator func(ref string) (data []byte, ok bool)

// First hash match wins; bytes with no match → Mismatch; nothing → Missing. A kind with no locator is skipped, so
// future kinds degrade gracefully on older builds.
func ResolveWavRef(ref *WavRef, locators map[WavSourceKind]WavLocator) WavResolveResult {
	if ref == nil {
		return WavResolveResult{Status: WavResolveMissing}
	}
	var mismatch *WavResolveResult
	for _, src := range ref.Sources {
		loc, ok := locators[src.Kind]
		if !ok || loc == nil {
			continue
		}
		data, ok := loc(src.Ref)
		if !ok || len(data) == 0 {
			continue
		}
		got := Sha256Hex(data)
		if got == ref.Sha256 {
			return WavResolveResult{Status: WavResolveMatched, Data: data, Source: src, GotSha: got}
		}
		if mismatch == nil {
			m := WavResolveResult{Status: WavResolveMismatch, Data: data, Source: src, GotSha: got}
			mismatch = &m
		}
	}
	if mismatch != nil {
		return *mismatch
	}
	return WavResolveResult{Status: WavResolveMissing}
}
