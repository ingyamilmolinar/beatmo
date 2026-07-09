package ui

import "testing"

func TestParityTimingActivationDefaults(t *testing.T) {
	// Desktop default: fatal on (per RuntimeProf().Timing.FatalDefault). Under
	// `go test`, the gate is forced dormant regardless of runtime profile (see
	// init() in game_parity_timing.go) so fast-forwarded/synthetic-time tests
	// elsewhere never false-trigger; this test only exercises SetParityTimingFatal
	// plumbing directly and must restore the prior (dormant) value on exit so it
	// doesn't leak an armed gate into later tests.
	restorePersist := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restorePersist()
	prevFatal := parityTimingFatalEnabled.Load()
	t.Cleanup(func() { parityTimingFatalEnabled.Store(prevFatal) })

	SetParityTimingFatal(desktopRuntimeProfile().Timing.FatalDefault)
	if !parityTimingFatalEnabled.Load() {
		t.Error("expected desktop timing fatal default ON")
	}
	SetParityTimingFatal(false)
	if parityTimingFatalEnabled.Load() {
		t.Error("SetParityTimingFatal(false) did not stick")
	}
}

func TestResolveParityTimingActivation(t *testing.T) {
	cases := []struct {
		name           string
		seedWatch      parityWatchMode
		seedFatal      bool
		isBrowser      bool
		timingEnv      string
		timingFatalEnv string
		wantWatch      parityWatchMode
		wantFatal      bool
	}{
		{
			name:      "desktop seed log/false no env",
			seedWatch: parityWatchLog,
			seedFatal: false,
			isBrowser: false,
			wantWatch: parityWatchLog,
			wantFatal: false,
		},
		{
			name:      "desktop seed panic/true no env",
			seedWatch: parityWatchPanic,
			seedFatal: true,
			isBrowser: false,
			wantWatch: parityWatchPanic,
			wantFatal: true,
		},
		{
			name:      "desktop PARITY_TIMING=off",
			seedWatch: parityWatchPanic,
			seedFatal: true,
			isBrowser: false,
			timingEnv: "off",
			wantWatch: parityWatchOff,
			wantFatal: false,
		},
		{
			name:      "desktop PARITY_TIMING=log",
			seedWatch: parityWatchOff,
			seedFatal: false,
			isBrowser: false,
			timingEnv: "log",
			wantWatch: parityWatchLog,
			wantFatal: false,
		},
		{
			name:      "desktop PARITY_TIMING=panic",
			seedWatch: parityWatchOff,
			seedFatal: false,
			isBrowser: false,
			timingEnv: "panic",
			wantWatch: parityWatchPanic,
			wantFatal: true,
		},
		{
			// Regression case: a leaked PARITY_TIMING=panic in a browser/CI
			// environment must NOT arm the fatal panic path.
			name:      "browser PARITY_TIMING=panic NOT armed stays log/false",
			seedWatch: parityWatchOff,
			seedFatal: false,
			isBrowser: true,
			timingEnv: "panic",
			wantWatch: parityWatchLog,
			wantFatal: false,
		},
		{
			name:           "browser PARITY_TIMING_FATAL=1 arms panic",
			seedWatch:      parityWatchOff,
			seedFatal:      false,
			isBrowser:      true,
			timingFatalEnv: "1",
			wantWatch:      parityWatchPanic,
			wantFatal:      true,
		},
		{
			name:           "browser PARITY_TIMING_FATAL=true arms panic",
			seedWatch:      parityWatchOff,
			seedFatal:      false,
			isBrowser:      true,
			timingFatalEnv: "true",
			wantWatch:      parityWatchPanic,
			wantFatal:      true,
		},
		{
			name:           "browser PARITY_TIMING_FATAL=panic arms panic",
			seedWatch:      parityWatchOff,
			seedFatal:      false,
			isBrowser:      true,
			timingFatalEnv: "panic",
			wantWatch:      parityWatchPanic,
			wantFatal:      true,
		},
		{
			name:      "browser no env stays log/false",
			seedWatch: parityWatchPanic,
			seedFatal: true,
			isBrowser: true,
			wantWatch: parityWatchLog,
			wantFatal: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotWatch, gotFatal := resolveParityTimingActivation(tc.seedWatch, tc.seedFatal, tc.isBrowser, tc.timingEnv, tc.timingFatalEnv)
			if gotWatch != tc.wantWatch {
				t.Errorf("watch = %v, want %v", gotWatch, tc.wantWatch)
			}
			if gotFatal != tc.wantFatal {
				t.Errorf("fatal = %v, want %v", gotFatal, tc.wantFatal)
			}
		})
	}
}
