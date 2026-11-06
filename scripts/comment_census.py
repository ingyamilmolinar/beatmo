#!/usr/bin/env python3
"""Count comment lines and blocks per file and package, and rank them.

    python3 scripts/comment_census.py [--top N] [--json] [--lang go] [--no-tests]

Tests: python3 scripts/comment_census_test.py (cross-checks the lexers against
go/parser and esprima on every file in the repo).
"""
import argparse
import fnmatch
import io
import json
import os
import re
import subprocess
import sys
import tokenize
from dataclasses import dataclass, field, asdict

EXCLUDED_DIRS = {
    ".git", ".tools", "node_modules", "vendor", "testdata", "build", "coverage",
    "bench-results", "screenshots", "scratchpad", "tmp", "tunkul", "dist",
}
EXCLUDED_GLOBS = ["src/c/miniaudio.*"]

EXT_LANG = {
    ".go": "go", ".js": "js", ".mjs": "js", ".cjs": "js", ".c": "c", ".h": "c",
    ".py": "py", ".sh": "sh", ".bash": "sh", ".yml": "yaml", ".yaml": "yaml", ".mk": "make",
}

DIRECTIVES = {
    "go": re.compile(r"^//(go:\w|export |line |extern |nolint|lint:)|^// \+build"),
    "js": re.compile(r"^#!|^//\s*(@ts-|eslint|prettier-ignore|istanbul|c8 )|^/\*\s*(eslint|global\s|istanbul|c8 )"),
    "c": None,
    "py": re.compile(r"^#!|^#.*-\*-\s*coding|^#\s*(type:|noqa|pragma)"),
    "sh": re.compile(r"^#!|^#\s*shellcheck"),
    "yaml": re.compile(r"^#\s*yaml-language-server"),
    "make": None,
}

DECLS = {
    "go": re.compile(r"^(func|type|var|const)\b"),
    "js": re.compile(r"^(export\s+)?(default\s+)?(async\s+)?(function|class|const|let|var)\b"),
    "c": re.compile(r"^(?!#\s*(include|if|ifdef|ifndef|endif|else|elif|pragma)\b)[A-Za-z_#]"),
    "py": re.compile(r"^(async\s+)?(def|class)\b"),
    "sh": re.compile(r"^(function\s+\w+|\w+\s*\(\s*\))"),
    "yaml": None,
    "make": None,
}

GENERATED = re.compile(r"Code generated .*DO NOT EDIT")
JS_REGEX_KEYWORDS = {"return", "typeof", "case", "do", "else", "in", "of", "new", "delete", "void", "throw", "yield", "await", "instanceof"}
JS_REGEX_AFTER = set("(,=:[!&|?{};+-*%<>~^")


@dataclass
class Line:
    text: str
    code: bool = False
    comment: bool = False
    directive: bool = False
    ctext: str = ""


@dataclass
class Block:
    path: str
    start: int
    lines: int
    kind: str
    chars: int
    preview: str


@dataclass
class FileStats:
    path: str
    lang: str
    package: str
    is_test: bool
    total_lines: int = 0
    code_lines: int = 0
    comment_lines: int = 0
    trailing_lines: int = 0
    directive_lines: int = 0
    blank_lines: int = 0
    comment_chars: int = 0
    blocks: list = field(default_factory=list)

    @property
    def largest_block(self):
        return max((b.lines for b in self.blocks), default=0)


@dataclass
class Package:
    name: str
    files: int = 0
    code_lines: int = 0
    comment_lines: int = 0
    test_comment_lines: int = 0
    trailing_lines: int = 0
    comment_chars: int = 0
    blocks: int = 0
    largest_block: int = 0


def lang_for(path):
    base = os.path.basename(path)
    if base in ("Makefile", "GNUmakefile"):
        return "make"
    return EXT_LANG.get(os.path.splitext(base)[1])


def is_test(path):
    base = os.path.basename(path)
    return bool(re.search(r"_test\.(go|py)$|\.test\.m?js$|^test_.*\.py$", base))


def package_of(path):
    return os.path.dirname(path) or "."


