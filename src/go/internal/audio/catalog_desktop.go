//go:build !js && !test

package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/ingyamilmolinar/tunkul/internal/assets"
)

var (
	catalogOnce sync.Once
	catalogMu   sync.RWMutex
	catalog     []SoundMeta
	catalogByID map[string]SoundMeta
	embedTemp   map[string]string
)

// InitDefaultCatalog scans the top-level assets directory for WAV files grouped
// by their first-level folder (excluding assets/wav) and registers embedded
// samples as lazy entries. It is safe to call multiple times; the scan runs
// once.
func InitDefaultCatalog() {
	catalogOnce.Do(func() {
		_ = InitCatalogFromDir(defaultAssetsRoot())
	})
}

// AssetsRoot returns the resolved assets directory for disk scans.
func AssetsRoot() string {
	return defaultAssetsRoot()
}

// defaultAssetsRoot attempts to locate the assets directory relative to the
// current working directory. This supports running from repo root (`make run`)
// as well as from module subdirs (e.g. `cd src/go && go run ...`).
func defaultAssetsRoot() string {
	if env := os.Getenv("TUNKUL_ASSETS"); env != "" {
		if info, err := os.Stat(env); err == nil && info.IsDir() {
			return env
		}
	}
	for up := 0; up <= 5; up++ {
		rel := []string{"assets"}
		for i := 0; i < up; i++ {
			rel = append([]string{".."}, rel...)
		}
		p := filepath.Clean(filepath.Join(rel...))
		abs, _ := filepath.Abs(p)
		// Skip the internal assets folder used for embeds.
		if strings.Contains(abs, string(filepath.Separator)+"internal"+string(filepath.Separator)+"assets") {
			continue
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs
		}
	}
	return "assets"
}

