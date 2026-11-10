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
	"strings"
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
//   - beatmo_record_bench.go) with js no-op stubs.
//   - internal/ui/textcache.go imported ebitenutil solely for a never-hit debug
//     -font fallback; ebitenutil's sibling NewImageFromURL drags in net/http.
//
// Together these pulled net + crypto/tls + runtime/pprof's profile machinery
// (~4-5 MiB raw) into main.wasm for zero browser benefit. See the //go:build js
// stubs in src/go/cmd/ for the pattern.
//
// 2026-09-04: budgets ratcheted DOWN again after two independent size cuts --
// the public-domain template curation (32 genre circuits -> 24 CC0-score-derived
// ones, 3.52 MB -> 1.35 MB of embedded JSON) and subsetting the embedded Inter
// faces (812 KiB -> 495 KiB raw, 397 KiB -> 236 KiB gzip; see
// internal/assets/font_embed.go). Only the font cut moved the GZIP number:
// template JSON is ~16x compressible, so its raw bulk was nearly free over the
// wire.
//
// WHICH NUMBER MATTERS. The gzip budgets are the real gate -- they are the bytes
// a visitor downloads. The raw budget is a coarser proxy for WASM decode and
// linear-memory init, and it is INFLATED by something we do not control: the Go
// wasm linker elides zero runs by splitting data segments but stops at
// maxNumSegments = 100000 (cmd/link/internal/wasm/asm.go), and this binary is
// pinned at that cap, so ~4.5 MiB of literal zero padding is written out. That
// padding gzips to almost nothing and scales with the number of Go functions,
// not with anything a size-conscious change can target. Read the gzip number
// first; treat a raw-only overage as a prompt to check function-count growth,
// not as a download regression.
//
// Attribution tooling for any future overage: scripts/wasm_size.py breaks a
// binary down by section, package and function (build an unstripped twin first
// -- `make wasm` strips the name section that per-package attribution needs).
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
	// -s -w. 27,181,246 B (25.92 MiB) as of 2026-09-04 (-1.62 MiB: PD template
	// curation -2.17 MB of embedded JSON, font subsetting -0.31 MiB);
	// 27,704,122 B (26.42 MiB) as of 2026-09-15 (+0.50 MiB: the runtime Theme
	// editor, the Synth-tab overview + signal-graph rework, the note-matrix
	// picker and the generated instrument-fundamentals table -- all of it
	// internal/ui, which is 5,506 functions / 5.98 MiB of the 15.12 MiB code
	// section. scripts/wasm_size.py finds NO foreign dependency in the build:
	// no net/http, no crypto/tls, no runtime/pprof, no internal/scoreref).
	// 29,303,608 B (27.95 MiB) as of 2026-10-03 (+1.04 MiB: 37 more groove
	// templates, 46 -> 83, measured against the same tree with only those 37
	// files overlaid away: 28,264,137 B. Embedded template JSON is already
	// compact; it is raw-only weight -- see the gzip note below).
	// 29,572,480 B (28.20 MiB) / gzip 6.25 MiB 2026-10-24, after the
	// screenshot scene catalog left the shipped build (`make wasm` builds with
	// -tags noscenes): -797 KB raw / -0.11 MiB gzip, i.e. ~150 KB of catalog
	// code plus its data and the linker zero padding that rides with every
	// function at the segment cap. With the catalog the tree measured
	// 30,369,669 B: +0.76 MiB since 10-03 (instrument documents, dials, chord
	// and note matrices, lane) and +0.27 MiB from the typed-entities refactor
	// (internal/enum's per-enum generic instantiations alone are 61 KB of code).
	// Then -171 KB raw / -14 KB gzip (28.05 MiB / 6.23 MiB): enum.Names'
	// operations became generic functions. As exported methods the linker kept
	// a wrapper per method per enum (Ebiten's text/template forces it to keep
	// every exported method of a type that reaches an interface) -- 58 KB of
	// code never called, plus its padding. internal/enum TestNamesHasNoMethods.
	// 29,573,853 B (28.20 MiB) 2026-10-25: the Synth-tab graphs drawn from
	// the rendered voice landed (+464 KB) and the old modelled graphs were
	// deleted (-336 KB net), so the cap is unchanged.
	defaultWasmRawMaxBytes = 29884416 // 28.50 MiB (~0.30 MiB headroom over the 28.20 MiB build)

	// main.wasm gzip(BestCompression) size -- the bytes a user on a
	// gzip-serving CDN actually downloads; the number that drives load time.
	// Stripped: 5,235,898 B (4.99 MiB) 2026-07-06; 5,496,177 B (5.24 MiB)
	// 2026-07-09 (synth-pipeline overhaul); 5.63 MiB 2026-07-13 (Go 1.26.5 +
	// ebiten 2.9.9 migration); 6,103,213 B (5.82 MiB) 2026-07-17 (synth/engine +
	// node-groups overhaul); 5,987,442 B (5.71 MiB) 2026-09-04 -- essentially all
	// of the -0.15 MiB is the Inter subset; the template curation barely moved
	// this number because JSON compresses ~16x; 6,132,072 B (5.85 MiB)
	// 2026-09-15 (+0.14 MiB, the UI work above -- code, which unlike embedded
	// JSON or zero padding compresses only ~4.5x, so it lands on this number
	// nearly in full); 6,388,279 B (6.09 MiB) 2026-10-03 -- +110 KB of it is
	// the 37 new groove templates (6,278,440 B with them overlaid away via
	// go build -overlay), the on-the-wire price of 83 templates; the rest of
	// the climb from 5.85 MiB predates them. (Measured with this test's own
	// compress/gzip BestCompression -- `gzip -9` reads ~60 KB smaller.)
	// 6,582,604 B (6.28 MiB) 2026-10-10 (+194 KB, a fresh `make wasm`-flag
	// build of the tree, not a leftover): instrument config documents (the new
	// internal/instrument package, 0.13 MiB of code; the module registry's
	// builtinModuleDecls 82 KB + ModularSynthParamDefs 67 KB), the value wheel /
	// TextField, synth macro dials and the audio lane -- internal/ui alone is
	// 6.44 MiB of code, up from 5.98 MiB on 09-15. scripts/wasm_size.py still
	// finds no foreign dependency: go/parser, text/template and go/build are
	// Ebiten's (Kage shader compiler, internal/debug), unchanged upstream.
	// The landing page's per-cut media (src/js/landing/<cut>.{mp4,webp,vtt}),
	// ~10% over the 2026-10-23 encode (desktop 1.31 MiB, phone 0.78 MiB).
	landingDesktopMaxBytes = 1519616 // 1.45 MiB
	landingPhoneMaxBytes   = 897024  // 0.86 MiB

	defaultWasmGzipMaxBytes = 6710886 // 6.40 MiB (~0.12 MiB headroom over 6.28 MiB)

	// Sum of gzip(BestCompression) sizes of every shipped asset (wasm + JS +
	// html) -- the total a cold visitor transfers. Stripped total: 5,503,664 B
	// (5.25 MiB) 2026-07-06; 5,779,343 B (5.51 MiB) 2026-07-09; 5.90 MiB
	// 2026-07-13 (toolchain migration); 6,385,540 B (6.09 MiB) 2026-07-17
	// (synth/engine + node-groups overhaul); 6.27 MB (5.98 MiB) 2026-09-04;
	// 6,424,022 B (6.13 MiB) 2026-09-15 -- main.wasm is 6,132,072 B of it and
	// every other asset together is 0.28 MiB, unchanged. The
	// second-largest asset is drums.single.js (0.17 MiB gzip), the Emscripten
	// C DSP bundle -- already minified, so main.wasm is the only real lever here.
	//
	// This is the budget the 2026-09-15 build actually broke, and it broke it by
	// 1,494 B -- the other two still had headroom. It is the tightest of the
	// three by design (the wasm budget alone cannot bind: 6.00 MiB of wasm plus
	// 0.28 MiB of JS is already over any total under 6.28 MiB), so read a
	// failure HERE as "the bundle has spent its headroom", not as a leak.
	//
	// 2026-10-03: 6.29 MiB with the 37 new groove templates (+110 KB, all of it
	// main.wasm; 6.18 MiB without them), so this was raised with the wasm
	// budget above rather than trimming templates the product asked for.
	//
	// 2026-10-10: 6.46 MiB -- the +194 KB main.wasm growth above, all of it;
	// the other assets are unchanged. Raised with the wasm budget.
	defaultAssetsGzipMaxBytes = 6899630 // 6.58 MiB (~0.12 MiB headroom over 6.46 MiB)
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

