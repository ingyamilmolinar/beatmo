package assets

import _ "embed"

// The startup demo shown by default when no external config is given.
//
//go:embed startup_demo.json
var StartupDemoJSON []byte

// A stable test fixture (nodes on multiples of 16 for subdivision tests); never shipped to users.
//
//go:embed test_fixture_demo.json
var TestFixtureDemoJSON []byte
