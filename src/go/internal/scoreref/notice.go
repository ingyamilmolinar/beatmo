package scoreref

import (
	"fmt"
	"sort"
	"strings"
)

// NoticeFile is the committed attribution notice rendered from the catalogue.
const NoticeFile = "NOTICE-scores.md"

// Generated for every entry though CC0 requires none: it is the catalogue's human-readable face, and what makes the
// CC-BY-4.0 allowance satisfiable rather than merely permitted.
func RenderNotice() (string, error) {
	c, err := Catalogue()
	if err != nil {
		return "", err
	}

	byCorpus := map[string][]Score{}
	var corpora []string
	for _, s := range c.Scores {
		if _, ok := byCorpus[s.Source.Repo]; !ok {
			corpora = append(corpora, s.Source.Repo)
		}
		byCorpus[s.Source.Repo] = append(byCorpus[s.Source.Repo], s)
	}
	sort.Strings(corpora)

	var b strings.Builder
	b.WriteString("# Reference Scores — Attribution\n\n")
	b.WriteString("Generated from `internal/scoreref/catalogue.json` — do not edit by hand.\n")
	b.WriteString("Regenerate with `go test ./internal/scoreref/ -run TestNoticeIsUpToDate -update`.\n\n")
	b.WriteString("Every score below is a public-domain WORK in a permissively licensed\n")
	b.WriteString("ENCODING, committed verbatim under `internal/scoreref/scores/` (a MIDI\n")
	b.WriteString("edition beside the LilyPond source it was generated from). The\n")
	b.WriteString("generated template circuits in `internal/assets/templates/` are derived\n")
	b.WriteString("from these files. See `docs/score-sources.md` for the admission rule and\n")
	b.WriteString("the full survey of sources considered.\n")

	for _, repo := range corpora {
		list := byCorpus[repo]
		sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
		fmt.Fprintf(&b, "\n## %s\n\n", repo)
		// A corpus credits either one editor for everything or — Mutopia —
		// a different typesetter per piece; only the latter needs a column.
		perPiece := false
		for _, s := range list {
			if s.Source.Attribution != list[0].Source.Attribution {
				perPiece = true
			}
		}
		if perPiece {
			fmt.Fprintf(&b, "| Score | Work | Composer | Licence | Credit |\n|---|---|---|---|---|\n")
		} else {
			fmt.Fprintf(&b, "%s\n\n", list[0].Source.Attribution)
			fmt.Fprintf(&b, "| Score | Work | Composer | Licence |\n|---|---|---|---|\n")
		}
		for _, s := range list {
			work := s.Work.Title
			if s.Work.Catalog != "" {
				work += " (" + s.Work.Catalog + ")"
			}
			if perPiece {
				fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n",
					s.ID, work, s.Work.Composer, s.Source.License, s.Source.Attribution)
				continue
			}
			fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n",
				s.ID, work, s.Work.Composer, s.Source.License)
		}
	}
	return b.String(), nil
}
