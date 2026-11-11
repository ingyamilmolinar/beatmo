package ui

import (
	"os"
	"strconv"
	"strings"
)

const (
	parityScanEveryDefault  = 4
	parityScanStrideDefault = 1
)

var parityScanEveryFrames = envIntDefault("PARITY_SCAN_EVERY", parityScanEveryDefault)
var parityScanStride = envIntDefault("PARITY_SCAN_STRIDE", parityScanStrideDefault)

func envIntDefault(name string, def int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val < 1 {
		return def
	}
	return val
}
