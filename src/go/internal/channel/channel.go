// Package channel names a mixer channel: the master bus or one instrument's. Volume, EQ, insert effects and the analysers
// key on it. Opaque: from Master / Of, or Parse for data; the master is "main", and the zero ID is no channel, never the
// master.
package channel

import (
	"fmt"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// ID identifies a mixer channel. The zero value is "no channel".
type ID struct{ name string }

// Master is the master bus every instrument channel feeds.
var Master = ID{instrument.ReservedMasterName}

// Of returns the channel an instrument plays through; the zero ID for no
// instrument.
func Of(inst instrument.ID) ID {
	if inst == "" || string(inst) == instrument.ReservedMasterName {
		return ID{}
	}
	return ID{string(inst)}
}

// Parse reads a channel name from data: "main" is the master bus, anything
// else must be an instrument id. The empty name is rejected — it names no bus.
func Parse(s string) (ID, error) {
	if s == instrument.ReservedMasterName {
		return Master, nil
	}
	inst, err := instrument.ParseID(s)
	if err != nil {
		return ID{}, fmt.Errorf("channel %q: %w", s, err)
	}
	return Of(inst), nil
}

// String is the channel's wire spelling ("main", "kick"; "" for no channel).
func (c ID) String() string { return c.name }

// IsZero reports whether c names no channel.
func (c ID) IsZero() bool { return c.name == "" }

// IsMaster reports whether c is the master bus.
func (c ID) IsMaster() bool { return c == Master }

// Instrument returns the instrument whose channel c is, and false for the
// master bus and the zero ID.
func (c ID) Instrument() (instrument.ID, bool) {
	if c.IsZero() || c.IsMaster() {
		return "", false
	}
	return instrument.ID(c.name), true
}

// MarshalText writes the wire spelling, so JSON fields and map keys encode
// exactly as the plain string did.
func (c ID) MarshalText() ([]byte, error) { return []byte(c.name), nil }

// UnmarshalText is Parse; an empty value is the zero ID (an absent channel).
func (c *ID) UnmarshalText(b []byte) error {
	if len(b) == 0 {
		*c = ID{}
		return nil
	}
	id, err := Parse(string(b))
	if err != nil {
		return err
	}
	*c = id
	return nil
}

// Compare orders channels by wire spelling (slices.SortFunc).
func Compare(a, b ID) int { return strings.Compare(a.name, b.name) }

// Strings returns the channels' wire spellings.
func Strings(ids []ID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.name
	}
	return out
}
