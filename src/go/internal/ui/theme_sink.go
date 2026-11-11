package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// ThemeSaveSink is the persistence shim the Theme overlay writes the user's
// theme through and reads at startup. userprefs.ThemeStore satisfies it;
// tests use a stub. Mirrors LanguageSaveSink (language_sink.go).
type ThemeSaveSink interface {
	LoadTheme() (theme.UserDoc, error)
	SaveTheme(d theme.UserDoc) error
}

var (
	themeSinkMu sync.RWMutex
	themeSink   ThemeSaveSink
)

// SetThemeSink registers the global theme persistence sink. Nil-safe:
// passing nil keeps theme editing working in-session without persistence.
func SetThemeSink(s ThemeSaveSink) {
	themeSinkMu.Lock()
	themeSink = s
	themeSinkMu.Unlock()
}

func activeThemeSink() ThemeSaveSink {
	themeSinkMu.RLock()
	defer themeSinkMu.RUnlock()
	return themeSink
}

// ApplyStoredTheme seeds g.userTheme from the sink and applies the effective
// theme before the first frame. Called by the production bootstrap right
// after New(); no sink or empty theme ⇒ default palette.
func ApplyStoredTheme(g *Game) {
	s := activeThemeSink()
	if s == nil || g == nil {
		return
	}
	d, err := s.LoadTheme()
	if err != nil {
		themeWarnf("[theme] load user theme: %v", err)
		return
	}
	g.userTheme = d
	g.recomputeEffectiveTheme()
}

// persistUserTheme writes the user theme through the sink (no-op without one).
func persistUserTheme(d theme.UserDoc) {
	if s := activeThemeSink(); s != nil {
		if err := s.SaveTheme(d); err != nil {
			themeWarnf("[theme] save user theme: %v", err)
		}
	}
}
