/**
 * LLM Visual Testing — Recording Engine
 *
 * Two recording modes (same output format):
 *   - Test mode: Proxy-wraps Playwright page.mouse/keyboard/evaluate
 *   - Interactive mode: Injects DOM event listeners via page.exposeFunction
 *
 * Captures periodic screenshots + game state snapshots.
 */

import fs from "fs";
import path from "path";

/**
 * Capture current game state from the WASM exports.
 */
async function captureState(page) {
  return page.evaluate(() => ({
    playing: typeof isPlaying === "function" ? isPlaying() : false,
    bpm: typeof getBPM === "function" ? getBPM() : 0,
    totalRows: typeof totalRows === "function" ? totalRows() : 0,
    totalNodes: typeof totalNodes === "function" ? totalNodes() : 0,
  }));
}

/**
 * Capture full exported JSON state.
 */
async function captureExported(page) {
  return page.evaluate(() => {
    if (typeof exportJSON === "function") return exportJSON();
    return "";
  });
}

/**
 * Create a recorder that wraps a Playwright page.
 *
 * @param {import('playwright').Page} page
 * @param {Object} options
 * @param {number} options.frameIntervalMs - Screenshot interval (default 500)
 * @param {string} options.mode - "test" (proxy Playwright APIs) or "interactive" (DOM listeners)
 * @returns {Recorder}
 */
