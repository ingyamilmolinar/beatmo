//go:build !test && !js

package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// dialRelevanceTablePath is the generated table, relative to this package.
var dialRelevanceTablePath = filepath.Join("..", "assets", "synth_dial_relevance.gen.json")

// Re-measures on the real engine and compares with the generated table; when stale the fix is
// `make gen-dial-relevance` (DIAL_RELEVANCE_UPDATE=1), never an exception.
func TestDialRelevanceTableIsCurrent(t *testing.T) {
	withPrivateInstrumentLibrary(t)
	audio.ResetInstrumentDocs()
	t.Cleanup(audio.ResetInstrumentDocs)

	var entries []dialRelevanceEntry
	var jobs []func() error
	for _, id := range roster.All {
		schemaDefs := audio.InstrumentParamDefs(id)
		doc, ok := audio.InstrumentDoc(id)
		if len(schemaDefs) == 0 || !ok {
			continue
		}
		merged := audio.MergedInstrumentParamsRO(id)
		voice := doc.Render.Seconds
		probes := dialProbes(buildMacroGroupsForVoice(schemaDefs, merged, voice), merged)
		i := len(entries)
		entries = append(entries, dialRelevanceEntry{ID: id, Signature: soundSignatureHex(audio.SoundSignature(doc))})
		snap := doc.Clone()
		jobs = append(jobs, func() error {
			hidden, err := measureDialRelevance(id, snap, probes, audio.RenderDocForGraph)
			entries[i].Hidden = hidden
			return err
		})
	}
	if len(entries) < 100 {
		t.Fatalf("only %d shipped instruments measured: the roster walk is broken", len(entries))
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	errs := make([]error, len(jobs))
	for i, j := range jobs {
		wg.Add(1)
		go func(i int, j func() error) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			errs[i] = j()
		}(i, j)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Slice(entries, func(a, b int) bool { return entries[a].ID < entries[b].ID })
	got := marshalDialRelevance(dialRelevanceFile{Envelope: schema.Current(), Instruments: entries})

	if os.Getenv("DIAL_RELEVANCE_UPDATE") == "1" {
		// The startup demo's hand-edited sounds are measured as a cache, never checked for staleness: an edited one is
		// measured at runtime.
		all := append(entries, measureStartupDemoRelevance(t, entries)...)
		got = marshalDialRelevance(dialRelevanceFile{Envelope: schema.Current(), Instruments: all})
		if err := os.WriteFile(dialRelevanceTablePath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		hidden := 0
		for _, e := range entries {
			hidden += len(e.Hidden)
		}
		t.Logf("wrote %s: %d instruments, %d inert dials hidden", dialRelevanceTablePath, len(entries), hidden)
		return
	}
	want, err := os.ReadFile(dialRelevanceTablePath)
	if err != nil {
		t.Fatal(err)
	}
	var old dialRelevanceFile
	if err := json.Unmarshal(want, &old); err != nil {
		t.Fatal(err)
	}
	byID := map[string]dialRelevanceEntry{}
	var shipped []dialRelevanceEntry
	for _, e := range old.Instruments {
		if e.Source == "" {
			byID[string(e.ID)] = e
			shipped = append(shipped, e)
		}
	}
	if bytes.Equal(got, marshalDialRelevance(dialRelevanceFile{Envelope: old.Envelope, Instruments: shipped})) {
		return
	}
	var diffs []string
	for _, e := range entries {
		o, ok := byID[string(e.ID)]
		switch {
		case !ok:
			diffs = append(diffs, fmt.Sprintf("%s: not in the table", e.ID))
		case o.Signature != e.Signature:
			diffs = append(diffs, fmt.Sprintf("%s: its sound changed (signature %s → %s)", e.ID, o.Signature, e.Signature))
		case !slices.Equal(o.Hidden, e.Hidden):
			diffs = append(diffs, fmt.Sprintf("%s: inert dials %v → %v", e.ID, o.Hidden, e.Hidden))
		}
		delete(byID, string(e.ID))
	}
	for id := range byID {
		diffs = append(diffs, fmt.Sprintf("%s: no longer shipped", id))
	}
	sort.Strings(diffs)
	if len(diffs) > 20 {
		diffs = append(diffs[:20], fmt.Sprintf("… and %d more", len(diffs)-20))
	}
	t.Fatalf("the dial relevance table is stale — run `make gen-dial-relevance`:\n  %s", strings.Join(diffs, "\n  "))
}

// Compact, one instrument per line: it ships in the WASM.
func marshalDialRelevance(f dialRelevanceFile) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "{\"schema_version\": %q, \"instruments\": [\n", f.SchemaVersion)
	for i, e := range f.Instruments {
		line, _ := json.Marshal(e)
		b.Write(line)
		if i < len(f.Instruments)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("]}\n")
	return b.Bytes()
}

// Only those no shipped entry covers.
func measureStartupDemoRelevance(t *testing.T, shipped []dialRelevanceEntry) []dialRelevanceEntry {
	t.Helper()
	known := map[string]bool{}
	for _, e := range shipped {
		known[e.Signature] = true
	}
	g := buildTestGame(t)
	g.buildDemo()
	t.Cleanup(audio.ResetInstrumentDocs)
	var out []dialRelevanceEntry
	for _, row := range g.drum.Rows {
		id := row.Instrument
		doc, ok := audio.InstrumentDoc(id)
		if !ok {
			continue
		}
		sig := soundSignatureHex(audio.SoundSignature(doc))
		if known[sig] {
			continue
		}
		known[sig] = true
		merged := audio.MergedInstrumentParamsRO(id)
		voice := doc.Render.Seconds
		probes := dialProbes(buildMacroGroupsForVoice(audio.InstrumentParamDefs(id), merged, voice), merged)
		hidden, err := measureDialRelevance(id, doc.Clone(), probes, audio.RenderDocForGraph)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, dialRelevanceEntry{ID: id, Signature: sig, Hidden: hidden, Source: "demo"})
	}
	return out
}
