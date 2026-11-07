package rank

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/cover"
)

// applyCoverage merges profiles per block (covered in any = covered: the tagged run never compiles !test files). A unit
// in no profile, and every js-only unit, stays unknown, never 0 %. A profile older than a file is ignored for that file
// (its line ranges point at moved code) and reported.
func applyCoverage(m *Model, root, module string, covers []string) ([]string, error) {
	if len(covers) == 0 {
		return nil, nil
	}
	var warns []string
	type key struct {
		file                   string
		sl, sc, el, ec, nstmts int
	}
	covered := map[key]bool{}
	byFile := map[string][]key{}
	for _, path := range covers {
		profiles, err := cover.ParseProfiles(path)
		if err != nil {
			return nil, fmt.Errorf("coverage %s: %w", path, err)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		stale := 0
		for _, p := range profiles {
			rel := strings.TrimPrefix(p.FileName, module+"/")
			if src, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil || src.ModTime().After(info.ModTime()) {
				stale++
				continue
			}
			for _, b := range p.Blocks {
				k := key{rel, b.StartLine, b.StartCol, b.EndLine, b.EndCol, b.NumStmt}
				if _, seen := covered[k]; !seen {
					byFile[rel] = append(byFile[rel], k)
				}
				covered[k] = covered[k] || b.Count > 0
			}
		}
		if stale > 0 {
			warns = append(warns, fmt.Sprintf("coverage %s: ignored for %d of %d files changed since it was written (re-run the coverage target)", filepath.Base(path), stale, len(profiles)))
		}
	}
	for _, u := range m.Units {
		if u.JSOnly {
			continue
		}
		total, hit := 0, 0
		for _, k := range byFile[u.File] {
			if k.sl >= u.Line && k.el <= u.EndLine {
				total += k.nstmts
				if covered[k] {
					hit += k.nstmts
				}
			}
		}
		if total > 0 {
			c := round3(float64(hit) / float64(total))
			u.Coverage = &c
		}
	}
	return warns, nil
}
