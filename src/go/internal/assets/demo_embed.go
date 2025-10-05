package assets

import _ "embed"

// DefaultDemoJSON contains the default demo configuration bundled with the app.
//
//go:embed default_demo.json
var DefaultDemoJSON []byte
