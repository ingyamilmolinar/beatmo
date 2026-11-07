/** setupFullWasm() plus recording and video when LLM_RECORD=1; without it, zero overhead. */

import path from "path";
import { fileURLToPath } from "url";
import { setupFullWasm } from "../real_input_test_helpers.js";
import { createRecorder } from "./recorder.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const recordingsDir = path.resolve(__dirname, "../recordings");

/** With LLM_RECORD=1: Playwright video, recorder proxies, saved to src/js/recordings/<name>/ on cleanup. options:
 * setupFullWasm's, plus recordingName (default LLM_RECORD_NAME or a timestamp) and frameIntervalMs (500). */
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
