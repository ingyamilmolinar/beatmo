package wavref

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The Makefile exports the same default, so make and the CLI behave alike.
var defaultRootRel = filepath.Join(".beatmo", "wav-refs")

// sourcesSubdir is the drop-in directory's name inside the root.
const sourcesSubdir = "sources"

// Anything else in the drop-in dir (a README, a DAW project) is ignored, so a bulk adopt never hands it to ffmpeg.
var audioExts = map[string]bool{
	".wav": true, ".aif": true, ".aiff": true, ".flac": true,
	".mp3": true, ".ogg": true, ".m4a": true,
}

// One tree, one variable: converted, hashed audio (all Resolve returns) beside the working dirs and sources/. Never "not
// configured": a drop-in directory you must export a variable to find is not a drop-in directory.
func Root() string {
	if d := os.Getenv(EnvVar); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return defaultRootRel
	}
	return filepath.Join(home, defaultRootRel)
}

// A subdirectory: one root to back up, ffmpeg never reads and writes one path on adopt, and raw takes stay invisible to
// Resolve until adopted.
func SourcesDir() string { return filepath.Join(Root(), sourcesSubdir) }

// A bare name is looked up in SourcesDir(); anything with a separator is used as given (a convenience, never a cage).
// A missing file is an error: adopting audio that does not exist must be seen.
func SourcePath(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("wavref: no source file named")
	}
	if strings.ContainsRune(name, filepath.Separator) {
		if _, err := os.Stat(name); err != nil {
			return "", fmt.Errorf("wavref: source %s: %w", name, err)
		}
		return name, nil
	}
	dir := SourcesDir()
	p := filepath.Join(dir, name)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("wavref: source %s not found in %s (set %s to change the root): %w",
			name, dir, EnvVar, err)
	}
	return p, nil
}

// Sorted absolute paths, audio only; a missing directory is zero files.
func ListSources() ([]string, error) {
	dir := SourcesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("wavref: read %s: %w", dir, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !audioExts[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}
