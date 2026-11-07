//go:build !js

package audio

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
)

// The struct is generated from the table, so the C slot loop bound is the one remaining place the two could disagree.
func TestModularGenSlotCountIsSingleSourceOfTruth(t *testing.T) {
	for _, f := range fieldsOfBlock("gen") {
		if f.Slots != modularGenSlots {
			t.Errorf("gen-bank field %s has %d slots, want modularGenSlots=%d", f.Name, f.Slots, modularGenSlots)
		}
	}
	src, err := os.ReadFile(filepath.Join(repoRootForC(t), "src", "c", "modular_params.gen.h"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`#define\s+MODULAR_GEN_SLOTS\s+(\d+)`).FindSubmatch(src)
	if m == nil {
		t.Fatal("MODULAR_GEN_SLOTS not found in modular_params.gen.h")
	}
	if n, _ := strconv.Atoi(string(m[1])); n != modularGenSlots {
		t.Fatalf("C MODULAR_GEN_SLOTS=%d != modularGenSlots=%d", n, modularGenSlots)
	}
}
