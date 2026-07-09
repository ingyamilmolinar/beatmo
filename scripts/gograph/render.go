package main

import (
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed template.html
var tmplHTML string

//go:embed app.js
var appJS string

// RenderHTML embeds the model and the app script into the self-contained page.
func RenderHTML(g *Graph) ([]byte, error) {
	model, err := json.Marshal(g)
	if err != nil {
		return nil, err
	}
	out := strings.Replace(tmplHTML, "__MODEL__", string(model), 1)
	out = strings.Replace(out, "__APP__", appJS, 1)
	return []byte(out), nil
}

func jsonMarshalIndent(g *Graph) ([]byte, error) {
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