// notShippedWasm skips when src/js/main.wasm is not the `make wasm` build.
// CI's release job builds it first and sets BEATMO_REQUIRE_SHIPPED_WASM=1, so
// there a missing or wrong artifact fails instead of passing as a skip.
func notShippedWasm(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("BEATMO_REQUIRE_SHIPPED_WASM") == "1" {
		t.Fatalf(format, args...)
	}
	t.Skipf(format, args...)
}

// TestWasmAssetSizeBudget measures the raw and gzip sizes of every shipped
// browser asset and enforces tight budgets on main.wasm and the total transfer.
func TestWasmAssetSizeBudget(t *testing.T) {
	root := repoRoot(t)
	jsDir := filepath.Join(root, "src", "js")

	wasmPath := filepath.Join(jsDir, "main.wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		notShippedWasm(t, "main.wasm not present (%v) -- build artifact is not committed; run `make wasm`. Skipping size budget.", err)
	}

	// The budgets below describe the SHIPPED binary, i.e. what `make wasm`
	// produces with -ldflags "-s -w". src/js/main.wasm is a shared, untracked
	// artifact and `make wasm` is not its only writer: ~15 *.browser.test.js
	// suites rebuild it in place with only -ldflags "-X main.defaultLog=INFO",
	// leaving an UNSTRIPPED binary behind. That binary carries the `name`
	// custom section -w exists to remove -- 864,511 B (0.82 MiB) raw and
	// ~0.12 MiB gzip on this build -- which is ENOUGH ON ITS OWN to blow every
	// budget here and report a phantom regression in code that did not grow.
	// (2026-09-15: a `make test-real` did exactly that, reporting 27.23 MiB
	// raw / 5.97 MiB gzip. The Go half runs before the browser half, so it
	// measured the previous run's leftover; a stripped build of the same tree
	// measured 26.42 MiB / 5.85 MiB -- inside the raw and wasm-gzip budgets of
	// the day, and 1,494 B over only the total, which is the real signal the
	// phantom 0.8 MiB was burying.)
	//
	// So: verify the artifact is the one the budgets are about before
	// measuring it. -w strips the `name` section, so its presence is proof
	// this file did not come from `make wasm`, and measuring it answers a
	// question nobody asked. Skip -- the same call the absent-file branch
	// above makes, for the same reason: this is not the shipped artifact.
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Fatalf("read main.wasm: %v", err)
	}
	if wasmHasCustomSection(wasmBytes, "name") {
		notShippedWasm(t, "main.wasm carries the `name` custom section, so it was NOT built by `make wasm` "+
			"(which links -s -w and strips it) -- a *.browser.test.js suite rebuilds this file "+
			"unstripped and leaves it behind. Measuring it overstates the shipped size by the "+
			"~0.8 MiB section alone. Run `make wasm` and re-run this gate. (size on disk: %s)",
			mib(int64(len(wasmBytes))))
	}
	// Same reasoning for the scene catalog: `make wasm` builds with -tags
	// noscenes, while `make wasm-dev` (test-browser, the screenshot targets)
	// keeps the catalog and its ~150 KB of code. A dev build left behind is
	// not the shipped artifact. The marker is the catalog's first scene name,
	// a string no other shipped code holds (TestShippedWasmLeavesOutTheSceneCatalog
	// pins that the shipped build compiles no scene definitions at all).
	if bytes.Contains(wasmBytes, []byte(sceneCatalogMarker)) {
		notShippedWasm(t, "main.wasm contains the screenshot scene catalog, so it is a `make wasm-dev` "+
			"build, not the shipped one (`make wasm` builds with -tags noscenes). Run `make wasm` "+
			"and re-run this gate. (size on disk: %s)", mib(int64(len(wasmBytes))))
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
	landingMedia := map[string]int64{} // cut -> raw bytes (stored uncompressed)

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
		// The landing's recordings, posters and cue files are stored
		// uncompressed and load only after the game's download has finished
		// (spec 2026-10-23-landing-page-design.md), so they are not part of
		// the critical-path transfer below; they have their own budget.
		if strings.HasPrefix(f, "landing/") {
			cut := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
			landingMedia[cut] += raw
			rows = append(rows, row{f, raw, raw})
			continue
		}
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
	// Landing media per cut (mp4 + webp poster + vtt). Measured 2026-10-23:
	// desktop 1,375,427 B (32.4 s at CRF 23), phone 815,622 B (30.9 s).
	for cut, max := range map[string]int64{"desktop": landingDesktopMaxBytes, "phone": landingPhoneMaxBytes} {
		if got := landingMedia[cut]; got > max {
			t.Errorf("landing %s media %s exceeds budget %s (re-encode, or shorten the tour)", cut, mib(got), mib(max))
		}
	}
}

