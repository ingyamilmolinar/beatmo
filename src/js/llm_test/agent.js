/**
 * LLM Visual Testing — Agent Loop
 *
 * Drives a Claude Haiku agent that visually interacts with Beatmo in the browser.
 * The agent takes screenshots, sends them to Claude via the computer use API,
 * receives click/type/scroll actions, and executes them via Playwright.
 *
 * Supports two platforms:
 *   - "desktop" (default): Standard mouse/keyboard interaction
 *   - "mobile": Mouse actions auto-translate to CDP touch events; adds touch_gesture tool
 *
 * Usage:
 *   const agent = createAgent(page, { recorder });
 *   const result = await agent.run("Click Play, wait 3 seconds, click Stop.");
 */

import {
  cdpTap,
  cdpLongPress,
  cdpPinch,
  cdpDrag,
  cdpTwoFingerPan,
} from "../touch_cdp_helpers.js";

const SYSTEM_PROMPT = `You are testing a drum machine web app called Beatmo.

The screen is split into two panes:
- GRID PANE (top ~40%): 2D grid of colored square nodes connected by edges with directional arrows. Nodes glow when "fired" during playback.
- DRUM PANE (bottom ~60%): Contains the transport bar, instrument rows, and EQ panel.

DRUM PANE layout (top to bottom):
- TRANSPORT BAR (single row at top of drum pane):
  Left to right: Play (green triangle), Stop (square), BPM input (text box), BPM +/- (stacked arrows), Subdiv button, Length +/-, Track/Follow, Upload, Import, Export, Master volume slider (far right)
- INSTRUMENT ROWS: Each row has: instrument label, edit (pencil) button, color swatch, volume slider, M (mute), S (solo), FX, O (origin), X (delete). To the right is the step timeline grid.
- ADD ROW (+) button below the last row
- EQ PANEL (bottom ~180px): Channel selector (top-left), HP/LP buttons, EQ/Wave toggle (top-right), 10 vertical band sliders

IMPORTANT: The Play button is a small GREEN TRIANGLE in the transport bar, directly below the grid pane. The Stop button is a small SQUARE immediately to its right. Do NOT confuse them with the EQ controls at the very bottom or with instrument labels in the drum rows.

You can click grid cells to add/remove nodes, shift-drag to create edges. Right-click nodes to open parameter popups or context menus.

IMPORTANT — COORDINATE HINTS:
After every screenshot, you receive text with exact center coordinates for ALL buttons, per-row controls, EQ controls, scrollbar, and UI state. These are pixel-perfect. ALWAYS use these coordinates instead of guessing from the screenshot.

Format: "Buttons: play=(33,337) | stop=(77,337) | ..."
Per-row: "Row0: label=(50,400) | mute=(100,400) | solo=(130,400) | ..."
EQ: "EQ: channelBtn=(x,y) | hpfBtn=(x,y) | band0=(x,y) | ..."
State: "State: playing=true bpm=120 rows=4"

You can also call the query_ui tool to get the full layout snapshot as JSON (includes rects with {x, y, w, h} for every element).

CONSTRAINTS:
- The BPM field is an Ebiten-rendered text input. Click it to focus, select all (Ctrl+A), type a new value, and press Enter. This is faster than clicking +/- repeatedly.
- Volume sliders respond to mouse drag, not click.
- EQ band sliders are vertical — drag up/down.

RELIABLE BUTTON CLICKING:
- Use the click_ui tool for ALL button clicks. It finds the exact pixel coordinates
  from the layout engine and performs a real mouse click, so it works regardless of
  screen layout or resolution.
- For per-row buttons (mute, solo, fx, etc.), pass the "row" parameter (0-based).
- Use the computer tool for non-button interactions: grid clicks, drag operations,
  shift-drag edges, right-click context menus, slider drags, scroll.

NODE SIDEBAR:
- Left-click a grid node (desktop) or tap a grid node (mobile) to select it and open the sidebar panel.
- The sidebar has collapsible sections. Click section headers ("> Volume", "> Logic", etc.) to expand.
- The Logic section has a dropdown that opens when clicked, showing logic kind options.
- After changing logic, the drum row's predicted step pattern updates automatically.
- Use the coordinate hints (sidebar section and button rects appear in nodeMenuRect output via query_ui) to find sidebar controls.

Always report any visual bugs you notice: misaligned elements, overlapping text,
broken rendering, unresponsive buttons, missing visual feedback, etc.
Prefix issues with "Bug:" for functional problems or "Glitch:" for visual issues.

STRUCTURED BUG REPORTING:
When you find an issue, report it on its own line in this exact format:

  ISSUE: [P0|P1|P2|P3] [Bug|Glitch|Suggestion] - <description>

Severity guide:
- P0 (Critical): Crash, data loss, feature completely broken
- P1 (Major): Feature partially broken, wrong behavior, blocks workflow
- P2 (Minor): Cosmetic issue affecting usability (clipped text, hard-to-click button)
- P3 (Trivial): Minor visual polish (slight misalignment, color inconsistency)

Report issues inline as you discover them. Do NOT wait until the final summary.

When you have completed the requested task, provide a summary of what you did,
what you observed, and any issues found. Then stop.`;

