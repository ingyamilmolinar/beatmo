//go:build test && !js

package ui

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
)

// The landing page's committed media (src/js/landing/<cut>.{mp4,webp,vtt},
// produced by `make showcase-video`) must be what landing.js and index.html
// assume, and must still match the tour script that recorded it:
//   - the cue file describes the CURRENT script (captions, chapters, order,
//     the theme sequence, the template's tempo) — a caption edited in
//     tour_steps.go without re-recording fails here, not on beatmo.io;
//   - the MP4 streams: moov before mdat (faststart — the browser can start
//     playing before the whole file arrives), H.264 + AAC (what
//     landing.js's canPlayType guard asks for), 25 fps, and as long as its
//     cue file;
//   - the poster is the video's own size, and both have the aspect ratio
//     index.html's CSS reserves for them (16:9 desktop, 390:844 phone), so
//     nothing jumps when the video replaces the poster.

var landingCuts = map[string]struct{ aspect float64 }{
	"desktop": {16.0 / 9},
	"phone":   {390.0 / 844},
}

func landingMediaDir() string { return filepath.Join("..", "..", "..", "js", "landing") }

func TestLandingMediaMatchesTheTourAndThePage(t *testing.T) {
	bpm := tourTemplateBPM(t)
	for cut, want := range landingCuts {
		t.Run(cut, func(t *testing.T) {
			dir := landingMediaDir()
			vttBytes, err := os.ReadFile(filepath.Join(dir, cut+".vtt"))
			if err != nil {
				t.Fatal(err)
			}
			cues, err := parseTourVTT(string(vttBytes))
			if err != nil {
				t.Fatalf("%s.vtt: %v", cut, err)
			}
			end := cues[0].end
			checkTourCues(t, cues, tourScripts[cut], 0, end, bpm, 60/bpm)
			if t.Failed() {
				t.Log("the committed recording no longer matches tour_steps.go: re-record with `make showcase-video`")
			}

			mp4, err := os.ReadFile(filepath.Join(dir, cut+".mp4"))
			if err != nil {
				t.Fatal(err)
			}
			info, err := parseMP4(mp4)
			if err != nil {
				t.Fatalf("%s.mp4: %v", cut, err)
			}
			t.Logf("%s.mp4: %dx%d, %.2f s, %.3f fps, faststart=%v avc1=%v mp4a=%v; cues end %.2f s", cut, info.w, info.h, info.duration, info.fps, info.faststart, info.avc1, info.mp4a, end)
			if !info.faststart {
				t.Errorf("%s.mp4: moov comes after mdat — not faststart, the browser must download the whole file before it plays", cut)
			}
			if !info.avc1 || !info.mp4a {
				t.Errorf("%s.mp4: H.264 video %v, AAC audio %v — landing.js only plays avc1 + mp4a", cut, info.avc1, info.mp4a)
			}
			if math.Abs(info.fps-25) > 0.01 {
				t.Errorf("%s.mp4: %.3f fps, the recorder writes 25", cut, info.fps)
			}
			if math.Abs(info.duration-end) > 0.6 {
				t.Errorf("%s.mp4 lasts %.2f s, its cue file %.2f s", cut, info.duration, end)
			}
			if a := float64(info.w) / float64(info.h); math.Abs(a/want.aspect-1) > 0.01 {
				t.Errorf("%s.mp4 is %dx%d (aspect %.4f); index.html reserves %.4f", cut, info.w, info.h, a, want.aspect)
			}

			webp, err := os.ReadFile(filepath.Join(dir, cut+".webp"))
			if err != nil {
				t.Fatal(err)
			}
			pw, ph, err := webpSize(webp)
			if err != nil {
				t.Fatalf("%s.webp: %v", cut, err)
			}
			if pw != info.w || ph != info.h {
				t.Errorf("%s.webp is %dx%d, the video %dx%d", cut, pw, ph, info.w, info.h)
			}
		})
	}
}

func tourTemplateBPM(t *testing.T) float64 {
	t.Helper()
	for _, tp := range assets.Templates() {
		if tp.Genre == tourTemplateStem {
			var doc struct {
				BPM float64 `json:"bpm"`
			}
			if err := json.Unmarshal(tp.Bytes, &doc); err != nil {
				t.Fatal(err)
			}
			return doc.BPM
		}
	}
	t.Fatalf("no template %q", tourTemplateStem)
	return 0
}

type mp4Info struct {
	faststart  bool
	avc1, mp4a bool
	duration   float64
	fps        float64
	w, h       int
}

