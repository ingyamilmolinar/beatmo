#!/usr/bin/env python3
"""Convert a (possibly 24-bit stereo) AIFF to 16-bit mono WAV using only the
Python standard library (no ffmpeg/sox). Caps length to keep files small."""
import sys, wave, struct, warnings
warnings.filterwarnings("ignore")
import aifc

def convert(src, dst, max_sec=2.0):
    a = aifc.open(src, "rb")
    nch, width, rate, nframes = a.getnchannels(), a.getsampwidth(), a.getframerate(), a.getnframes()
    nframes = min(nframes, int(rate * max_sec))
    raw = a.readframes(nframes)
    a.close()
    bpf = width * nch
    full = 1 << (8 * width - 1)
    def samp(off):
        # AIFF samples are big-endian two's-complement at every width (including
        # 8-bit), so a single signed decode is correct for all widths.
        v = int.from_bytes(raw[off:off+width], "big", signed=True)
        return v / full
    out = bytearray()
    for i in range(nframes):
        base = i * bpf
        s = sum(samp(base + c*width) for c in range(nch)) / nch
        iv = max(-32768, min(32767, int(s * 32767)))
        out += struct.pack("<h", iv)
    w = wave.open(dst, "wb")
    w.setnchannels(1); w.setsampwidth(2); w.setframerate(rate)
    w.writeframes(bytes(out)); w.close()

if __name__ == "__main__":
    convert(sys.argv[1], sys.argv[2])
    print("wrote", sys.argv[2])
