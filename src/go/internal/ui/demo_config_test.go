//go:build test

package ui

import (
	"os"
	"testing"
)

func TestDemoLoadsFromEnvConfig_WithInstruments(t *testing.T) {
	tmp, err := os.CreateTemp("", "tunkul-demo-*.json")
	if err != nil {
		t.Fatalf("tmp: %v", err)
	}
	defer os.Remove(tmp.Name())
	json := `{
        "version":1,
        "subdiv":32,
        "bpm":120,
        "instruments":[{"name":"Test","id":"snare","kind":"builtin","volume":1,"origin":0,"color":"#FFFFFFFF"}],
        "nodes":[{"id":0,"i":0,"j":0,"type":"regular","outputs":[1]},{"id":1,"i":16,"j":0,"type":"regular","outputs":[0]}]
    }`
	if _, err := tmp.WriteString(json); err != nil {
		t.Fatalf("write: %v", err)
	}
	tmp.Close()

	os.Setenv("TUNKUL_DEMO_CONFIG", tmp.Name())
	defer os.Unsetenv("TUNKUL_DEMO_CONFIG")

	g := New(testLogger)
	g.Layout(800, 600)
	g.buildDemo()
	if !g.demoBuilt {
		t.Fatalf("demo not built from env config")
	}
	if len(g.drum.Rows) != 1 {
		t.Fatalf("rows=%d want=1", len(g.drum.Rows))
	}
	if g.drum.BPM() != 120 {
		t.Fatalf("bpm=%d want=120", g.drum.BPM())
	}
	if g.graph.StartNodeID == 0 && g.drum.Rows[0].Origin != 0 {
		t.Fatalf("row 0 origin not assigned")
	}
	// Beat infos populated for row 0
	if len(g.beatInfosByRow) == 0 || len(g.beatInfosByRow[0]) == 0 {
		t.Fatalf("beat infos not populated from env config")
	}
}

func TestDemoLoadsFromEnvConfig_NoInstruments_FallbackRowAndStart(t *testing.T) {
	tmp, err := os.CreateTemp("", "tunkul-demo-*.json")
	if err != nil {
		t.Fatalf("tmp: %v", err)
	}
	defer os.Remove(tmp.Name())
	// No instruments provided; expect buildDemo to add a default row and set start.
	json := `{
        "version":1,
        "subdiv":32,
        "bpm":110,
        "nodes":[{"id":0,"i":0,"j":0,"type":"regular","outputs":[1]},{"id":1,"i":16,"j":0,"type":"regular","outputs":[0]}]
    }`
	if _, err := tmp.WriteString(json); err != nil {
		t.Fatalf("write: %v", err)
	}
	tmp.Close()

	os.Setenv("TUNKUL_DEMO_CONFIG", tmp.Name())
	defer os.Unsetenv("TUNKUL_DEMO_CONFIG")

	g := New(testLogger)
	g.Layout(800, 600)
	g.buildDemo()
	if !g.demoBuilt {
		t.Fatalf("demo not built from env config without instruments")
	}
	if len(g.drum.Rows) == 0 {
		t.Fatalf("rows not created for env config without instruments")
	}
	if g.graph.StartNodeID < 0 {
		t.Fatalf("start node not set for env config without instruments")
	}
	if len(g.beatInfosByRow) == 0 || len(g.beatInfosByRow[0]) == 0 {
		t.Fatalf("beat infos not computed for env config without instruments")
	}
}

func TestDemoLoadsFromEnvConfig_TildePath(t *testing.T) {
	// Create a temp HOME and write the file under it to exercise ~ expansion.
	dir := t.TempDir()
	tmp := dir + "/tunkul-demo.json"
	json := `{
        "version":1,
        "subdiv":32,
        "bpm":100,
        "nodes":[{"id":0,"i":0,"j":0,"type":"regular","outputs":[1]},{"id":1,"i":16,"j":0,"type":"regular","outputs":[0]}]
    }`
	if err := os.WriteFile(tmp, []byte(json), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	prevHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", prevHome)

	os.Setenv("TUNKUL_DEMO_CONFIG", "~/tunkul-demo.json")
	defer os.Unsetenv("TUNKUL_DEMO_CONFIG")

	g := New(testLogger)
	g.Layout(800, 600)
	g.buildDemo()
	if !g.demoBuilt {
		t.Fatalf("demo not built from tilde path env config")
	}
	if len(g.drum.Rows) == 0 {
		t.Fatalf("rows not created from tilde config")
	}
	if len(g.beatInfosByRow) == 0 || len(g.beatInfosByRow[0]) == 0 {
		t.Fatalf("beat infos not computed from tilde config")
	}
}
