/**
 * LLM Visual Testing — Claude Vision API Integration
 *
 * Key-frame selection, prompt templates, structured result parsing.
 * Uses raw fetch() to call the Anthropic API (no SDK dependency).
 */

import fs from "fs";
import path from "path";

/**
 * Built-in prompt templates for common evaluation scenarios.
 */
const PROMPT_TEMPLATES = {
  general_quality: `You are evaluating a drum machine web application called Beatmo.
Look at these screenshots taken during a session. Evaluate:

1. **Rendering quality**: Are UI elements rendered correctly? Are there visual glitches, missing elements, or rendering artifacts?
2. **Layout**: Is the layout well-structured? Are elements properly aligned? Is the grid visible and well-formed?
3. **Visual feedback**: Are active/highlighted elements clearly distinguishable from inactive ones?
4. **Overall impression**: Rate the overall visual quality from 1-10.

For each issue found, describe it specifically with reference to which frame(s) it appears in.

Respond in this format:
Rating: X/10
Issues:
- [issue description]
Summary: [1-2 sentence overall assessment]`,

  interaction_flow: `You are evaluating the interaction flow of a drum machine web application called Beatmo.
These screenshots were taken during a user interaction session. The timestamps and game state are annotated.

Evaluate:
1. **Responsiveness**: Do UI elements appear to respond to user actions between frames?
2. **Visual feedback**: Are there visible changes (highlights, animations, state transitions) that indicate the app is responding?
3. **Consistency**: Does the state shown in frames match the annotated game state (playing/stopped, BPM, row count)?
4. **Flow**: Does the interaction sequence make visual sense? Rate 1-10.

Respond in this format:
Rating: X/10
Issues:
- [issue description]
Summary: [1-2 sentence overall assessment]`,

  comparison: `You are comparing two sets of screenshots from a drum machine web application called Beatmo.
The first set is the "baseline" and the second set is the "current" version.

Look for visual regressions:
1. **Missing elements**: Are any UI elements present in baseline but missing in current?
2. **Layout shifts**: Has the positioning or sizing of elements changed?
3. **Color changes**: Have colors or contrast changed unexpectedly?
4. **New artifacts**: Are there new visual glitches not present in baseline?

Rate the visual similarity from 1-10 (10 = identical, 1 = completely different).

Respond in this format:
Rating: X/10
Issues:
- [issue description]
Summary: [1-2 sentence regression assessment]`,
};

/**
 * Select key frames from a recording for evaluation.
 *
 * @param {Array} frames - Frame entries from recording.json
 * @param {number} maxFrames - Maximum frames to select (default 20)
 * @returns {Array} Selected frame entries
 */
export function selectKeyFrames(frames, maxFrames = 20) {
  if (!frames || frames.length === 0) return [];
  if (frames.length <= maxFrames) return [...frames];

  const selected = new Set();

  // Always include first and last
  selected.add(0);
  selected.add(frames.length - 1);

  // Include all state-changed frames
  for (let i = 0; i < frames.length; i++) {
    if (frames[i].stateChanged) {
      selected.add(i);
    }
  }

  // Fill remaining with evenly-spaced samples
  const remaining = maxFrames - selected.size;
  if (remaining > 0) {
    const step = frames.length / (remaining + 1);
    for (let i = 1; i <= remaining; i++) {
      const idx = Math.min(Math.round(step * i), frames.length - 1);
      selected.add(idx);
    }
  }

  // Sort by index and return
  return [...selected].sort((a, b) => a - b).slice(0, maxFrames).map((i) => frames[i]);
}

/**
 * Build content blocks for the Claude API call.
 *
 * @param {Array} selectedFrames - Selected frame entries
 * @param {string} dirPath - Recording directory (for loading PNGs)
 * @param {string} prompt - Evaluation prompt text
 * @returns {Array} Content blocks for the messages API
 */
function buildContentBlocks(selectedFrames, dirPath, prompt) {
  const blocks = [];

  blocks.push({
    type: "text",
    text: prompt + "\n\n---\n\nBelow are the captured frames with timestamps and game state:\n",
  });

  for (const frame of selectedFrames) {
    const pngPath = path.join(dirPath, frame.file);
    if (!fs.existsSync(pngPath)) {
      console.warn(`[evaluator] Frame not found: ${pngPath}`);
      continue;
    }

    const pngData = fs.readFileSync(pngPath);
    const base64 = pngData.toString("base64");

    // Add frame annotation
    const stateStr = frame.state
      ? `playing=${frame.state.playing}, bpm=${frame.state.bpm}, rows=${frame.state.totalRows}, nodes=${frame.state.totalNodes}`
      : "unknown";
    blocks.push({
      type: "text",
      text: `\n**Frame at t=${frame.t}ms** (${stateStr})${frame.stateChanged ? " [STATE CHANGED]" : ""}:`,
    });

    blocks.push({
      type: "image",
      source: {
        type: "base64",
        media_type: "image/png",
        data: base64,
      },
    });
  }

  return blocks;
}

