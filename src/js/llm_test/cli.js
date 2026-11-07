#!/usr/bin/env node
/** Subcommands: record, replay, evaluate, agent, report, list-tests, run-tests. E.g. node src/js/llm_test/cli.js agent
 * --test play_stop_basic, or run-tests [--filter] [--tags] [--platform]. */

import { parseArgs } from "node:util";
import path from "path";
import fs from "fs";
import { fileURLToPath } from "url";
import { chromium, devices } from "playwright";
import { buildMainWasm, createServer } from "../real_input_test_helpers.js";
import { launchBrowser } from "../platform/launch.js";
import { createRecorder } from "./recorder.js";
import { loadRecording, createReplayer } from "./replayer.js";
import { createEvaluator, PROMPT_TEMPLATES } from "./evaluator.js";
import { createAgent } from "./agent.js";
import { generateReport } from "./reporter.js";
import { loadTest, listTests, loadFixture } from "./test_loader.js";
import { evaluateGate } from "./checkpoints.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = path.resolve(__dirname, "..");
const recordingsDir = path.resolve(jsDir, "recordings");

/** Chromium flags for headless WebGL rendering via SwiftShader. */
const HEADLESS_GPU_ARGS = [
  "--autoplay-policy=no-user-gesture-required",
  "--use-gl=angle",
  "--use-angle=swiftshader",
  "--disable-gpu-sandbox",
];

// --- Media Recording (canvas + audio) ---

/** In-page MediaRecorder for canvas video AND WebAudio: Playwright's recordVideo has no audio. */
async function startMediaRecording(page) {
  // playSound only enqueues; the context is created by unlockAudio() on a gesture.
  await page.evaluate(() => {
    document.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
  });
  await page.waitForTimeout(300);

  // A silent kick flushes the queue and wires the whole chain.
  await page.evaluate(async () => {
    if (typeof playSound === "function") {
      try { await playSound("kick", 0); } catch (_) {}
    }
  });
  await page.waitForTimeout(200);

  // At the Playwright level: an in-page polling loop would delay captureStream and kill the frame rate.
  await page.waitForFunction('!!window.__audioCtx', { timeout: 3000 }).catch(() => null);

  // Synchronous, so captureStream(30) starts at once.
  await page.evaluate(() => {
    const canvas = document.querySelector("canvas");
    if (!canvas) { console.warn("[mediaRec] No canvas found"); return; }

    const canvasStream = canvas.captureStream(5);
    let combinedStream;

    const audioCtx = window.__audioCtx;
    if (audioCtx) {
      const audioDest = audioCtx.createMediaStreamDestination();
      // connect() is additive: the existing connection stays.
      const limiter = typeof __getMainLimiter === "function" && __getMainLimiter();
      if (limiter) {
        limiter.connect(audioDest);
        console.log("[mediaRec] Audio tap connected to limiter");
      } else {
        // Created on the first real play.
        console.log("[mediaRec] Limiter not ready — polling for connection");
        const poll = setInterval(() => {
          const lim = typeof __getMainLimiter === "function" && __getMainLimiter();
          if (lim) {
            lim.connect(audioDest);
            console.log("[mediaRec] Audio tap connected to limiter (deferred)");
            clearInterval(poll);
          }
        }, 500);
        window.__mediaRecAudioPoll = poll;
      }
      combinedStream = new MediaStream([
        ...canvasStream.getVideoTracks(),
        ...audioDest.stream.getAudioTracks(),
      ]);
      window.__mediaRecAudioDest = audioDest;
    } else {
      console.warn("[mediaRec] No AudioContext — video only");
      combinedStream = canvasStream;
    }

    // Pick a supported mimeType
    const types = [
      "video/webm;codecs=vp8,opus",
      "video/webm;codecs=vp9,opus",
      "video/webm",
    ];
    let mimeType = "";
    for (const t of types) {
      if (MediaRecorder.isTypeSupported(t)) { mimeType = t; break; }
    }

    const chunks = [];
    const recorder = new MediaRecorder(combinedStream, mimeType ? { mimeType } : {});
    recorder.ondataavailable = (e) => {
      if (e.data && e.data.size > 0) chunks.push(e.data);
    };
    recorder.start(1000); // flush data every 1s

    window.__mediaRecorder = recorder;
    window.__mediaRecorderChunks = chunks;
    console.log("[mediaRec] Started", mimeType || "(default codec)");
  });
}

