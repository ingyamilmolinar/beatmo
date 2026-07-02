//go:build test && !js

package ui

func selectJSON() ([]byte, string, error) { return nil, "", nil }

func selectJSONAsync(cb func([]byte, string, error)) { cb(nil, "", nil) }

func saveJSONDefault(name string, data []byte) error { return nil }