// wasmHasCustomSection reports whether b -- a WebAssembly module -- carries a
// custom section named want. It walks the section table only (id + LEB128
// payload length), so it is O(number of sections) and never decodes code.
//
// Returns false for anything that is not a well-formed module; a file we
// cannot parse is not evidence of anything and the caller falls through to
// measuring it.
func wasmHasCustomSection(b []byte, want string) bool {
	if len(b) < 8 || string(b[:4]) != "\x00asm" {
		return false
	}
	i := 8
	for i < len(b) {
		id := b[i]
		i++
		size, n, ok := uleb128(b, i)
		if !ok || size > uint64(len(b)-n) {
			return false
		}
		payload := b[n : n+int(size)]
		i = n + int(size)
		if id != 0 { // 0 == custom
			continue
		}
		nameLen, m, ok := uleb128(payload, 0)
		if !ok || nameLen > uint64(len(payload)-m) {
			return false
		}
		if string(payload[m:m+int(nameLen)]) == want {
			return true
		}
	}
	return false
}

// uleb128 decodes the unsigned LEB128 at b[i:], returning the value and the
// index just past it. ok=false on a truncated or over-long encoding.
func uleb128(b []byte, i int) (v uint64, next int, ok bool) {
	var shift uint
	for ; i < len(b); i++ {
		c := b[i]
		if shift >= 64 {
			return 0, 0, false
		}
		v |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			return v, i + 1, true
		}
		shift += 7
	}
	return 0, 0, false
}