const MOBILE_SYSTEM_PROMPT_SUFFIX = `

This is a MOBILE test running on a touchscreen device.
- The layout is stacked (grid on top, drums on bottom).
- Your mouse clicks will be translated to touch taps automatically.
- For MULTI-TOUCH gestures (pinch zoom, two-finger pan), use the touch_gesture tool.
- Available gestures: tap, long_press, pinch_in, pinch_out, two_finger_pan, swipe
- Touch targets are larger than desktop (44px row height, 44px min target).
- Long press (600-700ms) opens context menus instead of right-click.

MOBILE TRANSPORT (compact, single row):
  Play | Stop | BPM input | BPM ± | Subdiv | Len ± | ViewSwitch | Overflow
  No master volume slider on mobile — it is hidden.

MOBILE ROW CONTROLS (minimal):
  Each row shows ONLY: instrument Label + Speaker icon (volume button).
  Inline M/S/FX/O/X buttons are NOT visible on mobile — their rects are zero-sized.
  WARNING: click_ui button="mute/solo/fx/color/edit/delete/origin" will FAIL on mobile (zero rects).

MOBILE CONTEXT MENU (bottom sheet):
  Tap the instrument label to open a bottom-sheet context menu with items:
  Identity group: Instrument, Rename, Color
  Playback group: Mute, Solo
  Effects group: Effects (FX panel)
  Destructive group: Origin, Delete
  Use the computer tool to click items inside the context menu.

MOBILE VOLUME POPUP:
  Tap the speaker icon (volume button) to open a vertical slider popup (52×160px).
  Drag the slider vertically to adjust volume. Tap outside to close.

MOBILE NODE SIDEBAR:
  Tap a grid node to open a left-anchored sidebar panel (260px wide) showing:
  - Instrument name header
  - Collapsible sections: Volume, Pitch, Duration, Logic, Groove, Audible
  - Each section has +/- buttons and value displays
  - Logic section has a dropdown for logic kind (probability, skip_every_n, etc.)
  After changing logic kind, check the drum row timeline — the step pattern should visibly change.
  Logic dropdown items: None, Trigger Every N, Skip Every N, Probability, If Prev Skipped, If Prev Triggered.
  "Skip Every N" defaults to N=2, "Probability" defaults to P=0.5.
  Tap outside or tap the node again to close.

MOBILE EQ / VIEW SWITCH:
  click_ui button="viewSwitch" toggles between Rows view and Audio (EQ) view.
  The viewSwitch button is in the transport bar.

MOBILE OVERFLOW MENU:
  click_ui button="overflow" opens a menu with: Upload, Import, Export.
  These buttons are hidden from the transport bar on mobile and moved here.`;

const TOUCH_GESTURE_TOOL = {
  name: "touch_gesture",
  description:
    "Perform a multi-touch gesture on the touchscreen. Use for: pinch zoom, two-finger pan, long press, swipe. Single taps are handled automatically via mouse click translation.",
  input_schema: {
    type: "object",
    properties: {
      gesture: {
        type: "string",
        enum: ["tap", "long_press", "pinch_in", "pinch_out", "two_finger_pan", "swipe"],
        description: "The gesture type to perform",
      },
      x: { type: "number", description: "Center X coordinate" },
      y: { type: "number", description: "Center Y coordinate" },
      duration: {
        type: "number",
        description: "Hold duration in ms (for long_press, default 600)",
      },
      delta_x: {
        type: "number",
        description: "Horizontal movement in px (for pan, swipe)",
      },
      delta_y: {
        type: "number",
        description: "Vertical movement in px (for pan, swipe)",
      },
    },
    required: ["gesture", "x", "y"],
  },
};

const QUERY_UI_TOOL = {
  name: "query_ui",
  description:
    "Get exact pixel positions of ALL UI buttons, controls, sliders, and EQ elements. Returns a JSON object with rects {x, y, w, h} for every interactive element. The center of a rect is (x + w/2, y + h/2). Use this to get precise click targets.",
  input_schema: {
    type: "object",
    properties: {},
    required: [],
  },
};

const CLICK_UI_TOOL = {
  name: "click_ui",
  description: "Click a UI button by name at its exact pixel position. " +
    "Resolves coordinates from the layout engine, then performs a real mouse click+hold. " +
    "Global buttons: play, stop, bpmInc, bpmDec, subdiv, lenInc, lenDec, track, " +
    "addRow, upload, import, export, overflow, viewSwitch, eqToggle, mainVol. " +
    "EQ buttons: eqChannel, hpf, lpf. " +
    "Row buttons (requires 'row', 0-based): label, edit, color, mute, solo, fx, origin, delete, volume. " +
    "Returns a screenshot showing the result.",
  input_schema: {
    type: "object",
    properties: {
      button: { type: "string", description: "Button name to click" },
      row: { type: "number", description: "Row index (0-based) for per-row buttons. Omit for global/EQ buttons." },
      hold_ms: { type: "number", description: "Additional hold duration in ms after the game loop processes the click. Default: 0." },
    },
    required: ["button"],
  },
};

const REPEAT_CLICK_UI_TOOL = {
  name: "repeat_click_ui",
  description: "Click a UI button multiple times in rapid succession. " +
    "Equivalent to calling click_ui N times but in a single round-trip. " +
    "Returns a screenshot after all clicks. " +
    "Same button names as click_ui. Row buttons require 'row' (0-based).",
  input_schema: {
    type: "object",
    properties: {
      button: { type: "string", description: "Button name to click" },
      count: { type: "number", description: "Number of times to click (1-20)" },
      row: { type: "number", description: "Row index (0-based) for per-row buttons." },
      delay_ms: { type: "number", description: "Delay between clicks in ms. Default: 150." },
    },
    required: ["button", "count"],
  },
};

/**
 * Query fullLayoutSnapshot() and build a compact coordinate hint string.
 * Returns null if the function is not available.
 *
 * @param {import('playwright').Page} page
 * @returns {Promise<string|null>}
 */
