import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { createRequire } from "module";

const jsDirForRequire = path.join(path.dirname(fileURLToPath(import.meta.url)), "../src/js");
const jsRequire = createRequire(path.join(jsDirForRequire, "package.json"));
const { chromium } = jsRequire("playwright");

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "..");
const jsDir = path.join(repoRoot, "src/js");
const goDir = path.join(repoRoot, "src/go");
const GO = process.env.GO || path.join(repoRoot, ".tools/go/bin/go");

const build = spawnSync(
  GO,
  [
    "build",
    "-ldflags",
    "-X main.defaultLog=INFO",
    "-o",
    path.join(jsDir, "main.wasm"),
    "./cmd/...",
  ],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");

const port = 8770 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((resolve) => server.listen(port, resolve));

const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage();
const client = await page.context().newCDPSession(page);

const traceDir = path.join(repoRoot, "trace");
if (!fs.existsSync(traceDir)) fs.mkdirSync(traceDir);
const tracePath = path.join(traceDir, "perf_e2e_trace.json");

const categories = [
  "devtools.timeline",
  "disabled-by-default-devtools.timeline",
  "disabled-by-default-devtools.timeline.frame",
  "disabled-by-default-devtools.timeline.stack",
];

await client.send("Tracing.start", {
  transferMode: "ReturnAsStream",
  traceConfig: {
    recordMode: "recordContinuously",
    includedCategories: categories,
  },
});

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function");
await page.evaluate(() => {
  if (typeof setSimpleDraw === "function") setSimpleDraw(true);
});
await page.evaluate(() => {
  buildPerfRect(4, 1);
  setBPM(200);
  startPlay();
});
await page.waitForTimeout(5000);

const events = [];
client.on("Tracing.dataCollected", ({ value }) => {
  events.push(...value);
});

const traceComplete = new Promise(async (resolve) => {
  client.once("Tracing.tracingComplete", async ({ stream }) => {
    const chunks = [];
    while (true) {
      const { data, eof } = await client.send("IO.read", { handle: stream });
      chunks.push(data);
      if (eof) break;
    }
    await client.send("IO.close", { handle: stream });
    resolve(chunks.join(""));
  });
});

await client.send("Tracing.end");
const traceData = await traceComplete;

await browser.close();
server.close();

const finalTrace = JSON.parse(traceData);
const traceEvents = events.concat(finalTrace.traceEvents || []);
fs.writeFileSync(tracePath, JSON.stringify({ ...finalTrace, traceEvents }, null, 2));

const totals = new Map();
for (const evt of traceEvents) {
  if (!evt || !evt.dur || evt.ph !== "X") continue;
  const key = `${evt.cat}|${evt.name}`;
  totals.set(key, (totals.get(key) || 0) + evt.dur);
}
const entries = Array.from(totals.entries())
  .map(([key, dur]) => ({ key, ms: dur / 1000 }))
  .sort((a, b) => b.ms - a.ms)
  .slice(0, 15);

console.log("Top trace events by total duration (ms):");
for (const { key, ms } of entries) {
  const [cat, name] = key.split("|");
  console.log(`${ms.toFixed(2)} ms\t${cat}\t${name}`);
}

console.log(`Trace written to ${tracePath}`);
