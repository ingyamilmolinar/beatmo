/** Dispatches recording.json's events through Playwright with their timing. */

import fs from "fs";
import path from "path";

/** The only exports a replay may evaluate. */
const SAFE_EXPORTS = new Set([
  "startPlay", "stopPlay", "addNode", "addEdgeGrid", "deleteNodeGrid",
  "importJSON", "exportJSON", "updateBeatInfos", "forceDraw",
  "setOrigin", "setBPM", "setMainVolume", "addDrumRow", "toggleMute",
  "toggleSolo", "setRowInstrument", "setRowStep", "setNodeLogicGrid",
  "nodeActionAt", "closeNodeMenu", "closeAllPopups", "ensureDefaultPath",
  "isPlaying", "getBPM", "totalRows", "totalNodes",
]);

/** Only whitelisted exports, optional chaining and basic arguments. */
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

export function loadRecording(dirPath) {
  const jsonPath = path.join(dirPath, "recording.json");
  if (!fs.existsSync(jsonPath)) {
    throw new Error(`Recording not found: ${jsonPath}`);
  }
  const recording = JSON.parse(fs.readFileSync(jsonPath, "utf-8"));
  recording._dirPath = dirPath;
  return recording;
}

/** options: speedFactor (default 1.0), skipEvaluates (false). */
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
    async play() {
      return this.playUntil(Infinity);
    },

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
