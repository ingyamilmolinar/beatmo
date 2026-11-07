import json
import os
import shutil
import subprocess
import sys
import tempfile
import textwrap
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import comment_census as cc

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
GO = os.path.join(REPO, ".tools", "go", "bin", "go")


def src(s):
    return textwrap.dedent(s).lstrip("\n")


def kinds(text, lang):
    """One letter per line: c=code, m=comment only, t=code+comment, d=directive, b=blank."""
    out = []
    for ln in cc.scan(text, lang):
        if ln.directive:
            out.append("d")
        elif ln.code and ln.comment:
            out.append("t")
        elif ln.comment:
            out.append("m")
        elif ln.code:
            out.append("c")
        else:
            out.append("b")
    return "".join(out)


class GoLexer(unittest.TestCase):
    def test_comment_markers_inside_strings_are_code(self):
        self.assertEqual(kinds('s := "// no /* no */"\nr := \'"\' // yes\n', "go"), "ct")

    def test_escaped_quote_does_not_end_the_string(self):
        self.assertEqual(kinds('s := "a\\"// still string"\n', "go"), "c")

    def test_raw_string_spans_lines_and_hides_markers(self):
        text = "s := `line\n// not a comment\n/* nor this */`\nx := 1 // c\n"
        self.assertEqual(kinds(text, "go"), "ccct")

    def test_raw_string_has_no_escapes(self):
        self.assertEqual(kinds("s := `a\\` // c\n", "go"), "t")

    def test_block_comment_spans_lines_and_code_after_close(self):
        text = "/* a\n b\n*/ x := 1\ny := 2 /* c */ + 3\n"
        self.assertEqual(kinds(text, "go"), "mmtt")

    def test_directives_are_not_comments(self):
        text = src("""
            //go:build linux
            // +build linux
            //go:generate go run gen.go
            //export Foo
            //go:embed x.json
            //nolint:errcheck
            // a real comment
        """)
        self.assertEqual(kinds(text, "go"), "ddddddm")

    def test_cgo_preamble_is_code(self):
        text = src("""
            package audio

            // #cgo LDFLAGS: -lm
            // #include "drums.h"
            import "C"

            /*
            #include <stdlib.h>
            */
            import "C"
            // just a comment
            func f() {}
        """)
        self.assertEqual(kinds(text, "go"), "cbddcbdddcmc")


class JSLexer(unittest.TestCase):
    def test_regex_literal_hides_comment_markers(self):
        self.assertEqual(kinds("const r = /\\/\\/x/g; // c\n", "js"), "t")

    def test_regex_character_class_may_hold_a_slash(self):
        self.assertEqual(kinds("const r = /[/]/ // c\n", "js"), "t")
        self.assertEqual(kinds("if (/[/*]/.test(s)) x()\n", "js"), "c")

    def test_regex_after_keyword(self):
        self.assertEqual(kinds("return /a\\/\\/b/.test(s)\n", "js"), "c")

    def test_division_is_not_a_regex(self):
        self.assertEqual(kinds("x = a / b // half\ny = (a) / 2 / c\n", "js"), "tc")

    def test_template_literal_hides_markers_and_spans_lines(self):
        text = "const t = `a // b\n/* c */ ${x}`\n"
        self.assertEqual(kinds(text, "js"), "cc")

    def test_comment_inside_template_interpolation_is_a_comment(self):
        self.assertEqual(kinds("const t = `${x /* c */}`\n", "js"), "t")

    def test_nested_template_and_braces_in_interpolation(self):
        text = "const t = `${f({a: `in // ner`})} // still template`\nx() // c\n"
        self.assertEqual(kinds(text, "js"), "ct")

    def test_single_quote_with_escape(self):
        self.assertEqual(kinds("s = 'it\\'s // not'\n", "js"), "c")

    def test_js_directives(self):
        text = src("""
            #!/usr/bin/env node
            // @ts-check
            /* eslint-disable no-console */
            // eslint-disable-next-line
            /* global window */
            // real
        """)
        self.assertEqual(kinds(text, "js"), "dddddm")


class CLexer(unittest.TestCase):
    def test_strings_and_chars(self):
        text = '#include "a//b.h"\nchar c = \'/\'; // c\nchar d = \'\\\'\'; /* x */\n'
        self.assertEqual(kinds(text, "c"), "ctt")

    def test_line_comment_continues_after_backslash(self):
        self.assertEqual(kinds("// a \\\nstill comment\nint x;\n", "c"), "mmc")


