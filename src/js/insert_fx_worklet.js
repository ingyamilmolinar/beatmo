/**
 * Insert Effects AudioWorklet Processor.
 *
 * Processes insert effect chains using Emscripten-compiled C effects
 * in the AudioWorklet thread. This replaces the JS WebAudio node graph
 * implementations, giving exact numerical parity with desktop C effects
 * and making bitcrusher work on WASM.
 *
 * Communication via MessagePort:
 *   Main → Worklet:
 *     { type: 'init', wasmModule: ArrayBuffer }
 *     { type: 'configure', slots: [{type, enabled, params}] }
 *     { type: 'setParam', slotIndex, param, value }
 *
 *   Worklet → Main:
 *     { type: 'ready' }
 */

// Effect struct sizes and helpers — these must match the C struct layouts.
// We allocate flat memory for each effect and use the C _init/_process/_reset
// functions via the WASM module.

/* eslint-disable no-undef */
const _EFFECT_TYPES = [
  'distortion', 'delay', 'reverb', 'chorus', 'bitcrusher', 'filter',
  'waveshaper', 'ringmod', 'tremolo', 'gate', 'limiter',
  'flanger', 'phaser', 'autowah',
  'compressor', 'transient', 'tape', 'pitchshift',
];

class InsertFXProcessor extends AudioWorkletProcessor {
  constructor(_options) {
    super();
    this._mod = null;       // Emscripten module instance
    this._slots = [];       // Array of {type, enabled, params, cPtr, auxPtrs}
    this._inPtr = 0;        // WASM heap pointer for input buffer
    this._outPtr = 0;       // WASM heap pointer for output buffer
    this._bufSize = 128;    // AudioWorklet render quantum
    this._sr = sampleRate || 44100;
    this._ready = false;

    this.port.onmessage = (e) => this._handleMessage(e.data);
  }

  async _handleMessage(msg) {
    switch (msg.type) {
    case 'init':
      await this._initModule(msg);
      break;
    case 'configure':
      this._configure(msg.slots || []);
      break;
    case 'setParam':
      this._setParam(msg.slotIndex, msg.param, msg.value);
      break;
    }
  }

  async _initModule(msg) {
    try {
      // The main thread sends the module factory function source or compiled module.
      // We instantiate the WASM module in this worklet context.
      if (msg.moduleFactory) {
        // msg.moduleFactory is a string containing the module factory code.
         
        const factory = new Function('return ' + msg.moduleFactory)();
        this._mod = await factory({});
      }

      if (this._mod) {
        // Allocate I/O buffers on WASM heap.
        this._inPtr = this._mod._malloc(this._bufSize * 4);  // float32 = 4 bytes
        this._outPtr = this._mod._malloc(this._bufSize * 4);
        this._ready = true;
        this.port.postMessage({ type: 'ready' });
      }
    } catch (err) {
       
      console.error('[InsertFXWorklet] init error:', err);
    }
  }