async function buildUIHints(page) {
  return page.evaluate(() => {
    if (typeof fullLayoutSnapshot !== "function") return null;
    const snap = fullLayoutSnapshot();
    if (!snap) return null;
    const center = (r) =>
      r && r.w > 0 && r.h > 0
        ? `(${r.x + Math.round(r.w / 2)}, ${r.y + Math.round(r.h / 2)})`
        : null;
    const lines = [];

    // Transport buttons
    const b = snap.buttons || {};
    const transport = [];
    for (const [name, rect] of Object.entries(b)) {
      const c = center(rect);
      if (c) transport.push(`${name}=${c}`);
    }
    if (transport.length) lines.push("Buttons: " + transport.join(" | "));

    // Per-row controls
    const rows = snap.rows || [];
    for (let i = 0; i < rows.length; i++) {
      const row = rows[i];
      if (!row) continue;
      const parts = [];
      for (const [name, rect] of Object.entries(row)) {
        const c = center(rect);
        if (c) parts.push(`${name}=${c}`);
      }
      if (parts.length) lines.push(`Row${i}: ${parts.join(" | ")}`);
    }

    // EQ controls
    const eq = snap.eq || {};
    const eqParts = [];
    for (const [name, val] of Object.entries(eq)) {
      if (name === "sliders" && Array.isArray(val)) {
        val.forEach((r, j) => {
          const c = center(r);
          if (c) eqParts.push(`band${j}=${c}`);
        });
      } else if (name === "muteBtns" && Array.isArray(val)) {
        val.forEach((r, j) => {
          const c = center(r);
          if (c) eqParts.push(`mute${j}=${c}`);
        });
      } else if (name === "rect") {
        // skip the overall rect
      } else {
        const c = center(val);
        if (c) eqParts.push(`${name}=${c}`);
      }
    }
    if (eqParts.length) lines.push("EQ: " + eqParts.join(" | "));

    // Scrollbar
    const sc = center(snap.scrollBar);
    const st = center(snap.scrollThumb);
    if (sc || st)
      lines.push(`Scroll: bar=${sc || "n/a"} thumb=${st || "n/a"}`);

    // State
    const s = snap.state || {};
    lines.push(
      `State: playing=${s.isPlaying} bpm=${s.bpm} rows=${s.totalRows}`
    );

    return lines.join("\n");
  }).catch(() => null);
}

/**
 * Resolve a button name to its rect from a fullLayoutSnapshot.
 *
 * @param {Object} snap - Result of fullLayoutSnapshot()
 * @param {string} button - Button name
 * @param {number|undefined} row - Row index for per-row buttons
 * @returns {{x: number, y: number, w: number, h: number}|null}
 */
function resolveButtonRect(snap, button, row) {
  // Row buttons
  if (row !== undefined && row !== null) {
    const rows = snap.rows || [];
    if (row >= 0 && row < rows.length && rows[row]) {
      return rows[row][button] || null;
    }
    return null;
  }
  // Global buttons
  if (snap.buttons && snap.buttons[button]) {
    return snap.buttons[button];
  }
  // EQ buttons (different key names in snapshot)
  if (snap.eq) {
    const eqMap = { eqChannel: "channelBtn", hpf: "hpfBtn", lpf: "lpfBtn" };
    const eqKey = eqMap[button];
    if (eqKey && snap.eq[eqKey]) return snap.eq[eqKey];
  }
  return null;
}

/**
 * Execute a touch gesture via CDP helpers.
 *
 * @param {import('playwright').Page} page
 * @param {Object} input - The touch_gesture tool input
 * @param {boolean} verbose
 */
async function executeTouchGesture(page, input, verbose) {
  const { gesture, x, y, duration, delta_x, delta_y } = input;

  if (verbose) {
    const extra = [];
    if (duration) extra.push(`duration=${duration}`);
    if (delta_x) extra.push(`dx=${delta_x}`);
    if (delta_y) extra.push(`dy=${delta_y}`);
    console.log(`  [touch] ${gesture} (${x}, ${y})${extra.length ? " " + extra.join(", ") : ""}`);
  }

  switch (gesture) {
    case "tap":
      await cdpTap(page, x, y);
      break;

    case "long_press":
      await cdpLongPress(page, x, y, duration ?? 600);
      break;

    case "pinch_in":
      // Pinch in = zoom out: fingers start spread (140px) and come together (40px)
      await cdpPinch(page, x, y, 140, 40);
      break;

    case "pinch_out":
      // Pinch out = zoom in: fingers start close (40px) and spread apart (140px)
      await cdpPinch(page, x, y, 40, 140);
      break;

    case "two_finger_pan":
      await cdpTwoFingerPan(page, x, y, delta_x ?? 0, delta_y ?? 0);
      break;

    case "swipe":
      await cdpDrag(page, x, y, x + (delta_x ?? 0), y + (delta_y ?? 0));
      break;

    default:
      if (verbose) {
        console.log(`  [touch] Unknown gesture: ${gesture}`);
      }
  }
}

/**
 * Capture the WebGL canvas content directly within the page, bypassing
 * CDP Page.captureScreenshot which hangs in headless mode when captureStream
 * is active. forceDraw() issues WebGL commands synchronously (WASM), and
 * drawImage() copies the drawing buffer before the browser presents/clears
 * it (same event-loop task, so preserveDrawingBuffer:false is fine).
 *
 * @param {import('playwright').Page} pg
 * @returns {Promise<Buffer|null>}
 */
async function captureCanvas(pg) {
  const b64 = await pg.evaluate(() => {
    if (typeof forceDraw === "function") forceDraw();
    const c = document.querySelector("canvas");
    if (!c) return null;
    const tmp = document.createElement("canvas");
    tmp.width = c.width;
    tmp.height = c.height;
    const ctx = tmp.getContext("2d");
    ctx.drawImage(c, 0, 0);
    return tmp.toDataURL("image/png").split(",")[1];
  });
  return b64 ? Buffer.from(b64, "base64") : null;
}

/**
 * Execute a Claude computer-use action via Playwright.
 * When platform is "mobile", mouse actions are translated to CDP touch events.
 *
 * @param {import('playwright').Page} page
 * @param {Object} action - Claude's tool_use input
 * @param {boolean} verbose - Log actions to console
 * @param {string} platform - "desktop" or "mobile"
 * @returns {Promise<Buffer|null>} Screenshot buffer if action was "screenshot", null otherwise
 */
