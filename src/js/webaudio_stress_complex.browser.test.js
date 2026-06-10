import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const repoRoot = path.resolve(jsDir, "..", "..");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  [
    "build",
    "-ldflags",
    "-X main.defaultLog=INFO",
    "-o",
    path.join(jsDir, "main.wasm"),
    "./cmd/...",
  ],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((resolve) => server.listen(0, resolve));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function");
await assertSimpleDrawMode(page, false, "stress complex");
await clearSchedulerMismatches(page);

const projectJson = fs.readFileSync(path.join(repoRoot, "src", "go", "internal", "assets", "beatmo_project_fixture.json"), "utf8");
const project = JSON.parse(projectJson);

const instrumentData = {};
if (project) {
  const assetDir = path.join(repoRoot, "assets", "wav");
  let wavFiles = new Map();
  try {
    wavFiles = collectWavFiles(assetDir);
  } catch (_) {
    // assets/wav may not exist in CI; builtin synths suffice.
  }

  if (Array.isArray(project.instruments)) { for (const inst of project.instruments) { if (!inst || !inst.id) continue;
      let candidate = null;
      if (inst.path) { candidate = wavFiles.get(path.basename(inst.path).toLowerCase());
      }
      if (!candidate) { candidate = wavFiles.get(`${inst.id.toLowerCase()}.wav`);
      }
      if (!candidate && inst.name) { candidate = wavFiles.get(`${inst.name.toLowerCase()}.wav`);
      }
      if (!candidate && typeof inst.id === "string") { const base = inst.id.replace(/\s+/g, "_").toLowerCase();
        candidate = wavFiles.get(`${base}.wav`);
      }
      if (!candidate) { const first = wavFiles.values().next().value;
        if (first) { candidate = first;
        }
      }
      if (candidate) { instrumentData[inst.id] = `data:audio/wav;base64,${candidate.toString("base64")}`;
      }
    }
  }
}

function collectWavFiles(dir) { const entries = fs.readdirSync(dir, { withFileTypes: true });
  const map = new Map();
  for (const entry of entries) { const full = path.join(dir, entry.name);
    if (entry.isDirectory()) { const child = collectWavFiles(full);
      for (const [k, v] of child.entries()) { map.set(k, v);
      }
    } else if (entry.isFile() && entry.name.toLowerCase().endsWith(".wav")) { const base = entry.name.toLowerCase();
      map.set(base, fs.readFileSync(full));
    }
  }
  return map;
}

await page.evaluate(async ({ projectJson, instrumentData }) => {
  // Unlock AudioContext — page.evaluate() doesn't trigger user gesture events.
  document.dispatchEvent(new Event('pointerdown'));
  resumeAudio?.();

  resetAudioScheduleMetrics?.();
  forceDraw?.();

  if (typeof importJSON === "function") { importJSON(projectJson);
  }

  forceDraw?.();

  if (instrumentData && typeof loadWav === "function") { const entries = Object.entries(instrumentData);
    for (const [id, url] of entries) { try { await loadWav(id, url);
      } catch (err) { console.warn(`loadWav failed for ${id}:`, err);
      }
    }
  }

  // ensure every row uses its instrument id (and refresh once more)
  const insts = typeof instOptions === "function" ? instOptions() : [];
  const rowCount = typeof totalRows === "function" ? totalRows() : 0;
  if (typeof setRowInstrument === "function") { for (let r = 0; r < rowCount; r++) { const id = instOptions?.()[r % insts.length] ?? insts[0];
      if (id) { setRowInstrument(r, id);
      }
    }
  }

  forceDraw?.();
  setBPM?.(210);
  startPlay?.();
}, { projectJson, instrumentData });

await page.evaluate(() => resetPerfStats?.());
const SAMPLE_INTERVAL_MS = Number(process.env.STRESS_COMPLEX_INTERVAL_MS ?? 5000);
const SAMPLE_COUNT = Number(process.env.STRESS_COMPLEX_SAMPLES ?? 6);
const samples = [];
for (let i = 0; i < SAMPLE_COUNT; i++) { await page.waitForTimeout(SAMPLE_INTERVAL_MS);
  samples.push(await page.evaluate(() => ({ perf: perfStats?.(),
    audio: getAudioScheduleMetrics?.(),
  })));
}

await assertNoSchedulerMismatches(page, "stress complex: scheduler mismatches");
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "stress_complex");
await browser.close();
server.close();

