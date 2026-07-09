package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.Root, "root", "../../src/go", "module source root")
	flag.StringVar(&cfg.Module, "module", "github.com/ingyamilmolinar/beatmo", "module import path")
	flag.StringVar(&cfg.GOOS, "goos", "linux", "GOOS for call resolution (e.g. js for wasm-only files)")
	flag.StringVar(&cfg.Tags, "tags", "", "comma-separated build tags for call resolution")
	out := flag.String("out", "../../build/go-graph.html", "output HTML path")
	jsonOut := flag.String("json", "", "optional path to also write the raw JSON model")
	flag.Parse()

	g, err := BuildGraph(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gograph:", err)
		os.Exit(1)
	}
	html, err := RenderHTML(g)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gograph:", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "gograph:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, html, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "gograph:", err)
		os.Exit(1)
	}
	fmt.Println("HTML:", *out)
	if *jsonOut != "" {
		model, err := writeJSON(g)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gograph:", err)
			os.Exit(1)
		}
		if err := os.WriteFile(*jsonOut, model, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "gograph:", err)
			os.Exit(1)
		}
		fmt.Println("JSON:", *jsonOut)
	}
}

func writeJSON(g *Graph) ([]byte, error) {
	return jsonMarshalIndent(g)
}
