package ui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func (dv *DrumView) saveInstrument(row int) {
	if row < 0 || row >= len(dv.Rows) {
		return
	}
	id := dv.Rows[row].Instrument
	if id == "" {
		dv.notifyError("Instrument missing")
		return
	}
	if !runningUnderGoTest() {
		audio.InitDefaultCatalog()
	}
	meta, ok := dv.instMeta[id]
	if !ok {
		for _, m := range audio.Catalog() {
			if m.ID == id {
				meta = m
				ok = true
				break
			}
		}
	}
	if ok {
		rel := strings.ToLower(strings.ReplaceAll(meta.RelPath, "\\", "/"))
		if strings.HasPrefix(rel, "saved/") {
			dv.notifyInfo("Instrument already saved")
			return
		}
		if meta.Source == "synth" {
			dv.notifyError("Only WAV instruments can be saved")
			return
		}
	}

	data, srcPath, err := dv.saveSourceForInstrument(id, ok, meta)
	if err != nil {
		dv.notifyError(err.Error())
		return
	}
	assetsRoot := audio.AssetsRoot()
	if assetsRoot == "" {
		dv.notifyError("Assets directory not found")
		return
	}
	savedDir := filepath.Join(assetsRoot, "Saved")
	if err := os.MkdirAll(savedDir, 0o755); err != nil {
		dv.notifyError("Failed to create Saved directory")
		return
	}
	if srcPath != "" {
		if alreadySaved(srcPath, savedDir) {
			dv.notifyInfo("Instrument already saved")
			return
		}
	}
	base := safeSavedBase(id)
	if base == "" && ok {
		base = safeSavedBase(meta.Name)
	}
	if base == "" {
		base = "instrument"
	}
	dest := filepath.Join(savedDir, base+".wav")
	if _, err := os.Stat(dest); err == nil {
		dv.notifyInfo("Instrument already saved")
		return
	}
	if err := writeSavedWav(dest, srcPath, data); err != nil {
		dv.notifyError("Failed to save instrument")
		return
	}
	if err := audio.InitCatalogFromDir(assetsRoot); err != nil {
		dv.notifyError("Failed to refresh instrument catalog")
		return
	}
	dv.instRefreshDirty = true
	dv.refreshInstruments()
	if dv.instMenuOpen {
		dv.buildInstMenu()
	}
	dv.notifyInfo("Saved instrument to Saved/")
}

func (dv *DrumView) saveSourceForInstrument(id string, hasMeta bool, meta audio.SoundMeta) ([]byte, string, error) {
	if hasMeta {
		if meta.Embedded && len(meta.Data) > 0 {
			return meta.Data, "", nil
		}
		if meta.Path != "" {
			return nil, meta.Path, nil
		}
	}
	if dv.samplePath != nil {
		if p := dv.samplePath[id]; p != "" {
			return nil, p, nil
		}
	}
	return nil, "", errors.New("no WAV data available to save")
}

func writeSavedWav(dest, srcPath string, data []byte) error {
	if len(data) > 0 {
		return os.WriteFile(dest, data, 0o644)
	}
	if srcPath == "" {
		return errors.New("missing source")
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, src); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func safeSavedBase(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == ' ':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return out
}

func alreadySaved(srcPath, savedDir string) bool {
	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		return false
	}
	absSaved, err := filepath.Abs(savedDir)
	if err != nil {
		return false
	}
	absSaved = filepath.Clean(absSaved)
	absSrc = filepath.Clean(absSrc)
	if absSrc == absSaved {
		return true
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(absSaved, sep) {
		absSaved += sep
	}
	return strings.HasPrefix(absSrc, absSaved)
}
