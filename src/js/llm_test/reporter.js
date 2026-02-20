/**
 * LLM Visual Testing — HTML Report Generator
 *
 * Generates a self-contained HTML report from an agent session, including:
 * - Summary (task, duration, actions, cost, issues)
 * - Annotated screenshot timeline filmstrip
 * - Issues list from Claude's observations
 * - Embedded video link
 * - Collapsible raw agent log
 */

import fs from "fs";
import path from "path";

/**
 * Generate an HTML report from a recording directory that contains
 * agent session data (recording.json + agent_log.json + frames).
 *
 * @param {string} dirPath - Path to the recording directory
 * @param {string} outputPath - Where to write the HTML file
 */
export function generateReport(dirPath, outputPath) {
  const recordingPath = path.join(dirPath, "recording.json");
  const agentLogPath = path.join(dirPath, "agent_log.json");

  if (!fs.existsSync(recordingPath)) {
    throw new Error(`Recording not found: ${recordingPath}`);
  }

  const recording = JSON.parse(fs.readFileSync(recordingPath, "utf-8"));
  let agentLog = null;
  if (fs.existsSync(agentLogPath)) {
    agentLog = JSON.parse(fs.readFileSync(agentLogPath, "utf-8"));
  }

  // Collect frame thumbnails as base64 (limit to ~30 key frames to keep file size reasonable)
  const frames = recording.frames ?? [];
  const selectedFrames = selectReportFrames(frames, 30);
  const frameThumbnails = [];
  for (const frame of selectedFrames) {
    const pngPath = path.join(dirPath, frame.file);
    if (fs.existsSync(pngPath)) {
      const data = fs.readFileSync(pngPath);
      frameThumbnails.push({
        t: frame.t,
        state: frame.state,
        stateChanged: frame.stateChanged,
        b64: data.toString("base64"),
        file: frame.file,
      });
    }
  }

  // Check for video
  const videoPath = path.join(dirPath, "video.webm");
  const hasVideo = fs.existsSync(videoPath);

  // Extract info from agent log
  const agentMeta = extractAgentMeta(agentLog);
  const issues = agentMeta.issues;
  const summary = agentMeta.summary;

  const html = buildHTML({
    name: recording.name ?? path.basename(dirPath),
    createdAt: recording.createdAt,
    durationMs: recording.durationMs,
    viewport: recording.viewport,
    mode: recording.mode,
    eventCount: (recording.events ?? []).length,
    frameCount: frames.length,
    frameThumbnails,
    hasVideo,
    videoFile: hasVideo ? "video.webm" : null,
    agentMeta,
    issues,
    summary,
    agentLog,
  });

  fs.mkdirSync(path.dirname(outputPath), { recursive: true });
  fs.writeFileSync(outputPath, html);
  return outputPath;
}

/**
 * Select frames for the report. Similar to evaluator's selectKeyFrames
 * but tuned for report display.
 */
function selectReportFrames(frames, maxFrames) {
  if (frames.length <= maxFrames) return [...frames];

  const selected = new Set();
  selected.add(0);
  selected.add(frames.length - 1);

  // State-changed frames
  for (let i = 0; i < frames.length; i++) {
    if (frames[i].stateChanged) selected.add(i);
  }

  // Fill with evenly-spaced
  const remaining = maxFrames - selected.size;
  if (remaining > 0) {
    const step = frames.length / (remaining + 1);
    for (let i = 1; i <= remaining; i++) {
      selected.add(Math.min(Math.round(step * i), frames.length - 1));
    }
  }

  return [...selected]
    .sort((a, b) => a - b)
    .slice(0, maxFrames)
    .map((i) => frames[i]);
}

/**
 * Parse a raw issue string into a structured object.
 * Handles:
 *   "ISSUE: P1 Bug - description"
 *   "Bug: description"
 *   raw string (fallback)
 *
 * @param {string} raw
 * @returns {{ priority: string, type: string, description: string }}
 */