  _configure(slots) {
    // Free old effect state.
    this._freeSlots();

    const m = this._mod;
    if (!m) return;

    const sr = this._sr;

    for (const slot of slots) {
      const entry = { type: slot.type, enabled: slot.enabled, params: slot.params || {}, cPtr: 0, auxPtrs: [] };

      if (slot.enabled) {
        const p = entry.params;
        switch (slot.type) {
        case 'distortion': {
          // ifx_distortion_t is small, allocate on heap.
          // Size: ~32 bytes, round up to 64 for safety.
          entry.cPtr = m._malloc(64);
          m._ifx_distortion_init(entry.cPtr, sr,
            p.drive ?? 2, p.tone ?? 4000, p.mix ?? 1);
          break;
        }
        case 'delay': {
          entry.cPtr = m._malloc(128);  // struct size
          const bufLen = Math.min(sr, Math.max(1, Math.round((p.time ?? 250) * 0.001 * sr + sr * 0.1)));
          const bufPtr = m._malloc(bufLen * 4);
          entry.auxPtrs.push(bufPtr);
          m._ifx_delay_init(entry.cPtr, sr, bufPtr, bufLen,
            p.time ?? 250, p.feedback ?? 0.4, p.mix ?? 0.3);
          break;
        }
        case 'reverb': {
          entry.cPtr = m._malloc(256);  // struct size (contains sub-structs)
          const memSize = m._ifx_reverb_mem_size(sr);
          const memPtr = m._malloc(memSize * 4);
          entry.auxPtrs.push(memPtr);
          m._ifx_reverb_init(entry.cPtr, sr, memPtr,
            p.room ?? 0.5, p.damping ?? 0.5, p.mix ?? 0.3);
          break;
        }
        case 'chorus': {
          entry.cPtr = m._malloc(128);
          const depthMs = p.depth ?? 5;
          const bufLen = Math.max(64, Math.round(depthMs * 0.001 * sr * 2) + 64);
          const bufPtr = m._malloc(bufLen * 4);
          entry.auxPtrs.push(bufPtr);
          m._ifx_chorus_init(entry.cPtr, sr, bufPtr, bufLen,
            p.rate ?? 1.5, p.depth ?? 5, p.mix ?? 0.5);
          break;
        }
        case 'bitcrusher': {
          entry.cPtr = m._malloc(32);
          m._ifx_bitcrusher_init(entry.cPtr,
            p.bits ?? 8, p.rate ?? 0.5, p.mix ?? 0.5);
          break;
        }
        case 'filter': {
          entry.cPtr = m._malloc(128);
          m._ifx_filter_init(entry.cPtr, sr,
            p.mode ?? 0, p.cutoff ?? 1000, p.q ?? 0.707, p.mix ?? 1);
          break;
        }
        case 'waveshaper': {
          entry.cPtr = m._malloc(32);
          m._ifx_waveshaper_init(entry.cPtr,
            p.curve ?? 0, p.drive ?? 2, p.mix ?? 1);
          break;
        }
        case 'ringmod': {
          entry.cPtr = m._malloc(64);
          m._ifx_ringmod_init(entry.cPtr, sr,
            p.frequency ?? 440, p.shape ?? 0, p.mix ?? 0.5);
          break;
        }
        case 'tremolo': {
          entry.cPtr = m._malloc(64);
          m._ifx_tremolo_init(entry.cPtr, sr,
            p.rate ?? 4, p.depth ?? 0.5, p.shape ?? 0, p.mix ?? 1);
          break;
        }
        case 'gate': {
          entry.cPtr = m._malloc(64);
          m._ifx_gate_init(entry.cPtr, sr,
            p.threshold ?? -30, p.attack ?? 1, p.release ?? 50, p.range ?? -90);
          break;
        }
        case 'limiter': {
          entry.cPtr = m._malloc(64);
          m._ifx_limiter_init(entry.cPtr, sr,
            p.threshold ?? -1, p.release ?? 50, p.ceiling ?? -0.3);
          break;
        }
        case 'flanger': {
          entry.cPtr = m._malloc(128);
          const depthMs = p.depth ?? 3;
          const bufLen = Math.max(64, Math.round(depthMs * 0.001 * sr * 2) + 64);
          const bufPtr = m._malloc(bufLen * 4);
          entry.auxPtrs.push(bufPtr);
          m._ifx_flanger_init(entry.cPtr, sr, bufPtr, bufLen,
            p.rate ?? 0.5, p.depth ?? 3, p.feedback ?? 0.5, p.mix ?? 0.5);
          break;
        }
        case 'phaser': {
          entry.cPtr = m._malloc(256);
          m._ifx_phaser_init(entry.cPtr, sr,
            p.stages ?? 4, p.rate ?? 0.5, p.depth ?? 0.7,
            p.feedback ?? 0.5, p.mix ?? 0.5);
          break;
        }
        case 'autowah': {
          entry.cPtr = m._malloc(128);
          m._ifx_autowah_init(entry.cPtr, sr,
            p.sensitivity ?? 0.5, p.rate ?? 2, p.depth ?? 0.7, p.mix ?? 0.5);
          break;
        }
        case 'compressor': {
          entry.cPtr = m._malloc(128);
          m._ifx_compressor_init(entry.cPtr, sr,
            p.threshold ?? -20, p.ratio ?? 4,
            p.attack ?? 10, p.release ?? 100,
            p.makeup ?? 0, p.mix ?? 1);
          break;
        }
        case 'transient': {
          entry.cPtr = m._malloc(128);
          m._ifx_transient_init(entry.cPtr, sr,
            p.attack ?? 100, p.sustain ?? 100, p.speed ?? 10);
          break;
        }
        case 'tape': {
          entry.cPtr = m._malloc(128);
          const tBufLen = Math.max(64, Math.round(0.01 * sr) + 64);
          const tBufPtr = m._malloc(tBufLen * 4);
          entry.auxPtrs.push(tBufPtr);
          m._ifx_tape_init(entry.cPtr, sr, tBufPtr, tBufLen,
            p.drive ?? 2, p.warmth ?? 0.5,
            p.wow ?? 0, p.flutter ?? 0, p.mix ?? 1);
          break;
        }
        case 'pitchshift': {
          entry.cPtr = m._malloc(128);
          const psBufLen = Math.max(64, Math.round(0.15 * sr) + 64);
          const psBufPtr = m._malloc(psBufLen * 4);
          entry.auxPtrs.push(psBufPtr);
          m._ifx_pitchshift_init(entry.cPtr, sr, psBufPtr, psBufLen,
            p.pitch ?? 0, p.mix ?? 1, p.window ?? 50);
          break;
        }
        }
      }

      this._slots.push(entry);
    }
  }