/** null if nothing was recording. */
async function stopMediaRecording(page) {
  const base64 = await page.evaluate(() => {
    return new Promise((resolve) => {
      // Clean up deferred audio poll if still running
      if (window.__mediaRecAudioPoll) {
        clearInterval(window.__mediaRecAudioPoll);
        window.__mediaRecAudioPoll = null;
      }
      const recorder = window.__mediaRecorder;
      if (!recorder || recorder.state === "inactive") { resolve(null); return; }

      recorder.onstop = async () => {
        try {
          const blob = new Blob(window.__mediaRecorderChunks, { type: recorder.mimeType || "video/webm" });
          const ab = await blob.arrayBuffer();
          const bytes = new Uint8Array(ab);
          // Base64-encode in chunks to avoid stack overflow
          let binary = "";
          const CHUNK = 8192;
          for (let i = 0; i < bytes.length; i += CHUNK) {
            binary += String.fromCharCode.apply(null, bytes.subarray(i, i + CHUNK));
          }
          resolve(btoa(binary));
        } catch (err) {
          console.error("[mediaRec] encode error:", err);
          resolve(null);
        }
      };
      recorder.stop();
    });
  });

  if (!base64) return null;
  return Buffer.from(base64, "base64");
}

function printUsage() {
  console.log(`
LLM Visual Testing Framework for Beatmo

Usage:
  node src/js/llm_test/cli.js <command> [options]

Commands:
  record      Interactive recording (opens browser, Ctrl+C to stop)
  replay      Replay a recorded session
  evaluate    Send recording frames to Claude for evaluation
  agent       Run Claude agent to visually interact with Beatmo
  report      Generate HTML report from a recording/agent session
  list-tests  List available prompt-based test cases
  run-tests   Run a batch of test cases sequentially

Record options:
  --name <name>         Session name (default: session_<timestamp>)
  --viewport <WxH>      Viewport size (default: 1280x720)
  --frame-interval <ms> Screenshot interval in ms (default: 500)

Replay options:
  <path>                Path to recording directory
  --speed <factor>      Playback speed (default: 1.0)
  --headless            Run headless (default: visible)

Evaluate options:
  <path>                Path to recording directory
  --template <name>     Prompt template: general_quality, interaction_flow, comparison
  --prompt <text>       Custom prompt (overrides template)
  --max-frames <n>      Maximum frames to send (default: 20)
  --model <model>       Claude model (default: claude-sonnet-4-5-20250929)
  --save                Save result to evaluation.json in recording directory

Agent options:
  --prompt <text>       What the agent should do (required unless --test is used)
  --test <name>         Load a .test.md test case by name (overrides --prompt)
  --platform <platform> "desktop" or "mobile" (default: from test or "desktop")
  --max-iterations <n>  Max agent actions (default: from test or 100)
  --model <model>       Claude model (default: claude-haiku-4-5-20251001)
  --viewport <WxH>      Browser size (default: from test or 1280x720)
  --name <session>      Session name (default: agent_<timestamp> or test name)
  --no-video            Disable video recording

List-tests options:
  --filter <pattern>    Substring filter on test name
  --tags <tag1,tag2>    Only tests with matching tags
  --platform <platform> Only tests for this platform

Run-tests options:
  --filter <pattern>    Substring filter on test name
  --tags <tag1,tag2>    Only tests with matching tags
  --platform <platform> Only tests for this platform
  --model <model>       Claude model (default: claude-haiku-4-5-20251001)
  --save                Save batch results to recordings/batch_<timestamp>.json

Environment:
  ANTHROPIC_API_KEY     Required for evaluate, agent, and run-tests commands
  GO                    Path to Go binary (for WASM build)
`);
}

// --- Record Command ---

