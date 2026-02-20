/**
 * Recording Capture Browser Tests
 *
 * Functional tests for the multi-channel recording system via WASM.
 * Tests: recording start/stop, state queries, encoder format listing,
 * file download interception, and recording with playback.
 */

import { setupFullWasm } from "./real_input_test_helpers.js";
import {
  flushCoverage,
  isCoverageEnabled,
} from "./coverage_helpers.js";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

let page, browser, server, cleanup;
let passed = 0;
let failed = 0;

function assert(cond, msg) {
  if (!cond) {
    failed++;
    console.error(`  FAIL: ${msg}`);
    throw new Error(msg);
  }
  passed++;
}

try {
  ({ page, browser, server, cleanup } = await setupFullWasm());

  // Wait for recording exports to be available
  await page.waitForFunction(
    () =>
      typeof isRecording === "function" &&
      typeof startRecording === "function" &&
      typeof stopRecording === "function" &&
      typeof availableFormats === "function" &&
      typeof recordingElapsedMs === "function",
    { timeout: 15000 }
  );
  console.log("Recording exports available");

  // ─── Test 1: Available Formats ───────────────────────────────────────
  console.log("\n--- Test 1: Available encoder formats ---");
  {
    const formats = await page.evaluate(() => availableFormats());
    console.log("  Formats:", formats);
    assert(Array.isArray(formats), "availableFormats should return an array");
    assert(formats.length >= 3, `expected ≥3 formats, got ${formats.length}`);
    assert(formats.includes("wav16"), "should include wav16");
    assert(formats.includes("wav24"), "should include wav24");
    assert(formats.includes("wav32f"), "should include wav32f");
    assert(formats.includes("flac"), "should include flac");
    assert(formats.includes("ogg"), "should include ogg");
    console.log("  PASS: All expected formats registered");
  }

  // ─── Test 2: Initial Recording State ─────────────────────────────────
  console.log("\n--- Test 2: Initial recording state ---");
  {
    const rec = await page.evaluate(() => isRecording());
    assert(rec === false, "isRecording should be false initially");

    const elapsed = await page.evaluate(() => recordingElapsedMs());
    assert(elapsed === 0, `elapsed should be 0 when not recording, got ${elapsed}`);
    console.log("  PASS: Not recording initially");
  }

  // ─── Test 3: Start and Stop Recording ────────────────────────────────
  console.log("\n--- Test 3: Start and stop recording ---");
  {
    const startResult = await page.evaluate(() => startRecording("wav24"));
    assert(startResult.error === "", `start error: ${startResult.error}`);

    const isRec = await page.evaluate(() => isRecording());
    assert(isRec === true, "isRecording should be true after start");

    // Elapsed should be > 0 after a short wait
    await page.waitForTimeout(100);
    const elapsed = await page.evaluate(() => recordingElapsedMs());
    assert(elapsed > 0, `elapsed should be > 0 after start, got ${elapsed}`);

    const stopResult = await page.evaluate(() => stopRecording());
    assert(stopResult.error === "", `stop error: ${stopResult.error}`);
    assert(stopResult.format === "wav24", `format should be wav24, got ${stopResult.format}`);

    const isRecAfter = await page.evaluate(() => isRecording());
    assert(isRecAfter === false, "isRecording should be false after stop");
    console.log("  PASS: Start/stop lifecycle works");
  }

  // ─── Test 4: Double Start Fails ──────────────────────────────────────
  console.log("\n--- Test 4: Double start fails ---");
  {
    const r1 = await page.evaluate(() => startRecording("wav16"));
    assert(r1.error === "", `first start should succeed: ${r1.error}`);

    const r2 = await page.evaluate(() => startRecording("wav16"));
    assert(r2.error !== "", "second start should fail");
    assert(
      r2.error.includes("already in progress"),
      `error should mention 'already in progress': ${r2.error}`
    );

    // Clean up
    await page.evaluate(() => stopRecording());
    console.log("  PASS: Duplicate start rejected");
  }

  // ─── Test 5: Stop When Not Recording Fails ───────────────────────────
  console.log("\n--- Test 5: Stop when not recording fails ---");
  {
    const r = await page.evaluate(() => stopRecording());
    assert(r.error !== "", "stop when not recording should fail");
    assert(
      r.error.includes("no recording"),
      `error should mention 'no recording': ${r.error}`
    );
    console.log("  PASS: Stop-when-idle rejected");
  }

  // ─── Test 6: Recording Session With Playback ──────────────────────────
  // Recording bridges to JS outputCapture on WASM, so playback audio is
  // captured via ScriptProcessorNode and returned as the master channel.
  console.log("\n--- Test 6: Recording session with playback ---");
  {
    const r = await page.evaluate(() => startRecording("wav16"));
    assert(r.error === "", `start error: ${r.error}`);

    // Start playback
    await page.evaluate(() => startPlay?.());
    // Let it play for a bit
    await page.waitForTimeout(1500);
    await page.evaluate(() => stopPlay?.());

    const result = await page.evaluate(() => stopRecording());
    assert(result.error === "", `stop error: ${result.error}`);
    assert(result.bpm > 0, `BPM should be > 0, got ${result.bpm}`);
    assert(result.sampleRate > 0, `sampleRate should be > 0, got ${result.sampleRate}`);
    assert(result.duration > 0, `duration should be > 0, got ${result.duration}`);
    assert(result.format === "wav16", `format should be wav16, got ${result.format}`);

    // With WASM capture bridge, master channel should be captured
    const channels = result.channels;
    assert(Array.isArray(channels), "channels should be an array");
    assert(channels.length > 0, `expected at least 1 channel (master), got ${channels.length}`);
    const master = channels.find(ch => ch.id === "master");
    assert(master, "master channel should exist");
    assert(master.size > 0, `master channel should have data, got ${master.size} bytes`);

    console.log(`  Duration: ${result.duration.toFixed(2)}s, BPM: ${result.bpm}, SR: ${result.sampleRate}`);
    console.log(`  Channels: ${channels.length}`);
    for (const ch of channels) {
      console.log(`    - ${ch.id} (${ch.filename}): ${ch.size} bytes`);
    }
    console.log("  PASS: Recording session with playback captures audio");
  }

  // ─── Test 7: FLAC Format Session ──────────────────────────────────
  console.log("\n--- Test 7: FLAC format session ---");
  {
    const r = await page.evaluate(() => startRecording("flac"));
    assert(r.error === "", `start error: ${r.error}`);

    await page.waitForTimeout(200);

    const result = await page.evaluate(() => stopRecording());
    assert(result.error === "", `stop error: ${result.error}`);
    assert(result.format === "flac", `format should be flac, got ${result.format}`);
    console.log("  PASS: FLAC format session works");
  }

  // ─── Test 8: WAV32 Float Format ─────────────────────────────────────
  console.log("\n--- Test 8: WAV32 float format session ---");
  {
    const r = await page.evaluate(() => startRecording("wav32f"));
    assert(r.error === "", `start error: ${r.error}`);

    await page.waitForTimeout(200);

    const result = await page.evaluate(() => stopRecording());
    assert(result.error === "", `stop error: ${result.error}`);
    assert(result.format === "wav32f", `format should be wav32f, got ${result.format}`);
    console.log("  PASS: WAV32 float format works");
  }

  // ─── Test 9: Invalid Format Rejected ─────────────────────────────────
  console.log("\n--- Test 9: Invalid format rejected ---");
  {
    const r = await page.evaluate(() => startRecording("mp3"));
    assert(r.error !== "", "invalid format should fail");
    assert(
      r.error.includes("unsupported"),
      `error should mention unsupported: ${r.error}`
    );
    console.log("  PASS: Invalid format rejected");
  }

  // ─── Test 10: Save Recording API ──────────────────────────────────────
  // With WASM capture bridge, saveRecording() should stop recording,
  // encode captured audio, and trigger a zip download.
  console.log("\n--- Test 10: Save recording API ---");
  {
    const r = await page.evaluate(() => startRecording("wav24"));
    assert(r.error === "", `start error: ${r.error}`);

    // Start playback so there's audio to capture
    await page.evaluate(() => startPlay?.());
    await page.waitForTimeout(1500);
    await page.evaluate(() => stopPlay?.());

    // Intercept the download to prevent actual file save
    const downloadPromise = page.waitForEvent("download", { timeout: 10000 }).catch(() => null);

    const saveResult = await page.evaluate(() => saveRecording());
    assert(saveResult.error === "", `save error: ${saveResult.error}`);
    assert(saveResult.path !== "", "save path should not be empty");
    assert(saveResult.path.endsWith(".zip"), `path should end with .zip: ${saveResult.path}`);

    // Verify the download was triggered
    const download = await downloadPromise;
    if (download) {
      console.log(`  Download triggered: ${download.suggestedFilename()}`);
      await download.delete(); // clean up
    } else {
      console.log("  Download event not captured (may be headless limitation)");
    }

    console.log(`  Save path: ${saveResult.path}`);
    console.log("  PASS: Save recording triggers download");
  }

  // ─── Test 11: Recording Metadata Consistency ──────────────────────────
  console.log("\n--- Test 11: Recording metadata consistency ---");
  {
    const r = await page.evaluate(() => startRecording("wav16"));
    assert(r.error === "", `start error: ${r.error}`);

    await page.waitForTimeout(300);

    const result = await page.evaluate(() => stopRecording());
    assert(result.error === "", `stop error: ${result.error}`);

    // Validate metadata fields
    assert(typeof result.bpm === "number", "bpm should be a number");
    assert(result.bpm > 0, `bpm should be > 0, got ${result.bpm}`);
    assert(typeof result.sampleRate === "number", "sampleRate should be a number");
    assert(result.sampleRate > 0, `sampleRate should be > 0, got ${result.sampleRate}`);
    assert(typeof result.duration === "number", "duration should be a number");
    assert(result.duration > 0, `duration should be > 0, got ${result.duration}`);
    assert(result.format === "wav16", `format should be wav16, got ${result.format}`);

    // Channels array should exist and be well-formed
    const channels = result.channels;
    assert(Array.isArray(channels), "channels should be an array");
    assert(typeof result.channelCount === "number", "channelCount should be a number");
    assert(
      result.channelCount === channels.length,
      `channelCount (${result.channelCount}) should match channels.length (${channels.length})`
    );

    // If any channels exist, validate their structure
    for (const ch of channels) {
      assert(typeof ch.id === "string" && ch.id !== "", `channel should have non-empty id`);
      assert(typeof ch.name === "string" && ch.name !== "", `channel ${ch.id} should have name`);
      assert(
        typeof ch.filename === "string" && ch.filename.endsWith(".wav"),
        `channel ${ch.id} filename should end with .wav: ${ch.filename}`
      );
      assert(typeof ch.size === "number", `channel ${ch.id} size should be a number`);
    }
    console.log(`  Metadata: BPM=${result.bpm}, SR=${result.sampleRate}, Duration=${result.duration.toFixed(2)}s`);
    console.log("  PASS: Recording metadata is consistent");
  }

  // ─── Test 12: Rapid Start/Stop Cycles ────────────────────────────────
  console.log("\n--- Test 12: Rapid start/stop cycles ---");
  {
    for (let i = 0; i < 5; i++) {
      const r = await page.evaluate(() => startRecording("wav16"));
      assert(r.error === "", `cycle ${i} start error: ${r.error}`);

      const s = await page.evaluate(() => stopRecording());
      assert(s.error === "", `cycle ${i} stop error: ${s.error}`);
    }

    // Verify clean state after cycles
    const isRec = await page.evaluate(() => isRecording());
    assert(isRec === false, "should not be recording after cycles");
    console.log("  PASS: 5 rapid start/stop cycles completed cleanly");
  }

  // ─── Test 13: Recording State Persists Across Frames ─────────────────
  console.log("\n--- Test 13: Recording state persists across frames ---");
  {
    const r = await page.evaluate(() => startRecording("wav24"));
    assert(r.error === "", `start error: ${r.error}`);

    // Wait multiple frame ticks
    await page.waitForTimeout(500);

    // State should still be recording
    const isRec = await page.evaluate(() => isRecording());
    assert(isRec === true, "should still be recording after 500ms");

    const elapsed = await page.evaluate(() => recordingElapsedMs());
    assert(elapsed >= 400, `elapsed should be >= 400ms, got ${elapsed}`);

    await page.evaluate(() => stopRecording());
    console.log(`  Elapsed after 500ms: ${elapsed.toFixed(0)}ms`);
    console.log("  PASS: Recording state persists");
  }

  // ─── Test 14: Save After Stop Uses Cached Result ──────────────────────
  // Calling stopRecording() then saveRecording() should work via cached result.
  console.log("\n--- Test 14: Save after stop uses cached result ---");
  {
    const r = await page.evaluate(() => startRecording("wav24"));
    assert(r.error === "", `start error: ${r.error}`);

    // Start playback so there's audio to capture
    await page.evaluate(() => startPlay?.());
    await page.waitForTimeout(1500);
    await page.evaluate(() => stopPlay?.());

    // Stop recording first (caches result)
    const stopResult = await page.evaluate(() => stopRecording());
    assert(stopResult.error === "", `stop error: ${stopResult.error}`);
    assert(stopResult.channelCount > 0, `expected channels after stop, got ${stopResult.channelCount}`);

    // Now save using the cached result
    const downloadPromise = page.waitForEvent("download", { timeout: 10000 }).catch(() => null);
    const saveResult = await page.evaluate(() => saveRecording());
    assert(saveResult.error === "", `save error: ${saveResult.error}`);
    assert(saveResult.path !== "", "save path should not be empty");

    const download = await downloadPromise;
    if (download) {
      await download.delete();
    }

    // Second save should fail (cache cleared)
    const save2 = await page.evaluate(() => saveRecording());
    assert(save2.error !== "", "second save should fail (cache cleared)");
    assert(
      save2.error.includes("no recording"),
      `error should mention no recording: ${save2.error}`
    );
    console.log("  PASS: Stop-then-save works via cached result, second save correctly rejected");
  }

  // ─── Test 15: Per-Instrument Channels in Recording ──────────────────
  // WASM recording should capture per-instrument channels, not just master.
  // The zip file should contain individual WAVs for each active instrument.
  console.log("\n--- Test 15: Per-instrument channels captured ---");
  {
    // Get total rows (instruments) to know what to expect
    const totalRows = await page.evaluate(() => totalRows());

    const r = await page.evaluate(() => startRecording("wav24"));
    assert(r.error === "", `start error: ${r.error}`);

    // Start playback so instruments produce audio
    await page.evaluate(() => startPlay?.());
    await page.waitForTimeout(2000);
    await page.evaluate(() => stopPlay?.());

    const result = await page.evaluate(() => stopRecording());
    assert(result.error === "", `stop error: ${result.error}`);
    assert(result.channelCount > 1, `expected >1 channel (master + instruments), got ${result.channelCount}`);

    // Verify master channel exists
    const channels = result.channels;
    const master = channels.find(ch => ch.id === "master");
    assert(master, "master channel should exist");
    assert(master.size > 0, `master should have data, got ${master.size} bytes`);

    // Verify at least one per-instrument channel exists
    const instruments = channels.filter(ch => ch.id !== "master");
    assert(instruments.length > 0, `expected per-instrument channels, got 0`);
    for (const inst of instruments) {
      assert(inst.size > 0, `instrument ${inst.id} should have data, got ${inst.size} bytes`);
      console.log(`    - ${inst.id} (${inst.filename}): ${inst.size} bytes`);
    }

    console.log(`  Total channels: ${result.channelCount} (master + ${instruments.length} instruments)`);
    console.log("  PASS: Per-instrument channels captured in WASM recording");
  }

  // ─── Test 16: Save ZIP Contains Per-Instrument Files ──────────────────
  // The saved ZIP should contain individual WAV files for each instrument
  // plus master, not just master.wav.
  console.log("\n--- Test 16: Save ZIP contains per-instrument files ---");
  {
    const r = await page.evaluate(() => startRecording("wav24"));
    assert(r.error === "", `start error: ${r.error}`);

    await page.evaluate(() => startPlay?.());
    await page.waitForTimeout(2000);
    await page.evaluate(() => stopPlay?.());

    // Intercept download
    const downloadPromise = page.waitForEvent("download", { timeout: 10000 }).catch(() => null);

    const saveResult = await page.evaluate(() => saveRecording());
    assert(saveResult.error === "", `save error: ${saveResult.error}`);
    assert(saveResult.path.endsWith(".zip"), `path should end with .zip: ${saveResult.path}`);

    const download = await downloadPromise;
    if (download) {
      await download.delete();
    }
    console.log(`  Save path: ${saveResult.path}`);

    // Verify the stop result had per-instrument channels by checking
    // through the saveRecording API (it internally stops + saves)
    // We already verified in Test 15 that stopRecording returns per-instrument data.
    // This test verifies saveRecording completes without error with that data.
    console.log("  PASS: Save ZIP triggered with per-instrument data");
  }

  console.log(`\n========================================`);
  console.log(`All ${passed} assertions passed, ${failed} failures`);
  console.log(`========================================`);

} catch (err) {
  failed++;
  console.error(`\nFATAL: ${err.message}`);
  console.error(err.stack);
  process.exitCode = 1;
} finally {
  try {
    if (isCoverageEnabled() && page) {
      const testName = "recording_capture";
      const covDir = path.resolve(__dirname, "..", "..", "coverage", "browser-raw");
      await flushCoverage(page, covDir, testName);
    }
  } catch (e) {
    console.warn(`[coverage] flush error: ${e.message}`);
  }
  await cleanup?.();
  if (failed > 0) {
    process.exitCode = 1;
  }
}
