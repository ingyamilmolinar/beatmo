// recording_capture_backpressure.browser.test.js
//
// Protocol-level backpressure test for recording_capture_worklet.js. The
// happy path (configure → batches → flush/stop → done) is covered by
// recording_capture.browser.test.js / recording_lifecycle.browser.test.js
// through the full WASM pipeline; what was UNTESTED is the bounded-outbox
// discipline the worklet header documents:
//
//   - pending >= maxPending → batch dropped, {type:'drop', n} posted on the
//     worker port, _drops incremented
//   - 'ack' from the worker decrements pending → flow resumes (recovery)
//   - final 'done' on the main port reports droppedBatches
//
// A regression here silently loses recorded audio under encoder stalls —
// the desktop pipeline's dropsTotal.Add(1) discipline has Go tests; this is
// the browser-side equivalent.
//
// The test drives the REAL worklet on a real audio rendering thread
// (oscillator → AudioWorkletNode), with the test page playing the encoder
// worker's role via a MessageChannel it never (then eventually) acks.
// No WASM build needed — this is a pure worklet-protocol test.
//
// Usage: node src/js/recording_capture_backpressure.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    const ct = filePath.endsWith(".js") ? "application/javascript" : "text/html";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

let failures = 0;
function check(name, cond, detail = "") {
  if (cond) {
    console.log(`✓ ${name}`);
  } else {
    console.error(`✗ ${name} ${detail}`);
    failures++;
  }
}

let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  if (process.env.TEST_LOG) {
    page.on("console", (msg) => console.log(`  [page] ${msg.text()}`));
  }
  await page.goto(`http://localhost:${port}/recording_capture_worklet.js`, { waitUntil: "commit" });
  await page.setContent("<html><body></body></html>");

  const result = await page.evaluate(async (origin) => {
    const ctx = new AudioContext();
    await ctx.audioWorklet.addModule(`${origin}/recording_capture_worklet.js`);
    if (ctx.state !== "running") await ctx.resume();

    const node = new AudioWorkletNode(ctx, "recording-capture-processor", {
      numberOfInputs: 1,
      numberOfOutputs: 1,
    });
    const osc = new OscillatorNode(ctx, { frequency: 440 });
    const sink = new GainNode(ctx, { gain: 0 }); // keep silent but pulled
    osc.connect(node).connect(sink).connect(ctx.destination);
    osc.start();

    // The test page plays the encoder worker: it owns port1 of the channel
    // whose port2 is transferred to the worklet as workerPort.
    const chan = new MessageChannel();
    const state = {
      batches: 0,
      batchesAtRecovery: -1,
      drops: 0,
      dropsAtRecovery: -1,
      workerDone: false,
      acking: false,
      pendingUnacked: 0,
    };
    chan.port1.onmessage = (e) => {
      const d = e.data;
      if (d instanceof ArrayBuffer) {
        state.batches++;
        if (state.acking) {
          chan.port1.postMessage({ type: "ack" });
        } else {
          state.pendingUnacked++;
        }
        return;
      }
      if (d && d.type === "drop") state.drops++;
      if (d && d.type === "done") state.workerDone = true;
    };

    const configured = new Promise((resolve) => {
      node.port.onmessage = (e) => {
        if (e.data && e.data.type === "configured") resolve();
      };
    });
    node.port.postMessage(
      {
        type: "configure",
        channelId: "backpressure-test",
        batchFrames: 128, // one batch per render quantum
        maxPending: 1, // outbox saturates after a single un-acked batch
        workerPort: chan.port2,
      },
      [chan.port2],
    );
    await configured;

    const waitUntil = (cond, timeoutMs) =>
      new Promise((resolve) => {
        const t0 = performance.now();
        const tick = () => {
          if (cond()) return resolve(true);
          if (performance.now() - t0 > timeoutMs) return resolve(false);
          setTimeout(tick, 10);
        };
        tick();
      });

    // Phase 1 — starve: never ack. First batch occupies the outbox
    // (pending=1); every subsequent full batch must be dropped.
    const sawDrops = await waitUntil(() => state.drops >= 5, 5000);
    const batchesDuringStarve = state.batches;

    // Phase 2 — recover: ack everything received so far and keep acking.
    // ALSO raise maxPending to the production depth (8): at batchFrames=128 a
    // batch ships every ~2.7ms, faster than an ack can round-trip
    // audio-thread → main thread → audio-thread, so with maxPending=1 a few
    // steady-state drops are physics, not a protocol bug. With depth 8 the
    // ack loop keeps pending well under the bound and drops must stop.
    state.acking = true;
    state.dropsAtRecovery = state.drops;
    state.batchesAtRecovery = state.batches;
    for (; state.pendingUnacked > 0; state.pendingUnacked--) {
      chan.port1.postMessage({ type: "ack" });
    }
    node.port.postMessage({ type: "configure", maxPending: 8 });
    const recovered = await waitUntil(
      () => state.batches >= state.batchesAtRecovery + 5,
      5000,
    );
    // Let the reconfigure + ack loop settle, then measure drop growth over a
    // fresh observation window.
    await new Promise((r) => setTimeout(r, 200));
    const dropsBaseline2 = state.drops;
    await new Promise((r) => setTimeout(r, 500));
    const dropsAfterRecovery = state.drops - dropsBaseline2;

    // Phase 3 — stop: main-port done must carry the drop count.
    const doneMsg = await new Promise((resolve) => {
      node.port.onmessage = (e) => {
        if (e.data && e.data.type === "done") resolve(e.data);
      };
      node.port.postMessage({ type: "stop" });
      setTimeout(() => resolve(null), 3000);
    });

    osc.stop();
    await ctx.close();
    return {
      sawDrops,
      batchesDuringStarve,
      recovered,
      dropsAtRecovery: state.dropsAtRecovery,
      dropsAfterRecovery,
      workerDone: state.workerDone,
      doneMsg,
    };
  }, `http://localhost:${port}`);

  check(
    "starved outbox emits drop notices (maxPending=1, no acks)",
    result.sawDrops,
    JSON.stringify(result),
  );
  check(
    "outbox bound holds during starvation (batches ≤ maxPending)",
    result.batchesDuringStarve <= 1,
    `got ${result.batchesDuringStarve} batches before any ack`,
  );
  check("flow resumes after acks (recovery)", result.recovered, JSON.stringify(result));
  check(
    "no drops once acks keep up at production depth (maxPending=8)",
    result.dropsAfterRecovery === 0,
    `post-recovery drops=${result.dropsAfterRecovery}`,
  );
  check("stop → done on main port", !!result.doneMsg, JSON.stringify(result.doneMsg));
  if (result.doneMsg) {
    check(
      "done.droppedBatches reports starvation drops",
      result.doneMsg.droppedBatches >= result.dropsAtRecovery,
      `done.droppedBatches=${result.doneMsg.droppedBatches} < observed ${result.dropsAtRecovery}`,
    );
    check(
      "done.samplesCaptured > 0",
      result.doneMsg.samplesCaptured > 0,
      JSON.stringify(result.doneMsg),
    );
  }
  check("worker port received done", result.workerDone);

  console.log(`\n[capture-backpressure] failures=${failures}`);
} catch (err) {
  console.error("FATAL:", err);
  failures++;
} finally {
  if (browser) await browser.close();
  server.close();
}
process.exit(failures === 0 ? 0 : 1);
