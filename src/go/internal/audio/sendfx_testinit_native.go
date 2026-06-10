//go:build !test && !js

package audio

import "testing"

// ensureSendFXForTest lazy-initializes sendFX on the native build path so
// ConfigureSendDelay / ConfigureSendReverb actually mutate state (they are
// early-return no-ops when sendFX is nil). The test-tag stub has its own
// no-op variant that does nothing because the stub-side ConfigureSend*
// always succeeds. Idempotent — subsequent calls skip the malloc path.
func ensureSendFXForTest(_ *testing.T) {
	if sendFX != nil && sendFX.initialized {
		return
	}
	initSendEffects(44100)
}
