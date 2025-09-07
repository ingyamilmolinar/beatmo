//go:build !js

package ui

import (
    "os"
    "os/exec"
    "strings"
)

func selectJSON() ([]byte, error) {
    if _, err := exec.LookPath("zenity"); err == nil {
        cmd := exec.Command("zenity", "--file-selection", "--file-filter=*.json")
        out, err := cmd.Output()
        if err == nil {
            path := strings.TrimSpace(string(out))
            if path != "" {
                return os.ReadFile(path)
            }
        }
    }
    return nil, nil
}