console.log("stress_complex samples:", samples);

const firstSample = samples[0] || {};
const lastSample = samples[samples.length - 1] || {};
const stats = lastSample.perf;
const audioMetrics = lastSample.audio;

console.log("stress_complex final perf:", stats);
console.log("stress_complex final audioMetrics:", audioMetrics);
console.log("stress_complex history:", audioMetrics?.history);
console.log("stress_complex drift:", { fpsStart: firstSample.perf?.fpsAvg ?? null,
  fpsEnd: stats?.fpsAvg ?? null,
  leadP99Start: firstSample.audio?.leadP99 ?? null,
  leadP99End: audioMetrics?.leadP99 ?? null,
  lagP99Start: firstSample.audio?.lagP99 ?? null,
  lagP99End: audioMetrics?.lagP99 ?? null,
});

if (!stats) { throw new Error("perfStats unavailable");
}
if (!audioMetrics) { throw new Error("audio schedule metrics unavailable");
}

// Thresholds calibrated 2026-05-06 from 8 fresh runs of this scenario
// (5 back-to-back hot + 3 with cooldown) on the development machine:
//   fpsAvg        observed 14.86–22.04 → gate 12   (~20% headroom over worst)
//   drawAvgMS     observed 46.07–69.50 → gate 80   (~15% headroom over worst)
//   drawMaxMS     observed 102.9–196.2 → gate 250  (~28% headroom; spikes are noisy)
//   audioCallAvg  observed 0.043–0.190 → gate 0.25 (~32% headroom)
//   audioCallMax  observed 0.10–22.85  → gate 50   (covers the outlier with margin)
// Each gate is tight enough to detect a meaningful regression beyond today's
// envelope while loose enough to absorb variance from system load when this
// test runs late in the sequential phase of `make test-real`. To make a gate
// stricter for a one-off experiment, override via the matching env var.
const minFps = Number(process.env.STRESS_COMPLEX_FPS_MIN ?? "12");
const maxDrawAvg = Number(process.env.STRESS_COMPLEX_DRAW_MAX_MS ?? "80");
const maxDrawMax = Number(process.env.STRESS_COMPLEX_DRAW_MAX_SPIKE_MS ?? "250");
const maxAudioCallAvg = Number(process.env.STRESS_COMPLEX_AUDIO_CALL_AVG_MS ?? "0.25");
const maxAudioCallMax = Number(process.env.STRESS_COMPLEX_AUDIO_CALL_MAX_MS ?? "50");
if (stats.fpsAvg < minFps) {
  throw new Error(`fpsAvg ${stats.fpsAvg.toFixed(2)} below ${minFps} in stress scenario`);
}
if (stats.drawAvgMS > maxDrawAvg) {
  throw new Error(`drawAvgMS ${stats.drawAvgMS.toFixed(2)}ms exceeded ${maxDrawAvg}ms target`);
}
if (stats.drawMaxMS > maxDrawMax) {
  throw new Error(`drawMaxMS ${stats.drawMaxMS.toFixed(2)}ms exceeded ${maxDrawMax}ms target (worst-frame spike)`);
}
if (audioMetrics.lagP90 != null && audioMetrics.lagP90 > 0.01) { throw new Error(`lagP90 ${(audioMetrics.lagP90 * 1000).toFixed(2)}ms exceeded 10ms target`);
}
if (audioMetrics.lagP99 != null && audioMetrics.lagP99 > 0.02) { throw new Error(`lagP99 ${(audioMetrics.lagP99 * 1000).toFixed(2)}ms exceeded 20ms target`);
}
if (audioMetrics.avgLag != null && audioMetrics.avgLag > 0.005) { throw new Error(`avgLag ${(audioMetrics.avgLag * 1000).toFixed(2)}ms exceeded 5ms target`);
}
if (audioMetrics.maxLag != null && audioMetrics.maxLag > 0.05) { throw new Error(`maxLag ${(audioMetrics.maxLag * 1000).toFixed(2)}ms exceeded 50ms target`);
}
if (stats.audioCallAvg > maxAudioCallAvg) { throw new Error(`audioCallAvg ${stats.audioCallAvg.toFixed(3)}ms exceeded ${maxAudioCallAvg}ms target`);
}
if (stats.audioCallMax > maxAudioCallMax) { throw new Error(`audioCallMax ${stats.audioCallMax.toFixed(2)}ms exceeded ${maxAudioCallMax}ms target`);
}
