package ui

import "strings"

// Separator-agnostic: Windows paths, POSIX paths and bare names all yield the last segment.
func importSourceName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
