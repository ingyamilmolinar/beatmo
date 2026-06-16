package assets

import (
	"embed"
	"encoding/json"
	"strings"
)

//go:embed templates/*.json
var templateFS embed.FS

// Template is a built-in genre circuit: display metadata + the importable JSON.
//
// Templates are authoritative, hand-authored/exported project files — exactly
// like the startup demo (see demo_embed.go). There is no procedural generator:
// the JSON in templates/*.json IS the source of truth. Display name and BPM are
// derived from each file, so the only metadata kept in Go is the menu order.
type Template struct {
	Genre   string
	Display string
	BPM     int
	Bytes   []byte
}

// templateOrder is the canonical menu order, by file stem (templates/<stem>.json).
// It is the ONLY hand-maintained template metadata; Display and BPM are derived
// from the embedded JSON so they cannot drift from what actually ships.
var templateOrder = []string{
	"rock", "hip-hop", "pop", "funk", "salsa", "house", "techno",
}

// Templates returns the built-in genre circuits in menu order. Panics at init
// only if an embedded file is missing or malformed — a build-time guarantee,
// never runtime (the embed + the JSON shape are both compile/test enforced).
func Templates() []Template {
	out := make([]Template, 0, len(templateOrder))
	for _, genre := range templateOrder {
		b, err := templateFS.ReadFile("templates/" + genre + ".json")
		if err != nil {
			panic("assets: missing embedded template " + genre + ".json: " + err.Error())
		}
		var meta struct {
			BPM int `json:"bpm"`
		}
		if err := json.Unmarshal(b, &meta); err != nil {
			panic("assets: malformed template " + genre + ".json: " + err.Error())
		}
		out = append(out, Template{
			Genre:   genre,
			Display: displayName(genre),
			BPM:     meta.BPM,
			Bytes:   b,
		})
	}
	return out
}

// displayName title-cases a file stem for the menu: "hip-hop" -> "Hip-Hop",
// "house" -> "House".
func displayName(genre string) string {
	parts := strings.Split(genre, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "-")
}
