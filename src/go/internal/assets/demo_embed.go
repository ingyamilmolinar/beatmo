package assets

import _ "embed"

// StartupDemoJSON contains the rock startup demo used by default when no
// external config is provided. This is the production demo shown to users.
//
//go:embed startup_demo.json
var StartupDemoJSON []byte

// TestFixtureDemoJSON contains a simpler demo configuration used as a stable
// test fixture. It has nodes aligned on multiples of 16 for subdivision tests
// and a predictable structure for regression testing. Not used in production.
//
//go:embed test_fixture_demo.json
var TestFixtureDemoJSON []byte