function parseStructuredIssue(raw) {
  // Structured format: "ISSUE: P1 Bug - description"
  const structuredMatch = raw.match(/^ISSUE:\s*(P[0-3])\s+(Bug|Glitch|Suggestion)\s*[-–—]\s*(.+)/i);
  if (structuredMatch) {
    return {
      priority: structuredMatch[1].toUpperCase(),
      type: structuredMatch[2].charAt(0).toUpperCase() + structuredMatch[2].slice(1).toLowerCase(),
      description: structuredMatch[3].trim(),
    };
  }

  // Paragraph form: "Bug: description" or "Glitch: description" or "Suggestion: description"
  const prefixMatch = raw.match(/^(Bug|Glitch|Suggestion)\s*:\s*(.+)/i);
  if (prefixMatch) {
    const type = prefixMatch[1].charAt(0).toUpperCase() + prefixMatch[1].slice(1).toLowerCase();
    const defaultPriority = type === "Bug" ? "P1" : type === "Glitch" ? "P2" : "P3";
    return {
      priority: defaultPriority,
      type,
      description: prefixMatch[2].trim(),
    };
  }

  // Fallback: raw string as P2 Bug
  return { priority: "P2", type: "Bug", description: raw };
}

/**
 * Extract phase results from the agent summary text.
 * Parses lines like: "- Phase 1: PASS — all controls visible"
 *
 * @param {string} summary
 * @returns {Array<{ phase: number, result: string, note: string }>}
 */
function extractPhaseResults(summary) {
  if (!summary) return [];
  const results = [];
  const lines = summary.split("\n");
  for (const line of lines) {
    const match = line.match(/Phase\s+(\d+)\s*:\s*(PASS|PARTIAL|FAIL)\s*[-–—]?\s*(.*)/i);
    if (match) {
      results.push({
        phase: parseInt(match[1]),
        result: match[2].toUpperCase(),
        note: match[3].trim(),
      });
    }
  }
  return results;
}

/**
 * Extract metadata from agent_log.json.
 */
function extractAgentMeta(agentLog) {
  if (!agentLog) {
    return { task: "", model: "", iterations: 0, cost: 0, issues: [], structuredIssues: [], phaseResults: [], summary: "", error: "", actions: [] };
  }

  // Extract actions from the conversation log entries
  const actions = extractAgentActions(agentLog.agentLog ?? []);

  const rawIssues = agentLog.issues ?? [];
  const structuredIssues = rawIssues.map(parseStructuredIssue);
  // Sort by priority (P0 first)
  structuredIssues.sort((a, b) => a.priority.localeCompare(b.priority));

  const summary = agentLog.summary ?? "";
  const phaseResults = extractPhaseResults(summary);

  return {
    task: agentLog.task ?? "",
    model: agentLog.model ?? "",
    iterations: agentLog.iterations ?? 0,
    inputTokens: agentLog.inputTokens ?? 0,
    outputTokens: agentLog.outputTokens ?? 0,
    cost: agentLog.estimatedCost ?? 0,
    issues: rawIssues,
    structuredIssues,
    phaseResults,
    summary,
    error: agentLog.error ?? "",
    actions,
  };
}

/**
 * Extract a timeline of agent actions from the conversation log.
 * Each entry in agentLog has { iteration, t, response: { content: [...] } }
 */
function extractAgentActions(logEntries) {
  const actions = [];
  for (const entry of logEntries) {
    if (entry.error || entry.actionError) {
      actions.push({
        iteration: entry.iteration,
        t: entry.t,
        type: "error",
        detail: entry.error ?? entry.actionError,
      });
      continue;
    }
    if (!entry.response?.content) continue;
    for (const block of entry.response.content) {
      if (block.type === "text" && block.text) {
        // Truncate long text to first 300 chars
        actions.push({
          iteration: entry.iteration,
          t: entry.t,
          type: "observation",
          detail: block.text.length > 300 ? block.text.slice(0, 300) + "..." : block.text,
        });
      }
      if (block.type === "tool_use") {
        const input = block.input ?? {};
        let detail = input.action ?? block.name ?? "unknown";
        if (input.coordinate) detail += ` (${input.coordinate[0]}, ${input.coordinate[1]})`;
        if (input.text) detail += ` "${input.text}"`;
        if (input.key) detail += ` "${input.key}"`;
        if (input.scroll_direction) detail += ` ${input.scroll_direction}`;
        actions.push({
          iteration: entry.iteration,
          t: entry.t,
          type: "action",
          detail,
        });
      }
    }
  }
  return actions;
}