async function executeAction(page, action, verbose, platform) {
  const { action: actionType, coordinate, text, key, start_coordinate, duration } = action;

  if (verbose) {
    const coords = coordinate ? ` (${coordinate[0]}, ${coordinate[1]})` : "";
    const extra = text ? ` "${text}"` : key ? ` "${key}"` : "";
    const prefix = platform === "mobile" ? "[mobile] " : "";
    console.log(`  [action] ${prefix}${actionType}${coords}${extra}`);
  }

  // Mobile mouse→touch translation
  if (platform === "mobile") {
    switch (actionType) {
      case "screenshot":
        return await captureCanvas(page) ?? await page.screenshot({ type: "png", timeout: 5000 });

      case "left_click":
        if (coordinate) {
          await cdpTap(page, coordinate[0], coordinate[1]);
        }
        return null;

      case "left_click_drag":
        if (start_coordinate && coordinate) {
          await cdpDrag(
            page,
            start_coordinate[0],
            start_coordinate[1],
            coordinate[0],
            coordinate[1]
          );
        }
        return null;

      case "right_click":
        // Right-click on mobile → long press (opens context menu)
        if (coordinate) {
          if (verbose) console.log(`  [mobile] right_click → long_press`);
          await cdpLongPress(page, coordinate[0], coordinate[1], 600);
        }
        return null;

      case "double_click":
        // Double-click on mobile → two quick taps
        if (coordinate) {
          await cdpTap(page, coordinate[0], coordinate[1]);
          await page.waitForTimeout(50);
          await cdpTap(page, coordinate[0], coordinate[1]);
        }
        return null;

      case "scroll":
        if (coordinate) {
          const dir = action.scroll_direction ?? "down";
          const amount = (action.scroll_amount ?? 3) * 100;
          const deltaX = dir === "left" ? -amount : dir === "right" ? amount : 0;
          const deltaY = dir === "up" ? -amount : dir === "down" ? amount : 0;
          await cdpDrag(
            page,
            coordinate[0],
            coordinate[1],
            coordinate[0] - deltaX,
            coordinate[1] - deltaY
          );
        }
        return null;

      case "wait":
        await page.waitForTimeout(duration ?? 1000);
        return null;

      // Keyboard actions work the same on mobile
      case "type":
      case "key":
        break;

      default:
        // Fall through to desktop handler for other actions
        break;
    }
  }

  // Desktop execution (and fallthrough for mobile keyboard actions)
  switch (actionType) {
    case "screenshot":
      return await captureCanvas(page) ?? await page.screenshot({ type: "png", timeout: 5000 });

    case "left_click":
      if (coordinate) {
        const mods = text ? text.split("+").map((m) => m.trim()) : [];
        for (const mod of mods) await page.keyboard.down(mod);
        // Force a game tick with the mouse at the click position.
        // In headless Chromium with SwiftShader, rAF fires infrequently
        // so Ebiten's Update() may never see the pressed state.
        // forceGameTick() overrides input and runs Update() directly.
        await page.mouse.move(coordinate[0], coordinate[1]);
        await page.mouse.down();
        await page.evaluate(([x, y]) => {
          if (typeof forceGameTick === 'function') {
            forceGameTick(x, y);
          }
        }, [coordinate[0], coordinate[1]]);
        await page.mouse.up();
        for (const mod of mods.reverse()) await page.keyboard.up(mod);
      }
      break;

    case "right_click":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
        await page.mouse.down({ button: "right" });
        // forceGameTick only overrides left button; for right-click,
        // fall back to rAF wait with long timeout.
        await page.evaluate(() => new Promise(r => {
          requestAnimationFrame(r);
          setTimeout(r, 5000);
        }));
        await page.mouse.up({ button: "right" });
      }
      break;

    case "double_click":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
        await page.mouse.down();
        await page.evaluate(([x, y]) => {
          if (typeof forceGameTick === 'function') forceGameTick(x, y);
        }, [coordinate[0], coordinate[1]]);
        await page.mouse.up();
        await page.waitForTimeout(50);
        await page.mouse.down();
        await page.evaluate(([x, y]) => {
          if (typeof forceGameTick === 'function') forceGameTick(x, y);
        }, [coordinate[0], coordinate[1]]);
        await page.mouse.up();
      }
      break;

    case "middle_click":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
        await page.mouse.down({ button: "middle" });
        await page.evaluate(() => new Promise(r => {
          requestAnimationFrame(r);
          setTimeout(r, 5000);
        }));
        await page.mouse.up({ button: "middle" });
      }
      break;

    case "type":
      if (text) {
        await page.keyboard.type(text);
      }
      break;

    case "key":
      if (key) {
        await page.keyboard.press(key);
      }
      break;

    case "scroll":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
        const dir = action.scroll_direction ?? "down";
        const amount = (action.scroll_amount ?? 3) * 100;
        const deltaX = dir === "left" ? -amount : dir === "right" ? amount : 0;
        const deltaY = dir === "up" ? -amount : dir === "down" ? amount : 0;
        await page.mouse.wheel(deltaX, deltaY);
      }
      break;

    case "mouse_move":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
      }
      break;

    case "left_click_drag":
      if (start_coordinate && coordinate) {
        await page.mouse.move(start_coordinate[0], start_coordinate[1]);
        await page.mouse.down();
        await page.mouse.move(coordinate[0], coordinate[1], { steps: 10 });
        await page.mouse.up();
      }
      break;

    case "wait":
      await page.waitForTimeout(duration ?? 1000);
      break;

    case "triple_click":
      if (coordinate) {
        await page.mouse.click(coordinate[0], coordinate[1], { clickCount: 3 });
      }
      break;

    case "hold_key":
      if (key) {
        await page.keyboard.down(key);
        await page.waitForTimeout((duration ?? 1) * 1000);
        await page.keyboard.up(key);
      }
      break;

    case "left_mouse_down":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
      }
      await page.mouse.down();
      break;

    case "left_mouse_up":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
      }
      await page.mouse.up();
      break;

    default:
      if (verbose) {
        console.log(`  [action] Unknown action: ${actionType}`);
      }
  }

  return null;
}