/**
 * Parse Claude's response into a structured result.
 *
 * @param {string} text - Claude's response text
 * @returns {Object} Parsed result with score, issues, summary
 */
function parseResult(text) {
  const result = { raw: text, score: null, issues: [], summary: "" };

  // Extract rating
  const ratingMatch = text.match(/Rating:\s*(\d+)\s*\/\s*10/i);
  if (ratingMatch) {
    result.score = parseInt(ratingMatch[1], 10);
  }

  // Extract issues (bullet points after "Issues:")
  const issuesMatch = text.match(/Issues:\s*\n((?:\s*-\s*.+\n?)+)/i);
  if (issuesMatch) {
    result.issues = issuesMatch[1]
      .split("\n")
      .map((line) => line.replace(/^\s*-\s*/, "").trim())
      .filter((line) => line.length > 0);
  }

  // Extract summary
  const summaryMatch = text.match(/Summary:\s*(.+?)(?:\n|$)/i);
  if (summaryMatch) {
    result.summary = summaryMatch[1].trim();
  }

  return result;
}

/**
 * Create an evaluator for Claude vision API calls.
 *
 * @param {Object} options
 * @param {string} options.apiKey - Anthropic API key (default: ANTHROPIC_API_KEY env)
 * @param {string} options.model - Model to use (default: claude-sonnet-4-5-20250929)
 * @param {number} options.maxTokens - Max response tokens (default: 1024)
 * @param {number} options.maxFrames - Max frames to send (default: 20)
 * @returns {Evaluator}
 */
export function createEvaluator(options = {}) {
  const apiKey = options.apiKey ?? process.env.ANTHROPIC_API_KEY;
  const model = options.model ?? "claude-sonnet-4-5-20250929";
  const maxTokens = options.maxTokens ?? 1024;
  const maxFrames = options.maxFrames ?? 20;

  if (!apiKey) {
    throw new Error("ANTHROPIC_API_KEY is required. Set it via env or pass apiKey option.");
  }

  async function callClaude(content) {
    const response = await fetch("https://api.anthropic.com/v1/messages", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "x-api-key": apiKey,
        "anthropic-version": "2023-06-01",
      },
      body: JSON.stringify({
        model,
        max_tokens: maxTokens,
        messages: [{ role: "user", content }],
      }),
    });

    if (!response.ok) {
      const errBody = await response.text();
      throw new Error(`Claude API error (${response.status}): ${errBody}`);
    }

    const data = await response.json();
    const textBlock = data.content?.find((b) => b.type === "text");
    return textBlock?.text ?? "";
  }

  return {
    /**
     * Evaluate a recording with a prompt template or custom prompt.
     *
     * @param {Object} recording - Loaded recording object
     * @param {Object} criteria
     * @param {string} criteria.template - Built-in template name
     * @param {string} criteria.prompt - Custom prompt (overrides template)
     * @returns {Object} Structured evaluation result
     */
    async evaluate(recording, criteria = {}) {
      const prompt =
        criteria.prompt ??
        PROMPT_TEMPLATES[criteria.template] ??
        PROMPT_TEMPLATES.general_quality;

      const dirPath = recording._dirPath;
      if (!dirPath) {
        throw new Error("Recording must be loaded with loadRecording() to have _dirPath");
      }

      const selectedFrames = selectKeyFrames(recording.frames, maxFrames);
      if (selectedFrames.length === 0) {
        throw new Error("No frames found in recording");
      }

      console.log(`[evaluator] Sending ${selectedFrames.length} frames to Claude (${model})...`);
      const content = buildContentBlocks(selectedFrames, dirPath, prompt);
      const responseText = await callClaude(content);
      return parseResult(responseText);
    },

    /**
     * Evaluate specific PNG files directly.
     *
     * @param {string[]} pngPaths - Array of PNG file paths
     * @param {string} prompt - Evaluation prompt
     * @returns {Object} Structured evaluation result
     */
    async evaluateFrames(pngPaths, prompt) {
      const content = [];
      content.push({ type: "text", text: prompt });

      for (const pngPath of pngPaths) {
        if (!fs.existsSync(pngPath)) {
          console.warn(`[evaluator] File not found: ${pngPath}`);
          continue;
        }
        const pngData = fs.readFileSync(pngPath);
        content.push({
          type: "image",
          source: {
            type: "base64",
            media_type: "image/png",
            data: pngData.toString("base64"),
          },
        });
      }

      console.log(`[evaluator] Sending ${pngPaths.length} frames to Claude (${model})...`);
      const responseText = await callClaude(content);
      return parseResult(responseText);
    },
  };
}

/** Export templates for external use. */
export { PROMPT_TEMPLATES };
