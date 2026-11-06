// gen-asset-pack rebuilds the embedded instrument pack from the readable
// documents in internal/assets/instruments. Run from src/go (make asset-pack).
package main

import (
	"log"
	"os"

	"github.com/ingyamilmolinar/beatmo/internal/assets/instrumentpack"
)

func main() {
	b, err := instrumentpack.Build("internal/assets/instruments")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile("internal/assets/instrumentpack/instruments.pack.json.gz", b, 0o644); err != nil {
		log.Fatal(err)
	}
}
