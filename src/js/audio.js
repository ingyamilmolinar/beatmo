// web/audio.js with procedural synthesis and plugin registry

class StubNode {
  connect() {
    return this;
  }
}

class StubBufferSource extends StubNode {
  start() {}
}

class StubFilter extends StubNode {
  constructor() {
    super();
    this.frequency = { setValueAtTime() {} };
  }
}

class StubGain extends StubNode {
  constructor() {
    super();
    this.gain = {
      setValueAtTime() {},
      exponentialRampToValueAtTime() {},
    };
  }
}

class StubAudioContext {
  constructor() {
    this.currentTime = 0;
    this.destination = new StubNode();
    this.sampleRate = 44100;
    this.state = 'suspended';
  }
  createBuffer() {
    return { getChannelData: () => new Float32Array(0) };
  }
  createBufferSource() {
    return new StubBufferSource();
  }
  createBiquadFilter() {
    return new StubFilter();
  }
  createGain() {
    return new StubGain();
  }
  resume() {
    console.log('[audio] stub context resume');
    this.state = 'running';
  }
}

const AC = globalThis.AudioContext ||
  globalThis.webkitAudioContext ||
  StubAudioContext;

export const audioCtx = new AC();
const plugins = {};

export function register(id, fn) {
  console.log("[audio] register", id);
  plugins[id] = fn;
}

export function play(id, when) {
  const fn = plugins[id];
  if (fn) {
    const t = when ?? audioCtx.currentTime;
    if (audioCtx.state === 'suspended') {
      console.log('[audio] resuming context');
      audioCtx.resume();
    }
    console.log("[audio] play", id, "at", t);
    fn(t);
  } else {
    console.warn("[audio] no plugin for", id);
  }
}

// basic snare using noise burst and envelope
register('snare', (when) => {
  console.log("[audio] snare callback at", when);
  const duration = 0.2;
  const buffer = audioCtx.createBuffer(1, audioCtx.sampleRate * duration, audioCtx.sampleRate);
  const data = buffer.getChannelData(0);
  for (let i = 0; i < data.length; i++) {
    data[i] = Math.random() * 2 - 1;
  }
  const noise = audioCtx.createBufferSource();
  noise.buffer = buffer;

  const filter = audioCtx.createBiquadFilter();
  filter.type = 'highpass';
  filter.frequency.setValueAtTime(1000, when);

  const envelope = audioCtx.createGain();
  envelope.gain.setValueAtTime(1, when);
  envelope.gain.exponentialRampToValueAtTime(0.01, when + duration);

  noise.connect(filter).connect(envelope).connect(audioCtx.destination);
  noise.start(when);
});

globalThis.play = play;
globalThis.register = register;
