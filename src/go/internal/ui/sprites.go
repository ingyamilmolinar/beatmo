package ui

import (
	"embed"
	"image"
	"github.com/hajimehoshi/ebiten/v2"
	_ "image/png"
)

//go:embed assets/*.png
var assetFS embed.FS

var (
        NodeAnim   *ebiten.Image   // spritesheet (8 frames)
        SignalDot  *ebiten.Image
        NodeFrames []*ebiten.Image // convenience slice [0..7]
       NodeSpriteSize int
)

func init() {
	load := func(name string) *ebiten.Image {
		f, _ := assetFS.Open(name)
		defer f.Close()
		img, _, _ := image.Decode(f)
		return ebiten.NewImageFromImage(img)
	}
       NodeAnim  = load("assets/node_anim.png")
       SignalDot = load("assets/signal_dot.png")

	// split sheet
       w, _ := NodeAnim.Size()
       frameW := 0
       if w > 0 {
               frameW = w / 8
               NodeSpriteSize = frameW
       } else {
               NodeSpriteSize = 32
               frameW = NodeSpriteSize
       }
	for i := 0; i < 8; i++ {
		sub := NodeAnim.SubImage(image.Rect(i*frameW, 0, (i+1)*frameW, frameW))
		NodeFrames = append(NodeFrames, sub.(*ebiten.Image))
	}
}

