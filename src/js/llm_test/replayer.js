/**
 * LLM Visual Testing — Replay Engine
 *
 * Reads recording.json and dispatches events via Playwright with correct timing.
 */

import fs from "fs";
import path from "path";

/**
 * Whitelist of WASM export function names that are safe to evaluate.
 */
const SAFE_EXPORTS = new Set([
  "startPlay", "stopPlay", "addNode", "addEdgeGrid", "deleteNodeGrid",
  "importJSON", "exportJSON", "updateBeatInfosJS", "forceDraw",
  "setOrigin", "setBPM", "setMainVolume", "addDrumRow", "toggleMute",
  "toggleSolo", "setRowInstrument", "setRowStep", "setNodeLogicGrid",
  "nodeActionAt", "closeNodeMenu", "closeAllPopups", "ensureDefaultPath",
  "isPlaying", "getBPM", "totalRows", "totalNodes",
]);

/**
 * Check if an evaluate string only calls whitelisted WASM exports.
 * Allows optional chaining (?.) and basic arguments.
 */
function isEvaluateSafe(fnStr) {
  // Match calls like: exportName?.(...) or exportName(...)
  const callPattern = /\b([a-zA-Z_]\w*)\s*\?\.\s*\(|([a-zA-Z_]\w*)\s*\(/g;
  let match;
  while ((match = callPattern.exec(fnStr)) !== null) {
    const name = match[1] || match[2];
    // Skip common JS builtins
    if (["function", "return", "typeof", "if", "for", "const", "let", "var"].includes(name)) continue;
    if (!SAFE_EXPORTS.has(name)) {
      return false;
    }
  }
  return true;
}

/**
 * Load a recording from disk.
 *
 * @param {string} dirPath - Path to recording directory
 * @returns {Object} recording object with parsed JSON
 */
export function loadRecording(dirPath) {
  const jsonPath = path.join(dirPath, "recording.json");
  if (!fs.existsSync(jsonPath)) {
    throw new Error(`Recording not found: ${jsonPath}`);
  }
  const recording = JSON.parse(fs.readFileSync(jsonPath, "utf-8"));
  recording._dirPath = dirPath;
  return recording;
}

/**
 * Create a replayer for a recorded session.
 *
 * @param {import('playwright').Page} page - Playwright page
 * @param {Object} recording - Parsed recording object
 * @param {Object} options
 * @param {number} options.speedFactor - Playback speed multiplier (default 1.0)
 * @param {boolean} options.skipEvaluates - Skip evaluate events (default false)
 * @returns {Replayer}
 */
export function createReplayer(page, recording, options = {}) {
  const speedFactor = options.speedFactor ?? 1.0;
  const skipEvaluates = options.skipEvaluates ?? false;

  async function dispatchEvent(evt) {
    switch (evt.type) {
      case "mouse.move":
        await page.mouse.move(evt.x, evt.y);
        break;
      case "mouse.down":
        await page.mouse.down({ button: evt.button ?? "left" });
        break;
      case "mouse.up":
        await page.mouse.up({ button: evt.button ?? "left" });
        break;
      case "mouse.click":
        await page.mouse.click(evt.x, evt.y, { button: evt.button ?? "left" });
        break;
      case "mouse.dblclick":
        await page.mouse.dblclick(evt.x, evt.y, { button: evt.button ?? "left" });
        break;
      case "mouse.wheel":
        await page.mouse.wheel(evt.deltaX ?? 0, evt.deltaY ?? 0);
        break;
      case "keyboard.down":
        await page.keyboard.down(evt.key);
        break;
      case "keyboard.up":
        await page.keyboard.up(evt.key);
        break;
      case "keyboard.press":
        await page.keyboard.press(evt.key);
        break;
      case "keyboard.type":
        await page.keyboard.type(evt.text);
        break;
      case "touch.start":
      case "touch.move":
      case "touch.end": {
        // Use CDP for touch events (trusted)
        const cdpSession = await page.context().newCDPSession(page);
        const touchType =
          evt.type === "touch.start" ? "touchStart" :
          evt.type === "touch.move" ? "touchMove" : "touchEnd";
        const touchPoints = evt.type === "touch.end" ? [] : [{ x: evt.x, y: evt.y }];
        await cdpSession.send("Input.dispatchTouchEvent", {
          type: touchType,
          touchPoints,
        });
        await cdpSession.detach();
        break;
      }
      case "evaluate":
        if (!skipEvaluates && isEvaluateSafe(evt.fn)) {
          try {
            // Wrap in AsyncFunction to support arrow expressions
            const wrapped = `return (${evt.fn})${evt.args ? `(${JSON.stringify(evt.args).slice(1, -1)})` : "()"}`;
            await page.evaluate(new Function(wrapped));
          } catch (e) {
            console.warn(`[replay] evaluate failed: ${e.message}`);
          }
        }
        break;
      case "wait":
        await page.waitForTimeout(evt.durationMs ?? 100);
        break;
      case "annotation":
        // No-op during replay
        break;
      default:
        console.warn(`[replay] Unknown event type: ${evt.type}`);
    }
  }

  return {
    /**
     * Replay the full session.
     */
    async play() {
      return this.playUntil(Infinity);
    },

    /**
     * Replay up to a given timestamp.
     *
     * @param {number} timeMs - Stop after this timestamp
     */
    async playUntil(timeMs) {
      // Restore initial state if exported JSON is available
      if (recording.initialState?.exported) {
        try {
          await page.evaluate((json) => {
            if (typeof importJSON === "function") importJSON(json);
            if (typeof forceDraw === "function") forceDraw();
          }, recording.initialState.exported);
          await page.waitForTimeout(300);
        } catch (e) {
          console.warn(`[replay] Failed to restore initial state: ${e.message}`);
        }
      }

      const sortedEvents = [...recording.events].sort((a, b) => a.t - b.t);
      let prevT = 0;

      for (const evt of sortedEvents) {
        if (evt.t > timeMs) break;

        const delay = (evt.t - prevT) / speedFactor;
        if (delay > 0) {
          await page.waitForTimeout(Math.max(1, Math.round(delay)));
        }

        await dispatchEvent(evt);
        prevT = evt.t;
      }
    },
  };
}
