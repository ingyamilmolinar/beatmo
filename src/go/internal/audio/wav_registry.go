package audio

import (
	"encoding/json"
	"sync"
)

// A friendly monotonic Version paired with the authoritative Sha256.
type WavVersion struct {
	Version int    `json:"version"`
	Sha256  string `json:"sha256"`
}

// So an instrument can pin "my-808.wav v3" and import resolve that exact content by hash. Never holds PCM.
type WavRegistry struct {
	mu       sync.Mutex
	versions map[string][]WavVersion
}

// NewWavRegistry returns an empty registry.
func NewWavRegistry() *WavRegistry {
	return &WavRegistry{versions: map[string][]WavVersion{}}
}

// A known hash keeps its version; new content gets the next one.
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
