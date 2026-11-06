package channel

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

func TestOfAndInstrumentRoundTrip(t *testing.T) {
	const inst instrument.ID = "kick"
	c := Of(inst)
	if c.String() != "kick" {
		t.Fatalf("Of(%q) = %q, want the instrument's spelling", inst, c)
	}
	if got, ok := c.Instrument(); !ok || got != inst {
		t.Fatalf("Instrument() = %q, %v; want %q, true", got, ok, inst)
	}
	if c.IsMaster() || c.IsZero() {
		t.Fatal("an instrument channel is neither the master bus nor no channel")
	}
}

// TestOneNamePerBus: the master bus is "main" and only "main"; the empty
// name is no bus at all, and no instrument can take the master's name.
func TestOneNamePerBus(t *testing.T) {
	if Master.String() != "main" || !Master.IsMaster() {
		t.Fatalf("Master = %q", Master)
	}
	var zero ID
	if zero.IsMaster() || !zero.IsZero() {
		t.Fatal("the zero ID must not be the master bus")
	}
	if _, ok := zero.Instrument(); ok {
		t.Fatal("the zero ID has no instrument")
	}
	if _, ok := Master.Instrument(); ok {
		t.Fatal("the master bus has no instrument")
	}
	if !Of("").IsZero() || !Of(instrument.ReservedMasterName).IsZero() {
		t.Fatal("Of must not mint the master bus or an empty channel")
	}
	if got, err := Parse("main"); err != nil || got != Master {
		t.Fatalf("Parse(main) = %v, %v", got, err)
	}
	if got, err := Parse("snare"); err != nil || got != Of("snare") {
		t.Fatalf("Parse(snare) = %v, %v", got, err)
	}
	for _, bad := range []string{"", "master", " main", "my kick"} {
		if bad == "master" {
			// "master" is an (odd) instrument id, not the master bus.
			if got, err := Parse(bad); err != nil || got.IsMaster() {
				t.Errorf("Parse(%q) = %v, %v; only \"main\" is the master bus", bad, got, err)
			}
			continue
		}
		if _, err := Parse(bad); err == nil {
			t.Errorf("Parse(%q) accepted", bad)
		}
	}
}

func TestJSONSpellingIsTheWireName(t *testing.T) {
	in := struct {
		A ID         `json:"a"`
		B ID         `json:"b"`
		Z ID         `json:"z"`
		M map[ID]int `json:"m"`
	}{A: Master, B: Of("kick"), M: map[ID]int{Master: 1, Of("kick"): 2}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"a":"main","b":"kick","z":"","m":{"kick":2,"main":1}}`; string(b) != want {
		t.Fatalf("got %s, want %s", b, want)
	}
	var out struct {
		A ID         `json:"a"`
		B ID         `json:"b"`
		Z ID         `json:"z"`
		M map[ID]int `json:"m"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.A != Master || out.B != Of("kick") || !out.Z.IsZero() || out.M[Master] != 1 || out.M[Of("kick")] != 2 {
		t.Fatalf("round trip = %+v", out)
	}
	if err := json.Unmarshal([]byte(`{"a":"my kick"}`), &out); err == nil {
		t.Fatal("an invalid channel name decoded")
	}
}