/**
 * Create an agent that drives Claude Haiku to interact with Beatmo.
 *
 * @param {import('playwright').Page} page - Playwright page (use recorder.getWrappedPage() for recording)
 * @param {Object} options
 * @param {string} options.model - Claude model (default: claude-haiku-4-5-20251001)
 * @param {number} options.maxIterations - Max agent turns (default: 50)
 * @param {Object} options.screenshotSize - {width, height} matching viewport
 * @param {string} options.apiKey - Anthropic API key
 * @param {boolean} options.verbose - Log actions to console (default: true)
 * @param {string} options.platform - "desktop" or "mobile" (default: "desktop")
 * @returns {Agent}
 */
export function createAgent(page, options = {}) {
  const model = options.model ?? "claude-haiku-4-5-20251001";
  const maxIterations = options.maxIterations ?? 50;
  const screenshotSize = options.screenshotSize ?? { width: 1280, height: 720 };
  const apiKey = options.apiKey ?? process.env.ANTHROPIC_API_KEY;
  const verbose = options.verbose ?? true;
  const platform = options.platform ?? "desktop";

  if (!apiKey) {
    throw new Error("ANTHROPIC_API_KEY is required. Set it via env or pass apiKey option.");
  }

  let stopped = false;

  // Build system prompt based on platform
  const systemPrompt =
    platform === "mobile" ? SYSTEM_PROMPT + MOBILE_SYSTEM_PROMPT_SUFFIX : SYSTEM_PROMPT;

  // Build tools array — always include query_ui; add touch_gesture for mobile
  const tools = [
    {
      type: "computer_20250124",
      name: "computer",
      display_width_px: screenshotSize.width,
      display_height_px: screenshotSize.height,
    },
    QUERY_UI_TOOL,
    CLICK_UI_TOOL,
    REPEAT_CLICK_UI_TOOL,
  ];
  if (platform === "mobile") {
    tools.push(TOUCH_GESTURE_TOOL);
  }

  /**
   * Take a screenshot with retry logic. Falls back to a 1x1 transparent PNG
   * placeholder if the page is unresponsive.
   */
  async function safeScreenshot(retries = 2) {
    for (let attempt = 0; attempt <= retries; attempt++) {
      try {
        // Primary: capture canvas in-page (avoids CDP compositor hang with captureStream)
        const buf = await captureCanvas(page);
        if (buf) return buf;
        // Fallback: CDP screenshot (if no canvas found)
        return await page.screenshot({ type: "png", timeout: 5000 });
      } catch (err) {
        if (attempt < retries) {
          if (verbose) console.log(`  [screenshot] Attempt ${attempt + 1} failed, retrying...`);
          await new Promise((r) => setTimeout(r, 1000));
        } else {
          if (verbose) console.log(`  [screenshot] All attempts failed: ${err.message}`);
          return null;
        }
      }
    }
    return null;
  }

  async function callClaude(messages) {
    const body = {
      model,
      max_tokens: 2048,
      system: systemPrompt,
      tools,
      messages,
    };

    // Retry with exponential backoff for transient errors (429, 5xx)
    const maxRetries = 3;
    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 60000);

      let response;
      try {
        response = await fetch("https://api.anthropic.com/v1/messages", {
          method: "POST",
          signal: controller.signal,
          headers: {
            "Content-Type": "application/json",
            "x-api-key": apiKey,
            "anthropic-version": "2023-06-01",
            "anthropic-beta": "computer-use-2025-01-24",
          },
          body: JSON.stringify(body),
        });
      } catch (err) {
        clearTimeout(timeoutId);
        if (err.name === "AbortError") {
          throw new Error("Claude API request timed out (60s)");
        }
        throw err;
      } finally {
        clearTimeout(timeoutId);
      }

      // Retry on rate limit or server errors
      if ((response.status === 429 || response.status >= 500) && attempt < maxRetries) {
        let delay;
        if (response.status === 429) {
          // Rate limit — respect Retry-After header or use longer backoff
          const retryAfter = response.headers.get("retry-after");
          delay = retryAfter
            ? parseInt(retryAfter) * 1000
            : Math.min(5000 * Math.pow(2, attempt), 60000);
        } else {
          delay = Math.min(1000 * Math.pow(2, attempt), 30000);
        }
        if (verbose) {
          console.log(`  [api] ${response.status} — retrying in ${delay}ms (attempt ${attempt + 1}/${maxRetries})`);
        }
        await new Promise((r) => setTimeout(r, delay));
        continue;
      }

      if (!response.ok) {
        const errBody = await response.text();
        throw new Error(`Claude API error (${response.status}): ${errBody}`);
      }

      return response.json();
    }
  }

  return {
    /**
     * Run the agent loop with the given task prompt.
     *
     * @param {string} taskPrompt - What the agent should do
     * @returns {Promise<AgentResult>}
     */
    async run(taskPrompt) {
      const startTime = Date.now();
      const agentLog = []; // Full message log for debugging
      const issues = [];   // Issues reported by Claude
      let iterations = 0;
      let totalInputTokens = 0;
      let totalOutputTokens = 0;
      stopped = false;

      if (verbose) {
        console.log(`[agent] Starting with model=${model}, platform=${platform}, maxIterations=${maxIterations}`);
        console.log(`[agent] Task: ${taskPrompt}`);
      }

      // Take initial screenshot
      const initialPng = await safeScreenshot();
      if (!initialPng) {
        return {
          task: taskPrompt, model, platform, iterations: 0, durationMs: Date.now() - startTime,
          inputTokens: 0, outputTokens: 0, estimatedCost: 0,
          issues: [], agentLog: [], summary: "",
          error: "Failed to take initial screenshot — page may be unresponsive",
        };
      }
      const initialB64 = initialPng.toString("base64");

      // Build initial message with screenshot + UI coordinate hints
      const initialContent = [
        {
          type: "text",
          text: taskPrompt,
        },
        {
          type: "image",
          source: {
            type: "base64",
            media_type: "image/png",
            data: initialB64,
          },
        },
      ];

      // Inject UI coordinate hints with initial screenshot
      const initialHints = await buildUIHints(page);
      if (initialHints) {
        initialContent.push({ type: "text", text: initialHints });
      }

      const messages = [
        {
          role: "user",
          content: initialContent,
        },
      ];

      let consecutiveScreenshotFailures = 0;
      const maxConsecutiveFailures = 3;

      while (iterations < maxIterations && !stopped) {
        iterations++;
        consecutiveScreenshotFailures = 0; // Reset per iteration

        if (verbose) {
          console.log(`\n[agent] --- Iteration ${iterations}/${maxIterations} ---`);
        }

        // Prune old screenshots to prevent context window overflow
        pruneConversationImages(messages);

        // Call Claude
        let response;
        try {
          response = await callClaude(messages);
        } catch (err) {
          console.error(`[agent] API error: ${err.message}`);
          agentLog.push({
            iteration: iterations,
            error: err.message,
            t: Date.now() - startTime,
          });
          break;
        }

        // Track usage
        if (response.usage) {
          totalInputTokens += response.usage.input_tokens ?? 0;
          totalOutputTokens += response.usage.output_tokens ?? 0;
        }

        agentLog.push({
          iteration: iterations,
          t: Date.now() - startTime,
          response: {
            stop_reason: response.stop_reason,
            content: response.content.map((b) => {
              // Don't store full image data in log — just mark presence
              if (b.type === "image") return { type: "image", note: "screenshot omitted" };
              return b;
            }),
            usage: response.usage,
          },
        });

        // Helper: detect positive/negated observations that should not count as issues.
        // E.g. "✅ No overlapping elements" or "No broken UI components".
        const ISSUE_KEYWORDS_RE = /(?:overlap|broken|unresponsive|misalign|artifact|bug|glitch|rendering error|truncat|clipped|stuck|suggestion)/i;
        function isNegatedObservation(text) {
          if (/✅/.test(text)) return true;
          if (/\b(?:no|not|without|none|zero)\s+\w*\s*/i.test(text) && ISSUE_KEYWORDS_RE.test(text)) {
            // Ensure the negation word appears BEFORE the keyword
            const negMatch = text.search(/\b(?:no|not|without|none|zero)\b/i);
            const kwMatch = text.search(ISSUE_KEYWORDS_RE);
            if (negMatch >= 0 && kwMatch >= 0 && negMatch < kwMatch) return true;
          }
          return false;
        }

        // Process response content blocks
        const toolResults = [];

        for (const block of response.content) {
          if (block.type === "text" && block.text) {
            if (verbose) {
              console.log(`  [claude] ${block.text.slice(0, 200)}${block.text.length > 200 ? "..." : ""}`);
            }

            // Extract issues from Claude's text
            const lines = block.text.split("\n");
            let inIssuesSection = false;
            for (const line of lines) {
              const trimmed = line.trim();
              const lower = trimmed.toLowerCase();

              // New structured format: ISSUE: P1 Bug - description
              const structuredMatch = trimmed.match(/^ISSUE:\s*(P[0-3])\s+(Bug|Glitch|Suggestion)\s*[-–—]\s*(.+)/i);
              if (structuredMatch) {
                issues.push(trimmed);
                continue;
              }

              if (/^(issues|bugs|problems|findings)\s*(found)?:/i.test(trimmed)) {
                inIssuesSection = true;
                continue;
              }
              if (inIssuesSection && trimmed === "") {
                inIssuesSection = false;
                continue;
              }

              if (inIssuesSection && /^[-*•]\s/.test(trimmed)) {
                if (!isNegatedObservation(trimmed)) {
                  issues.push(trimmed.replace(/^[-*•]\s*/, ""));
                }
                continue;
              }

              // Paragraph-form "Bug: ..." or "Glitch: ..." or "Suggestion: ..." at start of line
              if (/^(bug|glitch|suggestion)\s*:/i.test(trimmed)) {
                if (!isNegatedObservation(trimmed)) {
                  issues.push(trimmed);
                }
                continue;
              }

              // Numbered list items: "1. Bug: ..." or "2. Glitch: ..."
              if (/^\d+\.\s+(bug|glitch|suggestion)\s*:/i.test(trimmed)) {
                if (!isNegatedObservation(trimmed)) {
                  issues.push(trimmed.replace(/^\d+\.\s*/, ""));
                }
                continue;
              }

              if (/^[-*•]\s/.test(trimmed) && (
                lower.includes("bug:") ||
                lower.includes("glitch:") ||
                lower.includes("suggestion:") ||
                lower.includes("broken") ||
                lower.includes("misalign") ||
                lower.includes("overlap") ||
                lower.includes("unresponsive") ||
                lower.includes("truncat") ||
                lower.includes("clipped") ||
                lower.includes("stuck") ||
                lower.includes("visual artifact") ||
                lower.includes("rendering error")
              )) {
                if (!isNegatedObservation(trimmed)) {
                  issues.push(trimmed.replace(/^[-*•]\s*/, ""));
                }
              }
            }
          }

          if (block.type === "tool_use") {
            let screenshotResult = null;

            try {
              if (block.name === "query_ui") {
                // Handle query_ui tool — return full layout snapshot JSON
                const snapshot = await page.evaluate(() => {
                  if (typeof fullLayoutSnapshot !== "function") return null;
                  return fullLayoutSnapshot();
                });
                toolResults.push({
                  type: "tool_result",
                  tool_use_id: block.id,
                  content: [
                    {
                      type: "text",
                      text: snapshot
                        ? JSON.stringify(snapshot, null, 2)
                        : "fullLayoutSnapshot not available",
                    },
                  ],
                });
              } else if (block.name === "click_ui") {
                // Handle click_ui tool — resolve button rect and perform real mouse click
                const { button, row, hold_ms } = block.input;
                const snap = await page.evaluate(() => {
                  if (typeof fullLayoutSnapshot !== "function") return null;
                  return fullLayoutSnapshot();
                });

                if (!snap) {
                  toolResults.push({
                    type: "tool_result",
                    tool_use_id: block.id,
                    content: [{ type: "text", text: "fullLayoutSnapshot not available — WASM may not be loaded yet." }],
                    is_error: true,
                  });
                } else {
                  const rect = resolveButtonRect(snap, button, row);
                  if (!rect || rect.w <= 0 || rect.h <= 0) {
                    const available = [];
                    if (snap.buttons) available.push("Global: " + Object.keys(snap.buttons).join(", "));
                    if (snap.rows) {
                      snap.rows.forEach((r, i) => {
                        if (r) available.push(`Row${i}: ` + Object.keys(r).join(", "));
                      });
                    }
                    if (snap.eq) available.push("EQ: eqChannel, hpf, lpf");
                    toolResults.push({
                      type: "tool_result",
                      tool_use_id: block.id,
                      content: [{ type: "text", text: `Button "${button}"${row != null ? ` (row ${row})` : ""} not found. Available: ${available.join(" | ")}` }],
                      is_error: true,
                    });
                  } else {
                    const cx = rect.x + Math.round(rect.w / 2);
                    const cy = rect.y + Math.round(rect.h / 2);
                    if (verbose) {
                      console.log(`  [click_ui] ${button}${row != null ? `[row${row}]` : ""} → (${cx}, ${cy})`);
                    }
                    await page.mouse.move(cx, cy);
                    await page.mouse.down();
                    // Force a game tick with overridden input so the button
                    // handler fires regardless of rAF timing. In headless
                    // Chromium with SwiftShader, rAF fires infrequently and
                    // Update() may never run while the mouse is held.
                    await page.evaluate(([x, y]) => {
                      if (typeof forceGameTick === 'function') {
                        forceGameTick(x, y);
                      }
                    }, [cx, cy]);
                    if (hold_ms > 0) await page.waitForTimeout(hold_ms);
                    await page.mouse.up();

                    // Settle, then screenshot
                    await page.waitForTimeout(200);
                    const png = await safeScreenshot();
                    if (png) {
                      const b64 = png.toString("base64");
                      const toolResultContent = [
                        { type: "image", source: { type: "base64", media_type: "image/png", data: b64 } },
                      ];
                      const uiHints = await buildUIHints(page);
                      if (uiHints) toolResultContent.push({ type: "text", text: uiHints });
                      toolResults.push({ type: "tool_result", tool_use_id: block.id, content: toolResultContent });
                    } else {
                      toolResults.push({
                        type: "tool_result",
                        tool_use_id: block.id,
                        content: [{ type: "text", text: `Clicked ${button} at (${cx}, ${cy}) but screenshot failed.` }],
                        is_error: true,
                      });
                      consecutiveScreenshotFailures++;
                    }
                  }
                }
              } else if (block.name === "repeat_click_ui") {
                // Handle repeat_click_ui tool — click button N times in a single round-trip
                const { button, count: rawCount, row, delay_ms } = block.input;
                const clickCount = Math.max(1, Math.min(20, rawCount ?? 1));
                const delayBetween = delay_ms ?? 150;

                let lastError = null;
                for (let clickIdx = 0; clickIdx < clickCount; clickIdx++) {
                  // Re-resolve rect each iteration (button position may shift, e.g. BPM text width)
                  const snap = await page.evaluate(() => {
                    if (typeof fullLayoutSnapshot !== "function") return null;
                    return fullLayoutSnapshot();
                  });
                  if (!snap) { lastError = "fullLayoutSnapshot not available"; break; }

                  const rect = resolveButtonRect(snap, button, row);
                  if (!rect || rect.w <= 0 || rect.h <= 0) {
                    lastError = `Button "${button}"${row != null ? ` (row ${row})` : ""} not found on click ${clickIdx + 1}`;
                    break;
                  }

                  const cx = rect.x + Math.round(rect.w / 2);
                  const cy = rect.y + Math.round(rect.h / 2);
                  if (verbose && clickIdx === 0) {
                    console.log(`  [repeat_click_ui] ${button}${row != null ? `[row${row}]` : ""} ×${clickCount} → (${cx}, ${cy})`);
                  }

                  await page.mouse.move(cx, cy);
                  await page.mouse.down();
                  await page.evaluate(([x, y]) => {
                    if (typeof forceGameTick === 'function') forceGameTick(x, y);
                  }, [cx, cy]);
                  await page.mouse.up();

                  if (clickIdx < clickCount - 1) {
                    await page.waitForTimeout(delayBetween);
                  }
                }

                if (lastError) {
                  toolResults.push({
                    type: "tool_result",
                    tool_use_id: block.id,
                    content: [{ type: "text", text: lastError }],
                    is_error: true,
                  });
                } else {
                  // Settle, then screenshot
                  await page.waitForTimeout(200);
                  const png = await safeScreenshot();
                  if (png) {
                    const b64 = png.toString("base64");
                    const toolResultContent = [
                      { type: "image", source: { type: "base64", media_type: "image/png", data: b64 } },
                    ];
                    const uiHints = await buildUIHints(page);
                    if (uiHints) toolResultContent.push({ type: "text", text: uiHints });
                    toolResults.push({ type: "tool_result", tool_use_id: block.id, content: toolResultContent });
                  } else {
                    toolResults.push({
                      type: "tool_result",
                      tool_use_id: block.id,
                      content: [{ type: "text", text: `Clicked ${button} ×${clickCount} but screenshot failed.` }],
                      is_error: true,
                    });
                    consecutiveScreenshotFailures++;
                  }
                }
              } else if (block.name === "touch_gesture") {
                // Handle custom touch_gesture tool
                await executeTouchGesture(page, block.input, verbose);

                // Delay for gesture to take effect
                await page.waitForTimeout(500);
              } else {
                // Handle computer tool actions
                screenshotResult = await executeAction(page, block.input, verbose, platform);

                // Small delay after actions to let the UI update
                if (block.input.action !== "screenshot" && block.input.action !== "wait") {
                  await page.waitForTimeout(300);
                }
              }

              // Take screenshot for tool result (skip for query_ui/click_ui which handle their own results)
              if (block.name !== "query_ui" && block.name !== "click_ui" && block.name !== "repeat_click_ui") {
                const png = screenshotResult ?? await safeScreenshot();

                if (png) {
                  const b64 = png.toString("base64");
                  const toolResultContent = [
                    {
                      type: "image",
                      source: {
                        type: "base64",
                        media_type: "image/png",
                        data: b64,
                      },
                    },
                  ];

                  // Inject UI coordinate hints alongside every screenshot
                  const uiHints = await buildUIHints(page);
                  if (uiHints) {
                    toolResultContent.push({ type: "text", text: uiHints });
                  }

                  toolResults.push({
                    type: "tool_result",
                    tool_use_id: block.id,
                    content: toolResultContent,
                  });
                } else {
                  // Screenshot failed — send text fallback so the agent can continue
                  toolResults.push({
                    type: "tool_result",
                    tool_use_id: block.id,
                    content: [
                      {
                        type: "text",
                        text: "Screenshot temporarily unavailable — the page may be slow. Try a simpler action or wait a moment.",
                      },
                    ],
                    is_error: true,
                  });
                  consecutiveScreenshotFailures++;
                }
              }
            } catch (actionErr) {
              if (verbose) {
                console.log(`  [action] Error: ${actionErr.message}`);
              }
              agentLog.push({
                iteration: iterations,
                actionError: actionErr.message,
                t: Date.now() - startTime,
              });
              toolResults.push({
                type: "tool_result",
                tool_use_id: block.id,
                content: [
                  {
                    type: "text",
                    text: `Action failed: ${actionErr.message}. Try a different action.`,
                  },
                ],
                is_error: true,
              });
              consecutiveScreenshotFailures++;
            }
          }
        }

        // Append assistant response to conversation
        messages.push({ role: "assistant", content: response.content });

        // If there were tool uses, send results back
        if (toolResults.length > 0) {
          messages.push({ role: "user", content: toolResults });
        }

        // Bail out if page is consistently unresponsive
        if (consecutiveScreenshotFailures >= maxConsecutiveFailures) {
          if (verbose) {
            console.log(`\n[agent] Page unresponsive (${maxConsecutiveFailures} consecutive failures) — stopping`);
          }
          agentLog.push({
            iteration: iterations,
            error: `Stopped: ${maxConsecutiveFailures} consecutive screenshot/action failures`,
            t: Date.now() - startTime,
          });
          break;
        }

        // Check if agent is done
        if (response.stop_reason === "end_turn") {
          if (verbose) {
            console.log(`\n[agent] Agent finished (end_turn)`);
          }
          break;
        }

        // Pace iterations to stay within rate limits
        const iterationPaceMs = options.iterationPaceMs ?? 2000;
        if (iterations < maxIterations && !stopped && iterationPaceMs > 0) {
          await new Promise((r) => setTimeout(r, iterationPaceMs));
        }
      }

      if (iterations >= maxIterations && verbose) {
        console.log(`\n[agent] Reached max iterations (${maxIterations})`);
      }

      const durationMs = Date.now() - startTime;
      const estimatedCost = estimateCost(totalInputTokens, totalOutputTokens, model);

      const result = {
        task: taskPrompt,
        model,
        platform,
        iterations,
        durationMs,
        inputTokens: totalInputTokens,
        outputTokens: totalOutputTokens,
        estimatedCost,
        issues: [...new Set(issues)], // Deduplicate
        agentLog,
        summary: extractSummary(agentLog),
      };

      if (verbose) {
        console.log(`\n[agent] Done: ${iterations} iterations, ${(durationMs / 1000).toFixed(1)}s, ~$${estimatedCost.toFixed(4)}`);
        if (result.issues.length > 0) {
          console.log(`[agent] Issues found: ${result.issues.length}`);
        }
      }

      return result;
    },

    stop() {
      stopped = true;
    },
  };
}

