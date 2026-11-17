package clone

import (
	"errors"
	"fmt"
)

// Refusal codes: the analysis declines rather than guess (spec §7).
const (
	RefuseNoOnset    = "no_onset"
	RefuseShortDecay = "short_decay"
	RefuseClipped    = "clipped"
	RefuseNotModal   = "not_modal"
)

// Refusal is a typed "this input cannot be cloned" error.
type Refusal struct {
	Code   string
	Reason string
}

func (r *Refusal) Error() string { return fmt.Sprintf("clone refused (%s): %s", r.Code, r.Reason) }

func refuse(code, format string, args ...any) error {
	return &Refusal{Code: code, Reason: fmt.Sprintf(format, args...)}
}

// IsRefusal reports whether err is a Refusal with the given code.
func IsRefusal(err error, code string) bool {
	var r *Refusal
	return errors.As(err, &r) && r.Code == code
}
