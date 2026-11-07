/** Every channel's insert chain and EQ and the master soft clip, on the audio thread, by desktop's C. The module is
 * instantiated ONCE per scope and shared; each node owns its C state. audio.js routes no channel through a node before
 * the 'ready' probe reports, so nothing plays dry. processorOptions: {kind: 'inserts'|'eq'|'softclip'|'ready', init}. */
import Module from "./drums.single.js";
import { createStage } from "./dsp_host.js";

let M = null;
let failure = null;
const waiting = [];
Module({}).then(
  (m) => { M = m; for (const fn of waiting.splice(0)) fn(); },
  (e) => { failure = String((e && e.stack) || e); for (const fn of waiting.splice(0)) fn(); },
);

class BeatmoDSP extends AudioWorkletProcessor {
  constructor(options) {
    super();
    const o = (options && options.processorOptions) || {};
    this.kind = o.kind;
    this.init = o.init || {};
    this.stage = null;
    this.queue = Array.isArray(o.messages) ? o.messages.slice() : [];
    this.alive = true;
    this.zero = new Float32Array(128);
    this.port.onmessage = (e) => this.onMessage(e.data);
    if (this.kind === "ready") {
      const report = () => this.port.postMessage({ ready: !!M, error: failure });
      if (M || failure) report(); else waiting.push(report);
    }
  }

  ensureStage() {
    if (!this.stage && M && this.kind !== "ready") {
      this.stage = createStage(M, this.kind, sampleRate, this.init, () => this.port.postMessage({ quiet: true }));
      if (this.stage) {
        for (const d of this.queue) this.stage.message(d);
        this.queue = [];
        this.reportRefusals();
      }
    }
    return this.stage;
  }

  reportRefusals() {
    const r = this.stage && this.stage.refused();
    if (r && r.length) this.port.postMessage({ refused: r });
  }

  onMessage(d) {
    if (d === "dispose") {
      this.alive = false;
      if (this.stage) this.stage.destroy();
      this.stage = null;
      return;
    }
    const stage = this.ensureStage();
    if (stage) {
      stage.message(d);
      if (d && d.chain) this.reportRefusals();
    } else {
      this.queue.push(d);
    }
  }

  process(inputs, outputs) {
    if (!this.alive) return false;
    const out = outputs[0] && outputs[0][0];
    if (!out) return true;
    let inp = inputs[0] && inputs[0][0];
    if (!inp || inp.length !== out.length) {
      if (this.zero.length !== out.length) this.zero = new Float32Array(out.length);
      inp = this.zero; // a silent input still runs: delay and reverb tails ring on
    }
    const stage = this.ensureStage();
    if (stage) stage.process(inp, out);
    else out.set(inp);
    return true;
  }
}

registerProcessor("beatmo-dsp", BeatmoDSP);