class HashLexer(unittest.TestCase):
    def test_shell_hash_must_start_a_word(self):
        text = "echo $# ${#arr[@]} '#lit' \"#lit\" a#b  # real\nx=1\n"
        self.assertEqual(kinds(text, "sh"), "tc")

    def test_shell_heredoc_body_is_not_a_comment(self):
        text = src("""
            cat <<'EOF' > x.py
            # not a comment
            EOF
            cat <<-END
            \t# nor this
            \tEND
            # real
        """)
        self.assertEqual(kinds(text, "sh"), "ccccccm")

    def test_shebang_and_shellcheck_are_directives(self):
        self.assertEqual(kinds("#!/bin/bash\n# shellcheck disable=SC2086\n# c\n", "sh"), "ddm")

    def test_makefile(self):
        text = "FOO := bar # c\n# whole\n\techo \"#\" '#'\n"
        self.assertEqual(kinds(text, "make"), "tmc")

    def test_yaml(self):
        self.assertEqual(kinds('key: "a # b" # c\nurl: http://x#frag\n# c\n', "yaml"), "tcm")


class PythonLexer(unittest.TestCase):
    def test_python_uses_the_real_tokenizer(self):
        text = 'x = "#no"  # yes\n"""doc # no"""\n# whole\n'
        self.assertEqual(kinds(text, "py"), "tcm")

    def test_untokenizable_python_falls_back_to_hash_rules(self):
        self.assertEqual(kinds('x = "unterminated\n# c\n', "py"), "cm")


class Blocks(unittest.TestCase):
    def analyze(self, text, path="pkg/a.go"):
        return cc.analyze(path, text)

    def test_blank_line_and_code_split_blocks(self):
        text = src("""
            // a
            // b

            // c
            x := 1 // trailing
            // d
            /* e
               f */
            y := 2
        """)
        fs = self.analyze(text)
        self.assertEqual([(b.start, b.lines) for b in fs.blocks], [(1, 2), (4, 1), (6, 3)])
        self.assertEqual(fs.comment_lines, 6)
        self.assertEqual(fs.trailing_lines, 1)
        self.assertEqual(fs.code_lines, 2)
        self.assertEqual(fs.blank_lines, 1)

    def test_block_kinds(self):
        text = src("""
            // Package p is the header.
            package p

            // F is documented.
            func F() {
            \t// inside a body
            \tx := 1
            }

            // floating

            var y = 2
        """)
        fs = self.analyze(text)
        self.assertEqual([b.kind for b in fs.blocks], ["header", "doc", "body", "free"])

    def test_js_doc_kind(self):
        text = "/**\n * Doc.\n */\nexport function f() {}\n// lone\n\nconst x = 1\n"
        fs = self.analyze(text, "a.js")
        self.assertEqual([b.kind for b in fs.blocks], ["doc", "free"])

    def test_preview_is_the_first_text_line(self):
        fs = self.analyze("/*\n * Hello there\n */\nx()\n", "a.js")
        self.assertEqual(fs.blocks[0].preview, "Hello there")

    def test_comment_chars_count_only_comment_text(self):
        fs = self.analyze("x := 1 // abc\n// de\n")
        self.assertEqual(fs.comment_chars, len("// abc") + len("// de"))


class Classification(unittest.TestCase):
    def test_lang_for(self):
        self.assertEqual(cc.lang_for("a/b.go"), "go")
        self.assertEqual(cc.lang_for("a/b.mjs"), "js")
        self.assertEqual(cc.lang_for("a/Makefile"), "make")
        self.assertEqual(cc.lang_for("a/b.h"), "c")
        self.assertEqual(cc.lang_for(".github/workflows/ci.yml"), "yaml")
        self.assertIsNone(cc.lang_for("a/b.md"))

    def test_is_test(self):
        for p in ["a/x_test.go", "src/js/x.browser.test.js", "a/b.test.mjs", "scripts/comment_census_test.py"]:
            self.assertTrue(cc.is_test(p), p)
        for p in ["a/x.go", "src/js/audio.js", "a/test_helpers.go"]:
            self.assertFalse(cc.is_test(p), p)

    def test_is_generated(self):
        self.assertTrue(cc.is_generated("a/x.gen.go", "package a\n"))
        self.assertTrue(cc.is_generated("a/x.go", "// Code generated by foo. DO NOT EDIT.\n\npackage a\n"))
        self.assertTrue(cc.is_generated("a/x.go", "//go:build js\n\n// Code generated by foo; DO NOT EDIT.\npackage a\n"))
        self.assertFalse(cc.is_generated("a/x.go", "package a\n// Code generated elsewhere is fine\n"))

    def test_package_is_the_directory(self):
        self.assertEqual(cc.package_of("src/go/internal/ui/a.go"), "src/go/internal/ui")
        self.assertEqual(cc.package_of("Makefile"), ".")


