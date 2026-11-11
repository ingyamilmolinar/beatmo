package instrumentpack

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

func TestAssetPackIsCurrent(t *testing.T) {
	want, err := Build("../instruments")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, Embedded()) {
		t.Fatal("instruments.pack.json.gz is stale — run `make asset-pack`")
	}
}

// A template-pinned document that is no longer shipped lives in
// instrument_pins/<hex>.json (beside instruments/); Build packs it into Docs —
// never Shipped — so the template's ref still resolves.
func TestBuildPacksPinnedDocuments(t *testing.T) {
	root := t.TempDir()
	inst, pins := filepath.Join(root, "instruments"), filepath.Join(root, "instrument_pins")
	for _, d := range []string{inst, pins} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	kick, err := os.ReadFile("../instruments/kick.json")
	if err != nil {
		t.Fatal(err)
	}
	idx := `{"schema_version":"0.0.1","instruments":["kick"]}`
	if err := os.WriteFile(filepath.Join(inst, "index.json"), []byte(idx), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inst, "kick.json"), kick, 0o644); err != nil {
		t.Fatal(err)
	}
	old, _ := instrument.ParseFile(kick)
	old.ID, old.Name = "old-kick", "Old Kick"
	old.Render.Seconds = 0.123
	h := instrument.ContentHash(old)
	b, _ := instrument.MarshalFile(old)
	if err := os.WriteFile(filepath.Join(pins, strings.TrimPrefix(h, "sha256:")+".json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	gz, err := Build(inst)
	if err != nil {
		t.Fatal(err)
	}
	p := decodePackForTest(t, gz)
	if _, ok := p.Docs[h]; !ok {
		t.Fatalf("pinned document %s not packed", h)
	}
	if _, ok := p.Shipped["old-kick"]; ok || len(p.Order) != 1 {
		t.Fatalf("a pin must not ship: shipped=%v order=%v", p.Shipped, p.Order)
	}
	// A pin whose name is not its content hash is rejected.
	if err := os.WriteFile(filepath.Join(pins, "0000000000000000.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(inst); err == nil {
		t.Fatal("a misnamed pin must fail the build")
	}
}

func decodePackForTest(t *testing.T, gz []byte) Pack {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		t.Fatal(err)
	}
	var p Pack
	if err := json.NewDecoder(zr).Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p
}
