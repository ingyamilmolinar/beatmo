//go:build test || js

package audio

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

var (
	catalogOnce sync.Once
	catalogMu   sync.RWMutex
	catalog     []SoundMeta
	catalogByID map[instrument.ID]SoundMeta
	catalogReg  map[instrument.ID]bool
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
	m := map[instrument.ID]SoundMeta{}
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
		// Skip the legacy assets/Saved/ folder (see catalog_desktop.go for context).
		parts := strings.Split(relSlash, "/")
		if len(parts) > 0 && strings.EqualFold(parts[0], "Saved") {
			return nil
		}
		// Files at the asset root (no category folder) are not surfaced.
		if len(parts) < 2 || parts[0] == "" {
			return nil
		}
		cat := wavCategoryStub(parts[0])
		if cat == "" {
			return nil
		}
		id := instrument.SampleID(relTrim)
		if _, exists := m[id]; exists {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		absPath, _ := filepath.Abs(p)
		meta := SoundMeta{
			ID:      id,
			Name:    PrettyName(strings.TrimSuffix(filepath.Base(relTrim), filepath.Ext(relTrim))),
			Group:   cat,
			RelPath: relTrim,
			Path:    filepath.ToSlash(absPath),
			Size:    info.Size(),
			Source:  SourceWAV,
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
			ID:      id,
			Name:    PrettyName(string(id)),
			Group:   synthCatalogCategory(id),
			RelPath: "synth/" + string(id),
			Source:  SourceSynth,
		}
		m[id] = meta
		out = append(out, meta)
	}

	slices.SortFunc(out, func(a, b SoundMeta) int {
		if a.Group != b.Group {
			if a.Group < b.Group {
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
	if root := env.AssetsRoot.Get(); root != "" {
		if info, err := os.Stat(root); err == nil && info.IsDir() {
			return root
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

func Catalog() []SoundMeta {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	out := make([]SoundMeta, len(catalog))
	copy(out, catalog)
	return out
}

// CatalogLookup returns metadata for an instrument by ID, or false if not found.
func CatalogLookup(id instrument.ID) (SoundMeta, bool) {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	meta, ok := catalogByID[id]
	return meta, ok
}

func CatalogGroups() []string {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	seen := map[string]bool{}
	var out []string
	for _, m := range catalog {
		if m.Group == "" {
			continue
		}
		if !seen[m.Group] {
			seen[m.Group] = true
			out = append(out, m.Group)
		}
	}
	// Deterministic order for tests.
	slices.Sort(out)
	return out
}

func IsRegistered(id instrument.ID) bool {
	catalogMu.RLock()
	ok := catalogReg != nil && catalogReg[id]
	catalogMu.RUnlock()
	return ok
}

func EnsureInstrumentLoaded(id instrument.ID) error {
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

// ResetCatalogForTest allows tests to inject a synthetic catalog. When
// entries are provided, catalogOnce is left consumed so a subsequent
// InitDefaultCatalog call is a no-op and does not clobber the injection.
// When entries == nil, catalogOnce resets so the next InitDefaultCatalog
// performs a fresh disk scan.
func ResetCatalogForTest(entries []SoundMeta) {
	catalogMu.Lock()
	catalog = entries
	catalogByID = map[instrument.ID]SoundMeta{}
	catalogReg = map[instrument.ID]bool{}
	for _, m := range entries {
		catalogByID[m.ID] = m
	}
	if entries == nil {
		catalogOnce = sync.Once{}
	} else {
		catalogOnce = sync.Once{}
		catalogOnce.Do(func() {})
	}
	catalogMu.Unlock()
	bumpCatalogVersion()
}

func registerCatalogStub(id instrument.ID) {
	catalogMu.Lock()
	if catalogReg == nil {
		catalogReg = map[instrument.ID]bool{}
	}
	catalogReg[id] = true
	catalogMu.Unlock()
	Register(id, nil)
}

// wavCategoryStub maps recognised top-level folder names to user-facing
// category labels. Returns "" for unknown folders so callers can skip them
// rather than surfacing a catch-all "Samples" bucket.
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
	case "drum machines":
		return "Drum Machines (WAV)"
	default:
		return PrettyName(cat) + " (WAV)"
	}
}
