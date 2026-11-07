package ui

import "testing"

func TestParityTimingActivationDefaults(t *testing.T) {
	// The gate is dormant under go test whatever the profile; restore the prior value so no armed gate leaks into
	// later tests.
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
			// A leaked PARITY_TIMING=panic in a browser or CI environment must not arm the panic.
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