def is_generated(path, text):
    if ".gen." in os.path.basename(path):
        return True
    for line in text.splitlines()[:40]:
        s = line.strip()
        if GENERATED.search(s) and s[:2] in ("//", "/*", "# ", " *"):
            return True
        if s and not s.startswith(("//", "/*", "*", "#")):
            return False
    return False


def _split(text):
    parts = text.split("\n")
    if parts and parts[-1] == "":
        parts.pop()
    return [Line(p) for p in parts]


def _scan_clike(text, lang):
    lines = _split(text)
    if not lines:
        return lines
    js, go = lang == "js", lang == "go"
    n = len(text)
    li, i = 0, 0
    state = "code"
    quote = ""
    tpl_stack = []  # brace depth of each open ${ } interpolation
    prev, prev_word, word = None, "", ""

    def mark(kind):
        if li < len(lines):
            setattr(lines[li], kind, True)

    def addc(s):
        if li < len(lines):
            lines[li].ctext += s

    if js and text.startswith("#!"):
        state = "line"
    while i < n:
        ch = text[i]
        nx = text[i + 1] if i + 1 < n else ""
        if ch == "\n":
            if state == "line" and not (lang == "c" and i > 0 and text[i - 1] == "\\"):
                state = "code"
            elif state == "str":
                state = "code"
            elif state in ("regex", "class"):
                state = "code"
            li += 1
            word = ""
            if state in ("block", "line"):
                mark("comment")
            elif state in ("raw", "tpl"):
                mark("code")
            i += 1
            continue
        if state == "code":
            if ch == "/" and nx == "/":
                state = "line"
                mark("comment")
                addc("//")
                i += 2
                continue
            if ch == "/" and nx == "*":
                state = "block"
                mark("comment")
                addc("/*")
                i += 2
                continue
            if not ch.isspace():
                mark("code")
            if ch.isalnum() or ch in "_$":
                word += ch
                prev = "w"
                prev_word = word
                i += 1
                continue
            word = ""
            if ch in "\"'":
                state, quote = "str", ch
                prev = ch
            elif ch == "`" and go:
                state = "raw"
                prev = ch
            elif ch == "`" and js:
                state = "tpl"
            elif js and ch == "/" and (prev is None or prev in JS_REGEX_AFTER or (prev == "w" and prev_word in JS_REGEX_KEYWORDS)):
                state = "regex"
            elif js and tpl_stack and ch == "{":
                tpl_stack[-1] += 1
                prev = ch
            elif js and tpl_stack and ch == "}":
                if tpl_stack[-1] == 0:
                    tpl_stack.pop()
                    state = "tpl"
                else:
                    tpl_stack[-1] -= 1
                    prev = ch
            elif not ch.isspace():
                prev = ch
            i += 1
            continue
        if state == "line":
            mark("comment")
            addc(ch)
            i += 1
            continue
        if state == "block":
            mark("comment")
            if ch == "*" and nx == "/":
                addc("*/")
                state = "code"
                i += 2
                continue
            addc(ch)
            i += 1
            continue
        mark("code")
        if state == "str":
            if ch == "\\" and nx != "\n":
                i += 2
                continue
            if ch == quote:
                state = "code"
        elif state == "raw":
            if ch == "`":
                state = "code"
        elif state == "tpl":
            if ch == "\\" and nx != "\n":
                i += 2
                continue
            if ch == "`":
                state = "code"
                prev = "`"
            elif ch == "$" and nx == "{":
                tpl_stack.append(0)
                state = "code"
                prev = "{"
                i += 2
                continue
        elif state == "regex":
            if ch == "\\" and nx != "\n":
                i += 2
                continue
            if ch == "[":
                state = "class"
            elif ch == "/":
                state = "code"
                prev = "w"
                prev_word = ""
        elif state == "class":
            if ch == "\\" and nx != "\n":
                i += 2
                continue
            if ch == "]":
                state = "regex"
        i += 1
    if go:
        _mark_cgo_preamble(lines)
    return lines


def _mark_cgo_preamble(lines):
    for idx, ln in enumerate(lines):
        if ln.text.strip() != 'import "C"':
            continue
        j = idx - 1
        while j >= 0 and lines[j].comment and not lines[j].code:
            lines[j].directive = True
            j -= 1


