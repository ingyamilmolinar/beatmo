//go:build !js && !test

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
)

// Safe to call repeatedly; the scan runs once.
func InitDefaultCatalog() {
	catalogOnce.Do(func() {
		_ = InitCatalogFromDir(defaultAssetsRoot())
	})
}

// AssetsRoot returns the resolved assets directory for disk scans.
func AssetsRoot() string {
	return defaultAssetsRoot()
}

// Works from the repo root (`make run`) and from module subdirectories.
func defaultAssetsRoot() string {
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

func InitCatalogFromDir(root string) error {
	m := map[instrument.ID]SoundMeta{}
	var out []SoundMeta
	rootAbs := root
	if abs, err := filepath.Abs(root); err == nil {
		rootAbs = abs
	}

	// Instruments come from filesystem references only; embedded samples are not surfaced.
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
		// assets/Saved/ must never surface in the catalog.
		if strings.EqualFold(parts[0], "Saved") {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		relTrim := strings.TrimSuffix(relSlash, filepath.Ext(relSlash))
		cat := parts[0]
		id := instrument.SampleID(relTrim) // category + subpath keep it unique
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
			ID:      id,
			Name:    PrettyName(base),
			Group:   wavCategory(cat, relTrim),
			RelPath: relTrim,
			Path:    filepath.ToSlash(absPath),
			Size:    info.Size(),
			Source:  SourceWAV,
		}
		m[id] = meta
		out = append(out, meta)
		return nil
	})

	// Lightweight metadata so synths list apart from WAV assets.
	for _, id := range instOrder {
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

	// Stable ordering: category then relative path (both lowercased).
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

// Catalog returns a copy of the current sound catalog.
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

// CatalogGroups returns the sorted distinct catalog groups (excluding "Missing").
func CatalogGroups() []string {
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	seen := map[string]bool{}
	var cats []string
	for _, m := range catalog {
		if m.Group == "" {
			continue
		}
		if !seen[m.Group] {
			seen[m.Group] = true
			cats = append(cats, m.Group)
		}
	}
	slices.Sort(cats)
	return cats
}

// IsRegistered reports whether an instrument ID is already registered.
func IsRegistered(id instrument.ID) bool {
	instMu.RLock()
	defer instMu.RUnlock()
	_, ok := instruments[id]
	return ok
}

// Errors for an unknown ID.
func EnsureInstrumentLoaded(id instrument.ID) error {
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