async function cmdRecord(args) {
  const { values } = parseArgs({
    args,
    options: {
      name: { type: "string", default: `session_${Date.now()}` },
      viewport: { type: "string", default: "1280x720" },
      "frame-interval": { type: "string", default: "500" },
    },
    allowPositionals: false,
  });

  const name = values.name;
  const [vw, vh] = values.viewport.split("x").map(Number);
  const frameIntervalMs = parseInt(values["frame-interval"], 10);
  const savePath = path.join(recordingsDir, name);

  console.log(`[record] Building WASM...`);
  if (!buildMainWasm()) {
    console.error("WASM build failed");
    process.exit(1);
  }

  const server = await createServer();
  const port = server.address().port;
  console.log(`[record] Server started on port ${port}`);

  const browser = await chromium.launch({
    headless: false,
    args: ["--autoplay-policy=no-user-gesture-required"],
  });

  const context = await browser.newContext({
    viewport: { width: vw, height: vh },
    recordVideo: { dir: savePath, size: { width: vw, height: vh } },
  });

  const page = await context.newPage();
  page.on("console", (msg) => {
    try {
      console.log("[PAGE]", msg.type(), msg.text());
    } catch (_) {}
  });

  console.log(`[record] Navigating to http://localhost:${port}/`);
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof ensureDefaultPath === "function", {
    timeout: 30000,
  });
  await page.waitForTimeout(500);

  // Initialize the default path so the app is usable
  await page.evaluate(() => {
    if (typeof ensureDefaultPath === "function") ensureDefaultPath();
    if (typeof forceDraw === "function") forceDraw();
  });
  await page.waitForTimeout(300);

  // Start in-browser canvas+audio recording
  await startMediaRecording(page);

  // Create recorder in interactive mode (DOM event listeners)
  const recorder = createRecorder(page, {
    mode: "interactive",
    frameIntervalMs,
    viewport: { width: vw, height: vh },
  });

  await recorder.start();
  console.log(`\n[record] Recording "${name}" — interact with the browser`);
  console.log(`[record] Press Ctrl+C to stop and save.\n`);

  // Handle Ctrl+C or window close gracefully.
  let stopping = false;
  const stopAndSave = async () => {
    if (stopping) return;
    stopping = true;
    console.log("\n[record] Stopping recording...");

    await recorder.stop();
    const recording = await recorder.save(savePath);

    // Retrieve canvas+audio video from in-browser MediaRecorder
    const mediaRecBuffer = await stopMediaRecording(page);

    let video = null;
    try { video = page.video(); } catch (_) {}
    try { await context.close(); } catch (_) {}

    const destVideo = path.join(savePath, "video.webm");
    if (mediaRecBuffer && mediaRecBuffer.length > 0) {
      fs.writeFileSync(destVideo, mediaRecBuffer);
      console.log(`[record] Video with audio saved: ${destVideo}`);
    } else if (video) {
      try {
        const videoPath = await video.path();
        if (videoPath && fs.existsSync(videoPath)) {
          fs.copyFileSync(videoPath, destVideo);
          console.log(`[record] Video saved (no audio): ${destVideo}`);
        }
      } catch (_) {}
    }

    console.log(
      `[record] Saved to ${savePath}/ (${recording.events.length} events, ${recording.frames.length} frames)`
    );

    try { await browser.close(); } catch (_) {}
    server.close();
    process.exit(0);
  };

  process.on("SIGINT", stopAndSave);
  process.on("SIGTERM", stopAndSave);

  page.on("close", () => {
    if (!stopping) stopAndSave();
  });
}

// --- Replay Command ---

async function cmdReplay(args) {
  const { values, positionals } = parseArgs({
    args,
    options: {
      speed: { type: "string", default: "1.0" },
      headless: { type: "boolean", default: false },
    },
    allowPositionals: true,
  });

  const recordingPath = positionals[0];
  if (!recordingPath) {
    console.error("Usage: replay <recording-path> [--speed 1.0]");
    process.exit(1);
  }

  const absPath = path.resolve(recordingPath);
  const recording = loadRecording(absPath);
  const speedFactor = parseFloat(values.speed);

  console.log(
    `[replay] Loaded "${recording.name}" (${recording.events.length} events, ${recording.durationMs}ms)`
  );

  console.log(`[replay] Building WASM...`);
  if (!buildMainWasm()) {
    console.error("WASM build failed");
    process.exit(1);
  }

  const server = await createServer();
  const port = server.address().port;

  const vp = recording.viewport ?? { width: 1280, height: 720 };
  const browser = await chromium.launch({
    headless: values.headless,
    args: values.headless
      ? HEADLESS_GPU_ARGS
      : ["--autoplay-policy=no-user-gesture-required"],
  });

  const page = await browser.newPage({ viewport: vp });
  page.on("console", (msg) => {
    try {
      console.log("[PAGE]", msg.type(), msg.text());
    } catch (_) {}
  });

  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof ensureDefaultPath === "function", {
    timeout: 30000,
  });
  await page.waitForTimeout(500);

  const replayer = createReplayer(page, recording, { speedFactor });

  console.log(`[replay] Playing at ${speedFactor}x speed...`);
  await replayer.play();
  console.log(`[replay] Done.`);

  if (!values.headless) {
    await page.waitForTimeout(2000);
  }

  await browser.close();
  server.close();
}

// --- Evaluate Command ---

