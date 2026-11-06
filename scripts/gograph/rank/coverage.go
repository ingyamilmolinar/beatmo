package rank

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/cover"
)

// applyCoverage sets each unit's statement coverage from one or more Go
// coverage profiles. Profiles merge per block: a block counts as covered when
// any profile covered it (the tagged `make coverage-go` run never compiles the
// `!test` files, an untagged run covers those). A unit absent from every
// profile stays unknown (nil), never 0%; js-only units are always unknown.
// A profile older than a source file says nothing about that file (its line
// ranges point at code that has moved), so it is ignored for that file and
// counted in the returned warnings.
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
