package instrument

import "github.com/ingyamilmolinar/beatmo/internal/enum"

// Applying a kit rebinds every row whose role it names. A closed enum: an unknown spelling in a file is a decode error.
type Role int

// The kit roles. RoleNone is "no role" (spelled "", omitted in files).
const (
	RoleNone Role = iota
	RoleKick
	RoleSnare
	RoleHat
	RoleTom
	RoleClap
	RoleCowbell
	RoleRide
	RoleCrash
	RoleShaker
	RoleBass
)

var roleNames = enum.Names[Role]{Kind: "kit role", Names: []string{
	"", "kick", "snare", "hat", "tom", "clap", "cowbell", "ride", "crash", "shaker", "bass",
}}

func (r Role) String() string                { return enum.String(roleNames, r) }
func (r Role) MarshalText() ([]byte, error)  { return enum.Marshal(roleNames, r) }
func (r *Role) UnmarshalText(b []byte) error { return enum.Unmarshal(roleNames, b, r) }

// ParseRole reads a role spelled in data ("" is RoleNone).
func ParseRole(s string) (Role, error) { return enum.Parse(roleNames, s) }

// Roles lists every kit role (RoleNone excluded).
func Roles() []Role { return enum.All(roleNames)[1:] }
