/** One per instrument plus the master, on the audio thread: 128-frame quanta batch into a buffer that is TRANSFERRED to
 * the encoder worker over its own port, so the main thread is out of the path; a bounded outbox drops on overflow. Its
 * output stays silent: the tapped source already reaches the speakers, and passing it through would add +6 dB. */

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
      // Copied to a right-sized array so the transfer does not invalidate the unfilled tail.
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

  process(inputs, _outputs, _parameters) {
    const input = inputs[0];

    // Never write the outputs: a pass-through would double the signal at the speakers.

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
