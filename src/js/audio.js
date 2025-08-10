import init from './drums.js';

let modulePromise = init();
let mod;
let ctx;

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

export async function playSound(id) {
  const m = await ensureModule();
  const sr = 44100;
  const sec = id === 'snare' ? 0.25 : 0.5;
  const samples = Math.floor(sr * sec);
  const ptr = m._malloc(samples * 4);
  if (id === 'snare') {
    m.ccall('render_snare', null, ['number','number','number'], [ptr, sr, samples]);
  } else {
    m.ccall('render_kick', null, ['number','number','number'], [ptr, sr, samples]);
  }
  const data = new Float32Array(m.HEAPF32.buffer, ptr, samples).slice();
  m._free(ptr);
  const buffer = getCtx().createBuffer(1, samples, sr);
  buffer.copyToChannel(data, 0);
  const src = getCtx().createBufferSource();
  src.buffer = buffer;
  src.connect(getCtx().destination);
  src.start();
}

// Expose for Go
window.playSound = (id) => { playSound(id); };
