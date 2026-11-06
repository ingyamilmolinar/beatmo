package enum

import (
	"encoding/json"
	"reflect"
	"testing"
)

type color int

const (
	colorNone color = iota
	colorRed
	colorBlue
)

var colorNames = Names[color]{Kind: "color", Names: []string{"", "red", "blue"}}

func (c color) String() string                { return String(colorNames, c) }
func (c color) MarshalText() ([]byte, error)  { return Marshal(colorNames, c) }
func (c *color) UnmarshalText(b []byte) error { return Unmarshal(colorNames, b, c) }

type shape int

const (
	shapeSquare shape = iota
	shapeRound
)

var shapeNames = Names[shape]{Kind: "shape", Names: []string{"square", "round"}}

func (s shape) MarshalText() ([]byte, error)  { return Marshal(shapeNames, s) }
func (s *shape) UnmarshalText(b []byte) error { return Unmarshal(shapeNames, b, s) }

func TestRoundTripAndStrictDecode(t *testing.T) {
	type doc struct {
		C color         `json:"c"`
		S shape         `json:"s"`
		M map[color]int `json:"m"`
	}
	b, err := json.Marshal(doc{C: colorBlue, S: shapeRound, M: map[color]int{colorRed: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"c":"blue","s":"round","m":{"red":1}}`; string(b) != want {
		t.Fatalf("got %s, want %s", b, want)
	}
	var d doc
	if err := json.Unmarshal(b, &d); err != nil || d.C != colorBlue || d.S != shapeRound || d.M[colorRed] != 1 {
		t.Fatalf("round trip = %+v, %v", d, err)
	}
	for _, bad := range []string{`{"c":"green"}`, `{"s":""}`, `{"s":"Round"}`, `{"m":{"green":1}}`} {
		if err := json.Unmarshal([]byte(bad), &d); err == nil {
			t.Errorf("%s decoded", bad)
		}
	}
	if err := json.Unmarshal([]byte(`{"c":""}`), &d); err != nil || d.C != colorNone {
		t.Fatalf("the empty spelling of a none-able enum = %v, %v", d.C, err)
	}
	if _, err := json.Marshal(doc{C: color(7)}); err == nil {
		t.Fatal("an out-of-range value was written")
	}
	if got := color(7).String(); got != "color(7)" {
		t.Fatalf("String of an invalid value = %q", got)
	}
	if got := len(All(colorNames)); got != 3 {
		t.Fatalf("All = %d values", got)
	}
}

// TestNamesHasNoMethods pins the package's size contract: Names exposes its
// operations as generic functions, never as exported methods. Ebiten links
// text/template, so the linker keeps EVERY exported method of a type that
// reaches an interface -- a method on Names cost each enum a wrapper per
// method (and per pointer method) in main.wasm, ~58 KB over 15 enums, none
// of it ever called.
func TestNamesHasNoMethods(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[Names[color]](), reflect.TypeFor[*Names[color]]()} {
		for i := range typ.NumMethod() {
			t.Errorf("%v has exported method %s -- make it a generic function (see the package comment)", typ, typ.Method(i).Name)
		}
	}
}
