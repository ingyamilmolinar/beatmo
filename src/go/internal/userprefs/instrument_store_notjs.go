//go:build !js

package userprefs

import (
	"os"
	"path/filepath"
)

// fileInstrumentBackend persists the instrument store to its own JSON file,
// sibling to prefs.json, via atomic temp+rename.
type fileInstrumentBackend struct {
	path string
}

func newInstrumentBackend(opts Options) instrumentBackend {
	// Like prefs: empty → <UserConfigDir>/beatmo/instruments.json, *.json → exact, a bare dir → <dir>/instruments.json.
	var path string
	switch {
	case opts.Path == "":
		base, err := os.UserConfigDir()
		if err != nil {
			home, _ := os.UserHomeDir()
			if home == "" {
				home = os.TempDir()
			}
			base = filepath.Join(home, ".config")
		}
		path = filepath.Join(base, "beatmo", InstrumentsFileName)
	case filepath.Ext(opts.Path) == ".json":
		path = opts.Path
	default:
		path = filepath.Join(opts.Path, InstrumentsFileName)
	}
	return &fileInstrumentBackend{path: path}
}

func (b *fileInstrumentBackend) read() ([]byte, error) {
	data, err := os.ReadFile(b.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

func (b *fileInstrumentBackend) write(data []byte) error {
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return err
	}
	tmp := b.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, b.path)
}
