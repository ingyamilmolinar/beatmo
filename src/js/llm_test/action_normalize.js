/** Weaker models emit a bare action verb as the tool name and/or a stringified coordinate; either silently no-op'd in
 * the agent loop (mobile runs flailed into the wrong dialogs, even deleted rows), so both are coerced back to the
 * canonical shape. */

// Canonical computer-use action verbs (the `action` field values).
export const COMPUTER_ACTION_NAMES = new Set([
  "screenshot", "left_click", "right_click", "double_click", "middle_click",
  "triple_click", "type", "key", "scroll", "wait", "mouse_move",
  "left_click_drag", "hold_key", "left_mouse_down", "left_mouse_up",
]);

/** Arrays, JSON-string arrays and loose "x,y"/"(x, y)"; anything unparseable is returned unchanged. */
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

/** A new object with .action set and coordinates normalized. */
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