class Ranking(unittest.TestCase):
    def setUp(self):
        self.files = [
            cc.analyze("p/a.go", "// 1\n// 2\n// 3\nx := 1\n"),
            cc.analyze("p/a_test.go", "// 1\nx := 1\n"),
            cc.analyze("q/b.go", "// 1\n// 2\nx := 1\n\n// 3\n// 4\n// 5\n// 6\ny := 1\n"),
        ]

    def test_blocks_rank_largest_first(self):
        top = cc.top_blocks(self.files, 10)
        self.assertEqual([(b.path, b.lines) for b in top], [("q/b.go", 4), ("p/a.go", 3), ("q/b.go", 2), ("p/a_test.go", 1)])

    def test_files_rank_by_comment_lines(self):
        self.assertEqual([f.path for f in cc.rank_files(self.files)], ["q/b.go", "p/a.go", "p/a_test.go"])

    def test_packages_aggregate_prod_and_test(self):
        pkgs = cc.rank_packages(self.files)
        self.assertEqual([p.name for p in pkgs], ["q", "p"])
        p = pkgs[1]
        self.assertEqual((p.files, p.comment_lines, p.test_comment_lines, p.largest_block), (2, 4, 1, 3))


class Collection(unittest.TestCase):
    def test_default_excludes(self):
        tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, tmp)
        files = {
            "src/a.go": "// c\npackage a\n",
            "src/a.gen.go": "// c\npackage a\n",
            "node_modules/x/y.js": "// c\n",
            "src/testdata/f.go": "// c\n",
            "src/c/miniaudio.h": "// c\n",
            "docs/x.md": "# c\n",
        }
        for p, t in files.items():
            os.makedirs(os.path.join(tmp, os.path.dirname(p)), exist_ok=True)
            with open(os.path.join(tmp, p), "w") as f:
                f.write(t)
        got = [f.path for f in cc.collect(tmp, use_git=False)]
        self.assertEqual(got, ["src/a.go"])
        got = sorted(f.path for f in cc.collect(tmp, use_git=False, include_generated=True))
        self.assertEqual(got, ["src/a.gen.go", "src/a.go"])


class CLI(unittest.TestCase):
    def run_cli(self, *args):
        tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, tmp)
        os.makedirs(os.path.join(tmp, "p"))
        with open(os.path.join(tmp, "p", "a.go"), "w") as f:
            f.write("// one\n// two\npackage p\n")
        return subprocess.run(
            [sys.executable, os.path.join(REPO, "scripts", "comment_census.py"), "--root", tmp, "--no-git", *args],
            capture_output=True, text=True, check=True,
        ).stdout

    def test_text_report_has_every_section(self):
        out = self.run_cli("--top", "5")
        for heading in ["Totals", "Largest comment blocks", "Files by comment lines", "Packages by comment lines"]:
            self.assertIn(heading, out)
        self.assertIn("p/a.go:1", out)

    def test_json_report(self):
        data = json.loads(self.run_cli("--json"))
        self.assertEqual(data["totals"]["comment_lines"], 2)
        self.assertEqual(data["blocks"][0]["path"], "p/a.go")
        self.assertEqual(data["packages"][0]["name"], "p")


class Policy(unittest.TestCase):
    def rules(self, text, path="p/a.go"):
        return [(v.path, v.line, v.rule) for v in cc.check_file(path, text)]

    def test_block_of_four_lines_is_too_long(self):
        self.assertEqual(self.rules("// 1\n// 2\n// 3\n// 4\nx := 1\n"), [("p/a.go", 1, "block-too-long")])
        self.assertEqual(self.rules("// 1\n// 2\n// 3\nx := 1\n"), [])

    def test_history_markers_are_flagged(self):
        for c in ["// fixed 2026-09-19", "// it used to panic", "// Phase 5b of the refactor",
                  "// pre-fix this leaked", "// for now both coexist", "// As of 2026 it is"]:
            self.assertEqual(self.rules(c + "\nx := 1\n"), [("p/a.go", 1, "history-marker")], c)
        for c in ["// phase 0 is the start of the cycle", 's := "2026-09-19"', "//go:build linux",
                  "// the callback used to check input", "// skip an entry that no longer validates",
                  "// strip the prefix"]:
            self.assertEqual(self.rules(c + "\nx := 1\n"), [], c)

    def test_trailing_comments_are_checked_for_markers(self):
        self.assertEqual(self.rules("x := 1 // it used to be 2\n"), [("p/a.go", 1, "history-marker")])

    def test_cgo_preamble_is_exempt(self):
        text = "// 2026-01-01\n// a\n// b\n// c\nimport \"C\"\n"
        self.assertEqual(self.rules(text), [])

    def test_other_languages(self):
        self.assertEqual(self.rules("# 1\n# 2\n# 3\n# 4\necho\n", "s.sh"), [("s.sh", 1, "block-too-long")])
        self.assertEqual(self.rules("x = 1  # historically needed\n", "a.py"), [("a.py", 1, "history-marker")])
        self.assertEqual(self.rules("plain text", "notes.txt"), [])