// parseMP4 walks the box tree far enough to answer mp4Info.
func parseMP4(b []byte) (mp4Info, error) {
	var info mp4Info
	type box struct {
		typ        string
		start, end int // payload bounds
	}
	boxes := func(lo, hi int) ([]box, error) {
		var out []box
		for off := lo; off+8 <= hi; {
			size := int(binary.BigEndian.Uint32(b[off:]))
			typ := string(b[off+4 : off+8])
			hdr := 8
			if size == 1 {
				size, hdr = int(binary.BigEndian.Uint64(b[off+8:])), 16
			} else if size == 0 {
				size = hi - off
			}
			if size < hdr || off+size > hi {
				return nil, fmt.Errorf("box %q at %d: bad size %d", typ, off, size)
			}
			out = append(out, box{typ, off + hdr, off + size})
			off += size
		}
		return out, nil
	}
	top, err := boxes(0, len(b))
	if err != nil {
		return info, err
	}
	if len(top) == 0 || top[0].typ != "ftyp" {
		return info, fmt.Errorf("does not start with ftyp")
	}
	moovAt, mdatAt := -1, -1
	var moov box
	for i, x := range top {
		switch x.typ {
		case "moov":
			moovAt, moov = i, x
		case "mdat":
			if mdatAt < 0 {
				mdatAt = i
			}
		}
	}
	if moovAt < 0 || mdatAt < 0 {
		return info, fmt.Errorf("no moov or mdat")
	}
	info.faststart = moovAt < mdatAt
	find := func(parent box, typ string) (box, bool) {
		kids, _ := boxes(parent.start, parent.end)
		for _, k := range kids {
			if k.typ == typ {
				return k, true
			}
		}
		return box{}, false
	}
	if mvhd, ok := find(moov, "mvhd"); ok {
		p := b[mvhd.start:]
		if p[0] == 1 {
			info.duration = float64(binary.BigEndian.Uint64(p[24:])) / float64(binary.BigEndian.Uint32(p[20:]))
		} else {
			info.duration = float64(binary.BigEndian.Uint32(p[16:])) / float64(binary.BigEndian.Uint32(p[12:]))
		}
	}
	kids, _ := boxes(moov.start, moov.end)
	for _, trak := range kids {
		if trak.typ != "trak" {
			continue
		}
		mdia, _ := find(trak, "mdia")
		hdlr, _ := find(mdia, "hdlr")
		minf, _ := find(mdia, "minf")
		stbl, _ := find(minf, "stbl")
		stsd, _ := find(stbl, "stsd")
		entries := b[stsd.start:stsd.end]
		switch string(b[hdlr.start+8 : hdlr.start+12]) {
		case "vide":
			info.avc1 = bytes.Contains(entries, []byte("avc1"))
			tkhd, _ := find(trak, "tkhd")
			p := b[tkhd.start:tkhd.end]
			info.w, info.h = int(binary.BigEndian.Uint32(p[len(p)-8:])>>16), int(binary.BigEndian.Uint32(p[len(p)-4:])>>16)
			mdhd, _ := find(mdia, "mdhd")
			q := b[mdhd.start:]
			scale := binary.BigEndian.Uint32(q[12:])
			if q[0] == 1 {
				scale = binary.BigEndian.Uint32(q[20:])
			}
			// stts: the sample deltas; a constant-rate video has one entry.
			stts, _ := find(stbl, "stts")
			s := b[stts.start:]
			n := int(binary.BigEndian.Uint32(s[4:]))
			var samples, ticks uint64
			for i := 0; i < n; i++ {
				c, d := uint64(binary.BigEndian.Uint32(s[8+8*i:])), uint64(binary.BigEndian.Uint32(s[12+8*i:]))
				samples += c
				ticks += c * d
			}
			if ticks > 0 {
				info.fps = float64(samples) * float64(scale) / float64(ticks)
			}
		case "soun":
			info.mp4a = bytes.Contains(entries, []byte("mp4a"))
		}
	}
	return info, nil
}

// webpSize reads a WebP's canvas size (lossy VP8, lossless VP8L or VP8X).
func webpSize(b []byte) (int, int, error) {
	if len(b) < 30 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WEBP" {
		return 0, 0, fmt.Errorf("not a WebP file")
	}
	switch string(b[12:16]) {
	case "VP8 ":
		if !bytes.Equal(b[23:26], []byte{0x9d, 0x01, 0x2a}) {
			return 0, 0, fmt.Errorf("VP8: no key frame start code")
		}
		return int(binary.LittleEndian.Uint16(b[26:]) & 0x3fff), int(binary.LittleEndian.Uint16(b[28:]) & 0x3fff), nil
	case "VP8L":
		v := binary.LittleEndian.Uint32(b[21:])
		return int(v&0x3fff) + 1, int((v>>14)&0x3fff) + 1, nil
	case "VP8X":
		w := int(b[24]) | int(b[25])<<8 | int(b[26])<<16
		h := int(b[27]) | int(b[28])<<8 | int(b[29])<<16
		return w + 1, h + 1, nil
	}
	return 0, 0, fmt.Errorf("unknown WebP chunk %q", b[12:16])
}
