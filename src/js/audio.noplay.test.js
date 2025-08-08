import { execSync } from 'child_process';
import { readFileSync, rmSync } from 'fs';
import { dirname, join } from 'path';
import { fileURLToPath } from 'url';
import './wasm_exec.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const wasmPath = join(__dirname, 'playtest.wasm');
const goSrc = join(__dirname, '../go');

// Build the tiny harness that calls audio.Play.
execSync(`GOOS=js GOARCH=wasm go build -o ${wasmPath} ./internal/audio/playtest`, {
  stdio: 'inherit',
  cwd: goSrc,
});

const go = new Go();
const bytes = readFileSync(wasmPath);
try {
  const result = await WebAssembly.instantiate(bytes, go.importObject);
  await go.run(result.instance);
  console.log('audio bridge noop test passed');
} catch (e) {
  console.error('audio bridge noop test failed', e);
  rmSync(wasmPath, { force: true });
  process.exit(1);
}

rmSync(wasmPath, { force: true });

