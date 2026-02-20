//go:build test && !js

package ui

func selectJSON() ([]byte, error) { return nil, nil }

func selectJSONAsync(cb func([]byte, error)) { cb(nil, nil) }

func saveJSONDefault(name string, data []byte) error { return nil }
