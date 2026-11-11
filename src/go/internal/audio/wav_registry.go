package audio

import (
	"encoding/json"
	"sync"
)

// WavVersion is one recorded content version of a named WAV: a monotonic
// human-friendly Version paired with the authoritative content Sha256.
type WavVersion struct {
	Version int    `json:"version"`
	Sha256  string `json:"sha256"`
}

// WavRegistry is our own file-based versioning of user WAV files, kept in the
// decoupled instrument store. It maps a WAV name to the content versions seen
// for it, so an instrument can pin "my-808.wav v3" and import can resolve that
// exact version by hash (and show the friendly label). The registry never holds
// PCM — only names, versions, and hashes.
type WavRegistry struct {
	mu       sync.Mutex
	versions map[string][]WavVersion
}

// NewWavRegistry returns an empty registry.
func NewWavRegistry() *WavRegistry {
	return &WavRegistry{versions: map[string][]WavVersion{}}
}

// Register records (name, sha). If the hash is already known for that name it
// returns the existing version; otherwise it assigns the next monotonic version
// for that name and returns it.
func (r *WavRegistry) Register(name, sha string) WavVersion {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.versions == nil {
		r.versions = map[string][]WavVersion{}
	}
	list := r.versions[name]
	for _, v := range list {
		if v.Sha256 == sha {
			return v
		}
	}
	v := WavVersion{Version: len(list) + 1, Sha256: sha}
	r.versions[name] = append(list, v)
	return v
}

// Lookup returns the recorded version for (name, sha), or ok=false when that
// exact content isn't known for the name.
func (r *WavRegistry) Lookup(name, sha string) (WavVersion, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.versions[name] {
		if v.Sha256 == sha {
			return v, true
		}
	}
	return WavVersion{}, false
}

// Versions returns a copy of the recorded versions for a name (nil when none).
func (r *WavRegistry) Versions(name string) []WavVersion {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.versions[name]
	if len(list) == 0 {
		return nil
	}
	out := make([]WavVersion, len(list))
	copy(out, list)
	return out
}

// MarshalJSON serializes the name→versions map.
func (r *WavRegistry) MarshalJSON() ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return json.Marshal(r.versions)
}

// UnmarshalJSON restores the name→versions map.
func (r *WavRegistry) UnmarshalJSON(b []byte) error {
	m := map[string][]WavVersion{}
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.versions = m
	return nil
}
