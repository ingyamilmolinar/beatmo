/**
 * Normalization for Claude computer-use tool calls.
 *
 * Weaker models (e.g. Haiku) frequently emit non-canonical computer actions:
 *   - the tool name is a BARE action verb (`left_click`) instead of the
 *     canonical `computer` tool with `input.action: "left_click"`, and/or
 *   - the coordinate is a JSON **string** ("[568, 592]") instead of an array.
 * Either form silently no-op'd in the agent loop (coordinate[0] === "[",
 * action === undefined → "Unknown action"), which made mobile runs flail into
 * the wrong dialogs and even delete rows. These helpers coerce both back to the
 * canonical shape so the tap actually executes.
 */

// Canonical computer-use action verbs (the `action` field values).
export const COMPUTER_ACTION_NAMES = new Set([
  "screenshot", "left_click", "right_click", "double_click", "middle_click",
  "triple_click", "type", "key", "scroll", "wait", "mouse_move",
  "left_click_drag", "hold_key", "left_mouse_down", "left_mouse_up",
]);

/**
 * Coerce a coordinate into a numeric [x, y] array. Accepts arrays, JSON-string
 * arrays ("[568, 592]"), and loose "x,y"/"(x, y)" strings. Returns the input
 * unchanged when it can't be parsed (e.g. undefined).
 */
export function normCoord(c) {
  if (Array.isArray(c)) return c.map(Number);
  if (typeof c === "string") {
    try {
      const p = JSON.parse(c);
      if (Array.isArray(p)) return p.map(Number);
    } catch (_) {}
    const m = c.match(/-?\d+(?:\.\d+)?/g);
    if (m && m.length >= 2) return [Number(m[0]), Number(m[1])];
  }
  return c;
}

/**
 * Given a tool_use name + input, return a canonical computer-action input with
 * `.action` set and coordinates normalized.
 *
 * @param {string} toolName - block.name
 * @param {Object} input - block.input
 * @returns {Object} normalized action input (new object)
 */
export function normalizeComputerAction(toolName, input) {
  const src = input && typeof input === "object" ? input : {};
  let out;
  if (toolName === "computer") {
    out = { ...src };
  } else if (COMPUTER_ACTION_NAMES.has(toolName)) {
    out = { ...src, action: toolName };
  } else if (toolName === "tap" || toolName === "click") {
    out = { ...src, action: "left_click" };
  } else {
    out = { ...src };
  }
  // A bare coordinate with no action is almost always a click.
  if (!out.action && out.coordinate != null) out.action = "left_click";
  if (out.coordinate != null) out.coordinate = normCoord(out.coordinate);
  if (out.start_coordinate != null) out.start_coordinate = normCoord(out.start_coordinate);
  return out;
}
