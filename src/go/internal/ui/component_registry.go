package ui

import (
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

// RenderPhase identifiers determine when a component should draw within the
// DrumView pipeline. Phases are invoked in order by DrumView.Draw.
type RenderPhase int

const (
	RenderPhaseToolbar RenderPhase = iota
	RenderPhaseRowControls
)

// GraphChange summarizes high-level graph updates that UI components may care
// about (e.g., to refresh cached annotations).
type GraphChange struct {
	PathsChanged bool
}

// ComponentContext provides shared access to Game/DrumView state during
// component hooks.
type ComponentContext struct {
	Game *Game
	Drum *DrumView
}

// UIComponent defines lifecycle hooks for pluggable UI overlays/controls.
type UIComponent interface {
	ID() string
	Init(ComponentContext)
	OnGraphChange(ComponentContext, GraphChange)
	Render(ComponentContext, RenderPhase, *ebiten.Image)
}

// ComponentRegistry orchestrates registered UI components.
type ComponentRegistry struct {
	mu          sync.RWMutex
	comps       []UIComponent
	ctx         ComponentContext
	initialized bool
}

// NewComponentRegistry constructs an empty registry.
func NewComponentRegistry() *ComponentRegistry {
	return &ComponentRegistry{}
}

// Register appends a component. Callers should register components before Init.
func (r *ComponentRegistry) Register(c UIComponent) {
	if r == nil || c == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.comps = append(r.comps, c)
	if r.initialized {
		c.Init(r.ctx)
	}
}

// Init invokes each component's Init hook once.
func (r *ComponentRegistry) Init(ctx ComponentContext) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.initialized {
		return
	}
	r.ctx = ctx
	for _, c := range r.comps {
		c.Init(r.ctx)
	}
	r.initialized = true
}

// Render executes the Render hook for all components for the given phase.
func (r *ComponentRegistry) Render(phase RenderPhase, dst *ebiten.Image) {
	if r == nil {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.initialized {
		return
	}
	for _, c := range r.comps {
		c.Render(r.ctx, phase, dst)
	}
}

// NotifyGraphChange broadcasts a graph change event to all components.
func (r *ComponentRegistry) NotifyGraphChange(change GraphChange) {
	if r == nil {
		return
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.initialized {
		return
	}
	for _, c := range r.comps {
		c.OnGraphChange(r.ctx, change)
	}
}