async function cmdEvaluate(args) {
  const { values, positionals } = parseArgs({
    args,
    options: {
      template: { type: "string", default: "general_quality" },
      prompt: { type: "string" },
      "max-frames": { type: "string", default: "20" },
      model: { type: "string" },
      save: { type: "boolean", default: false },
    },
    allowPositionals: true,
  });

  const recordingPath = positionals[0];
  if (!recordingPath) {
    console.error("Usage: evaluate <recording-path> [--template name] [--prompt '...']");
    process.exit(1);
  }

  const absPath = path.resolve(recordingPath);
  const recording = loadRecording(absPath);
  const maxFrames = parseInt(values["max-frames"], 10);

  console.log(
    `[evaluate] Loaded "${recording.name}" (${recording.frames.length} frames)`
  );

  const evalOptions = { maxFrames };
  if (values.model) evalOptions.model = values.model;

  const evaluator = createEvaluator(evalOptions);

  const criteria = {};
  if (values.prompt) {
    criteria.prompt = values.prompt;
  } else {
    criteria.template = values.template;
  }

  const result = await evaluator.evaluate(recording, criteria);

  console.log("\n" + "=".repeat(60));
  console.log("EVALUATION RESULT");
  console.log("=".repeat(60));
  if (result.score !== null) {
    console.log(`Score: ${result.score}/10`);
  }
  if (result.issues.length > 0) {
    console.log("\nIssues:");
    for (const issue of result.issues) {
      console.log(`  - ${issue}`);
    }
  }
  if (result.summary) {
    console.log(`\nSummary: ${result.summary}`);
  }
  console.log("\n--- Full Response ---");
  console.log(result.raw);
  console.log("=".repeat(60));

  if (values.save) {
    const resultPath = path.join(absPath, "evaluation.json");
    fs.writeFileSync(resultPath, JSON.stringify(result, null, 2));
    console.log(`\n[evaluate] Result saved to ${resultPath}`);
  }
}

// --- Agent Core Logic ---

/** Throws on validation errors rather than exiting. */
function resolveAgentParams(opts) {
  let taskPrompt = opts.prompt ?? null;
  let platform = opts.platform ?? "desktop";
  let maxIterations = opts.maxIterations ?? 100;
  let viewportStr = opts.viewport ?? "1280x720";
  let name = opts.name ?? null;
  let fixtureJson = null;
  let deviceName = null;
  let testMeta = null;
  let requiredCheckpoints = [];
  const model = opts.model ?? "claude-haiku-4-5-20251001";
  const enableVideo = opts.enableVideo ?? true;

  if (opts.test) {
    const testCase = loadTest(opts.test, opts.platform);
    if (!testCase) {
      throw new Error(`Test case not found: ${opts.test}. Run 'list-tests' to see available tests.`);
    }
    testMeta = testCase.meta;
    taskPrompt = testCase.body;
    platform = opts.platform ?? testCase.meta.platform ?? "desktop";
    maxIterations = opts.maxIterations ?? testCase.meta.max_iterations ?? 50;
    viewportStr = opts.viewport ?? testCase.meta.viewport ?? "1280x720";
    name = opts.name ?? testCase.meta.name;
    deviceName = testCase.meta.device;
    if (Array.isArray(testCase.meta.required_checkpoints)) {
      requiredCheckpoints = testCase.meta.required_checkpoints;
    }

    if (testCase.meta.fixture && testCase.fixturePath) {
      fixtureJson = fs.readFileSync(testCase.fixturePath, "utf-8");
    }
  }

  if (!taskPrompt) {
    throw new Error('Either --prompt or --test is required.');
  }

  if (!process.env.ANTHROPIC_API_KEY) {
    throw new Error("ANTHROPIC_API_KEY environment variable is required.");
  }

  name = name ?? `agent_${Date.now()}`;
  const [vw, vh] = viewportStr.split("x").map(Number);

  return {
    taskPrompt, platform, maxIterations, name, model, enableVideo,
    vw, vh, fixtureJson, deviceName, testMeta, requiredCheckpoints,
  };
}

