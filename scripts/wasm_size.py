#!/usr/bin/env python3
"""Attribute a Go-built .wasm binary's bytes to sections, packages and functions.

Answers "where did the 28 MiB go?" for src/js/main.wasm: how much is Go code
(and whose), how much is static data, how much is embedded assets, how much is
zero padding the Go linker could not elide, and what any of it costs after gzip.

Usage:
    # Per-function/per-package attribution needs the `name` custom section,
    # which `make wasm` strips via -w. Build an unstripped twin first; its code
    # section is byte-identical to the shipped one, so the numbers transfer.
    cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build \
        -ldflags "-X main.defaultLog=INFO" -o /tmp/main.dbg.wasm ./cmd

    python3 scripts/wasm_size.py /tmp/main.dbg.wasm          # full report
    python3 scripts/wasm_size.py src/js/main.wasm            # sections only

Options:
    --top N     how many packages / functions to list (default 30)
    --gzip      also report gzip(-9) size (slow on a 30 MiB input)

Why raw and gzip differ so much: the data section is mostly repetitive JSON and
zero padding, which gzip erases. Raw size drives WASM decode + linear-memory
init; the CODE section drives compile time; the gzip size is what users
actually download. Read all three before deciding a budget is "too small".
"""

import argparse
import collections
import sys

SECTION_NAMES = {
    0: "custom", 1: "type", 2: "import", 3: "function", 4: "table",
    5: "memory", 6: "global", 7: "export", 8: "start", 9: "element",
    10: "code", 11: "data", 12: "datacount", 13: "tag",
}

# Hosts whose package paths contain a '.' before the first path separator.
# Go's wasm name mangling rewrites '/' to '_' but leaves '.' alone, so
# "github.com/x/y.Func" arrives as "github.com_x_y.Func" and a naive
# split('.') would bucket everything under "github".
DOT_HOSTS = ("github", "golang", "gopkg", "google", "cloud", "honnef", "go")

# The Kage shader compiler Ebiten runs at startup to build its own builtin
# shaders. Pulled in by ebiten/v2/internal/{shader,builtinshader}, not by us.
KAGE_DEPS = frozenset((
    "go/parser", "go/ast", "go/token", "go/scanner", "go/constant",
    "go/build/constraint", "go/printer", "go/format", "go/version",
    "math/big", "text/template", "text/template/parse",
    "regexp", "regexp/syntax", "internal/buildcfg",
))

BEATMO = "github.com/ingyamilmolinar/beatmo/"


def leb(buf, i):
    """Decode an unsigned LEB128 at buf[i:]; return (value, next_index)."""
    result = shift = 0
    while True:
        byte = buf[i]
        i += 1
        result |= (byte & 0x7F) << shift
        if not byte & 0x80:
            return result, i
        shift += 7


def mib(n):
    return f"{n / (1 << 20):.2f} MiB"


class Wasm:
    """A parsed WebAssembly module: sections, function sizes, symbol names."""

    def __init__(self, data):
        self.data = data
        self.sections = []          # [(section_id, custom_name, payload_off, size)]
        self.func_sizes = []        # code-section body size per defined function
        self.func_names = {}        # wasm function index -> mangled Go symbol
        self.num_imported_funcs = 0
        self.data_segments = []     # [(vaddr, length, payload_off)]
        self._parse()

    def _parse(self):
        d = self.data
        i = 8  # skip magic + version
        while i < len(d):
            sid = d[i]
            i += 1
            size, i = leb(d, i)
            custom = ""
            if sid == 0:
                n, j = leb(d, i)
                custom = d[j:j + n].decode("utf8", "replace")
                if custom == "name":
                    self._parse_names(i, size)
            elif sid == 2:
                self._parse_imports(i)
            elif sid == 10:
                self._parse_code(i)
            elif sid == 11:
                self._parse_data(i)
            self.sections.append((sid, custom, i, size))
            i += size

    def _parse_imports(self, off):
        d = self.data
        count, i = leb(d, off)
        for _ in range(count):
            for _ in range(2):                      # module name, field name
                n, i = leb(d, i)
                i += n
            kind = d[i]
            i += 1
            if kind == 0:                           # func
                self.num_imported_funcs += 1
                _, i = leb(d, i)
            elif kind == 1:                         # table
                i += 1
                limited = d[i]
                i += 1
                _, i = leb(d, i)
                if limited:
                    _, i = leb(d, i)
            elif kind == 2:                         # memory
                limited = d[i]
                i += 1
                _, i = leb(d, i)
                if limited:
                    _, i = leb(d, i)
            elif kind == 3:                         # global
                i += 2

    def _parse_code(self, off):
        d = self.data
        count, i = leb(d, off)
        for _ in range(count):
            body, j = leb(d, i)
            self.func_sizes.append(body + (j - i))  # body + its length prefix
            i = j + body

    def _parse_data(self, off):
        d = self.data
        count, i = leb(d, off)
        for _ in range(count):
            flags, i = leb(d, i)
            if flags != 0:                          # passive//indexed segment
                break
            i += 1                                  # i32.const opcode
            vaddr, i = leb(d, i)
            i += 1                                  # end opcode
            length, i = leb(d, i)
            self.data_segments.append((vaddr, length, i))
            i += length

    def _parse_names(self, off, size):
        d = self.data
        end = off + size
        n, i = leb(d, off)
        i += n                                      # skip the literal "name"
        while i < end:
            sub = d[i]
            i += 1
            sub_size, i = leb(d, i)
            nxt = i + sub_size
            if sub == 1:                            # function-name subsection
                count, j = leb(d, i)
                for _ in range(count):
                    idx, j = leb(d, j)
                    ln, j = leb(d, j)
                    self.func_names[idx] = d[j:j + ln].decode("utf8", "replace")
                    j += ln
            i = nxt


