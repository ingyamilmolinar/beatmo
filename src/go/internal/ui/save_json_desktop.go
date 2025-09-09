//go:build !js

package ui

import (
	"os"
	"os/exec"
	"strings"
)

func saveJSONDefault(name string, data []byte) error {
	// Try a save dialog via zenity when available
	if _, err := exec.LookPath("zenity"); err == nil {
		cmd := exec.Command("zenity", "--file-selection", "--save", "--confirm-overwrite", "--filename="+name)
		out, err := cmd.Output()
		if err == nil {
			path := strings.TrimSpace(string(out))
			if path != "" {
				return os.WriteFile(path, data, 0644)
			}
		}
	}
	return os.WriteFile(name, data, 0644)
}
