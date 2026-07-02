//go:build !js && !test

package audio

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

var (
	catalogOnce sync.Once
	catalogMu   sync.RWMutex
	catalog     []SoundMeta
	catalogByID map[string]SoundMeta
)

// InitDefaultCatalog scans the top-level assets directory for WAV files grouped
// by their first-level folder. It is safe to call multiple times; the scan runs
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
	if env := os.Getenv("BEATMO_ASSETS"); env != "" {
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

	// On-disk WAVs grouped by top-level folder. Embedded samples are no longer
	// surfaced — instruments come from filesystem references only.
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, _ error) error {
		if d == nil {
			return nil
		}
		if d.IsDir() {
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
		// Skip the legacy assets/Saved/ folder. It used to host the on-disk
		// "Save Instrument" output and is being removed; any leftover files
		// must not surface in the catalog (tests pin this invariant).
		if strings.EqualFold(parts[0], "Saved") {
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
			Name:     PrettyName(base),
			Category: wavCategory(cat, relTrim),
			RelPath:  relTrim,
			Path:     filepath.ToSlash(absPath),
			Size:     info.Size(),
			Source:   "wav",
			Scope:    "shipped",
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
			Name:     PrettyName(id),
			Category: synthCategory(id),
			RelPath:  "synth/" + id,
			Source:   "synth",
			Scope:    "builtin",
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
	if meta.Path == "" {
		return errors.New("catalog entry missing path")
	}
	return RegisterAudio(meta.ID, meta.Path)
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
		name := PrettyName(cat)
		// If nested folders exist, append first subfolder for finer grouping.
		parts := strings.Split(rel, "/")
		if len(parts) > 1 {
			name = PrettyName(parts[0])
		}
		return name + " (WAV)"
	}
}

// synthCategory maps built-in synth IDs to grouped categories.
func synthCategory(id string) string {
	lower := strings.ToLower(id)
	switch {
	case strings.HasPrefix(lower, "rimshot"), strings.HasPrefix(lower, "sidestick"):
		return "Snares (Synth)"
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
	case strings.HasPrefix(lower, "ride"), strings.HasPrefix(lower, "crash"):
		return "Cymbals (Synth)"
	case strings.HasPrefix(lower, "shaker"), strings.HasPrefix(lower, "conga"):
		return "Percussion (Synth)"
	case strings.HasPrefix(lower, "bass"):
		return "Bass (Synth)"
	case strings.HasPrefix(lower, "violin"), strings.HasPrefix(lower, "viola"), strings.HasPrefix(lower, "cello"),
		strings.HasPrefix(lower, "guitar"):
		return "Strings (Synth)"
	case strings.HasPrefix(lower, "piano"):
		return "Keys (Synth)"
	case strings.HasPrefix(lower, "flute"), strings.HasPrefix(lower, "oboe"),
		strings.HasPrefix(lower, "trumpet"), strings.HasPrefix(lower, "french-horn"):
		return "Winds (Synth)"
	default:
		return "Synth (Other)"
	}
}

// slugReplacer is hoisted to package level to avoid re-creating it on every
// slugPath call. The replacer is immutable and safe for concurrent use.
var slugReplacer = strings.NewReplacer(" ", "-", "_", "-", "/", "-", ":", "-", "+", "-", ".", "-", "(", "-", ")", "-", "[", "-", "]", "-", "{", "-", "}", "-", ",", "-", "'", "-", "\"", "-", "#", "-", "$", "-", "%", "-", "@", "-", "!", "-", "^", "-", "&", "-", "*", "-", "?", "-", "|", "-", "<", "-", ">", "-", "~", "-", "`", "-")

// slugPath converts a path-like string into a stable, ascii-only identifier.
// It preserves folder structure by replacing separators with dashes.
func slugPath(rel string) string {
	lower := strings.ToLower(rel)
	lower = strings.ReplaceAll(lower, "\\", "/")
	lower = strings.TrimSuffix(lower, "/")
	lower = strings.TrimPrefix(lower, "/")
	slug := slugReplacer.Replace(lower)
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
