//go:build test

package main

// renderIsStub reports whether synthmatch's render path is the deterministic
// test stub rather than the real C DSP. See drift_test.go for why the drift
// guard has to know.
const renderIsStub = true
