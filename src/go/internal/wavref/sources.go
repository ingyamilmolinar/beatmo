package wavref

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// defaultRootRel is the reference-audio root under $HOME when EnvVar is
// unset. It matches the default the Makefile exports, so the tooling behaves
// the same whether you go through make or call the CLI directly.
var defaultRootRel = filepath.Join(".beatmo", "wav-refs")

// sourcesSubdir is the drop-in directory's name inside the root.
const sourcesSubdir = "sources"

// audioExts are the container extensions a drop-in may use. Everything else
// in the drop-in dir (a README, a DAW project, a stray .txt) is ignored, so
// a bulk adopt never hands a non-audio file to ffmpeg.
var audioExts = map[string]bool{
	".wav": true, ".aif": true, ".aiff": true, ".flac": true,
	".mp3": true, ".ogg": true, ".m4a": true,
}

// Root is THE reference-audio directory: $BEATMO_WAV_REFS, or
// ~/.beatmo/wav-refs when that is unset.
//
// It is deliberately one tree with one environment variable. The root itself
// holds CONVERTED, catalogue-hashed audio -- the only audio Resolve will ever
// hand back -- alongside the tool's working directories (candidates/,
// compare/, probes/) and the sources/ drop-in dir.
//
// Unlike Dir(), Root never reports "not configured": a drop-in directory you
// have to export a variable to find is not a drop-in directory. Resolve and
// Path read from Root, so `go test` finds the same audio with or without the
// variable the Makefile exports.
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

// SourcesDir is the drop-in directory for RAW recordings waiting to be
// adopted as references: $BEATMO_WAV_REFS/sources.
//
// It is a subdirectory rather than a sibling tree for two reasons. One root
// means one place to back up and one variable to set. And the drop-in must
// not be the root itself: adopting snare-2.wav would otherwise have ffmpeg
// read and write the same path. Keeping raw takes one level down also keeps
// them invisible to Resolve, so an unconverted drop-in can never be mistaken
// for a canonical, hashed reference.
func SourcesDir() string { return filepath.Join(Root(), sourcesSubdir) }

// SourcePath resolves a -wav argument to an existing file.
//
// A bare filename ("snare_hot.wav") is looked up inside SourcesDir(); a name
// carrying a separator ("./x.wav", "/tmp/x.wav") is used exactly as given,
// so the drop-in dir stays a convenience rather than a cage. A file that
// isn't there is an error rather than a silent miss: adopting audio that
// doesn't exist is a mistake the caller has to see.
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

// ListSources returns the audio files waiting in the drop-in directory,
// sorted, as absolute paths. Directories and non-audio files are skipped.
// A missing directory is zero files rather than an error -- nobody has
// dropped anything yet.
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
