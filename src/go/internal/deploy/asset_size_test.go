package deploy

// Asset-size budget gate for the browser build. The dominant download is
// main.wasm; the GCS deploy serves it gzip-compressed (Content-Encoding: gzip),
// so we gate BOTH the raw size and the gzip-compressed size users actually
// transfer. Budgets are intentionally tight (set just above the measured
// post-strip numbers) so a regression -- or a missing optimization -- fails
// loudly and drives the size back down.
//
// main.wasm is a build artifact and is NOT committed (see CLAUDE.md), so this
// test SKIPS cleanly when it is absent. It runs in CI after `make wasm`.
//
// Every budget is overridable via an env var so CI can widen headroom without
// editing source; the defaults are the enforced tight numbers.

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

// Tight budgets. Each is set just above the measured stripped+gzipped number,
// so an UNSTRIPPED or UNCOMPRESSED-thinking regression fails the gate. Bytes
// quoted are from `make wasm` (-s -w) on 2026-06-17. Each cites its basis.
const (
	// main.wasm raw size. Stripped: 30,136,514 B (28.74 MiB) as of 2026-06-24,
	// up from 29,462,878 B (28.10 MiB) on 2026-06-17 -- the add-core-node-types
	// branch replaced 8 small genre templates with 15+ larger song-recreation
	// templates (~656 KiB embedded JSON) plus the musicxml parser, gen-showcase,
	// and 3 new synth instruments. This is legitimate content growth: the gzip
	// transfer size (what users download) stayed within budget, confirming it is
	// well-compressing data, not code bloat. An unstripped build still exceeds
	// this, so the gate continues to enforce -s -w.
	defaultWasmRawMaxBytes = 30670848 // 29.25 MiB

	// main.wasm gzip(BestCompression) size -- the bytes a user on a
	// gzip-serving CDN actually downloads; the number that drives load time.
	// Stripped: ~6.47 MiB. Unstripped (~6.61 MiB) exceeds this.
	defaultWasmGzipMaxBytes = 6868172 // 6.55 MiB

	// Sum of gzip(BestCompression) sizes of every shipped asset (wasm + JS +
	// html) -- the total a cold visitor transfers. Stripped total: ~6.71 MiB.
	defaultAssetsGzipMaxBytes = 7130316 // 6.80 MiB
)

func envBytes(t *testing.T, name string, def int64) int64 {
	t.Helper()
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		t.Fatalf("%s=%q: not an integer: %v", name, v, err)
	}
	return n
}

// gzipLen returns the gzip(BestCompression) length of b without holding the
// compressed bytes longer than needed.
func gzipLen(t *testing.T, b []byte) int64 {
	t.Helper()
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		t.Fatalf("gzip.NewWriterLevel: %v", err)
	}
	if _, err := zw.Write(b); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return int64(buf.Len())
}

func mib(n int64) string { return fmt.Sprintf("%.2f MiB", float64(n)/(1024*1024)) }

// TestWasmAssetSizeBudget measures the raw and gzip sizes of every shipped
// browser asset and enforces tight budgets on main.wasm and the total transfer.
func TestWasmAssetSizeBudget(t *testing.T) {
	root := repoRoot(t)
	jsDir := filepath.Join(root, "src", "js")

	wasmPath := filepath.Join(jsDir, "main.wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		t.Skipf("main.wasm not present (%v) -- build artifact is not committed; run `make wasm`. Skipping size budget.", err)
	}

	// Derive the shipped-asset list from the SAME deploy manifest the drift
	// test parses, so this can never diverge from what actually ships.
	script := mustRead(t, filepath.Join(root, "scripts", "deploy-wasm-gcs.sh"))
	manifest := extractDeployManifest(t, script)
	files := sortedKeys(manifest)

	type row struct {
		name string
		raw  int64
		gz   int64
	}
	var rows []row
	var totalGzip int64
	var wasmRaw, wasmGzip int64

	for _, f := range files {
		p := filepath.Join(jsDir, f)
		b, err := os.ReadFile(p)
		if err != nil {
			// A declared asset that doesn't exist is the drift test's
			// concern, not ours; skip it here so a missing codegen file
			// doesn't masquerade as a size failure.
			t.Logf("  (skip %s: %v)", f, err)
			continue
		}
		raw := int64(len(b))
		gz := gzipLen(t, b)
		rows = append(rows, row{f, raw, gz})
		totalGzip += gz
		if f == "main.wasm" {
			wasmRaw, wasmGzip = raw, gz
		}
	}

	// Always print the table so the numbers are visible in CI logs.
	sort.Slice(rows, func(i, j int) bool { return rows[i].raw > rows[j].raw })
	t.Logf("shipped asset sizes (raw / gzip-9):")
	for _, r := range rows {
		ratio := 0.0
		if r.gz > 0 {
			ratio = float64(r.raw) / float64(r.gz)
		}
		t.Logf("  %-28s %10s / %10s  (%.2fx)", r.name, mib(r.raw), mib(r.gz), ratio)
	}
	t.Logf("  %-28s %10s / %10s", "TOTAL", "", mib(totalGzip))

	rawMax := envBytes(t, "BEATMO_WASM_RAW_MAX_BYTES", defaultWasmRawMaxBytes)
	gzipMax := envBytes(t, "BEATMO_WASM_GZIP_MAX_BYTES", defaultWasmGzipMaxBytes)
	totalMax := envBytes(t, "BEATMO_ASSETS_GZIP_MAX_BYTES", defaultAssetsGzipMaxBytes)

	if wasmRaw > rawMax {
		t.Errorf("main.wasm raw size %s exceeds budget %s (override BEATMO_WASM_RAW_MAX_BYTES)", mib(wasmRaw), mib(rawMax))
	}
	if wasmGzip > gzipMax {
		t.Errorf("main.wasm gzip size %s exceeds budget %s (override BEATMO_WASM_GZIP_MAX_BYTES)", mib(wasmGzip), mib(gzipMax))
	}
	if totalGzip > totalMax {
		t.Errorf("total gzip transfer %s exceeds budget %s (override BEATMO_ASSETS_GZIP_MAX_BYTES)", mib(totalGzip), mib(totalMax))
	}
}
