package assets

import _ "embed"

// DefaultDemoJSON contains the default demo configuration bundled with the app.
//
//go:embed default_demo.json
var DefaultDemoJSON []byte

// StartupDemoJSON contains the rock startup demo used by default when no
// external config is provided. Tests continue to rely on DefaultDemoJSON.
//
//go:embed startup_demo.json
var StartupDemoJSON []byte
