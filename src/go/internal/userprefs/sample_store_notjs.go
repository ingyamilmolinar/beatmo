//go:build !js

package userprefs

import (
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
)

// Desktop SampleStore backend. Each user sample is one file under
// <configdir>/beatmo/samples/<base64url(id)>.bsmp containing a 4-byte
// little-endian sample-rate header followed by the raw little-endian float32
// PCM. Kept out of prefs.json because PCM payloads are large; the filename
// encodes the id reversibly so no manifest is needed. Writes are atomic
// (temp + rename).

const sampleFileExt = ".bsmp"

func (s *fileStore) samplesDir() string {
	return filepath.Join(filepath.Dir(s.path), "samples")
}

func (s *fileStore) LoadSamples() (map[string]SampleBlob, error) {
	out := map[string]SampleBlob{}
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
		data, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
		if rerr != nil || len(data) < 4 {
			continue
		}
		sr := int(binary.LittleEndian.Uint32(data[:4]))
		out[string(idBytes)] = SampleBlob{SampleRate: sr, PCM: append([]byte(nil), data[4:]...)}
	}
	return out, nil
}

func (s *fileStore) SaveSample(id string, blob SampleBlob) error {
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

func (s *fileStore) DeleteSample(id string) error {
	if id == "" {
		return nil
	}
	path := filepath.Join(s.samplesDir(), base64.RawURLEncoding.EncodeToString([]byte(id))+sampleFileExt)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
