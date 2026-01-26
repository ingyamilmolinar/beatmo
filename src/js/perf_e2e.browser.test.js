import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Build the main WASM so JS exports are present.
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");

// Serve the whole src/js directory so index.html can load main.wasm and audio.js.
const port = 8330 + Math.floor(Math.random() * 1000);
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
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
// Wait for wasm runtime to be ready (either exported hook or a while for game to start).
await page.waitForFunction(() => typeof startPlay === 'function');
await assertSimpleDrawMode(page, false, "perf.e2e");
// Prep perf counters, then build graph.
await page.evaluate(() => {
  if (typeof resetAudioScheduleMetrics === 'function') resetAudioScheduleMetrics();
  if (typeof resetPerfStats === 'function') resetPerfStats();
});
await clearSchedulerMismatches(page);
// Build stress graph, set BPM, start playback.
await page.evaluate(() => {
  buildPerfRect(4, 1);
  if (typeof forceDraw === 'function') {
    forceDraw();
    forceDraw();
  }
  setBPM(200);
  startPlay();
});
// Let it run for a few seconds to accumulate perf stats including Draw times.
await page.waitForTimeout(3200);
const stats = await page.evaluate(() => perfStats());
await assertNoSchedulerMismatches(page, "perf.e2e: scheduler mismatches");
await page.evaluate(() => {
  if (typeof stopPlay === "function") stopPlay();
});
console.log('perf.e2e.browser:', stats);

const audioMetrics = await page.evaluate(() => { if (typeof getAudioScheduleMetrics !== 'function') { return null;
  }
  return getAudioScheduleMetrics();
});
console.log('perf.e2e.audioMetrics:', audioMetrics);
console.log('perf.e2e.audioHistory:', audioMetrics?.history);

if (!audioMetrics) { throw new Error('audio schedule metrics unavailable');
}
if (audioMetrics.count <= 0) { throw new Error('no audio events observed during e2e playback');
}
if (audioMetrics.overdue > 0) { throw new Error(`observed ${audioMetrics.overdue} overdue audio events`);
}
const minLeadMs = audioMetrics.minLead == null ? null : (audioMetrics.minLead * 1000);
if (minLeadMs == null || minLeadMs < 3.5) {
  throw new Error(`audio min lead ${minLeadMs == null ? 'null' : minLeadMs.toFixed(2)}ms below 3.5ms threshold`);
}
const maxSmallLead = Number(process.env.PERF_E2E_SMALL_LEAD_MAX ?? "2");
if (audioMetrics.smallLeadCount > maxSmallLead) {
  throw new Error(`observed ${audioMetrics.smallLeadCount} audio events scheduled with <4ms lead (max ${maxSmallLead})`);
}
if (audioMetrics.lagP90 != null && audioMetrics.lagP90 > 0.01) { throw new Error(`audio lagP90 ${(audioMetrics.lagP90 * 1000).toFixed(2)}ms exceeded 10ms`);
}
if (audioMetrics.lagP99 != null && audioMetrics.lagP99 > 0.02) { throw new Error(`audio lagP99 ${(audioMetrics.lagP99 * 1000).toFixed(2)}ms exceeded 20ms`);
}

await browser.close();
server.close();

if (stats.frames <= 0) { throw new Error('no frames recorded in e2e WASM');
}

const minFps = Number(process.env.PERF_E2E_FPS_MIN ?? "4");
const maxUpdateAvg = Number(process.env.PERF_E2E_UPDATE_MAX_MS ?? "4.5");
// Some CI/headless environments render slower; keep a modest ceiling but allow
// env override when tighter budgets are required.
const maxDrawAvg = Number(process.env.PERF_E2E_DRAW_MAX_MS ?? "40");
const maxAudioCallAvg = Number(process.env.PERF_E2E_AUDIO_CALL_MAX_MS ?? "0.45");
const maxAudioQLatMax = Number(process.env.PERF_E2E_AUDIO_QLAT_MAX_MS ?? "10");

if (stats.fpsAvg < minFps) { throw new Error(`fpsAvg ${stats.fpsAvg.toFixed(2)} below floor ${minFps}`);
}
if (stats.updateAvgMS > maxUpdateAvg) { throw new Error(`updateAvgMS ${stats.updateAvgMS.toFixed(3)}ms exceeded limit ${maxUpdateAvg}ms`);
}
if (stats.drawAvgMS > maxDrawAvg) { throw new Error(`drawAvgMS ${stats.drawAvgMS.toFixed(3)}ms exceeded limit ${maxDrawAvg}ms`);
}
if (stats.audioCallAvg > maxAudioCallAvg) { throw new Error(`audioCallAvg ${stats.audioCallAvg.toFixed(3)}ms exceeded limit ${maxAudioCallAvg}ms`);
}
if (stats.audioQLatMax > maxAudioQLatMax) { throw new Error(`audioQLatMax ${stats.audioQLatMax.toFixed(3)}ms exceeded limit ${maxAudioQLatMax}ms`);
}
