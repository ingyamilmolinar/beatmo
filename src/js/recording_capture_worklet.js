/**
 * Recording Capture AudioWorkletProcessor.
 *
 * Per-channel capture worklet for the WASM recording pipeline. One instance
 * is instantiated per instrument (and one for the master tap). Runs on the
 * audio rendering thread, NOT the main thread — so per-quantum work cannot
 * stall the JS game loop or the Go-WASM goroutine scheduler.
 *
 * Data path (per processor instance) — zero main-thread cost in steady state:
 *
 *   audio thread ──128-frame quantum──▶  copy via Float32Array.set()
 *                                          │
 *                                          ▼
 *                                       batch buffer (BATCH_FRAMES)
 *                                          │
 *                                  full?   │  no  → keep accumulating
 *                                          ▼  yes
 *                       workerPort.postMessage(buf, [buf.buffer])
 *                                          │  ZERO-COPY transferable
 *                                          ▼
 *                                  Web Worker (encoder)  ◀─── direct
 *                                                              (main thread
 *                                                              not in path)
 *
 * The worklet receives a transferred MessagePort (the "worker end" of a
 * MessageChannel created on the main thread) via its built-in this.port.
 * It then posts sample batches directly on that worker port — bypassing
 * the main thread entirely once the channel is wired.
 *
 * Bounded outbox: the worklet keeps a `pending` counter (incremented on
 * post, decremented on `ack` from worker). If pending > maxPending the
 * batch is dropped and a `drop` notice fires on this.port. This mirrors
 * the desktop pipeline's `dropsTotal.Add(1)` discipline.
 *
 * Pass-through: the worklet writes the input to its output verbatim so
 * the channel chain is not silenced by the parallel tap.
 *
 * Communication via MessagePort (this.port — to/from main thread):
 *   Main → Worklet (this.port):
 *     { type: 'configure', channelId, batchFrames?, maxPending?,
 *       workerPort: MessagePort }     // [transfer workerPort]
 *     { type: 'flush' }                // post any partial batch immediately
 *     { type: 'stop' }                 // stop accepting input; post 'done'
 *
 *   Worker → Worklet (workerPort, after configure):
 *     { type: 'ack' }                  // worker consumed a batch
 *
 *   Worklet → Worker (workerPort, hot path):
 *     ArrayBuffer (transferable Float32 samples)
 *     { type: 'done' }                 // sent on stop, after flush
 *     { type: 'drop', n }              // back-pressure signal
 *
 *   Worklet → Main (this.port):
 *     { type: 'configured', channelId }
 *     { type: 'done', samplesCaptured, droppedBatches }
 */

/* eslint-disable no-undef */

const DEFAULT_BATCH_FRAMES = 4096;        // 32 quanta @ 128 frames; ~85 ms @ 48 kHz
const DEFAULT_MAX_PENDING = 8;            // bounded outbox depth per channel

class RecordingCaptureProcessor extends AudioWorkletProcessor {
  constructor(_options) {
    super();
    this._channelId = '';
    this._batchFrames = DEFAULT_BATCH_FRAMES;
    this._maxPending = DEFAULT_MAX_PENDING;

    this._workerPort = null;  // direct port to encoder Worker (set on configure)
    this._batch = null;       // active Float32Array
    this._batchOffset = 0;    // write cursor within _batch
    this._pending = 0;        // batches in flight to worker
    this._drops = 0;          // batches dropped due to backpressure
    this._samplesCaptured = 0;
    this._stopped = false;
    this._stopAcknowledged = false;

    this.port.onmessage = (e) => this._handleMain(e.data);
    this._allocBatch();
  }

  _allocBatch() {
    this._batch = new Float32Array(this._batchFrames);
    this._batchOffset = 0;
  }

  _handleMain(msg) {
    if (!msg || typeof msg !== 'object') return;
    switch (msg.type) {
    case 'configure':
      this._channelId = msg.channelId || this._channelId;
      if (msg.batchFrames && msg.batchFrames > 0) {
        this._batchFrames = msg.batchFrames | 0;
        this._allocBatch();
      }
      if (msg.maxPending && msg.maxPending > 0) {
        this._maxPending = msg.maxPending | 0;
      }
      if (msg.workerPort) {
        this._workerPort = msg.workerPort;
        this._workerPort.onmessage = (e) => this._handleWorker(e.data);
      }
      this.port.postMessage({ type: 'configured', channelId: this._channelId });
      break;
    case 'flush':
      this._postBatch(true);
      break;
    case 'stop':
      this._stopped = true;
      this._postBatch(true);
      this._postWorkerDone();
      this._postMainDone();
      break;
    }
  }

  _handleWorker(msg) {
    if (!msg || typeof msg !== 'object') return;
    if (msg.type === 'ack' && this._pending > 0) this._pending--;
  }

  _postBatch(force) {
    if (this._batchOffset === 0) return;
    if (!this._workerPort) return;
    if (!force && this._pending >= this._maxPending) {
      this._drops++;
      try { this._workerPort.postMessage({ type: 'drop', n: this._batchOffset }); } catch (_) { /* port closed */ }
      this._batchOffset = 0;
      return;
    }
    let out;
    if (this._batchOffset === this._batch.length) {
      out = this._batch;
    } else {
      // Partial batch (only on stop/flush): copy to a right-sized array
      // so the transferable detach doesn't invalidate the unfilled tail.
      out = new Float32Array(this._batch.subarray(0, this._batchOffset));
    }
    this._pending++;
    try {
      this._workerPort.postMessage(out.buffer, [out.buffer]);
    } catch (_) {
      this._pending--;
    }
    this._allocBatch();
  }

  _postWorkerDone() {
    if (!this._workerPort) return;
    try { this._workerPort.postMessage({ type: 'done' }); } catch (_) { /* port closed */ }
  }

  _postMainDone() {
    if (this._stopAcknowledged) return;
    this._stopAcknowledged = true;
    this.port.postMessage({
      type: 'done',
      channelId: this._channelId,
      samplesCaptured: this._samplesCaptured,
      droppedBatches: this._drops,
    });
  }

  process(inputs, outputs, _parameters) {
    const input = inputs[0];
    const output = outputs[0];

    // Pass-through to keep the parallel tap from sinking the signal.
    if (input && input[0] && output && output[0]) {
      output[0].set(input[0]);
    }

    if (this._stopped) {
      return false; // tear down processor
    }

    if (!this._workerPort) {
      // Not yet configured: drop silently (rare — startup race).
      return true;
    }

    if (!input || !input[0] || input[0].length === 0) {
      return true;
    }
    const ch0 = input[0];
    const n = ch0.length;
    this._samplesCaptured += n;

    let consumed = 0;
    while (consumed < n) {
      const space = this._batchFrames - this._batchOffset;
      const take = Math.min(space, n - consumed);
      this._batch.set(ch0.subarray(consumed, consumed + take), this._batchOffset);
      this._batchOffset += take;
      consumed += take;
      if (this._batchOffset >= this._batchFrames) {
        this._postBatch(false);
      }
    }

    return true;
  }
}

registerProcessor('recording-capture-processor', RecordingCaptureProcessor);