/** Reuses an external server and browser through `shared`. */
async function runAgentTest(params, shared) {
  const {
    taskPrompt, platform, maxIterations, name, model, enableVideo,
    vw, vh, fixtureJson, deviceName, testMeta, requiredCheckpoints = [],
  } = params;

  const savePath = path.join(recordingsDir, name);
  const ownsResources = !shared;
  let server = shared?.server;
  let port = shared?.port;
  let browser = shared?.browser;

  // Set up WASM + server + browser if not shared
  if (ownsResources) {
    console.log(`[agent] Building WASM...`);
    if (!buildMainWasm()) {
      throw new Error("WASM build failed");
    }
    server = await createServer();
    port = server.address().port;
    console.log(`[agent] Server started on port ${port}`);
    // So TEST_PLATFORM can drive a real device; chromium-local keeps the headless GPU args so WebGL renders.
    const launch = await launchBrowser({
      headless: true,
      extraArgs: HEADLESS_GPU_ARGS.filter((a) => a !== "--autoplay-policy=no-user-gesture-required"),
    });
    browser = launch.browser;
    // Tears down the local browser and any BrowserStack tunnel together.
    if (launch.cleanup && browser && !browser.__beatmoCleanup) {
      browser.__beatmoCleanup = launch.cleanup;
    }
  }

  // acceptDownloads lets measure_audio capture the recording zip.
  const contextOpts = { acceptDownloads: true };

  if (platform === "mobile") {
    const resolvedDevice = deviceName ?? "iPhone 12 landscape";
    const descriptor = devices[resolvedDevice];
    if (descriptor) {
      Object.assign(contextOpts, descriptor);
      console.log(`[agent] Mobile device: ${resolvedDevice} (${descriptor.viewport.width}x${descriptor.viewport.height})`);
    } else {
      console.warn(`[agent] Unknown device "${resolvedDevice}", using custom viewport`);
      contextOpts.viewport = { width: vw, height: vh };
      contextOpts.hasTouch = true;
      contextOpts.isMobile = true;
    }
  } else {
    contextOpts.viewport = { width: vw, height: vh };
  }

  // Code before recorder.save() needs it.
  fs.mkdirSync(savePath, { recursive: true });
  // No recordVideo: its screencast monopolizes the compositor headless and page.screenshot() hangs; the MediaRecorder
  // captures video with audio instead.

  const context = await browser.newContext(contextOpts);
  const page = await context.newPage();
  page.on("console", (msg) => {
    try {
      console.log("[PAGE]", msg.type(), msg.text());
    } catch (_) {}
  });

  // Ebiten's preserveDrawingBuffer:false clears the buffer after each composite, so in-page captures were blank.
  await page.addInitScript(() => {
    const origGetContext = HTMLCanvasElement.prototype.getContext;
    HTMLCanvasElement.prototype.getContext = function(type, attrs) {
      if (type === "webgl" || type === "webgl2") {
        attrs = Object.assign({}, attrs, { preserveDrawingBuffer: true });
      }
      return origGetContext.call(this, type, attrs);
    };
  });

  if (testMeta) {
    console.log(`[agent] Loaded test: ${testMeta.name} (${platform})`);
    if (testMeta.tags?.length > 0) {
      console.log(`[agent] Tags: ${testMeta.tags.join(", ")}`);
    }
  }

  console.log(`[agent] Navigating to http://localhost:${port}/`);
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof ensureDefaultPath === "function", {
    timeout: 30000,
  });
  await page.waitForTimeout(500);

  // Initialize the app
  await page.evaluate(() => {
    if (typeof ensureDefaultPath === "function") ensureDefaultPath();
    if (typeof forceDraw === "function") forceDraw();
  });

  // Wait for layout to stabilize (button rects valid)
  await page.waitForFunction(() => {
    if (typeof fullLayoutSnapshot !== "function") return false;
    const snap = fullLayoutSnapshot();
    if (!snap || !snap.buttons) return false;
    const play = snap.buttons.play;
    const stop = snap.buttons.stop;
    return play && play.w > 0 && stop && stop.w > 0;
  }, { timeout: 5000 }).catch(() => {
    console.warn("[agent] Warning: Layout not ready after 5s");
  });
  await page.waitForTimeout(500);

  // Import fixture if specified
  if (fixtureJson) {
    console.log(`[agent] Importing fixture: ${testMeta?.fixture ?? "fixture"}`);
    await page.evaluate((json) => {
      if (typeof importJSON === "function") importJSON(json);
      if (typeof forceDraw === "function") forceDraw();
    }, fixtureJson);
    await page.waitForTimeout(500);
  }

  // The LIVE rows and node count are prepended, so tests never hardcode the demo and stay right when it changes.
  let finalTaskPrompt = taskPrompt;
  try {
    const live = await page.evaluate(() => {
      const out = { rows: [], totalRows: 0, totalNodes: 0, bpm: null, subdiv: null };
      if (typeof fullLayoutSnapshot === "function") {
        const s = fullLayoutSnapshot();
        if (s && s.state) {
          out.totalRows = s.state.totalRows;
          out.bpm = s.state.bpm;
          out.subdiv = s.state.subdiv;
        }
      }
      if (typeof exportJSON === "function") {
        try {
          const doc = JSON.parse(exportJSON());
          if (Array.isArray(doc.instruments)) {
            out.rows = doc.instruments.map((ins, i) => ({ row: i, name: ins.name, id: ins.id, volume: ins.volume }));
            if (!out.totalRows) out.totalRows = doc.instruments.length;
          }
          if (Array.isArray(doc.nodes)) out.totalNodes = doc.nodes.length;
        } catch (_) {}
      }
      return out;
    });
    const rowLines = (live.rows || [])
      .map((r) => `  - row ${r.row}: ${r.name} (id=${r.id}, volume=${r.volume})`)
      .join("\n");
    const liveStateBlock =
      `## LIVE DEMO STATE (authoritative — read from the running app at test start)\n` +
      `- Total rows: ${live.totalRows}\n` +
      `- Total nodes: ${live.totalNodes}\n` +
      (live.bpm != null ? `- BPM: ${live.bpm}\n` : "") +
      (live.subdiv != null ? `- Subdiv: ${live.subdiv}\n` : "") +
      (rowLines ? `- Instrument rows (0-indexed):\n${rowLines}\n` : "") +
      `\nUse THESE values as ground truth. Where a step says "row N" or names an ` +
      `instrument, map it to this list. Do NOT assume a fixed row count or names.\n\n`;
    finalTaskPrompt = liveStateBlock + taskPrompt;
    console.log(`[agent] Live demo state: ${live.totalRows} rows, ${live.totalNodes} nodes`);
  } catch (err) {
    console.warn(`[agent] Could not read live demo state: ${err.message}`);
  }

  const effectiveVP = contextOpts.viewport ?? { width: vw, height: vh };

  // Start in-browser canvas+audio recording (replaces Playwright's silent video)
  if (enableVideo) {
    await startMediaRecording(page);
  }

  // Create recorder in test mode
  const recorder = createRecorder(page, {
    mode: "test",
    frameIntervalMs: 1000,
    viewport: effectiveVP,
  });
  await recorder.start();
  const wrappedPage = recorder.getWrappedPage();

  // Create and run the agent
  const agent = createAgent(wrappedPage, {
    model,
    maxIterations,
    screenshotSize: effectiveVP,
    verbose: true,
    platform,
  });

  console.log(`[agent] Running agent with prompt: "${taskPrompt.slice(0, 100)}${taskPrompt.length > 100 ? "..." : ""}"\n`);

  let result;
  try {
    result = await agent.run(finalTaskPrompt);
  } catch (err) {
    console.error(`[agent] Agent error: ${err.message}`);
    result = {
      task: taskPrompt, model, platform, error: err.message,
      iterations: 0, durationMs: 0, inputTokens: 0, outputTokens: 0,
      estimatedCost: 0, issues: [], checkpoints: [], agentLog: [], summary: "",
    };
  }

  // Computed before save so it lands in agent_log.json.
  const gate = evaluateGate(result, requiredCheckpoints);
  result.gate = gate;
  result.requiredCheckpoints = requiredCheckpoints;

  // Stop recording and save
  await recorder.stop();
  recorder.setAgentLog(result);
  const recording = await recorder.save(savePath);

  // Retrieve canvas+audio video from in-browser MediaRecorder
  const mediaRecBuffer = enableVideo ? await stopMediaRecording(page) : null;

  // Handle video — prefer MediaRecorder (has audio) over Playwright (silent)
  let video = null;
  try { video = page.video(); } catch (_) {}
  try { await context.close(); } catch (_) {}

  const destVideo = path.join(savePath, "video.webm");
  if (mediaRecBuffer && mediaRecBuffer.length > 0) {
    fs.writeFileSync(destVideo, mediaRecBuffer);
    console.log(`[agent] Video with audio saved: ${destVideo}`);
  } else if (video) {
    try {
      const videoPath = await video.path();
      if (videoPath && fs.existsSync(videoPath)) {
        if (videoPath !== destVideo) {
          fs.copyFileSync(videoPath, destVideo);
        }
        console.log(`[agent] Video saved (no audio): ${destVideo}`);
      }
    } catch (_) {}
  }

  console.log(`\n[agent] Session saved to ${savePath}/`);
  console.log(`[agent]   ${recording.events.length} events, ${recording.frames.length} frames`);
  if (result.iterations) {
    console.log(`[agent]   ${result.iterations} iterations, ~$${(result.estimatedCost ?? 0).toFixed(4)}`);
  }
  if (result.issues?.length > 0) {
    console.log(`[agent]   ${result.issues.length} issues found`);
  }
  // Objective gate summary (unique checkpoint names; retries collapse to final).
  console.log(
    `[agent]   checkpoints: ${gate.checkpointsPassed}/${gate.checkpointsTotal} passed` +
    (requiredCheckpoints.length ? ` (${requiredCheckpoints.length} required)` : "")
  );
  if (gate.ok) {
    console.log(`[agent]   GATE: PASS ✓`);
  } else {
    console.log(`[agent]   GATE: FAIL ✗`);
    for (const r of gate.reasons) console.log(`[agent]     - ${r}`);
  }
  // Auto-generate HTML report
  try {
    const reportPath = path.join(savePath, "report.html");
    generateReport(savePath, reportPath);
    console.log(`[agent] Report: ${reportPath}`);
  } catch (err) {
    console.error(`[agent] Report generation failed: ${err.message}`);
    console.log(`[agent] Manual: node src/js/llm_test/cli.js report --name ${name}`);
  }

  // The launch wrapper's cleanup also tears down a BrowserStack tunnel.
  if (ownsResources) {
    if (browser && typeof browser.__beatmoCleanup === "function") {
      try { await browser.__beatmoCleanup(); } catch (_) {}
    } else {
      try { await browser.close(); } catch (_) {}
    }
    server.close();
  }

  return result;
}

