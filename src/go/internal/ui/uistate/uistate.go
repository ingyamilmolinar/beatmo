// Boot-time UI state the project file does not carry (camera, splitter, profile, view, sidebar), for the
// screenshot harness; circuits and audio settings still load through -import.
package uistate

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/ingyamilmolinar/beatmo/internal/enum"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
	"github.com/ingyamilmolinar/beatmo/internal/ui"
)

// Pointers: a missing field leaves that state alone.
type Config struct {
	schema.Envelope
	Profile       string `json:"profile,omitempty"` // "desktop" | "mobile"
	ForceAutoSize *bool  `json:"force_auto_size,omitempty"`
	DefaultStart  *bool  `json:"default_start,omitempty"`

	Camera   *CameraConfig   `json:"camera,omitempty"`
	Splitter *SplitterConfig `json:"splitter,omitempty"`
	Sidebar  *SidebarConfig  `json:"sidebar,omitempty"`
	View     *ViewConfig     `json:"view,omitempty"`
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
	Mode              ViewMode `json:"mode,omitempty"`
	MobileEQCollapsed *bool    `json:"mobile_eq_collapsed,omitempty"`
}

// ViewMode is the mobile view a config boots into (ViewUnset = leave it).
type ViewMode int

const (
	ViewUnset ViewMode = iota
	ViewRows
	ViewAudio
)

var viewModeNames = enum.Names[ViewMode]{Kind: "view mode", Names: []string{"", "rows", "audio"}}

func (m ViewMode) String() string                { return enum.String(viewModeNames, m) }
func (m ViewMode) MarshalText() ([]byte, error)  { return enum.Marshal(viewModeNames, m) }
func (m *ViewMode) UnmarshalText(b []byte) error { return enum.Unmarshal(viewModeNames, b, m) }

// Lets the decision tree be unit-tested without the stub-Ebiten harness.
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
	// Any version but the current one is a hard error: the user named this file.
	if err := c.Validate("ui-state"); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Menu state is not in Config: scenes own it.
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
		case ViewRows:
			a.SetViewMode(false)
		case ViewAudio:
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
