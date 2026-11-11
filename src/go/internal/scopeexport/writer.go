package scopeexport

import (
	"encoding/json"
	"os"
)

// writer handles append-only JSONL file output.
type writer struct {
	f *os.File
}

func openWriter(path string) (*writer, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &writer{f: f}, nil
}

func (w *writer) writeLine(snap *Snapshot) error {
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = w.f.Write(data)
	return err
}

func (w *writer) close() error {
	if w.f != nil {
		return w.f.Close()
	}
	return nil
}