// InitCatalogFromDir replaces the current catalog with WAV metadata discovered
// under root. Primarily used by UI/Game startup; it may be re-run in tests.
func InitCatalogFromDir(root string) error {
	m := map[string]SoundMeta{}
	var out []SoundMeta
	rootAbs := root
	if abs, err := filepath.Abs(root); err == nil {
		rootAbs = abs
	}
	skipWavRoot := strings.EqualFold(filepath.Base(rootAbs), "assets") || strings.EqualFold(filepath.Base(root), "assets")

	// 1) Embedded samples (kept lazy; only stored as metadata).
	if wavs, err := assets.ListEmbeddedWAVs(); err == nil {
		for _, w := range wavs {
			if _, exists := m[w.ID]; exists {
				continue
			}
			meta := SoundMeta{
				ID:       w.ID,
				Name:     w.Name,
				Category: "Samples (WAV)",
				RelPath:  "embedded/" + w.ID,
				Size:     int64(len(w.Data)),
				Embedded: true,
				Data:     w.Data,
				Source:   "embedded",
			}
			if dur, sr, ch := wavHeaderMeta(bytes.NewReader(w.Data)); sr > 0 {
				meta.DurationMS = dur
				meta.SampleRate = sr
				meta.Channels = ch
			}
			m[w.ID] = meta
			out = append(out, meta)
		}
	}

	// 2) On-disk WAVs grouped by top-level folder (exclude assets/wav to
	// avoid huge copies; that folder already mirrors a curated subset).
	filepath.WalkDir(root, func(p string, d os.DirEntry, _ error) error {
		if d == nil {
			return nil
		}
		if d.IsDir() {
			if skipWavRoot {
				if rel, err := filepath.Rel(root, p); err == nil && strings.EqualFold(rel, "wav") {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".wav" {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 2 { // require category folder
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		relTrim := strings.TrimSuffix(relSlash, filepath.Ext(relSlash))
		cat := parts[0]
		id := slugPath(relTrim) // include category + subpath for uniqueness
		if _, exists := m[id]; exists {
			return nil
		}
		base := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		info, err := d.Info()
		if err != nil {
			return nil
		}
		absPath := filepath.Join(rootAbs, rel)
		meta := SoundMeta{
			ID:       id,
			Name:     prettyName(base),
			Category: wavCategory(cat, relTrim),
			RelPath:  relTrim,
			Path:     filepath.ToSlash(absPath),
			Size:     info.Size(),
			Source:   "wav",
		}
		m[id] = meta
		out = append(out, meta)
		return nil
	})

	// 3) Built-in synthesized instruments: add lightweight metadata so they
	// appear in category lists distinct from WAV assets.
	for _, id := range instOrder {
		if _, exists := m[id]; exists {
			continue
		}
		meta := SoundMeta{
			ID:       id,
			Name:     prettyName(id),
			Category: synthCategory(id),
			RelPath:  "synth/" + id,
			Source:   "synth",
		}
		m[id] = meta
		out = append(out, meta)
	}

	// Stable ordering: category then relative path (both lowercased).
	slices.SortFunc(out, func(a, b SoundMeta) int {
		if a.Category != b.Category {
			if a.Category < b.Category {
				return -1
			}
			return 1
		}
		if a.RelPath < b.RelPath {
			return -1
		}
		if a.RelPath > b.RelPath {
			return 1
		}
		return 0
	})

	catalogMu.Lock()
	catalog = out
	catalogByID = m
	embedTemp = map[string]string{}
	catalogMu.Unlock()
	bumpCatalogVersion()
	return nil
}

// Catalog returns a copy of the current sound catalog.
func Catalog() []SoundMeta {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	out := make([]SoundMeta, len(catalog))
	copy(out, catalog)
	return out
}

// CatalogLookup returns metadata for an instrument by ID, or false if not found.
func CatalogLookup(id string) (SoundMeta, bool) {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	meta, ok := catalogByID[id]
	return meta, ok
}

// CatalogCategories returns sorted distinct categories (excluding "Missing").
func CatalogCategories() []string {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	seen := map[string]bool{}
	var cats []string
	for _, m := range catalog {
		if m.Category == "" {
			continue
		}
		if !seen[m.Category] {
			seen[m.Category] = true
			cats = append(cats, m.Category)
		}
	}
	slices.Sort(cats)
	return cats
}

// IsRegistered reports whether an instrument ID is already registered.
func IsRegistered(id string) bool {
	instMu.RLock()
	defer instMu.RUnlock()
	_, ok := instruments[id]
	return ok
}

// EnsureInstrumentLoaded registers an instrument on first use using the catalog
// metadata. It returns an error if the ID is unknown.
func EnsureInstrumentLoaded(id string) error {
	if id == "" || IsRegistered(id) {
		return nil
	}
	catalogMu.RLock()
	meta, ok := catalogByID[id]
	catalogMu.RUnlock()
	if !ok {
		return errors.New("instrument not found in catalog")
	}
	if meta.Embedded {
		path, err := embeddedTempPath(meta)
		if err != nil {
			return err
		}
		return RegisterAudio(meta.ID, path)
	}
	if meta.Path == "" {
		return errors.New("catalog entry missing path")
	}
	return RegisterAudio(meta.ID, meta.Path)
}

func embeddedTempPath(meta SoundMeta) (string, error) {
	catalogMu.Lock()
	defer catalogMu.Unlock()
	if embedTemp == nil {
		embedTemp = map[string]string{}
	}
	if p, ok := embedTemp[meta.ID]; ok && p != "" {
		return p, nil
	}
	tmp, err := os.CreateTemp("", "tunkul-"+meta.ID+"-*.wav")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(meta.Data); err != nil {
		_ = tmp.Close()
		return "", err
	}
	_ = tmp.Close()
	embedTemp[meta.ID] = tmp.Name()
	return tmp.Name(), nil
}

func wavHeaderMeta(r io.ReadSeeker) (durationMS int, sampleRate int, channels int) {
	defer func() { _, _ = r.Seek(0, io.SeekStart) }()
	var hdr struct {
		ChunkID   [4]byte
		ChunkSize uint32
		Format    [4]byte
	}
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return 0, 0, 0
	}
	if string(hdr.ChunkID[:]) != "RIFF" {
		return 0, 0, 0
	}
	// Walk chunks until "fmt " then "data".
	var fmtFound bool
	var dataSize uint32
	var byteRate uint32
	for {
		var chunkID [4]byte
		var chunkSize uint32
		if err := binary.Read(r, binary.LittleEndian, &chunkID); err != nil {
			break
		}
		if err := binary.Read(r, binary.LittleEndian, &chunkSize); err != nil {
			break
		}
		switch string(chunkID[:]) {
		case "fmt ":
			var audioFmt uint16
			var numCh uint16
			var sr uint32
			var br uint32
			if err := binary.Read(r, binary.LittleEndian, &audioFmt); err != nil {
				return 0, 0, 0
			}
			if err := binary.Read(r, binary.LittleEndian, &numCh); err != nil {
				return 0, 0, 0
			}
			if err := binary.Read(r, binary.LittleEndian, &sr); err != nil {
				return 0, 0, 0
			}
			if err := binary.Read(r, binary.LittleEndian, &br); err != nil {
				return 0, 0, 0
			}
			// skip rest of fmt chunk
			if _, err := r.Seek(int64(chunkSize-10), io.SeekCurrent); err != nil {
				return 0, 0, 0
			}
			fmtFound = true
			sampleRate = int(sr)
			channels = int(numCh)
			byteRate = br
		case "data":
			dataSize = chunkSize
			if _, err := r.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
				return 0, 0, 0
			}
		default:
			if _, err := r.Seek(int64(chunkSize), io.SeekCurrent); err != nil {
				return 0, 0, 0
			}
		}
		if fmtFound && dataSize > 0 {
			break
		}
	}
	if !fmtFound || dataSize == 0 || byteRate == 0 {
		return 0, sampleRate, channels
	}
	secs := float64(dataSize) / float64(byteRate)
	return int(secs * 1000), sampleRate, channels
}

// wavCategory normalizes top-level folder names into user-facing categories,
// distinguishing WAV assets from synthesized sources.
func wavCategory(cat string, rel string) string {
	switch strings.ToLower(cat) {
	case "snare":
		return "Snares (WAV)"
	case "kick":
		return "Kick Drums (WAV)"
	case "hihat", "hi-hat":
		return "Hi-Hats (WAV)"
	case "cymbals":
		return "Cymbals (WAV)"
	case "toms":
		return "Toms (WAV)"
	case "percussion":
		return "Percussion (WAV)"
	case "drum machines":
		return "Drum Machines (WAV)"
	default:
		name := prettyName(cat)
		// If nested folders exist, append first subfolder for finer grouping.
		parts := strings.Split(rel, "/")
		if len(parts) > 1 {
			name = prettyName(parts[0])
		}
		return name + " (WAV)"
	}
}

// synthCategory maps built-in synth IDs to grouped categories.
func synthCategory(id string) string {
	lower := strings.ToLower(id)
	switch {
	case strings.HasPrefix(lower, "snare"):
		return "Snares (Synth)"
	case strings.HasPrefix(lower, "kick"):
		return "Kick Drums (Synth)"
	case strings.HasPrefix(lower, "hihat"):
		return "Hi-Hats (Synth)"
	case strings.HasPrefix(lower, "tom"):
		return "Toms (Synth)"
	case strings.HasPrefix(lower, "clap"):
		return "Claps (Synth)"
	case strings.HasPrefix(lower, "cowbell"):
		return "Cowbells (Synth)"
	case strings.HasPrefix(lower, "bass"):
		return "Bass (Synth)"
	default:
		return "Synth (Other)"
	}
}

// slugPath converts a path-like string into a stable, ascii-only identifier.
// It preserves folder structure by replacing separators with dashes.
func slugPath(rel string) string {
	lower := strings.ToLower(rel)
	lower = strings.ReplaceAll(lower, "\\", "/")
	lower = strings.TrimSuffix(lower, "/")
	lower = strings.TrimPrefix(lower, "/")
	repl := strings.NewReplacer(" ", "-", "_", "-", "/", "-", ":", "-", "+", "-", ".", "-", "(", "-", ")", "-", "[", "-", "]", "-", "{", "-", "}", "-", ",", "-", "'", "-", "\"", "-", "#", "-", "$", "-", "%", "-", "@", "-", "!", "-", "^", "-", "&", "-", "*", "-", "?", "-", "|", "-", "<", "-", ">", "-", "~", "-", "`", "-")
	slug := repl.Replace(lower)
	// collapse repeated dashes
	slug = strings.ReplaceAll(slug, "--", "-")
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "sample"
	}
	return slug
}

// prettyName converts a file base into Title Case without separators.
func prettyName(base string) string {
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