class Digest(unittest.TestCase):
    def same(self, a, b, lang):
        self.assertEqual(cc.code_digest(a, lang), cc.code_digest(b, lang), (lang, a, b))

    def differ(self, a, b, lang):
        self.assertNotEqual(cc.code_digest(a, lang), cc.code_digest(b, lang), (lang, a, b))

    def test_digest_ignores_comment_edits(self):
        cases = [
            ("go", "x := 1 // a\n", "// b\nx  :=  1\n"),
            ("go", "// doc\n// more\nfunc f() {\n\t/* x */ g()\n}\n", "func f() {\n  g()\n}\n"),
            ("js", "/* a */ const x = 1;\n", "const x = 1; // b\n\n"),
            ("c", "int x = 1; /* a\n b */\n", "  int x = 1;\n"),
            ("sh", "echo hi # a\n", "# b\necho hi\n"),
            ("make", "a:\n\techo # c\n", "# top\na:\n\techo\n"),
            ("py", "x = 1  # a\n", "# b\nx = 1\n"),
        ]
        for lang, a, b in cases:
            self.same(a, b, lang)

    def test_digest_sees_code_edits(self):
        cases = [
            ("go", "x := 1\n", "x := 2\n"),
            ("go", 's := "a b"\n', 's := "a c"\n'),
            ("go", "f()\ng()\n", "f()\n"),
            ("go", "a/*c*/b\n", "ab\n"),
            ("js", "const r = /a/;\n", "const r = /b/;\n"),
            ("c", "int x = 1;\n", "int x = 2;\n"),
            ("sh", "echo hi\n", "echo ho\n"),
            ("py", "x = 1\n", "x = 2\n"),
        ]
        for lang, a, b in cases:
            self.differ(a, b, lang)

    def test_digest_sees_directive_and_cgo_edits(self):
        self.differ("//go:build linux\n\npackage p\n", "package p\n", "go")
        self.differ("//export Foo\nfunc Foo() {}\n", "func Foo() {}\n", "go")
        cgo = '// #include "x.h"\n// #include "y.h"\nimport "C"\n'
        self.differ(cgo, '// #include "x.h"\nimport "C"\n', "go")
        self.differ("#!/bin/sh\necho\n", "echo\n", "sh")


class Selection(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp)
        for rel in ["p/a.go", "p/a_test.go", "q/b.go", "r/c.go"]:
            os.makedirs(os.path.join(self.tmp, os.path.dirname(rel)), exist_ok=True)
            self.write(rel, "// one\npackage x\n")

    def write(self, rel, text):
        with open(os.path.join(self.tmp, rel), "w") as f:
            f.write(text)

    def cli(self, *args):
        return subprocess.run(
            [sys.executable, os.path.join(REPO, "scripts", "comment_census.py"), "--root", self.tmp, "--no-git", *args],
            capture_output=True, text=True,
        )

    def paths(self, *args):
        res = self.cli("--json", "--top", "100", *args)
        self.assertEqual(res.returncode, 0, res.stderr)
        return sorted(f["path"] for f in json.loads(res.stdout)["files"])

    def test_only_and_tests_only_select_files(self):
        self.assertEqual(self.paths("--only", "p/*", "--tests-only"), ["p/a_test.go"])
        self.assertEqual(self.paths("--only", "p/*", "q/*", "--exclude", "p/a_test.go", "q/*"), ["p/a.go"])
        self.assertEqual(self.paths("--only", "p/*", "--only", "r/*", "--no-tests"), ["p/a.go", "r/c.go"])

    def test_cli_check_and_compare(self):
        self.write("p/a.go", "// 1\n// 2\n// 3\n// 4\npackage p\n\nvar x = 1 // c\n")
        res = self.cli("--check")
        self.assertEqual(res.returncode, 1)
        self.assertIn("p/a.go:1: block-too-long", res.stdout)

        before = os.path.join(self.tmp, "before.json")
        res = self.cli("--digest")
        self.assertEqual(res.returncode, 0, res.stderr)
        with open(before, "w") as f:
            f.write(res.stdout)
        self.assertIn("p/a.go", json.loads(res.stdout))

        self.write("p/a.go", "// why\npackage p\n\nvar x = 1\n")
        res = self.cli("--compare", before)
        self.assertEqual((res.returncode, res.stdout), (0, ""))
        self.assertEqual(self.cli("--check").returncode, 0)

        self.write("p/a.go", "// why\npackage p\n\nvar x = 2\n")
        res = self.cli("--compare", before)
        self.assertEqual(res.returncode, 1)
        self.assertEqual(res.stdout.split(), ["p/a.go"])

        os.remove(os.path.join(self.tmp, "q", "b.go"))
        self.assertIn("q/b.go", self.cli("--compare", before).stdout.split())