/**
 * Prune old screenshot images from conversation history to prevent unbounded growth.
 * Keeps the first message (task prompt + initial screenshot) and the last `keepRecent`
 * message pairs intact. For older messages, replaces base64 image content with a
 * text placeholder.
 *
 * @param {Array} messages - The conversation messages array (mutated in place)
 * @param {number} keepRecent - Number of recent message pairs to keep images for (default: 6)
 */
function pruneConversationImages(messages, keepRecent = 6) {
  const protectedTail = keepRecent * 2;
  const pruneEnd = messages.length - protectedTail;

  for (let i = 1; i < pruneEnd; i++) {
    const msg = messages[i];
    if (!msg.content || !Array.isArray(msg.content)) continue;

    msg.content = msg.content.map((block) => {
      if (block.type === "image" && block.source?.type === "base64") {
        return { type: "text", text: "[screenshot pruned from history]" };
      }
      if (block.type === "tool_result" && Array.isArray(block.content)) {
        block.content = block.content.map((inner) => {
          if (inner.type === "image" && inner.source?.type === "base64") {
            return { type: "text", text: "[screenshot pruned from history]" };
          }
          return inner;
        });
      }
      return block;
    });
  }
}

/**
 * Estimate API cost based on token counts.
 */
function estimateCost(inputTokens, outputTokens, model) {
  const isHaiku = model.includes("haiku");
  const inputRate = isHaiku ? 0.80 : 3.00;
  const outputRate = isHaiku ? 4.00 : 15.00;
  return (inputTokens / 1_000_000) * inputRate + (outputTokens / 1_000_000) * outputRate;
}

/**
 * Extract a summary from the last text block in the agent log.
 */
function extractSummary(agentLog) {
  for (let i = agentLog.length - 1; i >= 0; i--) {
    const entry = agentLog[i];
    if (!entry.response?.content) continue;
    for (let j = entry.response.content.length - 1; j >= 0; j--) {
      const block = entry.response.content[j];
      if (block.type === "text" && block.text) {
        return block.text;
      }
    }
  }
  return "";
}
