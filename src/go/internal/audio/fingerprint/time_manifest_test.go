package fingerprint

import "testing"

func TestLoadTimeManifest_OptionalFields(t *testing.T) {
	m, err := LoadTimeManifest([]byte(`{
		"ref":"songrefs/bach-toccata.wav","template":"bach-toccata","bpm":70,
		"compare_window":{"start_sec":4,"end_sec":20},
		"sections":[{"start_sec":0,"end_sec":4,"label":"intro","instruments":["organ"]}],
		"tuning_passages":[{"start_sec":0,"end_sec":4,"instrument":"organ"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Template != "bach-toccata" || m.BPM != 70 {
		t.Errorf("scalar fields wrong: %+v", m)
	}
	if m.CompareWindow == nil || m.CompareWindow.EndSec != 20 {
		t.Errorf("compare window not parsed: %+v", m.CompareWindow)
	}
	if len(m.Sections) != 1 || m.Sections[0].Instruments[0] != "organ" {
		t.Errorf("sections not parsed: %+v", m.Sections)
	}
	if len(m.TuningPassages) != 1 || m.TuningPassages[0].Instrument != "organ" {
		t.Errorf("tuning passages not parsed: %+v", m.TuningPassages)
	}
}

func TestLoadTimeManifest_EmptyIsAllOptional(t *testing.T) {
	m, err := LoadTimeManifest([]byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.CompareWindow != nil || len(m.Sections) != 0 || len(m.TuningPassages) != 0 {
		t.Errorf("empty manifest should leave everything nil/zero: %+v", m)
	}
}
