package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// logicKindLabel is a logic rule's name in the node sidebar's rule list.
func logicKindLabel(k model.LogicKind) string {
	switch k {
	case model.LogicEveryN:
		return i18n.T(i18n.KeyLogicEveryN)
	case model.LogicSkipEveryN:
		return i18n.T(i18n.KeyLogicSkipN)
	case model.LogicProbability:
		return i18n.T(i18n.KeyLogicProbability)
	case model.LogicIfPrevSkipped:
		return i18n.T(i18n.KeyLogicIfPrevSkipped)
	case model.LogicIfPrevTriggered:
		return i18n.T(i18n.KeyLogicIfPrevTriggered)
	}
	return i18n.T(i18n.KeyLogicNone)
}

// logicKindShortLabel is a logic rule's name on the sidebar's selector button.
func logicKindShortLabel(k model.LogicKind) string {
	switch k {
	case model.LogicEveryN:
		return i18n.T(i18n.KeyLogicShortEveryN)
	case model.LogicSkipEveryN:
		return i18n.T(i18n.KeyLogicShortSkipN)
	case model.LogicProbability:
		return i18n.T(i18n.KeyLogicShortProb)
	case model.LogicIfPrevSkipped:
		return i18n.T(i18n.KeyLogicShortPrevSkip)
	case model.LogicIfPrevTriggered:
		return i18n.T(i18n.KeyLogicShortPrevTrig)
	}
	return i18n.T(i18n.KeyLogicNone)
}

// grooveKindLabel is a groove's name in the node sidebar.
func grooveKindLabel(k model.GrooveKind) string {
	switch k {
	case model.GrooveDelay:
		return i18n.T(i18n.KeyGrooveDelay)
	case model.GrooveRush:
		return i18n.T(i18n.KeyGrooveRush)
	}
	return i18n.T(i18n.KeyLogicNone)
}
