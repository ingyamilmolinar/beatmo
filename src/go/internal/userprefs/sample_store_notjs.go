//go:build !js

package userprefs

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// One file per sample under <configdir>/beatmo/samples/<base64url(id)>.bsmp: a 4-byte LE rate, then LE float32 PCM.
// Out of prefs.json because PCM is large; the reversible filename needs no manifest. Atomic temp + rename.

const sampleFileExt = ".bsmp"

func (s *fileStore) samplesDir() string {
	return filepath.Join(filepath.Dir(s.path), "samples")
}

func (s *fileStore) LoadSamples() (map[instrument.ID]SampleBlob, error) {
	out := map[instrument.ID]SampleBlob{}
	dir := s.samplesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != sampleFileExt {
			continue
		}
		base := e.Name()[:len(e.Name())-len(sampleFileExt)]
		idBytes, derr := base64.RawURLEncoding.DecodeString(base)
		if derr != nil {
			continue
		}
		id, perr := instrument.ParseID(string(idBytes))
		if perr != nil {
			continue
		}
		data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil || len(data) < 4 {
			continue
		}
		sr := int(binary.LittleEndian.Uint32(data[:4]))
		out[id] = SampleBlob{SampleRate: sr, PCM: append([]byte(nil), data[4:]...)}
	}
	return out, nil
}

func (s *fileStore) SaveSample(id instrument.ID, blob SampleBlob) error {
	if id == "" {
		return nil
	}
	dir := s.samplesDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	buf := make([]byte, 4+len(blob.PCM))
	binary.LittleEndian.PutUint32(buf[:4], uint32(blob.SampleRate))
	copy(buf[4:], blob.PCM)
	path := filepath.Join(dir, base64.RawURLEncoding.EncodeToString([]byte(id))+sampleFileExt)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *fileStore) DeleteSample(id instrument.ID) error {
	if id == "" {
		return nil
	}
	path := filepath.Join(s.samplesDir(), base64.RawURLEncoding.EncodeToString([]byte(id))+sampleFileExt)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
