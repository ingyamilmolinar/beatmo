//go:build !js

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePNG(t *testing.T, img image.Image) string {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAFlatCaptureIsBlank(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := range img.Pix {
		img.Pix[i] = 0x20
	}
	err := checkCapture(writePNG(t, img))
	if err == nil || !strings.Contains(err.Error(), "blank") {
		t.Fatalf("a one-colour capture passed: %v", err)
	}
}

func TestADrawnCapturePasses(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 0x80, 0xff})
		}
	}
	if err := checkCapture(writePNG(t, img)); err != nil {
		t.Fatalf("a drawn capture failed: %v", err)
	}
}

func TestAMissingCaptureFails(t *testing.T) {
	if err := checkCapture(filepath.Join(t.TempDir(), "none.png")); err == nil {
		t.Fatal("a missing capture passed")
	}
}
