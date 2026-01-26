package ui

import "github.com/hajimehoshi/ebiten/v2"

type toolbarComponent struct{}

func (toolbarComponent) ID() string { return "toolbar" }

func (toolbarComponent) Init(ctx ComponentContext) {}

func (toolbarComponent) OnGraphChange(ctx ComponentContext, change GraphChange) {}

func (toolbarComponent) Render(ctx ComponentContext, phase RenderPhase, dst *ebiten.Image) {
	if phase != RenderPhaseToolbar || ctx.Drum == nil || ctx.Drum.simpleDraw {
		return
	}
	ctx.Drum.renderToolbarControls(dst)
}

type notificationsComponent struct{}

func (notificationsComponent) ID() string { return "notifications" }

func (notificationsComponent) Init(ctx ComponentContext) {}

func (notificationsComponent) OnGraphChange(ctx ComponentContext, change GraphChange) {}

func (notificationsComponent) Render(ctx ComponentContext, phase RenderPhase, dst *ebiten.Image) {
	if phase != RenderPhaseToolbar || ctx.Drum == nil || ctx.Drum.simpleDraw {
		return
	}
	ctx.Drum.drawNotifications(dst)
}

type rowControlsComponent struct{}

func (rowControlsComponent) ID() string { return "row-controls" }

func (rowControlsComponent) Init(ctx ComponentContext) {}

func (rowControlsComponent) OnGraphChange(ctx ComponentContext, change GraphChange) {}

func (rowControlsComponent) Render(ctx ComponentContext, phase RenderPhase, dst *ebiten.Image) {
	if phase != RenderPhaseRowControls || ctx.Drum == nil || ctx.Drum.simpleDraw {
		return
	}
	ctx.Drum.renderRowControlOverlay(dst)
}
