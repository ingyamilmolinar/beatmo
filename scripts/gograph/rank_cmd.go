package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ingyamilmolinar/beatmo/scripts/gograph/rank"
)

// listFlag is a repeatable string flag.
type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

// runRank is `gograph rank`: measure, generate candidates, merge verdicts and
// suppressions, and write rank.json / rank.md / rank.html (+ packets).
func runRank(args []string) error {
	fs := flag.NewFlagSet("rank", flag.ExitOnError)
	root := fs.String("root", "../../src/go", "module source root")
	module := fs.String("module", "github.com/ingyamilmolinar/beatmo", "module import path")
	out := fs.String("out", "../../build/refactor", "output directory")
	var covers listFlag
	fs.Var(&covers, "cover", "Go coverage profile (repeatable; profiles merge per block)")
	baseline := fs.String("baseline", "", "previous rank.json to diff against")
	verdicts := fs.String("verdicts", "", "agent verdict directory (default <out>/verdicts)")
	suppress := fs.String("suppress", "refactor_suppress.json", "committed suppression list")
	top := fs.Int("top", 40, "candidates listed in detail in rank.md")
	packets := fs.Int("packets", 0, "write this many agent packets to <out>/packets (0 = none)")
	fast := fs.Bool("fast", false, "skip the test-reachability pass (unreachable is reported unsplit)")
	_ = fs.Parse(args)

	cfg := rank.DefaultConfig(*root, *module)
	cfg.Covers = covers
	cfg.Fast = *fast
	m, err := rank.Analyze(cfg)
	if err != nil {
		return err
	}
	cs := rank.Generate(m)
	vdir := *verdicts
	if vdir == "" {
		vdir = filepath.Join(*out, "verdicts")
	}
	vs, warns := rank.LoadVerdicts(vdir)
	sups, err := rank.LoadSuppressions(*suppress)
	if err != nil {
		return err
	}
	ov := rank.ApplyOverlays(cs, vs, sups)
	ov.Warnings = append(append(ov.Warnings, m.Warnings...), warns...)
	r := rank.BuildReport(m, cs, ov, 400)
	if *baseline != "" {
		old, err := rank.LoadReport(*baseline)
		if err != nil {
			return err
		}
		r.Delta = rank.Diff(old, r)
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	js, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	html, err := rank.RenderHTML(r)
	if err != nil {
		return err
	}
	for name, b := range map[string][]byte{
		"rank.json": append(js, '\n'),
		"rank.md":   []byte(rank.RenderMarkdown(r, *top)),
		"rank.html": html,
	} {
		if err := os.WriteFile(filepath.Join(*out, name), b, 0o644); err != nil {
			return err
		}
	}
	if *packets > 0 {
		n, err := rank.WritePackets(m, cs, vs, filepath.Join(*out, "packets"), *packets)
		if err != nil {
			return err
		}
		fmt.Printf("packets: %d written to %s\n", n, filepath.Join(*out, "packets"))
	}
	t := m.Totals
	fmt.Printf("units %d · Σ cognitive %d · units >30 %d · clone lines %d · dead %d funcs/%d lines · test-only %d funcs/%d lines · merged ids %d\n",
		t.Units, t.SumCognitive, t.UnitsOverCog30, t.CloneLOC, t.DeadUnits, t.DeadLOC, t.TestOnlyUnits, t.TestOnlyLOC, t.MergedIDs)
	fmt.Printf("candidates %d · verdicts %d · warnings %d\n", len(cs), len(vs), len(ov.Warnings))
	fmt.Printf("report: %s\n", filepath.Join(*out, "rank.md"))
	return nil
}
