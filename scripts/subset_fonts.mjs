#!/usr/bin/env node
// Regenerate the subset Inter faces that ship inside main.wasm.
//
// Stock Inter carries 2,851 reachable glyphs -- Cyrillic, Greek Extended,
// Vietnamese, IPA, and ~740 private-use alternates that Beatmo never renders:
// only EN and ES locales ship, and a future CJK locale swaps the whole face
// through i18n's FontProvider rather than widening these files. Subsetting to
// the blocks the UI can actually draw halves both faces (831 KB -> 417 KB raw,
// 406 KB -> 201 KB gzip) -- the single largest download saving available, and
// the gzip budget in internal/deploy/asset_size_test.go is the tight one.
//
//   npm install subset-font        (or: make gen-fonts, which does it for you)
//   node scripts/subset_fonts.mjs
//
// internal/assets/fonts_coverage_test.go pins the resulting coverage: it fails
// if a regenerated subset drops a codepoint the UI is known to render, so a
// narrower KEEP_RANGES cannot silently ship tofu.

import subsetFont from 'subset-font';
import fs from 'fs';
import path from 'path';
import url from 'url';
import zlib from 'zlib';

const HERE = path.dirname(url.fileURLToPath(import.meta.url));
const FONT_DIR = path.join(HERE, '..', 'src', 'go', 'internal', 'assets', 'fonts');

// Blocks the UI can render. Deliberately generous: every block cheap enough
// that keeping it beats debugging a tofu report from a locale we do ship.
const KEEP_RANGES = [
  [0x0020, 0x007E, 'ASCII printable'],
  [0x00A0, 0x00FF, 'Latin-1 Supplement (ES accents, °, ±, ×, ÷, µ)'],
  [0x0100, 0x017F, 'Latin Extended-A'],
  [0x0180, 0x024F, 'Latin Extended-B'],
  [0x02B0, 0x02FF, 'Spacing modifiers (ʻ)'],
  // Combining marks: macOS/iOS hand back NFD-decomposed text, so a Spanish
  // name typed on an iPhone can arrive as "o" + U+0301 rather than "ó".
  [0x0300, 0x036F, 'Combining Diacritical Marks (NFD input from iOS/macOS)'],
  [0x0370, 0x03FF, 'Greek (Δ Σ ε μ π σ τ in DSP labels)'],
  [0x2000, 0x206F, 'General Punctuation (– — ’ “ ” … ‹ ›)'],
  [0x2070, 0x209F, 'Super/subscripts'],
  [0x20A0, 0x20CF, 'Currency'],
  [0x2100, 0x214F, 'Letterlike'],
  [0x2150, 0x218F, 'Number Forms (⅓ ⅔)'],
  [0x2190, 0x21FF, 'Arrows'],
  [0x2200, 0x22FF, 'Math Operators (− ≈ ≠ ≤ ≥ ∞ √)'],
  [0x2300, 0x23FF, 'Misc Technical'],
  [0x2500, 0x259F, 'Box Drawing + Block Elements'],
  [0x25A0, 0x25FF, 'Geometric Shapes'],
  [0x2600, 0x26FF, 'Misc Symbols (★)'],
  [0x2700, 0x27BF, 'Dingbats (✓ ✕)'],
  [0x27F0, 0x27FF, 'Supplemental Arrows-A (⟹)'],
];

const FACES = ['Inter-Regular', 'Inter-SemiBold'];

let text = '';
for (const [lo, hi] of KEEP_RANGES) {
  for (let c = lo; c <= hi; c++) text += String.fromCodePoint(c);
}

const gzip = (b) => zlib.gzipSync(b, { level: 9 }).length;
const kib = (n) => (n / 1024).toFixed(1).padStart(7) + ' KiB';

let rawBefore = 0, rawAfter = 0, gzBefore = 0, gzAfter = 0;
for (const face of FACES) {
  const srcPath = path.join(FONT_DIR, `${face}.ttf`);
  const outPath = path.join(FONT_DIR, `${face}.subset.ttf`);
  const src = fs.readFileSync(srcPath);
  const out = await subsetFont(src, text, { targetFormat: 'truetype' });
  fs.writeFileSync(outPath, out);
  rawBefore += src.length; rawAfter += out.length;
  gzBefore += gzip(src);   gzAfter += gzip(out);
  console.log(`${face.padEnd(18)} raw ${kib(src.length)} -> ${kib(out.length)}` +
              `   gzip ${kib(gzip(src))} -> ${kib(gzip(out))}`);
}
console.log(`${'TOTAL'.padEnd(18)} raw ${kib(rawBefore)} -> ${kib(rawAfter)}` +
            `   gzip ${kib(gzBefore)} -> ${kib(gzAfter)}`);
