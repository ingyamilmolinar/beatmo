import init from './drums.js';

let modulePromise = init();
let mod;
let ctx;
const samples = {};

function getCtx() {
  if (!ctx) ctx = new (window.AudioContext || window.webkitAudioContext)();
  return ctx;
}

async function ensureModule() {
  if (!mod) {
    mod = await modulePromise;
  }
  return mod;
}

const RENDER = {
  snare: 'render_snare',
  kick: 'render_kick',
  hihat: 'render_hihat',
  tom: 'render_tom',
  clap: 'render_clap',
};

export async function loadWav(id, url) {
  const res = await fetch(url);
  const arr = await res.arrayBuffer();
  const buf = await getCtx().decodeAudioData(arr);
  samples[id] = buf;
}

export async function playSound(id) {
  const sr = 44100;
  if (RENDER[id]) {
    const m = await ensureModule();
    const sec = id === 'snare' ? 0.25 : id === 'hihat' ? 0.125 : 0.5;
    const frames = Math.floor(sr * sec);
    const ptr = m._malloc(frames * 4);
    if (!ptr) {
      throw new Error('Failed to allocate memory for audio buffer.');
    }
    m.ccall(RENDER[id], null, ['number','number','number'], [ptr, sr, frames]);
    const data = new Float32Array(m.HEAPF32.buffer, ptr, frames).slice();
    m._free(ptr);
    const gain = id === 'snare' ? 0.5 : id === 'kick' ? 1.0 : 0.8;
    for (let i = 0; i < data.length; i++) data[i] *= gain;
    const buffer = getCtx().createBuffer(1, frames, sr);
    buffer.copyToChannel(data, 0);
    const src = getCtx().createBufferSource();
    src.buffer = buffer;
    src.connect(getCtx().destination);
    src.start();
    return;
  }
  const buf = samples[id];
  if (!buf) throw new Error('Unknown sound: ' + id);
  const src = getCtx().createBufferSource();
  src.buffer = buf;
  src.connect(getCtx().destination);
  src.start();
}

// Expose for Go
window.playSound = async (id) => {
  try {
    await playSound(id);
  } catch (err) {
    console.error('Error playing sound:', err);
  }
};

window.loadWav = async (id, url) => {
  try {
    await loadWav(id, url);
  } catch (err) {
    console.error('Error loading wav:', err);
  }
};

