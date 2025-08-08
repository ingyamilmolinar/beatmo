import { register, play } from './audio.js';

let called = false;
register('test', (when) => {
  console.log('[audio.test] callback invoked at', when);
  called = true;
});

play('test', 0);

if (!called) {
  throw new Error('callback not invoked');
}

console.log('audio plugin test passed');

