package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

type ThemeSaveSink interface {
	LoadTheme() (theme.UserDoc, error)
	SaveTheme(d theme.UserDoc) error
}

var (
	themeSinkMu sync.RWMutex
	themeSink   ThemeSaveSink
)

// Nil keeps editing working in-session without persistence.
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

// Before the first frame; nothing stored means the default palette.
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
