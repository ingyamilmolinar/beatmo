//go:build !js && !test

package ui

import (
	"os"
	"os/exec"
	"strings"
)

// selectJSON returns the picked file's bytes and its path (for the "Loaded
// <name>" notification). path is "" when nothing was picked.
func selectJSON() (data []byte, path string, err error) {
	if _, e := exec.LookPath("zenity"); e == nil {
		cmd := exec.Command("zenity", "--file-selection", "--file-filter=*.json")
		out, e := cmd.Output()
		if e == nil {
			p := strings.TrimSpace(string(out))
			if p != "" {
				b, rerr := os.ReadFile(p)
				return b, p, rerr
			}
		}
	}
	return nil, "", nil
}
