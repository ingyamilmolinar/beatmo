package ui

// Off by default, leaving the bench output unchanged.
type benchStormScenario int

const (
	benchStormOff benchStormScenario = iota
	benchStormToggle
	benchStormCtrlSweep
	benchStormCombined
)

func parseBenchStormScenario(s string) benchStormScenario {
	switch s {
	case "toggle":
		return benchStormToggle
	case "ctrl", "ctrl-sweep":
		return benchStormCtrlSweep
	case "combined":
		return benchStormCombined
	default:
		return benchStormOff
	}
}

func ParseBenchStormScenario(s string) benchStormScenario { return parseBenchStormScenario(s) }

// The canonical (non-alias) spelling.
func benchStormName(sc benchStormScenario) string {
	switch sc {
	case benchStormToggle:
		return "toggle"
	case benchStormCtrlSweep:
		return "ctrl-sweep"
	case benchStormCombined:
		return "combined"
	default:
		return "off"
	}
}

type benchStormMutation struct {
	Param string
	Value float64
}

// Deterministic (no time or random) so the unit test and the live bench agree.
func benchStormMutationsForTick(sc benchStormScenario, tick int) []benchStormMutation {
	switch sc {
	case benchStormToggle:
		v := 1
		if tick&1 == 1 {
			v = 0
		}
		return []benchStormMutation{{Param: "filter_enabled", Value: float64(v)}}
	case benchStormCtrlSweep:
		return []benchStormMutation{{Param: "filter_cutoff", Value: float64(tick%33) / 32.0}}
	case benchStormCombined:
		out := []benchStormMutation{{Param: "filter_cutoff", Value: float64(tick%25) / 24.0}}
		if tick%8 == 0 {
			v := 1
			if (tick/8)&1 == 1 {
				v = 0
			}
			out = append(out, benchStormMutation{Param: "filter_enabled", Value: float64(v)})
		}
		return out
	default:
		return nil
	}
}
