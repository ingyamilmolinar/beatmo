// web/audio.js with procedural synthesis and plugin registry
export const audioCtx = new (window.AudioContext || window.webkitAudioContext)();
const plugins = {};

export function register(id, fn) {
  plugins[id] = fn;
}

export function play(id, when) {
  const fn = plugins[id];
  if (fn) {
    fn(when ?? audioCtx.currentTime);
  }
}

// basic snare using noise burst and envelope
register('snare', (when) => {
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
