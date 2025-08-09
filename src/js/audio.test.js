import { register, play, audioCtx, setBPM } from './audio.js';

let called = false;
let resumed = false;
audioCtx.resume = () => { resumed = true; audioCtx.state = 'running'; };
register('test', (when) => {
  console.log('[audio.test] callback invoked at', when);
  called = true;
});

await play('test');

if (!called || !resumed) {
  throw new Error('callback or resume not invoked');
}

console.log('audio plugin test passed');

// verify snare duration scales with BPM
let lastLen = 0;
audioCtx.createBuffer = (ch, len, sr) => {
  lastLen = len;
  return { getChannelData: () => new Float32Array(len) };
};

setBPM(120);
await play('snare');
const len120 = lastLen;

setBPM(240);
await play('snare');
const len240 = lastLen;

if (len240 >= len120) {
  throw new Error(`expected shorter snare at 240 BPM: ${len240} vs ${len120}`);
}

console.log('snare BPM scaling test passed');

