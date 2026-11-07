package deploy

// Gates BOTH the raw main.wasm and the gzip bytes a visitor transfers, tight so a regression or a missing optimisation
// fails loudly. main.wasm is never committed, so this skips when it is absent; every budget has an env override for CI.

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

// Each budget sits just above the measured stripped size. The gzip budgets are the real gate (the download); raw is a
// proxy for decode and memory init, inflated by linker zero padding at the wasm segment cap that gzips to nothing, so
// a raw-only overage means function-count growth. scripts/wasm_size.py attributes an overage (on an unstripped twin).
const (
	// Over the 28.20 MiB stripped build. Embedded template JSON and zero padding are raw-only weight; an unstripped build
	// far exceeds this, so it still enforces -s -w.
	defaultWasmRawMaxBytes = 29884416 // 28.50 MiB (~0.30 MiB headroom over the 28.20 MiB build)

	// Code compresses ~4.5x and lands on this number nearly in full; JSON and padding barely move it (measured with
	// compress/gzip BestCompression, ~60 KB above gzip -9). Landing media below: ~10% over the encoded cuts.
	landingDesktopMaxBytes = 1519616 // 1.45 MiB
	landingPhoneMaxBytes   = 897024  // 0.86 MiB

	defaultWasmGzipMaxBytes = 6710886 // 6.40 MiB (~0.12 MiB headroom over 6.28 MiB)

	// The total a cold visitor transfers, tightest of the three by design: main.wasm is nearly all of it (drums.single.js,
	// next largest, is already minified), so a failure here means the bundle has spent its headroom, not a leak.
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

// TestWasmAssetSizeBudget measures the raw and gzip sizes of every shipped
// browser asset and enforces tight budgets on main.wasm and the total transfer.
func TestWasmAssetSizeBudget(t *testing.T) {
	root := repoRoot(t)
	jsDir := filepath.Join(root, "src", "js")

	wasmPath := filepath.Join(jsDir, "main.wasm")
	if _, err := os.Stat(wasmPath); err != nil {
		t.Skipf("main.wasm not present (%v) -- build artifact is not committed; run `make wasm`. Skipping size budget.", err)
	}

	// The budgets describe the SHIPPED binary (-s -w). Browser tests rebuild src/js/main.wasm unstripped, and the name
	// section alone would blow every budget with a phantom regression; its presence proves the file did not come from make
	// wasm, so skip, as for an absent file.
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		t.Fatalf("read main.wasm: %v", err)
	}
	if wasmHasCustomSection(wasmBytes, "name") {
		t.Skipf("main.wasm carries the `name` custom section, so it was NOT built by `make wasm` "+
			"(which links -s -w and strips it) -- a *.browser.test.js suite rebuilds this file "+
			"unstripped and leaves it behind. Measuring it overstates the shipped size by the "+
			"~0.8 MiB section alone. Run `make wasm` and re-run this gate. (size on disk: %s)",
			mib(int64(len(wasmBytes))))
	}
	// A make wasm-dev build keeps the scene catalog and is not the shipped artifact either; the marker is a string only
	// the catalog holds.
	if bytes.Contains(wasmBytes, []byte(sceneCatalogMarker)) {
		t.Skipf("main.wasm contains the screenshot scene catalog, so it is a `make wasm-dev` "+
			"build, not the shipped one (`make wasm` builds with -tags noscenes). Run `make wasm` "+
			"and re-run this gate. (size on disk: %s)", mib(int64(len(wasmBytes))))
	}

	// From the SAME deploy manifest the drift test parses, so this never diverges from what ships.
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
			// A missing declared asset is the drift test's concern, not a size failure.
			t.Logf("  (skip %s: %v)", f, err)
			continue
		}
		raw := int64(len(b))
		// Stored uncompressed and loaded only after the game's download, so off the critical path, with their own budget.
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
	for cut, max := range map[string]int64{"desktop": landingDesktopMaxBytes, "phone": landingPhoneMaxBytes} {
		if got := landingMedia[cut]; got > max {
			t.Errorf("landing %s media %s exceeds budget %s (re-encode, or shorten the tour)", cut, mib(got), mib(max))
		}
	}
}

// Walks the section table only (id + LEB128 length), never decoding code. An unparseable file is evidence of nothing:
// false, and the caller measures it.
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