HEREDOC = re.compile(r"<<(-?)\s*(['\"]?)([A-Za-z_][A-Za-z0-9_]*)\2")


def _scan_hash(text, lang):
    lines = _split(text)
    sh = lang == "sh"
    quote = ""
    heredocs = []  # pending (terminator, strip_tabs), in order
    current = None
    for ln in lines:
        if current:
            ln.code = True
            term, strip = current
            if (ln.text.lstrip("\t") if strip else ln.text) == term:
                current = heredocs.pop(0) if heredocs else None
            continue
        if not sh:
            quote = ""
        t = ln.text
        i = 0
        while i < len(t):
            ch = t[i]
            if quote:
                ln.code = True
                if ch == "\\" and quote == '"':
                    i += 2
                    continue
                if ch == quote:
                    quote = ""
                i += 1
                continue
            at_word_start = i == 0 or t[i - 1].isspace()
            if ch == "#" and at_word_start:
                ln.comment = True
                ln.ctext = t[i:]
                break
            if not ch.isspace():
                ln.code = True
            if ch in "'\"" and (sh or at_word_start or t[i - 1] in ":[{,-=("):
                quote = ch
            elif ch == "\\":
                i += 2
                continue
            elif sh and t.startswith("<<", i) and not t.startswith("<<<", i):
                m = HEREDOC.match(t, i)
                if m:
                    heredocs.append((m.group(3), m.group(1) == "-"))
                    i = m.end()
                    continue
            i += 1
        if heredocs and current is None:
            current = heredocs.pop(0)
    return lines


def _scan_py(text):
    lines = _split(text)
    try:
        for tok in tokenize.generate_tokens(io.StringIO(text).readline):
            if tok.type == tokenize.COMMENT:
                ln = lines[tok.start[0] - 1]
                ln.comment = True
                ln.ctext = tok.string
            elif tok.type not in (tokenize.NL, tokenize.NEWLINE, tokenize.INDENT, tokenize.DEDENT, tokenize.ENDMARKER):
                for r in range(tok.start[0], min(tok.end[0], len(lines)) + 1):
                    lines[r - 1].code = True
    except (tokenize.TokenError, SyntaxError):
        return _scan_hash(text, "pyfallback")
    return lines


def scan(text, lang):
    if lang in ("go", "js", "c"):
        lines = _scan_clike(text, lang)
    elif lang == "py":
        lines = _scan_py(text)
    else:
        lines = _scan_hash(text, lang)
    rx = DIRECTIVES.get(lang)
    if rx:
        for ln in lines:
            if ln.comment and rx.search(ln.ctext.strip()):
                ln.directive = True
    return lines


MARKERS = re.compile(r"^\s*(//+|/\*+|\*+/?|#+)\s?|\s*\*+/\s*$")


def _preview(lines):
    for ln in lines:
        s = MARKERS.sub("", ln.ctext).strip()
        if re.search(r"\w", s):
            return s[:100]
    return ""


def _kind(lang, lines, start, end):
    first = lines[start].text
    if first[: len(first) - len(first.lstrip())]:
        return "body"
    rx = DECLS.get(lang)
    if end + 1 < len(lines) and lines[end + 1].code and rx and rx.search(lines[end + 1].text):
        return "doc"
    if not any(ln.code for ln in lines[:start]):
        return "header"
    return "free"


def analyze(path, text):
    lang = lang_for(path)
    fs = FileStats(path=path, lang=lang, package=package_of(path), is_test=is_test(path))
    lines = scan(text, lang)
    fs.total_lines = len(lines)
    run_start = None
    for idx, ln in enumerate(lines + [Line("", code=True)]):
        only = ln.comment and not ln.code and not ln.directive
        if idx < len(lines):
            if ln.directive:
                fs.directive_lines += 1
            elif ln.code and ln.comment:
                fs.trailing_lines += 1
            elif only:
                fs.comment_lines += 1
            elif not ln.code:
                fs.blank_lines += 1
            if ln.code:
                fs.code_lines += 1
            if ln.comment and not ln.directive:
                fs.comment_chars += len(ln.ctext)
        if only and run_start is None:
            run_start = idx
        elif not only and run_start is not None:
            run = lines[run_start:idx]
            fs.blocks.append(Block(path, run_start + 1, len(run), _kind(lang, lines, run_start, idx - 1),
                                   sum(len(r.ctext) for r in run), _preview(run)))
            run_start = None
    return fs