  _setParam(slotIndex, param, value) {
    if (slotIndex < 0 || slotIndex >= this._slots.length) return;
    const slot = this._slots[slotIndex];
    if (!slot.enabled || !slot.cPtr || !this._mod) return;
    slot.params[param] = value;

    const m = this._mod;
    // Allocate a temporary C string for the param name.
    const nameBytes = new TextEncoder().encode(param + '\0');
    const namePtr = m._malloc(nameBytes.length);
    const heap = new Uint8Array(m.HEAPU8.buffer, namePtr, nameBytes.length);
    heap.set(nameBytes);

    switch (slot.type) {
    case 'distortion':  m._ifx_distortion_set_param(slot.cPtr, namePtr, value); break;
    case 'delay':       m._ifx_delay_set_param(slot.cPtr, namePtr, value); break;
    case 'reverb':      m._ifx_reverb_set_param(slot.cPtr, namePtr, value); break;
    case 'chorus':      m._ifx_chorus_set_param(slot.cPtr, namePtr, value); break;
    case 'bitcrusher':  m._ifx_bitcrusher_set_param(slot.cPtr, namePtr, value); break;
    case 'filter':      m._ifx_filter_set_param(slot.cPtr, namePtr, value); break;
    case 'waveshaper':  m._ifx_waveshaper_set_param(slot.cPtr, namePtr, value); break;
    case 'ringmod':     m._ifx_ringmod_set_param(slot.cPtr, namePtr, value); break;
    case 'tremolo':     m._ifx_tremolo_set_param(slot.cPtr, namePtr, value); break;
    case 'gate':        m._ifx_gate_set_param(slot.cPtr, namePtr, value); break;
    case 'limiter':     m._ifx_limiter_set_param(slot.cPtr, namePtr, value); break;
    case 'flanger':     m._ifx_flanger_set_param(slot.cPtr, namePtr, value); break;
    case 'phaser':      m._ifx_phaser_set_param(slot.cPtr, namePtr, value); break;
    case 'autowah':     m._ifx_autowah_set_param(slot.cPtr, namePtr, value); break;
    case 'compressor':  m._ifx_compressor_set_param(slot.cPtr, namePtr, value); break;
    case 'transient':   m._ifx_transient_set_param(slot.cPtr, namePtr, value); break;
    case 'tape':        m._ifx_tape_set_param(slot.cPtr, namePtr, value); break;
    case 'pitchshift':  m._ifx_pitchshift_set_param(slot.cPtr, namePtr, value); break;
    }

    m._free(namePtr);
  }

  _freeSlots() {
    if (!this._mod) return;
    for (const slot of this._slots) {
      if (slot.cPtr) this._mod._free(slot.cPtr);
      for (const p of slot.auxPtrs) {
        if (p) this._mod._free(p);
      }
    }
    this._slots = [];
  }

  process(inputs, outputs) {
    const input = inputs[0];
    const output = outputs[0];

    if (!input || !input[0] || !output || !output[0]) return true;

    const ch0In = input[0];
    const ch0Out = output[0];
    const n = ch0In.length;

    // If no module or no active effects, passthrough.
    if (!this._ready || !this._mod || this._slots.length === 0) {
      for (let ch = 0; ch < output.length; ch++) {
        if (input[ch]) output[ch].set(input[ch]);
      }
      return true;
    }

    const m = this._mod;
    const heap = m.HEAPF32;
    const inOff = this._inPtr >> 2;   // float32 offset
    const outOff = this._outPtr >> 2;

    // Process channel 0 through the effect chain (mono processing).
    // Copy input to WASM heap.
    heap.set(ch0In, inOff);

    // Ping-pong through effects.
    let srcOff = inOff;
    let dstOff = outOff;

    for (const slot of this._slots) {
      if (!slot.enabled || !slot.cPtr) continue;

      switch (slot.type) {
      case 'distortion':  m._ifx_distortion_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'delay':       m._ifx_delay_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'reverb':      m._ifx_reverb_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'chorus':      m._ifx_chorus_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'bitcrusher':  m._ifx_bitcrusher_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'filter':      m._ifx_filter_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'waveshaper':  m._ifx_waveshaper_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'ringmod':     m._ifx_ringmod_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'tremolo':     m._ifx_tremolo_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'gate':        m._ifx_gate_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'limiter':     m._ifx_limiter_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'flanger':     m._ifx_flanger_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'phaser':      m._ifx_phaser_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'autowah':     m._ifx_autowah_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'compressor':  m._ifx_compressor_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'transient':   m._ifx_transient_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'tape':        m._ifx_tape_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      case 'pitchshift':  m._ifx_pitchshift_process(slot.cPtr, srcOff * 4, dstOff * 4, n); break;
      }

      // Swap src/dst for ping-pong.
      const tmp = srcOff;
      srcOff = dstOff;
      dstOff = tmp;
    }

    // Copy result back from WASM heap.
    ch0Out.set(heap.subarray(srcOff, srcOff + n));

    // Copy mono to all output channels (if stereo output expected).
    for (let ch = 1; ch < output.length; ch++) {
      output[ch].set(ch0Out);
    }

    return true;
  }
}

registerProcessor('insert-fx-processor', InsertFXProcessor);
