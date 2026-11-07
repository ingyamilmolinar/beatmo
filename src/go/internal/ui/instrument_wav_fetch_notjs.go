//go:build !js

package ui

import (
	"io"
	"net/http"
	"os"
	"time"
)

func readWavFileBytes(path string) ([]byte, bool) {
	if path == "" {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return data, true
}

// Bytes are trusted only after ResolveWavRef checks them against the pin.
func fetchWavURLBytes(url string) ([]byte, bool) {
	if url == "" {
		return nil, false
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20)) // 256 MB cap
	if err != nil {
		return nil, false
	}
	return data, true
}