def _excluded(rel, extra):
    parts = rel.split("/")
    if any(p in EXCLUDED_DIRS for p in parts[:-1]):
        return True
    return any(fnmatch.fnmatch(rel, g) for g in EXCLUDED_GLOBS + list(extra))


def _list(root, use_git):
    if use_git:
        try:
            out = subprocess.run(["git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard"],
                                 capture_output=True, check=True).stdout
            return sorted(set(p for p in out.decode().split("\0") if p))
        except (OSError, subprocess.CalledProcessError):
            pass
    found = []
    for d, dirs, files in os.walk(root):
        dirs[:] = [x for x in dirs if x not in EXCLUDED_DIRS]
        found += [os.path.relpath(os.path.join(d, f), root).replace(os.sep, "/") for f in files]
    return sorted(found)


def collect(root, use_git=True, include_generated=False, include_tests=True, langs=None, exclude=()):
    result = []
    for rel in _list(root, use_git):
        lang = lang_for(rel)
        if not lang or (langs and lang not in langs) or _excluded(rel, exclude):
            continue
        if not include_tests and is_test(rel):
            continue
        full = os.path.join(root, rel)
        if not os.path.isfile(full):
            continue
        with open(full, encoding="utf-8", errors="replace") as f:
            text = f.read()
        if not include_generated and is_generated(rel, text):
            continue
        result.append(analyze(rel, text))
    return result


def top_blocks(files, n):
    blocks = [b for f in files for b in f.blocks]
    return sorted(blocks, key=lambda b: (-b.lines, b.path, b.start))[:n]


def rank_files(files):
    return sorted(files, key=lambda f: (-f.comment_lines, f.path))


def rank_packages(files):
    pkgs = {}
    for f in files:
        p = pkgs.setdefault(f.package, Package(f.package))
        p.files += 1
        p.code_lines += f.code_lines
        p.comment_lines += f.comment_lines
        p.test_comment_lines += f.comment_lines if f.is_test else 0
        p.trailing_lines += f.trailing_lines
        p.comment_chars += f.comment_chars
        p.blocks += len(f.blocks)
        p.largest_block = max(p.largest_block, f.largest_block)
    return sorted(pkgs.values(), key=lambda p: (-p.comment_lines, p.name))


SIZE_BUCKETS = [(1, 1), (2, 2), (3, 5), (6, 10), (11, 20), (21, 50), (51, 10**9)]


def summarize(files):
    def tot(fs):
        return {
            "files": len(fs),
            "lines": sum(f.total_lines for f in fs),
            "code_lines": sum(f.code_lines for f in fs),
            "comment_lines": sum(f.comment_lines for f in fs),
            "trailing_lines": sum(f.trailing_lines for f in fs),
            "directive_lines": sum(f.directive_lines for f in fs),
            "comment_chars": sum(f.comment_chars for f in fs),
            "blocks": sum(len(f.blocks) for f in fs),
        }

    by_lang = {lang: tot([f for f in files if f.lang == lang]) for lang in sorted({f.lang for f in files})}
    blocks = [b for f in files for b in f.blocks]
    buckets = []
    for lo, hi in SIZE_BUCKETS:
        sel = [b for b in blocks if lo <= b.lines <= hi]
        label = str(lo) if lo == hi else (f"{lo}+" if hi > 10**8 else f"{lo}-{hi}")
        buckets.append({"size": label, "blocks": len(sel), "lines": sum(b.lines for b in sel)})
    kinds = {}
    for b in blocks:
        k = kinds.setdefault(b.kind, {"blocks": 0, "lines": 0})
        k["blocks"] += 1
        k["lines"] += b.lines
    return {
        "totals": tot(files),
        "prod": tot([f for f in files if not f.is_test]),
        "test": tot([f for f in files if f.is_test]),
        "by_lang": by_lang,
        "block_sizes": buckets,
        "block_kinds": dict(sorted(kinds.items())),
    }


