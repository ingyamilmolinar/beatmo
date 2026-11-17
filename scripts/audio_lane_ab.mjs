#!/usr/bin/env node
// audio_lane_ab.mjs — interleaved A/B/C runner for the audio lane
// (docs/superpowers/specs/2026-10-02-audio-lane-scheduling-horizon-design.md §7).
//
// Arms run in rotation ROUNDS times so slow drift in machine load hits every
// arm alike. Each run records the 1-minute load before and after; runs above
// MAX_LOAD are discarded and retried once. Arms:
//   baseline — the frozen pre-change build ($AB_SCRATCH/baseline_site)
//   legacy   — the new build with the lane OFF (proves the old path unchanged)
//   lane     — the new build with the lane ON
// Output: bench-results/audio_lane_ab_<date>.{json,md}
//
// Usage: AB_SCRATCH=<dir with baseline_site/ and lane.wasm> [ROUNDS=3]
//        [ARMS=baseline,legacy,lane] [SCENARIOS=a,b] [MAX_LOAD=15] [TAG=before]
//        node scripts/audio_lane_ab.mjs
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { systemLoad } from "../src/js/audio_probe_bench_helpers.js";

const repo = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");
const ROUNDS = Number(process.env.ROUNDS ?? 3);
const MAX_LOAD = Number(process.env.MAX_LOAD ?? 15);
const SCEN = process.env.SCENARIOS ?? "idle,stall-150,stall-300,tab-synth,pan-storm,edit-mute,edit-vol";
const scratch = process.env.AB_SCRATCH;
if (!scratch) throw new Error("AB_SCRATCH=<dir with baseline_site/ and lane.wasm> required");
const arms = [
  { name: "baseline", env: {}, extra: { SITE_DIR: path.join(scratch, "baseline_site") } },
  { name: "legacy", env: { BEATMO_AUDIO_LANE: "0" }, extra: { WASM_FILE: path.join(scratch, "lane.wasm") } },
  { name: "lane", env: { BEATMO_AUDIO_LANE: "1" }, extra: { WASM_FILE: path.join(scratch, "lane.wasm") } },
].filter((a) => !process.env.ARMS || process.env.ARMS.split(",").includes(a.name));

const load = systemLoad;
const results = [];
for (let r = 0; r < ROUNDS; r++) {
  for (const arm of arms) {
    for (let attempt = 0; attempt < 2; attempt++) {
      const l0 = load();
      const out = path.join(repo, "bench-results", `audio_isolation_${arm.name}.json`);
      try { fs.unlinkSync(out); } catch (_) {}
      const res = spawnSync("node", ["src/js/webaudio_audio_isolation_bench.browser.test.js"], {
        cwd: repo, stdio: "inherit",
        env: { ...process.env, ...arm.extra, AB_ARM: arm.name,
          ...(process.env.AB_DUMP ? { AUDIO_PROBE_DUMP: path.join(process.env.AB_DUMP, `${arm.name}_r${r}_a${attempt}`) } : {}), ISOLATION_SCENARIOS: SCEN, ISOLATION_ENV: JSON.stringify(arm.env), WASM_PREBUILT: "1" },
      });
      const l1 = load();
      const data = fs.existsSync(out) ? JSON.parse(fs.readFileSync(out, "utf8")) : null;
      const ok = !!data && Math.max(l0, l1) <= MAX_LOAD;
      results.push({ round: r, arm: arm.name, attempt, load: [l0, l1], exit: res.status, discarded: !ok, scenarios: data?.scenarios ?? [] });
      console.log(`[ab] round ${r} arm ${arm.name} attempt ${attempt}: exit ${res.status} load ${l0}→${l1}${ok ? "" : " DISCARDED"}`);
      if (ok) break;
    }
  }
}

const date = new Date().toISOString().slice(0, 10);
const tag = process.env.TAG ? `_${process.env.TAG}` : "";
const base = path.join(repo, "bench-results", `audio_lane_ab${tag}_${date}`);
fs.writeFileSync(`${base}.json`, JSON.stringify(results, null, 2));

const keys = ["probeArrivedLate", "probeLateOver20", "probeLagIdealP99MS", "probeIoiErrP95MS", "probeEditStaleNotes",
  "probeEditStaleMaxMS", "probeNotesMissed", "probeNotesDropped", "probeNotesCancelled", "probeMasterClicks",
  "probeSlips", "updateAvgMS", "drawAvgMS"];
const med = (v) => { const s = v.slice().sort((a, b) => a - b); return s[Math.floor(s.length / 2)]; };
const fmt = (x) => (Number.isInteger(x) ? String(x) : x.toFixed(1));
let md = `# Audio lane A/B${tag ? " (" + process.env.TAG + ")" : ""} — ${date}\n\n` +
  `Rounds ${ROUNDS}, interleaved; runs with load > ${MAX_LOAD} discarded. ` +
  `Cells: median (min–max) over kept runs, n = kept runs.\n\n`;
const kept = (arm) => results.filter((x) => x.arm === arm && !x.discarded);
md += `| arm | kept runs | loads |\n|---|---|---|\n` + arms.map((a) => `| ${a.name} | ${kept(a.name).length} | ${kept(a.name).map((x) => x.load.map((l) => l.toFixed(1)).join("→")).join(", ")} |`).join("\n") + "\n\n";
for (const sc of SCEN.split(",")) {
  md += `## ${sc}\n\n| metric | ${arms.map((a) => a.name).join(" | ")} |\n|---|${arms.map(() => "---").join("|")}|\n`;
  for (const k of keys) {
    const cells = arms.map((a) => {
      const v = kept(a.name).map((x) => x.scenarios.find((s) => s.scenario === sc)?.[k]).filter((x) => Number.isFinite(x));
      return v.length ? `${fmt(med(v))} (${fmt(Math.min(...v))}–${fmt(Math.max(...v))}) n=${v.length}` : "n/a";
    });
    md += `| ${k} | ${cells.join(" | ")} |\n`;
  }
  md += "\n";
}
fs.writeFileSync(`${base}.md`, md);
console.log(md);
console.log(`wrote ${path.relative(repo, base)}.{json,md}`);
