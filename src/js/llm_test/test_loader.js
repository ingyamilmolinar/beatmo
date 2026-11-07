/** .test.md files: YAML frontmatter (name, platform, tags, fixture, max_iterations) and a markdown body that is the
 * agent prompt. */

import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const testsDir = path.resolve(__dirname, "tests");
const fixturesDir = path.resolve(testsDir, "fixtures");

/** A simple parser (strings, numbers, booleans, arrays), so no YAML dependency. */
function parseFrontmatter(content) {
  const match = content.match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n([\s\S]*))?$/);
  if (!match) {
    return { meta: {}, body: content.trim() };
  }

  const yamlBlock = match[1];
  const body = match[2].trim();
  const meta = {};

  for (const line of yamlBlock.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;

    const colonIdx = trimmed.indexOf(":");
    if (colonIdx === -1) continue;

    const key = trimmed.slice(0, colonIdx).trim();
    let value = trimmed.slice(colonIdx + 1).trim();

    // Array: [item1, item2, ...]
    if (value.startsWith("[") && value.endsWith("]")) {
      value = value
        .slice(1, -1)
        .split(",")
        .map((s) => s.trim().replace(/^["']|["']$/g, ""))
        .filter(Boolean);
    }
    // Number
    else if (/^\d+$/.test(value)) {
      value = parseInt(value, 10);
    }
    // Float
    else if (/^\d+\.\d+$/.test(value)) {
      value = parseFloat(value);
    }
    // Boolean
    else if (value === "true") {
      value = true;
    } else if (value === "false") {
      value = false;
    }
    // Null
    else if (value === "null" || value === "~") {
      value = null;
    }
    // Quoted string
    else if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1);
    }

    meta[key] = value;
  }

  return { meta, body };
}

/** Searches tests/desktop/ and tests/mobile/; null if absent. */
export function loadTest(name, platform) {
  // The requested platform first, or a name both share always resolves to desktop.
  const subdirs = platform
    ? [platform, ...["desktop", "mobile"].filter((d) => d !== platform)]
    : ["desktop", "mobile"];

  for (const subdir of subdirs) {
    const filePath = path.join(testsDir, subdir, `${name}.test.md`);
    if (fs.existsSync(filePath)) {
      const content = fs.readFileSync(filePath, "utf-8");
      const { meta, body } = parseFrontmatter(content);

      // Resolve fixture path
      let fixturePath = null;
      if (meta.fixture) {
        fixturePath = path.resolve(fixturesDir, meta.fixture);
        if (!fs.existsSync(fixturePath)) {
          console.warn(`[test_loader] Warning: fixture not found: ${fixturePath}`);
          fixturePath = null;
        }
      }

      return {
        meta: {
          name: meta.name ?? name,
          platform: meta.platform ?? "desktop",
          device: meta.device ?? ((meta.platform ?? "desktop") === "mobile" ? "iPhone 12 landscape" : null),
          tags: meta.tags ?? [],
          fixture: meta.fixture ?? null,
          max_iterations: meta.max_iterations ?? 50,
          viewport: meta.viewport ?? "1280x720",
          ...meta,
        },
        body,
        filePath,
        fixturePath,
      };
    }
  }

  return null;
}

/** filter (glob/substring), tags (any match), platform. */
export function listTests(options = {}) {
  const { filter, tags, platform } = options;
  const results = [];
  const subdirs = ["desktop", "mobile"];

  for (const subdir of subdirs) {
    const dirPath = path.join(testsDir, subdir);
    if (!fs.existsSync(dirPath)) continue;

    const files = fs.readdirSync(dirPath).filter((f) => f.endsWith(".test.md"));

    for (const file of files) {
      const name = file.replace(/\.test\.md$/, "");
      const filePath = path.join(dirPath, file);
      const content = fs.readFileSync(filePath, "utf-8");
      const { meta } = parseFrontmatter(content);

      const testMeta = {
        name: meta.name ?? name,
        platform: meta.platform ?? subdir,
        tags: meta.tags ?? [],
        device: meta.device,
        fixture: meta.fixture,
        max_iterations: meta.max_iterations ?? 50,
        filePath,
      };

      // Apply filters
      if (filter && !testMeta.name.includes(filter)) continue;
      if (platform && testMeta.platform !== platform) continue;
      if (tags && tags.length > 0) {
        const hasMatch = tags.some((t) => testMeta.tags.includes(t));
        if (!hasMatch) continue;
      }

      results.push(testMeta);
    }
  }

  // Sort: desktop first, then by name
  results.sort((a, b) => {
    if (a.platform !== b.platform) return a.platform === "desktop" ? -1 : 1;
    return a.name.localeCompare(b.name);
  });

  return results;
}

/** null if not found. */
export function loadFixture(fixtureName) {
  const fixturePath = path.resolve(fixturesDir, fixtureName);
  if (!fs.existsSync(fixturePath)) return null;
  return fs.readFileSync(fixturePath, "utf-8");
}
