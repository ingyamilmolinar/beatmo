//go:build test

package audio

import "testing"

// ensureSendFXForTest is a no-op under the test build tag. The stub
// ConfigureSend* functions in stub.go always succeed and store their state
// in stubSendDelayParams/stubSendReverbParams.
func ensureSendFXForTest(_ *testing.T) {}
