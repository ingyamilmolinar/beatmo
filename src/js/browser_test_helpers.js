import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "..", "..");

export function resolveGoBinary() {
  if (process.env.GO) {
    const envGo = process.env.GO;
    return path.isAbsolute(envGo) ? envGo : path.resolve(process.cwd(), envGo);
  }
  const bundled = path.resolve(repoRoot, ".tools/go/bin/go");
  return fs.existsSync(bundled) ? bundled : "go";
}

export async function clearSchedulerMismatches(page) {
  await page.evaluate(() => {
    if (typeof clearSchedulerMismatches === "function") {
      clearSchedulerMismatches();
    }
  });
}

export async function schedulerMismatches(page) {
  return await page.evaluate(() => {
    if (typeof recentSchedulerMismatches !== "function") return [];
    return recentSchedulerMismatches() ?? [];
  });
}

export async function assertNoSchedulerMismatches(page, label = "scheduler mismatches") {
  const mismatches = await schedulerMismatches(page);
  if (!mismatches || !mismatches.length) return;
  const sample = mismatches.slice(-10);
  throw new Error(`${label}: ${mismatches.length} mismatch(es)\n${JSON.stringify(sample, null, 2)}`);
}

export async function assertSimpleDrawMode(page, expected, label = "simpleDraw") {
  await page.waitForFunction(
    (want) => typeof gridCacheInfo === "function" && gridCacheInfo()?.simpleDraw === want,
    expected
  );
  const info = await page.evaluate(() => (typeof gridCacheInfo === "function" ? gridCacheInfo() : null));
  if (!info || typeof info.simpleDraw !== "boolean") {
    throw new Error(`missing gridCacheInfo.simpleDraw for ${label}`);
  }
  if (info.simpleDraw !== expected) {
    throw new Error(`expected ${label}=${expected} got ${info.simpleDraw}`);
  }
}