// --- Agent Command ---

async function cmdAgent(args) {
  const { values } = parseArgs({
    args,
    options: {
      prompt: { type: "string" },
      test: { type: "string" },
      platform: { type: "string" },
      "max-iterations": { type: "string" },
      model: { type: "string", default: "claude-haiku-4-5-20251001" },
      viewport: { type: "string" },
      name: { type: "string" },
      "no-video": { type: "boolean", default: false },
    },
    allowPositionals: false,
  });

  const params = resolveAgentParams({
    prompt: values.prompt,
    test: values.test,
    platform: values.platform,
    maxIterations: values["max-iterations"] ? parseInt(values["max-iterations"], 10) : undefined,
    model: values.model,
    viewport: values.viewport,
    name: values.name,
    enableVideo: !values["no-video"],
  });

  const result = await runAgentTest(params);
  // Non-zero exit on gate failure so CI / make agent reflects a real regression.
  if (result?.gate && !result.gate.ok) {
    console.error(`[agent] TEST FAILED — see GATE reasons above.`);
    process.exitCode = 1;
  }
}

// --- Report Command ---

async function cmdReport(args) {
  const { values } = parseArgs({
    args,
    options: {
      name: { type: "string" },
      output: { type: "string" },
    },
    allowPositionals: false,
  });

  if (!values.name) {
    console.error("Usage: report --name <session> [--output <path>]");
    process.exit(1);
  }

  const dirPath = path.join(recordingsDir, values.name);
  if (!fs.existsSync(dirPath)) {
    console.error(`Recording not found: ${dirPath}`);
    process.exit(1);
  }

  const outputPath = values.output ?? path.join(dirPath, "report.html");
  const result = generateReport(dirPath, outputPath);
  console.log(`[report] HTML report generated: ${result}`);
}

