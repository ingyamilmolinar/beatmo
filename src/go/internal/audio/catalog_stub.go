//go:build test || js

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
	catalogReg  map[string]bool
)

func InitDefaultCatalog() {
	catalogOnce.Do(func() {
		_ = InitCatalogFromDir(defaultAssetsRootStub())
	})
}

// AssetsRoot returns the resolved assets directory for disk scans.
func AssetsRoot() string {
	return defaultAssetsRootStub()
}

func InitCatalogFromDir(root string) error {
	m := map[string]SoundMeta{}
	var out []SoundMeta

	filepath.WalkDir(root, func(p string, d os.DirEntry, _ error) error {
		if d == nil || d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(d.Name())) != ".wav" {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		relTrim := strings.TrimSuffix(relSlash, filepath.Ext(relSlash))
		cat := "Samples (WAV)"
		if parts := strings.Split(relSlash, "/"); len(parts) > 0 && parts[0] != "" {
			cat = wavCategoryStub(parts[0])
		}
		id := slugPathStub(relTrim)
		if _, exists := m[id]; exists {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		absPath, _ := filepath.Abs(p)
		meta := SoundMeta{
			ID:       id,
			Name:     prettyNameStub(filepath.Base(relTrim)),
			Category: cat,
			RelPath:  relTrim,
			Path:     filepath.ToSlash(absPath),
			Size:     info.Size(),
			Source:   "wav",
		}
		m[id] = meta
		out = append(out, meta)
		return nil
	})

	// Synth placeholders mirror stub Instruments ordering for stability.
	for _, id := range Instruments() {
		if _, exists := m[id]; exists {
			continue
		}
		meta := SoundMeta{
			ID:       id,
			Name:     prettyNameStub(id),
			Category: synthCategoryStub(id),
			RelPath:  "synth/" + id,
			Source:   "synth",
		}
		m[id] = meta
		out = append(out, meta)
	}

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

func defaultAssetsRootStub() string {
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
		if strings.Contains(abs, string(filepath.Separator)+"internal"+string(filepath.Separator)+"assets") {
			continue
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			return abs
		}
	}
	return "assets"
}

// defaultAssetsRoot mirrors the desktop helper for test/js builds so callers
// like catalog_test can use a single name across build tags.
func defaultAssetsRoot() string {
	return defaultAssetsRootStub()
}

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

func CatalogCategories() []string {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	seen := map[string]bool{}
	var out []string
	for _, m := range catalog {
		if m.Category == "" {
			continue
		}
		if !seen[m.Category] {
			seen[m.Category] = true
			out = append(out, m.Category)
		}
	}
	// Deterministic order for tests.
	slices.Sort(out)
	return out
}

func IsRegistered(id string) bool {
	catalogMu.RLock()
	ok := catalogReg != nil && catalogReg[id]
	catalogMu.RUnlock()
	return ok
}

func EnsureInstrumentLoaded(id string) error {
	if IsRegistered(id) {
		return nil
	}
	catalogMu.RLock()
	meta, ok := catalogByID[id]
	catalogMu.RUnlock()
	if !ok {
		return errors.New("instrument not found in catalog")
	}
	registerCatalogStub(meta.ID)
	return nil
}

// ResetCatalogForTest allows tests to inject a synthetic catalog.
func ResetCatalogForTest(entries []SoundMeta) {
	catalogOnce = sync.Once{}
	catalogMu.Lock()
	catalog = entries
	catalogByID = map[string]SoundMeta{}
	catalogReg = map[string]bool{}
	for _, m := range entries {
		catalogByID[m.ID] = m
	}
	catalogMu.Unlock()
	bumpCatalogVersion()
}

func registerCatalogStub(id string) {
	catalogMu.Lock()
	if catalogReg == nil {
		catalogReg = map[string]bool{}
	}
	catalogReg[id] = true
	catalogMu.Unlock()
	Register(id, nil)
}

func slugPathStub(rel string) string {
	rel = strings.ToLower(rel)
	rel = strings.ReplaceAll(rel, " ", "-")
	rel = strings.ReplaceAll(rel, "_", "-")
	rel = strings.ReplaceAll(rel, string(filepath.Separator), "-")
	rel = strings.ReplaceAll(rel, "/", "-")
	rel = strings.Trim(rel, "-")
	return rel
}

func prettyNameStub(base string) string {
	base = strings.TrimSuffix(base, filepath.Ext(base))
	base = strings.ReplaceAll(base, "_", " ")
	base = strings.ReplaceAll(base, "-", " ")
	return strings.Title(base)
}

func wavCategoryStub(cat string) string {
	switch strings.ToLower(cat) {
	case "snare":
		return "Snares (WAV)"
	case "kick":
		return "Kick Drums (WAV)"
	case "hihat", "hi-hat", "hi hat":
		return "Hi-Hats (WAV)"
	case "cymbals":
		return "Cymbals (WAV)"
	case "toms":
		return "Toms (WAV)"
	case "percussion":
		return "Percussion (WAV)"
	case "saved":
		return "Saved (WAV)"
	default:
		return "Samples (WAV)"
	}
}

func synthCategoryStub(id string) string {
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
