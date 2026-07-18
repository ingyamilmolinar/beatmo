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
// quoted are from `make wasm` (-s -w) on 2026-07-01. Each cites its basis.
//
// 2026-07-01: budgets ratcheted DOWN ~8 MiB raw / ~1.7 MiB gzip after stripping
// desktop/dev-only code that was leaking into the browser build:
//   - cmd/beatmo.go unconditionally imported net/http + net/http/pprof +
//     runtime/pprof (the PPROF=1 localhost:6060 server and file CPU profiling --
//     both meaningless in a browser). Moved behind //go:build !js (profiling.go
//     + beatmo_record_bench.go) with js no-op stubs.
//   - internal/ui/textcache.go imported ebitenutil solely for a never-hit debug
//     -font fallback; ebitenutil's sibling NewImageFromURL drags in net/http.
// Together these pulled net + crypto/tls + runtime/pprof's profile machinery
// (~4-5 MiB raw) into main.wasm for zero browser benefit. See the //go:build js
// stubs in src/go/cmd/ for the pattern.
const (
	// main.wasm raw size. Stripped: 22,606,472 B (21.56 MiB) as of 2026-07-01;
	// 23,139,178 B (22.07 MiB) as of 2026-07-06 (synth-panel + loudness-
	// normalization + sp_* DSP-primitives work); 26,085,653 B (24.88 MiB) as of
	// 2026-07-09 (synth-pipeline overhaul + instrument tuning: the modular voice
	// DSP + expanded instrument/recipe tables); 26,459,907 B (25.23 MiB) as of
	// 2026-07-11 (canonical `make wasm` build of the overhaul HEAD -- the 07-09
	// number was measured from a pre-final build and under-recorded the shipped
	// size by ~0.35 MiB); 27,700,709 B (26.42 MiB) as of 2026-07-13 (toolchain
	// migration Go 1.23.1 -> 1.26.5 + ebiten v2.8.8 -> v2.9.9 / oto v3.4.0 /
	// x/image v0.31.0: +1.19 MiB raw of compiler codegen + dep growth, traded
	// for Green Tea GC default, ~30% cheaper cgo calls, and finer-grained wasm
	// heap management); 28,874,089 B (27.54 MiB) as of 2026-07-17 (synth/engine
	// UI + instruments overhaul + node-groups feature: +1.12 MiB raw of the
	// expanded synth stage/instrument tables + node-group graph code). An
	// unstripped build still far exceeds this, so the gate continues to enforce
	// -s -w.
	defaultWasmRawMaxBytes = 29360128 // 28.00 MiB (~0.46 MiB headroom over the 27.54 MiB overhaul build; branch add-core-node-types is still growing it)

	// main.wasm gzip(BestCompression) size -- the bytes a user on a
	// gzip-serving CDN actually downloads; the number that drives load time.
	// Stripped: 5,235,898 B (4.99 MiB) 2026-07-06; 5,496,177 B (5.24 MiB)
	// 2026-07-09 (synth-pipeline overhaul); 5.63 MiB 2026-07-13 (Go 1.26.5 +
	// ebiten 2.9.9 migration); 6,103,213 B (5.82 MiB) 2026-07-17 (synth/engine +
	// node-groups overhaul).
	defaultWasmGzipMaxBytes = 6291456 // 6.00 MiB (~0.18 MiB headroom over 5.82 MiB)

	// Sum of gzip(BestCompression) sizes of every shipped asset (wasm + JS +
	// html) -- the total a cold visitor transfers. Stripped total: 5,503,664 B
	// (5.25 MiB) 2026-07-06; 5,779,343 B (5.51 MiB) 2026-07-09; 5.90 MiB
	// 2026-07-13 (toolchain migration); 6,385,540 B (6.09 MiB) 2026-07-17
	// (synth/engine + node-groups overhaul).
	defaultAssetsGzipMaxBytes = 6553600 // 6.25 MiB (~0.16 MiB headroom over 6.09 MiB)
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