// --- List Tests Command ---

async function cmdListTests(args) {
  const { values } = parseArgs({
    args,
    options: {
      filter: { type: "string" },
      tags: { type: "string" },
      platform: { type: "string" },
    },
    allowPositionals: false,
  });

  const opts = {};
  if (values.filter) opts.filter = values.filter;
  if (values.tags) opts.tags = values.tags.split(",").map((t) => t.trim());
  if (values.platform) opts.platform = values.platform;

  const tests = listTests(opts);

  if (tests.length === 0) {
    console.log("No tests found matching the given filters.");
    return;
  }

  // Print formatted table
  const nameW = Math.max(25, ...tests.map((t) => t.name.length)) + 2;
  const platW = 10;
  const tagW = 30;

  console.log(
    `${"NAME".padEnd(nameW)}${"PLATFORM".padEnd(platW)}${"TAGS".padEnd(tagW)}MAX_ITER`
  );
  console.log("-".repeat(nameW + platW + tagW + 10));

  for (const t of tests) {
    const tags = (t.tags ?? []).join(", ");
    console.log(
      `${t.name.padEnd(nameW)}${t.platform.padEnd(platW)}${tags.padEnd(tagW)}${t.max_iterations}`
    );
  }

  console.log(`\n${tests.length} test(s) found.`);
}

// --- Run Tests Command ---

