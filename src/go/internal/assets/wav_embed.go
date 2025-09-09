package assets

// Embedding that gathers all built-in WAV samples for use at runtime.

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

// NOTE: embed patterns are evaluated relative to this file's directory.
// The repository keeps .wav files under the top-level assets/wav directory.
// The following pattern includes those files for both native and WASM builds.
// The path stays inside the module root.
// Files are embedded from a local wav/ directory under this package.
// The Makefile's sync-wav target copies repo-level assets/wav/* here.
//
//go:embed wav/*.wav
var wavFS embed.FS

// EmbeddedWAV describes an embedded sample: a stable instrument ID and bytes.
type EmbeddedWAV struct {
	ID   string
	Name string
	Data []byte
}

// ListEmbeddedWAVs returns all embedded WAV samples with generated IDs.
func ListEmbeddedWAVs() ([]EmbeddedWAV, error) {
	var out []EmbeddedWAV
	matches, err := fs.Glob(wavFS, "wav/*.wav")
	if err != nil {
		return nil, err
	}
	for _, p := range matches {
		b, err := wavFS.ReadFile(p)
		if err != nil {
			return nil, err
		}
		base := path.Base(p)
		base = strings.TrimSuffix(base, path.Ext(base))
		// Avoid clashing with built-in synth IDs like "snare"/"kick".
		id := "sample-" + strings.ToLower(strings.ReplaceAll(base, " ", "-"))
		id = strings.ReplaceAll(id, "_", "-")
		returnName := prettyName(base)
		out = append(out, EmbeddedWAV{ID: id, Name: returnName, Data: b})
	}
	return out, nil
}

func prettyName(base string) string {
	// Turn file base name into Title Case without extension for nicer labels.
	// E.g., "house-open-hi-hat" -> "House Open Hi Hat".
	base = strings.NewReplacer("_", " ", "-", " ").Replace(base)
	words := strings.Fields(base)
	for i, w := range words {
		if len(w) == 0 {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
	}
	return strings.Join(words, " ")
}
