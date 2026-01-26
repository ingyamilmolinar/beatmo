import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build WASM target used by the in-browser UI.
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");

// Minimal static server.
const port = 8405 + Math.floor(Math.random() * 200);
const server = http.createServer((req, res) => { const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
await page.goto(`http://localhost:${port}/`);

await page.waitForFunction(() =>
  typeof widgetLayoutSnapshot === "function" &&
  typeof forceDraw === "function" &&
  typeof nudgeWidgetSplit === "function"
);
await assertSimpleDrawMode(page, false, "widget layout");

// Helper to fetch the latest snapshot after a draw.
async function snapshot() { return await page.evaluate(() => { forceDraw?.();
    return widgetLayoutSnapshot?.();
  });
}

const snapDefault = await snapshot();
if (!snapDefault?.rack || !snapDefault?.addButton) { throw new Error("missing widget layout snapshot");
}
const insideRack =
  snapDefault.addButton.x >= snapDefault.rack.x &&
  snapDefault.addButton.x + snapDefault.addButton.w <= snapDefault.rack.x + snapDefault.rack.w &&
  snapDefault.addButton.y >= snapDefault.rack.y &&
  snapDefault.addButton.y + snapDefault.addButton.h <= snapDefault.rack.y + snapDefault.rack.h;
if (!insideRack) { throw new Error(`add button not inside rack: add=${JSON.stringify(snapDefault.addButton)} rack=${JSON.stringify(snapDefault.rack)}`);
}
if (!(snapDefault.timeline.x >= snapDefault.rack.x + snapDefault.rack.w - 4)) { throw new Error("timeline should sit to the right of rack in default layout");
}
if (!(snapDefault.wave.y >= snapDefault.timeline.y)) { throw new Error("wave widget should live below/at timeline header");
}

// Resize the column split and ensure rack grows.
const resized = await page.evaluate(() => { const before = widgetLayoutSnapshot();
  nudgeWidgetSplit("col", 0, 40);
  forceDraw?.();
  const after = widgetLayoutSnapshot();
  return { beforeRackW: before.rack.w,
    afterRackW: after.rack.w,
    beforeTimelineW: before.timeline.w,
    afterTimelineW: after.timeline.w,
  };
});
if (!(resized.afterRackW > resized.beforeRackW)) { throw new Error(`expected rack to widen after resize; before=${resized.beforeRackW} after=${resized.afterRackW}`);
}
if (!(resized.afterTimelineW < resized.beforeTimelineW)) { throw new Error(`timeline width not reduced after resize; before=${resized.beforeTimelineW} after=${resized.afterTimelineW}`);
}

// Compact viewport (tablet/phone) still keeps add button inside rack.
await page.setViewportSize({ width: 720, height: 520 });
const snapCompact = await snapshot();
const insideRackCompact =
  snapCompact.addButton.x >= snapCompact.rack.x &&
  snapCompact.addButton.x + snapCompact.addButton.w <= snapCompact.rack.x + snapCompact.rack.w;
if (!insideRackCompact) { throw new Error("add button escaped rack after viewport shrink");
}

await browser.close();
server.close();