def _pct(c, code):
    return f"{100 * c / (c + code):.0f}%" if c + code else "-"


def report_text(files, top, root):
    s = summarize(files)
    out = [f"Comment census of {root} ({len(files)} files; generated and vendored code excluded)", ""]
    out.append("Totals  (comment = comment-only lines; trailing = code line with a comment; % = comment/(comment+code))")
    out.append(f"  {'':8} {'files':>6} {'lines':>8} {'code':>8} {'comment':>8} {'trailing':>8} {'blocks':>7} {'%':>5}")
    rows = list(s["by_lang"].items()) + [("prod", s["prod"]), ("test", s["test"]), ("all", s["totals"])]
    for name, t in rows:
        out.append(f"  {name:8} {t['files']:6} {t['lines']:8} {t['code_lines']:8} {t['comment_lines']:8} "
                   f"{t['trailing_lines']:8} {t['blocks']:7} {_pct(t['comment_lines'], t['code_lines']):>5}")
    out += ["", "Block sizes"]
    for b in s["block_sizes"]:
        out.append(f"  {b['size']:>6} lines: {b['blocks']:7} blocks {b['lines']:8} lines")
    out += ["", "Block kinds  (doc = above a top-level declaration, body = indented, header = before the first code)"]
    for k, v in s["block_kinds"].items():
        out.append(f"  {k:8} {v['blocks']:7} blocks {v['lines']:8} lines")
    out += ["", f"Largest comment blocks (top {top})", f"  {'lines':>5} {'kind':6} location"]
    for b in top_blocks(files, top):
        out.append(f"  {b.lines:5} {b.kind:6} {b.path}:{b.start}  {b.preview}")
    out += ["", f"Files by comment lines (top {top})",
            f"  {'comment':>7} {'code':>6} {'%':>4} {'blocks':>6} {'max':>4} path"]
    for f in rank_files(files)[:top]:
        out.append(f"  {f.comment_lines:7} {f.code_lines:6} {_pct(f.comment_lines, f.code_lines):>4} "
                   f"{len(f.blocks):6} {f.largest_block:4} {f.path}")
    out += ["", f"Packages by comment lines (top {top})",
            f"  {'comment':>7} {'tests':>6} {'code':>7} {'%':>4} {'files':>5} {'blocks':>6} {'max':>4} package"]
    for p in rank_packages(files)[:top]:
        out.append(f"  {p.comment_lines:7} {p.test_comment_lines:6} {p.code_lines:7} {_pct(p.comment_lines, p.code_lines):>4} "
                   f"{p.files:5} {p.blocks:6} {p.largest_block:4} {p.name}")
    return "\n".join(out) + "\n"


def report_json(files, top):
    data = summarize(files)
    data["blocks"] = [asdict(b) for b in top_blocks(files, top)]
    data["files"] = [{k: v for k, v in asdict(f).items() if k != "blocks"} | {"blocks": len(f.blocks), "largest_block": f.largest_block}
                     for f in rank_files(files)[:top]]
    data["packages"] = [asdict(p) for p in rank_packages(files)[:top]]
    return json.dumps(data, indent=1)


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--root", default=os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    ap.add_argument("--top", type=int, default=25)
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--lang", action="append", help="restrict to a language (repeatable): go js c py sh yaml make")
    ap.add_argument("--no-tests", action="store_true", help="skip test files")
    ap.add_argument("--include-generated", action="store_true")
    ap.add_argument("--exclude", action="append", default=[], help="extra path glob to skip (repeatable)")
    ap.add_argument("--no-git", action="store_true", help="walk the directory instead of git ls-files")
    a = ap.parse_args(argv)
    files = collect(a.root, use_git=not a.no_git, include_generated=a.include_generated,
                    include_tests=not a.no_tests, langs=a.lang, exclude=a.exclude)
    sys.stdout.write(report_json(files, a.top) if a.json else report_text(files, a.top, a.root))


if __name__ == "__main__":
    main()
