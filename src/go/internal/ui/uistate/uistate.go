// Package uistate provides a small declarative config for booting Beatmo
// into a specific UI state, used primarily by the screenshot harness. It
// covers fields that fall outside the existing tunkul.json import schema
// (camera position, splitter fraction, layout profile, view mode, sidebar).
//
// Anything covered by tunkul.json — circuits, instruments, EQ band gains,
// transport BPM/subdiv/length, master volume, send effects — should be
// loaded via the existing -import flag. This package complements that
// schema; it does not duplicate it.
package uistate

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ingyamilmolinar/beatmo/internal/ui"
)

// Config is the JSON schema for -ui-state. Pointers (*int, *bool) are
// nullable: a missing field leaves the corresponding game state alone.
type Config struct {
	Profile         string  `json:"profile,omitempty"`           // "desktop" | "mobile"
	ForceAutoSize   *bool   `json:"force_auto_size,omitempty"`
	DefaultStart    *bool   `json:"default_start,omitempty"`

	Camera *CameraConfig   `json:"camera,omitempty"`
	Splitter *SplitterConfig `json:"splitter,omitempty"`
	Sidebar *SidebarConfig  `json:"sidebar,omitempty"`
	View    *ViewConfig     `json:"view,omitempty"`
}

type CameraConfig struct {
	X      *float64 `json:"x,omitempty"`
	Y      *float64 `json:"y,omitempty"`
	Scale  *float64 `json:"scale,omitempty"`
	Center bool     `json:"center,omitempty"`
}

type SplitterConfig struct {
	Frac *float64 `json:"frac,omitempty"` // 0..1; horizontal => Y/winH, vertical => X/winW
}

type SidebarConfig struct {
	OpenNodeID *int `json:"open_node_id,omitempty"`
}

type ViewConfig struct {
	Mode               string `json:"mode,omitempty"` // "rows" | "audio"
	MobileEQCollapsed  *bool  `json:"mobile_eq_collapsed,omitempty"`
}

// applier captures every Game setter Apply touches. Decoupling the
// package from *ui.Game lets us unit-test the decision tree without the
// stub-Ebiten harness.
type applier interface {
	SetForceMobileProfile(bool)
	SetForceAutoSize(bool)
	SetDefaultStart(bool)
	SetSplitterFrac(float64)
	SetCameraOffsetX(float64)
	SetCameraOffsetY(float64)
	SetCameraScale(float64)
	CenterCamera()
	SetViewMode(audio bool)
	SetMobileEQCollapsed(bool)
	OpenSidebarForNodeID(int)
}

// ApplyFile loads cfg from path and applies it.
func ApplyFile(g *ui.Game, path string) error {
	return applyFile(g, path)
}

func applyFile(a applier, path string) error {
	c, err := loadFile(path)
	if err != nil {
		return err
	}
	return apply(a, c)
}

func loadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read ui-state: %w", err)
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse ui-state: %w", err)
	}
	return c, nil
}

// Apply walks the config in deterministic order: profile → defaults →
// splitter → camera → view → sidebar. Menu state is intentionally not in
// Config; scenes own that.
func Apply(g *ui.Game, c Config) error {
	return apply(g, c)
}

func apply(a applier, c Config) error {
	if c.Profile == "mobile" {
		a.SetForceMobileProfile(true)
	}
	if c.ForceAutoSize != nil {
		a.SetForceAutoSize(*c.ForceAutoSize)
	}
	if c.DefaultStart != nil {
		a.SetDefaultStart(*c.DefaultStart)
	}
	if c.Splitter != nil && c.Splitter.Frac != nil {
		a.SetSplitterFrac(*c.Splitter.Frac)
	}
	if c.Camera != nil {
		if c.Camera.X != nil {
			a.SetCameraOffsetX(*c.Camera.X)
		}
		if c.Camera.Y != nil {
			a.SetCameraOffsetY(*c.Camera.Y)
		}
		if c.Camera.Scale != nil {
			a.SetCameraScale(*c.Camera.Scale)
		}
		if c.Camera.Center {
			a.CenterCamera()
		}
	}
	if c.View != nil {
		switch c.View.Mode {
		case "rows":
			a.SetViewMode(false)
		case "audio":
			a.SetViewMode(true)
		}
		if c.View.MobileEQCollapsed != nil {
			a.SetMobileEQCollapsed(*c.View.MobileEQCollapsed)
		}
	}
	if c.Sidebar != nil && c.Sidebar.OpenNodeID != nil {
		a.OpenSidebarForNodeID(*c.Sidebar.OpenNodeID)
	}
	return nil
}