export function createRecorder(page, options = {}) {
  const frameIntervalMs = options.frameIntervalMs ?? 500;
  const mode = options.mode ?? "test";
  const viewport = options.viewport ?? { width: 1280, height: 720 };

  let events = [];
  let frames = [];
  let startTime = 0;
  let running = false;
  let frameTimer = null;
  let frameIndex = 0;
  let framePngs = []; // { index, buffer }
  let initialState = null;
  let finalState = null;
  let lastState = null;
  let annotations = [];
  let agentLogData = null; // Set via setAgentLog() for agent mode

  // Serialize all page.screenshot() calls to prevent concurrent CDP corruption.
  // Both the recorder's setInterval loop and the agent's safeScreenshot() call
  // page.screenshot() — overlapping calls corrupt Playwright's CDP session.
  let screenshotInFlight = null;

  async function takeScreenshot(opts) {
    if (screenshotInFlight) {
      try { await screenshotInFlight; } catch (_) {}
    }
    let resolve;
    screenshotInFlight = new Promise((r) => { resolve = r; });
    try {
      const merged = { timeout: 15000, type: "png", ...opts };
      return await page.screenshot(merged);
    } finally {
      screenshotInFlight = null;
      resolve();
    }
  }

  function elapsed() {
    return Date.now() - startTime;
  }

  function pushEvent(evt) {
    if (!running) return;
    events.push({ t: elapsed(), ...evt });
  }

  // --- Screenshot loop ---
  async function captureFrame() {
    if (!running) return;
    try {
      const t = elapsed();
      const buffer = await takeScreenshot({ type: "png" });
      const state = await captureState(page);
      const stateChanged =
        lastState !== null &&
        (state.playing !== lastState.playing ||
          state.bpm !== lastState.bpm ||
          state.totalRows !== lastState.totalRows ||
          state.totalNodes !== lastState.totalNodes);
      lastState = state;

      const file = `frame_${String(frameIndex).padStart(4, "0")}.png`;
      framePngs.push({ index: frameIndex, buffer });
      frames.push({ t, file, state, stateChanged });
      frameIndex++;
    } catch (_) {
      // Page may have closed
    }
  }

  function startFrameLoop() {
    frameTimer = setInterval(() => captureFrame(), frameIntervalMs);
  }

  function stopFrameLoop() {
    if (frameTimer) {
      clearInterval(frameTimer);
      frameTimer = null;
    }
  }

  // --- Test mode: Playwright API proxies ---
  function createProxiedMouse(origMouse) {
    // Track last mouse position for debouncing
    let lastMoveX = -1;
    let lastMoveY = -1;
    let lastMoveTime = 0;

    return {
      async click(x, y, opts) {
        pushEvent({ type: "mouse.click", x: Math.round(x), y: Math.round(y), button: opts?.button ?? "left" });
        return origMouse.click(x, y, opts);
      },
      async dblclick(x, y, opts) {
        pushEvent({ type: "mouse.dblclick", x: Math.round(x), y: Math.round(y), button: opts?.button ?? "left" });
        return origMouse.dblclick(x, y, opts);
      },
      async move(x, y, opts) {
        const rx = Math.round(x);
        const ry = Math.round(y);
        const now = elapsed();
        const dist = Math.hypot(rx - lastMoveX, ry - lastMoveY);
        // Debounce: only record if moved >3px or >33ms since last
        if (dist > 3 || now - lastMoveTime > 33) {
          pushEvent({ type: "mouse.move", x: rx, y: ry });
          lastMoveX = rx;
          lastMoveY = ry;
          lastMoveTime = now;
        }
        return origMouse.move(x, y, opts);
      },
      async down(opts) {
        pushEvent({ type: "mouse.down", x: lastMoveX, y: lastMoveY, button: opts?.button ?? "left" });
        return origMouse.down(opts);
      },
      async up(opts) {
        pushEvent({ type: "mouse.up", x: lastMoveX, y: lastMoveY, button: opts?.button ?? "left" });
        return origMouse.up(opts);
      },
      async wheel(deltaX, deltaY) {
        pushEvent({ type: "mouse.wheel", deltaX, deltaY });
        return origMouse.wheel(deltaX, deltaY);
      },
    };
  }

  function createProxiedKeyboard(origKeyboard) {
    return {
      async down(key) {
        pushEvent({ type: "keyboard.down", key });
        return origKeyboard.down(key);
      },
      async up(key) {
        pushEvent({ type: "keyboard.up", key });
        return origKeyboard.up(key);
      },
      async press(key) {
        pushEvent({ type: "keyboard.press", key });
        return origKeyboard.press(key);
      },
      async type(text, opts) {
        pushEvent({ type: "keyboard.type", text });
        return origKeyboard.type(text, opts);
      },
      async insertText(text) {
        pushEvent({ type: "keyboard.type", text });
        return origKeyboard.insertText(text);
      },
    };
  }

  function createProxiedTouchscreen(origTouch) {
    return {
      async tap(x, y) {
        pushEvent({ type: "touch.start", x: Math.round(x), y: Math.round(y), id: 0 });
        pushEvent({ type: "touch.end", x: Math.round(x), y: Math.round(y), id: 0 });
        return origTouch.tap(x, y);
      },
    };
  }

  // --- Interactive mode: DOM event listeners ---
  async function injectDOMListeners() {
    await page.exposeFunction("__llmRecordEvent", (evtJson) => {
      if (!running) return;
      try {
        const evt = JSON.parse(evtJson);
        pushEvent(evt);
      } catch (_) {}
    });

    await page.addInitScript(() => {
      let lastMoveX = -1, lastMoveY = -1, lastMoveTime = 0;

      document.addEventListener("mousedown", (e) => {
        window.__llmRecordEvent(JSON.stringify({
          type: "mouse.down", x: e.clientX, y: e.clientY,
          button: e.button === 0 ? "left" : e.button === 2 ? "right" : "middle",
        }));
      }, { capture: true });

      document.addEventListener("mouseup", (e) => {
        window.__llmRecordEvent(JSON.stringify({
          type: "mouse.up", x: e.clientX, y: e.clientY,
          button: e.button === 0 ? "left" : e.button === 2 ? "right" : "middle",
        }));
      }, { capture: true });

      document.addEventListener("mousemove", (e) => {
        const now = performance.now();
        const dist = Math.hypot(e.clientX - lastMoveX, e.clientY - lastMoveY);
        if (dist > 3 || now - lastMoveTime > 33) {
          window.__llmRecordEvent(JSON.stringify({
            type: "mouse.move", x: e.clientX, y: e.clientY,
          }));
          lastMoveX = e.clientX;
          lastMoveY = e.clientY;
          lastMoveTime = now;
        }
      }, { capture: true });

      document.addEventListener("wheel", (e) => {
        window.__llmRecordEvent(JSON.stringify({
          type: "mouse.wheel", deltaX: e.deltaX, deltaY: e.deltaY,
        }));
      }, { capture: true });

      document.addEventListener("keydown", (e) => {
        window.__llmRecordEvent(JSON.stringify({
          type: "keyboard.down", key: e.key,
        }));
      }, { capture: true });

      document.addEventListener("keyup", (e) => {
        window.__llmRecordEvent(JSON.stringify({
          type: "keyboard.up", key: e.key,
        }));
      }, { capture: true });

      document.addEventListener("touchstart", (e) => {
        for (const t of e.changedTouches) {
          window.__llmRecordEvent(JSON.stringify({
            type: "touch.start", x: Math.round(t.clientX), y: Math.round(t.clientY), id: t.identifier,
          }));
        }
      }, { capture: true });

      document.addEventListener("touchmove", (e) => {
        for (const t of e.changedTouches) {
          window.__llmRecordEvent(JSON.stringify({
            type: "touch.move", x: Math.round(t.clientX), y: Math.round(t.clientY), id: t.identifier,
          }));
        }
      }, { capture: true });

      document.addEventListener("touchend", (e) => {
        for (const t of e.changedTouches) {
          window.__llmRecordEvent(JSON.stringify({
            type: "touch.end", x: Math.round(t.clientX), y: Math.round(t.clientY), id: t.identifier,
          }));
        }
      }, { capture: true });
    });
  }

  // --- Wrapped page for test mode ---
  let wrappedPage = null;

  function getWrappedPage() {
    if (wrappedPage) return wrappedPage;

    const proxiedMouse = createProxiedMouse(page.mouse);
    const proxiedKeyboard = createProxiedKeyboard(page.keyboard);
    const proxiedTouchscreen = createProxiedTouchscreen(page.touchscreen);

    // Create a proxy that intercepts mouse/keyboard/touchscreen/evaluate
    wrappedPage = new Proxy(page, {
      get(target, prop) {
        if (prop === "mouse") return proxiedMouse;
        if (prop === "keyboard") return proxiedKeyboard;
        if (prop === "touchscreen") return proxiedTouchscreen;
        if (prop === "screenshot") {
          return (opts) => takeScreenshot(opts);
        }
        if (prop === "evaluate") {
          return async (fn, ...args) => {
            const fnStr = typeof fn === "function" ? fn.toString() : String(fn);
            pushEvent({ type: "evaluate", fn: fnStr, args: args.length > 0 ? args : undefined });
            return target.evaluate(fn, ...args);
          };
        }
        const val = target[prop];
        return typeof val === "function" ? val.bind(target) : val;
      },
    });

    return wrappedPage;
  }

  return {
    async start() {
      startTime = Date.now();
      running = true;
      events = [];
      frames = [];
      framePngs = [];
      frameIndex = 0;
      annotations = [];

      // Capture initial state
      initialState = await captureState(page);
      try {
        initialState.exported = await captureExported(page);
      } catch (_) {}
      lastState = { ...initialState };

      if (mode === "interactive") {
        await injectDOMListeners();
      }

      // Capture first frame immediately
      await captureFrame();
      startFrameLoop();
    },

    async stop() {
      stopFrameLoop();

      // Capture final frame + state while page may still be alive.
      // If the browser window was closed, these calls will fail —
      // that's fine, we still save whatever we already captured.
      try {
        await captureFrame(); // running must still be true for this to work
      } catch (_) {}

      running = false;

      try {
        finalState = await captureState(page);
      } catch (_) {
        finalState = lastState ? { ...lastState } : {};
      }
      try {
        finalState.exported = await captureExported(page);
      } catch (_) {}
    },

    async save(dirPath) {
      fs.mkdirSync(dirPath, { recursive: true });

      // Write frame PNGs
      for (const { index, buffer } of framePngs) {
        const file = `frame_${String(index).padStart(4, "0")}.png`;
        fs.writeFileSync(path.join(dirPath, file), buffer);
      }

      // Write recording.json
      const recording = {
        version: 1,
        name: path.basename(dirPath),
        createdAt: new Date(startTime).toISOString(),
        durationMs: Date.now() - startTime,
        viewport,
        mode: agentLogData ? "agent" : mode,
        initialState,
        events,
        frames: frames.map(({ buffer, ...rest }) => rest),
        finalState,
      };

      // Write agent_log.json separately if present (keeps recording.json compatible)
      if (agentLogData) {
        fs.writeFileSync(
          path.join(dirPath, "agent_log.json"),
          JSON.stringify(agentLogData, null, 2)
        );
      }

      fs.writeFileSync(
        path.join(dirPath, "recording.json"),
        JSON.stringify(recording, null, 2)
      );

      return recording;
    },

    getWrappedPage,

    addAnnotation(text) {
      pushEvent({ type: "annotation", text });
    },

    /** Get raw events (for inspection/debugging). */
    getEvents() {
      return events;
    },

    /** Get raw frames (for inspection/debugging). */
    getFrames() {
      return frames;
    },

    /** Check if currently recording. */
    isRecording() {
      return running;
    },

    /** Set agent log data to be saved alongside the recording. */
    setAgentLog(logData) {
      agentLogData = logData;
    },
  };
}
