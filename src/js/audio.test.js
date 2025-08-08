import { register, play, audioCtx } from './audio.js';

let called = false;
let resumed = false;
audioCtx.resume = () => { resumed = true; audioCtx.state = 'running'; };
register('test', (when) => {
  console.log('[audio.test] callback invoked at', when);
  called = true;
});

play('test');

if (!called || !resumed) {
  throw new Error('callback or resume not invoked');
}

console.log('audio plugin test passed');

