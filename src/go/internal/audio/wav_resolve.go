package audio

// WavResolveStatus is the outcome of resolving a WavRef against its sources.
type WavResolveStatus int

const (
	// WavResolveMissing — no source produced bytes (file not found, no network,
	// unknown storage key, or no usable sources).
	WavResolveMissing WavResolveStatus = iota
	// WavResolveMatched — a source produced bytes whose content hash equals the
	// pinned Sha256. PCM/SampleRate are populated.
	WavResolveMatched
	// WavResolveMismatch — a source produced bytes, but none matched the pinned
	// version. PCM/SampleRate/GotSha describe what was found so the caller can
	// offer "use current version anyway".
	WavResolveMismatch
)

// WavResolveResult reports what ResolveWavRef found.
type WavResolveResult struct {
	Status WavResolveStatus
	Data   []byte    // raw WAV bytes the matching/first source produced
	Source WavSource // the source that produced bytes (Matched/Mismatch)
	GotSha string    // content hash of the produced bytes (Matched/Mismatch)
}

// WavLocator fetches the raw WAV bytes for a source ref, returning ok=false when
// it can't (missing file, network error, unknown key). Platform code supplies a
// locator per kind: a filesystem reader on desktop (WavSourceFile), a
// localStorage/IndexedDB reader in the browser (WavSourceLocalStorage), and a
// URL fetcher on either (WavSourceURL). Bytes — not decoded PCM — so the pin is
// a decoder-independent "file version": the caller decodes only after the hash
// matches.
type WavLocator func(ref string) (data []byte, ok bool)

// ResolveWavRef tries each of ref.Sources in order using the locator registered
// for its Kind, hashes the bytes it gets, and verifies against ref.Sha256:
//   - the first source whose bytes hash-match the pin → Matched (returns immediately);
//   - if some source produced bytes but none matched → Mismatch (the first such);
//   - if nothing produced bytes → Missing.
//
// Unknown source kinds (no locator) are skipped, never fatal — future kinds
// degrade gracefully on older builds.
func ResolveWavRef(ref *WavRef, locators map[string]WavLocator) WavResolveResult {
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