def package_of(symbol):
    """Map a mangled wasm symbol to the Go import path that defined it."""
    parts = symbol.split(".")
    if parts[0] in DOT_HOSTS and len(parts) > 1 and "_" in parts[1]:
        return (parts[0] + "." + parts[1]).replace("_", "/")
    head = parts[0]
    if head.startswith("type") or head.startswith("go:"):
        return "«linker-generated»"
    return head.replace("_", "/")


def report_sections(w):
    total = len(w.data)
    print(f"TOTAL {len(w.data):,} B ({mib(total)})\n")
    print("=== SECTIONS ===")
    for sid, custom, _, size in w.sections:
        label = SECTION_NAMES.get(sid, str(sid))
        print(f"  {label:10s} {custom:12s} {size:12,} B  {mib(size):>10}  "
              f"{100 * size / total:5.1f}%")


def report_data(w):
    """Zero padding, segment count, and the Go linker's 100k-segment cap."""
    if not w.data_segments:
        return
    payload = sum(l for _, l, _ in w.data_segments)
    framing = sum(
        len_leb(l) + len_leb(a) + 3 for a, l, _ in w.data_segments
    )
    zeros = sum(w.data[o:o + l].count(0) for _, l, o in w.data_segments)
    n = len(w.data_segments)
    print("\n=== DATA SECTION ===")
    print(f"  segments          {n:12,}"
          + ("   <-- AT the Go linker's maxNumSegments cap (100,000)" if n >= 100000 else ""))
    print(f"  payload           {payload:12,} B  {mib(payload):>10}")
    print(f"  segment framing   {framing:12,} B  {mib(framing):>10}")
    print(f"  zero bytes        {zeros:12,} B  {mib(zeros):>10}  "
          f"({100 * zeros / payload:.1f}% of payload)")
    if n >= 100000:
        print("  NOTE: past the cap the linker stops eliding zero runs and writes")
        print("        them literally. These bytes cost raw size only -- gzip")
        print("        removes them. Fewer Go functions is the only real lever.")


def len_leb(v):
    n = 1
    while v >= 0x80:
        v >>= 7
        n += 1
    return n


def report_code(w, top):
    if not w.func_sizes:
        return
    total = sum(w.func_sizes)
    if not w.func_names:
        print("\n=== CODE SECTION ===")
        print(f"  {total:,} B ({mib(total)}) over {len(w.func_sizes):,} functions")
        print("  No `name` section: rebuild WITHOUT -w for package attribution.")
        print("  (see the module docstring for the exact command)")
        return

    by_pkg = collections.Counter()
    fn_count = collections.Counter()
    funcs = []
    for k, size in enumerate(w.func_sizes):
        sym = w.func_names.get(w.num_imported_funcs + k, "<unnamed>")
        pkg = package_of(sym)
        by_pkg[pkg] += size
        fn_count[pkg] += 1
        funcs.append((size, sym))

    print(f"\n=== CODE SECTION BY PACKAGE ===  "
          f"{total:,} B ({mib(total)}), {len(w.func_sizes):,} functions")
    print(f"  {'bytes':>11} {'MiB':>9} {'%':>6} {'fns':>7}  package")
    for pkg, size in by_pkg.most_common(top):
        print(f"  {size:11,} {mib(size):>9} {100 * size / total:5.1f}% "
              f"{fn_count[pkg]:7,}  {pkg}")

    rollups = [
        ("beatmo application code", lambda p: p.startswith(BEATMO)),
        ("ebiten + its deps",       lambda p: "hajimehoshi" in p),
        ("Kage shader compiler (Ebiten's, not ours)", lambda p: p in KAGE_DEPS),
        ("Go runtime + GC",         lambda p: p == "runtime" or p.startswith("internal/runtime")),
        ("reflect + encoding/json", lambda p: p in ("reflect", "encoding/json", "internal/reflectlite")),
    ]
    print("\n=== ROLL-UPS ===")
    for label, pred in rollups:
        size = sum(s for p, s in by_pkg.items() if pred(p))
        print(f"  {size:11,} B {mib(size):>10} {100 * size / total:5.1f}%   {label}")

    print(f"\n=== TOP {top} FUNCTIONS ===")
    for size, sym in sorted(funcs, reverse=True)[:top]:
        print(f"  {size:9,} B  {sym[:110]}")


def main():
    ap = argparse.ArgumentParser(
        description="Attribute a Go .wasm binary's bytes to sections, packages and functions.")
    ap.add_argument("wasm", help="path to the .wasm binary")
    ap.add_argument("--top", type=int, default=30, help="rows per table (default 30)")
    ap.add_argument("--gzip", action="store_true", help="also report gzip(-9) size")
    args = ap.parse_args()

    with open(args.wasm, "rb") as f:
        data = f.read()
    if data[:4] != b"\0asm":
        sys.exit(f"{args.wasm}: not a WebAssembly binary")

    print(f"FILE {args.wasm}")
    w = Wasm(data)
    report_sections(w)
    if args.gzip:
        import gzip
        n = len(gzip.compress(data, 9))
        print(f"\n  gzip(-9)   {n:,} B  {mib(n)}   "
              f"({len(data) / n:.2f}x) <- what users download")
    report_data(w)
    report_code(w, args.top)


if __name__ == "__main__":
    main()