function escapeHtml(str) {
  return String(str)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;");
}

function formatMs(ms) {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  const min = Math.floor(ms / 60000);
  const sec = Math.round((ms % 60000) / 1000);
  return `${min}m ${sec}s`;
}

/**
 * Build the complete HTML string.
 */
function buildHTML(data) {
  const {
    name, createdAt, durationMs, viewport, mode, eventCount, frameCount,
    frameThumbnails, hasVideo, videoFile, agentMeta, issues, summary, agentLog,
  } = data;

  const isAgent = mode === "agent";
  const title = `Beatmo ${isAgent ? "Agent" : ""} Report: ${escapeHtml(name)}`;

  // Build timeline filmstrip
  let filmstripHTML = "";
  for (const frame of frameThumbnails) {
    const stateLabel = frame.state
      ? `${frame.state.playing ? "Playing" : "Stopped"} | ${frame.state.bpm} BPM | ${frame.state.totalRows} rows`
      : "";
    const changedClass = frame.stateChanged ? "state-changed" : "";
    filmstripHTML += `
      <div class="frame ${changedClass}">
        <div class="frame-time">${formatMs(frame.t)}</div>
        <img src="data:image/png;base64,${frame.b64}" alt="${frame.file}" loading="lazy" />
        <div class="frame-state">${escapeHtml(stateLabel)}</div>
        ${frame.stateChanged ? '<div class="frame-badge">STATE CHANGED</div>' : ""}
      </div>`;
  }

  // Build phase results table
  const phaseResults = agentMeta.phaseResults ?? [];
  let phaseHTML = "";
  if (phaseResults.length > 0) {
    const phaseRows = phaseResults.map((p) => {
      const badgeClass = `phase-badge-${p.result.toLowerCase()}`;
      return `<tr><td>Phase ${p.phase}</td><td><span class="phase-badge ${badgeClass}">${p.result}</span></td><td>${escapeHtml(p.note)}</td></tr>`;
    }).join("");
    phaseHTML = `<table class="phase-table"><thead><tr><th>Phase</th><th>Result</th><th>Notes</th></tr></thead><tbody>${phaseRows}</tbody></table>`;
  }

  // Build structured issues table
  const structuredIssues = agentMeta.structuredIssues ?? [];
  const bugs = structuredIssues.filter((i) => i.type !== "Suggestion");
  const suggestions = structuredIssues.filter((i) => i.type === "Suggestion");

  let issuesHTML = "";
  if (bugs.length > 0) {
    const issueRows = bugs.map((i) => {
      const prioClass = `priority-${i.priority.toLowerCase()}`;
      return `<tr><td><span class="priority-badge ${prioClass}">${i.priority}</span></td><td>${escapeHtml(i.type)}</td><td>${escapeHtml(i.description)}</td></tr>`;
    }).join("");
    issuesHTML = `<table class="issues-table"><thead><tr><th>Priority</th><th>Type</th><th>Description</th></tr></thead><tbody>${issueRows}</tbody></table>`;
  } else if (issues.length > 0) {
    // Fallback to raw list if no structured issues parsed
    issuesHTML = "<ul>" + issues.map((i) => `<li>${escapeHtml(typeof i === "string" ? i : i.description ?? "")}</li>`).join("") + "</ul>";
  } else {
    issuesHTML = "<p>No issues detected.</p>";
  }

  let suggestionsHTML = "";
  if (suggestions.length > 0) {
    suggestionsHTML = `<ul class="suggestions-list">` + suggestions.map((s) => `<li>${escapeHtml(s.description)}</li>`).join("") + "</ul>";
  }

  // Build agent log section
  let logHTML = "";
  if (agentLog) {
    const logJson = JSON.stringify(agentLog, null, 2);
    logHTML = `
      <details class="log-section">
        <summary>Raw Agent Log (${(agentLog.agentLog ?? []).length} entries)</summary>
        <pre>${escapeHtml(logJson)}</pre>
      </details>`;
  }

  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>${title}</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #1a1a2e; color: #e0e0e0; padding: 20px; line-height: 1.6; }
  h1 { color: #e94560; margin-bottom: 8px; font-size: 24px; }
  h2 { color: #0f3460; background: #e94560; display: inline-block; padding: 4px 12px; border-radius: 4px; margin: 20px 0 12px; font-size: 16px; }
  .meta { display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px; margin: 16px 0; }
  .meta-card { background: #16213e; border: 1px solid #0f3460; border-radius: 8px; padding: 12px; }
  .meta-card .label { color: #888; font-size: 12px; text-transform: uppercase; letter-spacing: 1px; }
  .meta-card .value { font-size: 20px; font-weight: bold; color: #e94560; }
  .summary-text { background: #16213e; border-left: 4px solid #e94560; padding: 12px 16px; margin: 12px 0; border-radius: 0 8px 8px 0; white-space: pre-wrap; }
  .filmstrip { display: flex; gap: 12px; overflow-x: auto; padding: 12px 0; }
  .frame { flex-shrink: 0; width: 280px; background: #16213e; border: 1px solid #0f3460; border-radius: 8px; overflow: hidden; position: relative; }
  .frame.state-changed { border-color: #e94560; }
  .frame img { width: 100%; height: auto; display: block; }
  .frame-time { padding: 4px 8px; font-size: 11px; color: #888; background: #0a0a1a; }
  .frame-state { padding: 4px 8px; font-size: 10px; color: #aaa; }
  .frame-badge { position: absolute; top: 24px; right: 4px; background: #e94560; color: #fff; font-size: 9px; padding: 2px 6px; border-radius: 3px; font-weight: bold; }
  .issues ul { list-style: none; padding: 0; }
  .issues li { background: #16213e; border-left: 3px solid #e94560; padding: 8px 12px; margin: 6px 0; border-radius: 0 6px 6px 0; }
  .issues p { color: #888; }
  .video-link { display: inline-block; background: #e94560; color: #fff; padding: 8px 16px; border-radius: 6px; text-decoration: none; margin: 8px 0; }
  .video-link:hover { background: #c73651; }
  .log-section { margin: 16px 0; }
  .log-section summary { cursor: pointer; color: #e94560; font-weight: bold; padding: 8px 0; }
  .log-section pre { background: #0a0a1a; padding: 12px; border-radius: 8px; overflow-x: auto; font-size: 11px; max-height: 500px; overflow-y: auto; color: #aaa; }
  .task-prompt { background: #0f3460; padding: 12px 16px; border-radius: 8px; margin: 12px 0; font-family: monospace; white-space: pre-wrap; max-height: 300px; overflow-y: auto; }
  .error-banner { background: #3d0000; border: 2px solid #e94560; border-radius: 8px; padding: 16px; margin: 16px 0; }
  .error-banner .error-title { color: #e94560; font-weight: bold; font-size: 16px; margin-bottom: 8px; }
  .error-banner .error-detail { color: #ccc; font-family: monospace; font-size: 12px; white-space: pre-wrap; }
  .action-timeline { margin: 12px 0; }
  .action-item { display: flex; gap: 12px; padding: 6px 12px; border-left: 3px solid #0f3460; margin: 4px 0; font-size: 12px; }
  .action-item.action-type-action { border-left-color: #4CAF50; }
  .action-item.action-type-observation { border-left-color: #2196F3; }
  .action-item.action-type-error { border-left-color: #e94560; }
  .action-meta { color: #888; min-width: 80px; flex-shrink: 0; }
  .action-badge { display: inline-block; font-size: 10px; padding: 1px 6px; border-radius: 3px; font-weight: bold; text-transform: uppercase; }
  .action-badge-action { background: #1b5e20; color: #81C784; }
  .action-badge-observation { background: #0d47a1; color: #64B5F6; }
  .action-badge-error { background: #b71c1c; color: #ef9a9a; }
  .action-detail { color: #ddd; word-break: break-word; }
  .phase-table, .issues-table { width: 100%; border-collapse: collapse; margin: 12px 0; }
  .phase-table th, .issues-table th { text-align: left; padding: 8px 12px; background: #0a0a1a; color: #888; font-size: 11px; text-transform: uppercase; letter-spacing: 1px; }
  .phase-table td, .issues-table td { padding: 8px 12px; border-bottom: 1px solid #0f3460; }
  .phase-badge { display: inline-block; font-size: 11px; padding: 2px 8px; border-radius: 3px; font-weight: bold; text-transform: uppercase; }
  .phase-badge-pass { background: #1b5e20; color: #81C784; }
  .phase-badge-partial { background: #e65100; color: #FFB74D; }
  .phase-badge-fail { background: #b71c1c; color: #ef9a9a; }
  .priority-badge { display: inline-block; font-size: 11px; padding: 2px 8px; border-radius: 3px; font-weight: bold; }
  .priority-p0 { background: #b71c1c; color: #fff; }
  .priority-p1 { background: #e65100; color: #fff; }
  .priority-p2 { background: #f9a825; color: #000; }
  .priority-p3 { background: #424242; color: #bbb; }
  .suggestions-list { list-style: none; padding: 0; }
  .suggestions-list li { background: #16213e; border-left: 3px solid #4CAF50; padding: 8px 12px; margin: 6px 0; border-radius: 0 6px 6px 0; }
</style>
</head>
<body>

<h1>${title}</h1>
<p style="color: #888; font-size: 13px;">${escapeHtml(createdAt ?? "")}</p>

${agentMeta.error ? `
<div class="error-banner">
  <div class="error-title">Agent Error</div>
  <div class="error-detail">${escapeHtml(agentMeta.error)}</div>
</div>` : ""}

${isAgent && agentMeta.task ? `<h2>Task</h2><div class="task-prompt">${escapeHtml(agentMeta.task)}</div>` : ""}

<h2>Summary</h2>
<div class="meta">
  <div class="meta-card"><div class="label">Duration</div><div class="value">${formatMs(durationMs ?? 0)}</div></div>
  <div class="meta-card"><div class="label">Viewport</div><div class="value">${viewport ? `${viewport.width}x${viewport.height}` : "?"}</div></div>
  <div class="meta-card"><div class="label">Events</div><div class="value">${eventCount}</div></div>
  <div class="meta-card"><div class="label">Frames</div><div class="value">${frameCount}</div></div>
  ${isAgent ? `
  <div class="meta-card"><div class="label">Agent Iterations</div><div class="value">${agentMeta.iterations}</div></div>
  <div class="meta-card"><div class="label">Model</div><div class="value">${escapeHtml(agentMeta.model)}</div></div>
  <div class="meta-card"><div class="label">Tokens (in/out)</div><div class="value">${agentMeta.inputTokens}/${agentMeta.outputTokens}</div></div>
  <div class="meta-card"><div class="label">Est. Cost</div><div class="value">$${(agentMeta.cost ?? 0).toFixed(4)}</div></div>
  ` : ""}
</div>

${summary ? `<div class="summary-text">${escapeHtml(summary)}</div>` : ""}

${phaseHTML ? `<h2>Phase Results</h2>\n<div class="phases">\n  ${phaseHTML}\n</div>` : ""}

<h2>Issues Found (${bugs.length})</h2>
<div class="issues">
  ${issuesHTML}
</div>

${suggestions.length > 0 ? `<h2>UI Suggestions (${suggestions.length})</h2>\n<div class="suggestions">\n  ${suggestionsHTML}\n</div>` : ""}

${agentMeta.actions?.length > 0 ? `<h2>Agent Actions (${agentMeta.actions.length})</h2>
<details class="log-section" open>
  <summary>Action Timeline</summary>
  <div class="action-timeline">
    ${agentMeta.actions.map((a) => `
    <div class="action-item action-type-${a.type}">
      <div class="action-meta">${formatMs(a.t)} <span class="action-badge action-badge-${a.type}">${a.type}</span></div>
      <div class="action-detail">${escapeHtml(a.detail)}</div>
    </div>`).join("")}
  </div>
</details>` : ""}

${hasVideo ? `<h2>Video</h2><a class="video-link" href="${videoFile}">Watch Recording (WebM)</a>` : ""}

<h2>Timeline</h2>
<div class="filmstrip">
  ${filmstripHTML}
</div>

${logHTML}

<p style="color:#555; margin-top:30px; font-size:11px;">Generated by Beatmo LLM Visual Testing Framework</p>
</body>
</html>`;
}