async function cmdRunTests(args) {
  const { values } = parseArgs({
    args,
    options: {
      filter: { type: "string" },
      tags: { type: "string" },
      platform: { type: "string" },
      model: { type: "string", default: "claude-haiku-4-5-20251001" },
      save: { type: "boolean", default: false },
    },
    allowPositionals: false,
  });

  if (!process.env.ANTHROPIC_API_KEY) {
    console.error("ERROR: Set ANTHROPIC_API_KEY environment variable");
    process.exit(1);
  }

  const opts = {};
  if (values.filter) opts.filter = values.filter;
  if (values.tags) opts.tags = values.tags.split(",").map((t) => t.trim());
  if (values.platform) opts.platform = values.platform;

  const tests = listTests(opts);

  if (tests.length === 0) {
    console.log("No tests found matching the given filters.");
    return;
  }

  console.log(`[batch] Running ${tests.length} test(s)...\n`);

  // Build WASM and start server once for the entire batch
  console.log(`[batch] Building WASM...`);
  if (!buildMainWasm()) {
    console.error("WASM build failed");
    process.exit(1);
  }

  const server = await createServer();
  const port = server.address().port;
  console.log(`[batch] Server started on port ${port}`);

  const browser = await chromium.launch({
    headless: true,
    args: HEADLESS_GPU_ARGS,
  });

  const shared = { server, port, browser };
  const batchResults = [];
  const batchStart = Date.now();

  for (let i = 0; i < tests.length; i++) {
    const t = tests[i];
    console.log(`\n${"=".repeat(60)}`);
    console.log(`[batch] Test ${i + 1}/${tests.length}: ${t.name} (${t.platform})`);
    console.log("=".repeat(60));

    const testStart = Date.now();
    try {
      const params = resolveAgentParams({
        test: t.name,
        platform: t.platform,
        model: values.model,
        name: `batch_${t.name}_${t.platform}_${Date.now()}`,
      });
      const res = await runAgentTest(params, shared);
      const gateOk = !res?.gate || res.gate.ok;
      batchResults.push({
        name: t.name,
        platform: t.platform,
        status: gateOk ? "completed" : "failed",
        gateReasons: gateOk ? [] : (res.gate?.reasons ?? []),
        durationMs: Date.now() - testStart,
      });
    } catch (err) {
      console.error(`[batch] Test "${t.name}" failed: ${err.message}`);
      batchResults.push({
        name: t.name,
        platform: t.platform,
        status: "error",
        error: err.message,
        durationMs: Date.now() - testStart,
      });
    }
  }

  // Clean up shared resources
  try { await browser.close(); } catch (_) {}
  server.close();

  // Print summary table
  const totalDuration = Date.now() - batchStart;
  console.log(`\n\n${"=".repeat(60)}`);
  console.log("BATCH RESULTS");
  console.log("=".repeat(60));

  const nameW = Math.max(25, ...batchResults.map((r) => r.name.length)) + 2;
  console.log(`${"TEST".padEnd(nameW)}${"PLATFORM".padEnd(10)}${"STATUS".padEnd(12)}DURATION`);
  console.log("-".repeat(nameW + 10 + 12 + 10));

  for (const r of batchResults) {
    const dur = r.durationMs < 1000
      ? `${r.durationMs}ms`
      : `${(r.durationMs / 1000).toFixed(1)}s`;
    console.log(
      `${r.name.padEnd(nameW)}${r.platform.padEnd(10)}${r.status.padEnd(12)}${dur}`
    );
  }

  const completed = batchResults.filter((r) => r.status === "completed").length;
  const failed = batchResults.filter((r) => r.status === "failed").length;
  const errors = batchResults.filter((r) => r.status === "error").length;
  console.log(`\nTotal: ${batchResults.length} | Completed: ${completed} | Failed: ${failed} | Errors: ${errors} | ${(totalDuration / 1000).toFixed(1)}s`);
  for (const r of batchResults.filter((r) => r.status === "failed")) {
    console.log(`  FAILED ${r.name} (${r.platform}):`);
    for (const reason of r.gateReasons) console.log(`    - ${reason}`);
  }
  // Non-zero exit if any test failed its gate or errored, so CI reflects it.
  if (failed > 0 || errors > 0) process.exitCode = 1;

  // Save batch results
  if (values.save) {
    const batchFile = path.join(recordingsDir, `batch_${Date.now()}.json`);
    fs.mkdirSync(recordingsDir, { recursive: true });
    fs.writeFileSync(batchFile, JSON.stringify({ results: batchResults, totalDurationMs: totalDuration }, null, 2));
    console.log(`[batch] Results saved to ${batchFile}`);
  }
}

// --- Main ---

const allArgs = process.argv.slice(2);
const command = allArgs[0];
const restArgs = allArgs.slice(1);

try {
  switch (command) {
    case "record":
      await cmdRecord(restArgs);
      break;
    case "replay":
      await cmdReplay(restArgs);
      break;
    case "evaluate":
      await cmdEvaluate(restArgs);
      break;
    case "agent":
      await cmdAgent(restArgs);
      break;
    case "report":
      await cmdReport(restArgs);
      break;
    case "list-tests":
      await cmdListTests(restArgs);
      break;
    case "run-tests":
      await cmdRunTests(restArgs);
      break;
    case "--help":
    case "-h":
    case "help":
      printUsage();
      break;
  default:
    if (command) console.error(`Unknown command: ${command}\n`);
    printUsage();
    process.exit(command ? 1 : 0);
  }
} catch (err) {
  console.error(`ERROR: ${err.message}`);
  process.exit(1);
}