GO_ORACLE = r"""
package main

import (
	"bufio"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	out := map[string][]int{}
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		path := sc.Text()
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		lines := map[int]bool{}
		for _, g := range f.Comments {
			for _, c := range g.List {
				for l := fset.Position(c.Pos()).Line; l <= fset.Position(c.End()).Line; l++ {
					lines[l] = true
				}
			}
		}
		list := []int{}
		for l := range lines {
			list = append(list, l)
		}
		out[path] = list
	}
	json.NewEncoder(os.Stdout).Encode(out)
}
"""

JS_ORACLE = r"""
const esprima = require(process.argv[1]);
const fs = require('fs');
const out = {};
for (const path of fs.readFileSync(0, 'utf8').split('\n').filter(Boolean)) {
  let text = fs.readFileSync(path, 'utf8');
  if (text.startsWith('#!')) text = '//' + text.slice(2);
  try {
    const toks = esprima.tokenize(text, { comment: true, loc: true });
    const lines = new Set();
    for (const t of toks) {
      if (t.type !== 'LineComment' && t.type !== 'BlockComment') continue;
      for (let l = t.loc.start.line; l <= t.loc.end.line; l++) lines.add(l);
    }
    out[path] = [...lines];
  } catch (e) { /* esprima 4 predates syntax the repo uses */ }
}
process.stdout.write(JSON.stringify(out));
"""


def our_comment_lines(path):
    with open(path, encoding="utf-8", errors="replace") as f:
        text = f.read()
    return {i + 1 for i, ln in enumerate(cc.scan(text, cc.lang_for(path))) if ln.comment}


class Oracles(unittest.TestCase):
    """The hand lexers must agree with real parsers on every file in the repo."""

    def repo_files(self, lang):
        return [os.path.join(REPO, f.path) for f in cc.collect(REPO, include_generated=True) if f.lang == lang]

    def compare(self, oracle_out, min_files):
        self.assertGreaterEqual(len(oracle_out), min_files)
        bad = []
        for path, lines in oracle_out.items():
            ours = our_comment_lines(path)
            if ours != set(lines):
                bad.append((os.path.relpath(path, REPO), sorted(ours ^ set(lines))[:5]))
        self.assertEqual(bad, [])

    @unittest.skipUnless(os.path.exists(GO), "bundled go missing")
    def test_go_matches_go_parser(self):
        tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, tmp)
        with open(os.path.join(tmp, "oracle.go"), "w") as f:
            f.write(GO_ORACLE)
        files = self.repo_files("go")
        res = subprocess.run([GO, "run", "oracle.go"], cwd=tmp, input="\n".join(files),
                             capture_output=True, text=True, check=True,
                             env={**os.environ, "GOFLAGS": "", "GO111MODULE": "off"})
        self.compare(json.loads(res.stdout), min_files=len(files) * 9 // 10)

    @unittest.skipUnless(shutil.which("node") and os.path.isdir(os.path.join(REPO, "node_modules", "esprima")), "node/esprima missing")
    def test_js_matches_esprima_where_it_parses(self):
        files = self.repo_files("js")
        res = subprocess.run(["node", "-e", JS_ORACLE, os.path.join(REPO, "node_modules", "esprima")],
                             input="\n".join(files), capture_output=True, text=True, check=True)
        self.compare(json.loads(res.stdout), min_files=len(files) * 9 // 10)


if __name__ == "__main__":
    unittest.main()
