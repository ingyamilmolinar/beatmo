package ui

import "strings"

// importSourceName reduces an import source (a file path on desktop, an upload
// filename on web, or a template display name) to the short label shown in the
// "Loaded …" notification. Separator-agnostic so a Windows path, a POSIX path,
// or a bare name all yield the trailing segment.
func importSourceName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
