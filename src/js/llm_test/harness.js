/**
 * LLM Visual Testing — Test Integration Harness
 *
 * Drop-in wrapper around setupFullWasm() that adds recording + video
 * when LLM_RECORD=1 env var is set.
 */

import path from "path";
import { fileURLToPath } from "url";
import { setupFullWasm } from "../real_input_test_helpers.js";
import { createRecorder } from "./recorder.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const recordingsDir = path.resolve(__dirname, "../recordings");

/**
 * Set up a full WASM test environment with optional recording.
 *
 * When LLM_RECORD=1 is set:
 *   - Enables Playwright video recording (WebM)
 *   - Wraps the page with recorder proxies
 *   - On cleanup, saves recording to src/js/recordings/<name>/
 *
 * When LLM_RECORD is not set, delegates directly to setupFullWasm() — zero overhead.
 *
 * @param {Object} options - All options from setupFullWasm() plus:
 * @param {string} options.recordingName - Recording session name (default: LLM_RECORD_NAME or timestamp)
 * @param {number} options.frameIntervalMs - Screenshot interval when recording (default 500)
 * @returns {Promise<{page, browser, server, cleanup, recorder?}>}
 */
export async function setupWithRecording(options = {}) {
  const shouldRecord = process.env.LLM_RECORD === "1";

  if (!shouldRecord) {
    return setupFullWasm(options);
  }

  const name =
    options.recordingName ??
    process.env.LLM_RECORD_NAME ??
    `session_${Date.now()}`;
  const frameIntervalMs = options.frameIntervalMs ?? 500;

  // Set up with video recording enabled
  const result = await setupFullWasm({
    ...options,
    headless: options.headless ?? false, // Non-headless by default when recording
  });

  const { page, browser, server, cleanup: origCleanup } = result;

  // Create recorder in test mode (proxy wrapping)
  const viewport = page.viewportSize() ?? { width: 1280, height: 720 };
  const recorder = createRecorder(page, {
    mode: "test",
    frameIntervalMs,
    viewport,
  });

  await recorder.start();

  // Get the proxied page
  const wrappedPage = recorder.getWrappedPage();

  // Enhanced cleanup: stop recording, save, then original cleanup
  const savePath = path.join(recordingsDir, name);
  const cleanup = async () => {
    try {
      if (recorder.isRecording()) {
        await recorder.stop();
        const recording = await recorder.save(savePath);
        console.log(
          `[llm_test] Recording saved to ${savePath}/ (${recording.events.length} events, ${recording.frames.length} frames)`
        );
      }
    } catch (e) {
      console.warn(`[llm_test] Failed to save recording: ${e.message}`);
    }
    await origCleanup();
  };

  return {
    page: wrappedPage,
    browser,
    server,
    cleanup,
    recorder,
    port: result.port,
  };
}
