// Package instrumentpack is the embedded instrument pack and its builder: the
// readable sources in internal/assets/instruments, packed (spec §3, §8).
// Its own package so internal/audio can embed it without importing
// internal/assets (whose tests import audio). Build is deterministic:
// encoding/json sorts map keys and gzip writes a zero header.
package instrumentpack

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// Pack is the embedded instrument archive: the shipped order, each shipped
// id's content hash, and every document by content hash.
type Pack struct {
	schema.Envelope
	Order   []instrument.ID           `json:"order"`
	Shipped map[instrument.ID]string  `json:"shipped"`
	Docs    map[string]instrument.Doc `json:"docs"`
}

// Build reads dir/index.json and dir/<id>.json — plus every pinned document in
// the sibling instrument_pins/ directory — and returns the gzip'd pack.
func Build(dir string) ([]byte, error) {
	idxBytes, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return nil, err
	}
	var idx struct {
		schema.Envelope
		Instruments []instrument.ID `json:"instruments"`
	}
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return nil, err
	}
	if err := idx.Envelope.Validate("instrument index"); err != nil {
		return nil, err
	}
	p := Pack{Envelope: schema.Current(), Order: idx.Instruments, Shipped: map[instrument.ID]string{}, Docs: map[string]instrument.Doc{}}
	for _, id := range idx.Instruments {
		b, err := os.ReadFile(filepath.Join(dir, string(id)+".json"))
		if err != nil {
			return nil, err
		}
		d, err := instrument.ParseFile(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		if d.ID != id {
			return nil, fmt.Errorf("%s.json holds id %q", id, d.ID)
		}
		h := instrument.ContentHash(d)
		p.Shipped[id] = h
		if _, dup := p.Docs[h]; !dup {
			p.Docs[h] = d
		}
	}
	if err := addPins(&p, filepath.Join(filepath.Dir(dir), "instrument_pins")); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// addPins packs every instrument_pins/<hex>.json: a document a template still
// references by hash although it is no longer shipped (it goes in Docs, never
// in Shipped/Order). The file name must be its content hash. A missing
// directory is no pins.
func addPins(p *Pack, pinsDir string) error {
	entries, err := os.ReadDir(pinsDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(pinsDir, e.Name()))
		if err != nil {
			return err
		}
		d, err := instrument.ParseFile(b)
		if err != nil {
			return fmt.Errorf("pin %s: %w", e.Name(), err)
		}
		h := instrument.ContentHash(d)
		if want := strings.TrimPrefix(h, "sha256:") + ".json"; e.Name() != want {
			return fmt.Errorf("pin %s holds %s (name it %s)", e.Name(), h, want)
		}
		if _, dup := p.Docs[h]; !dup {
			p.Docs[h] = d
		}
	}
	return nil
}
