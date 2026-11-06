package hooks

import "github.com/ingyamilmolinar/beatmo/internal/enum"

// Closed enums carried by payloads. Each spells itself on the wire exactly as
// the string it replaced; an unknown spelling is a decode error.

// SeekOrigin is the control a transport seek came from.
type SeekOrigin int

const (
	SeekRibbon SeekOrigin = iota
	SeekTransport
	SeekCounter
	SeekKeyboard
	SeekJS
	SeekUndo
)

var seekOriginNames = enum.Names[SeekOrigin]{Kind: "seek origin", Names: []string{
	"ribbon", "transport", "counter", "keyboard", "js", "undo",
}}

func (o SeekOrigin) String() string                { return enum.String(seekOriginNames, o) }
func (o SeekOrigin) MarshalText() ([]byte, error)  { return enum.Marshal(seekOriginNames, o) }
func (o *SeekOrigin) UnmarshalText(b []byte) error { return enum.Unmarshal(seekOriginNames, b, o) }

// EQFilter names a channel's high- or low-pass filter (FilterNone = neither).
type EQFilter int

const (
	FilterNone EQFilter = iota
	FilterHPF
	FilterLPF
)

var eqFilterNames = enum.Names[EQFilter]{Kind: "EQ filter", Names: []string{"", "hpf", "lpf"}}

func (f EQFilter) String() string                { return enum.String(eqFilterNames, f) }
func (f EQFilter) MarshalText() ([]byte, error)  { return enum.Marshal(eqFilterNames, f) }
func (f *EQFilter) UnmarshalText(b []byte) error { return enum.Unmarshal(eqFilterNames, b, f) }

// ThemeScope is whose theme an edit changes: the user's profile or the open
// circuit's.
type ThemeScope int

const (
	ThemeScopeUser ThemeScope = iota
	ThemeScopeCircuit
)

var themeScopeNames = enum.Names[ThemeScope]{Kind: "theme scope", Names: []string{"user", "circuit"}}

func (s ThemeScope) String() string                { return enum.String(themeScopeNames, s) }
func (s ThemeScope) MarshalText() ([]byte, error)  { return enum.Marshal(themeScopeNames, s) }
func (s *ThemeScope) UnmarshalText(b []byte) error { return enum.Unmarshal(themeScopeNames, b, s) }
