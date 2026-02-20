---
name: live_edit_during_playback
platform: desktop
tags: [regression, workflow]
fixture: simple_loop.json
max_iterations: 35
---

# Live Edit During Playback

Test editing the circuit while playback is active.

## Steps
1. Click the Play button to start playback.
2. Wait 2 seconds to confirm animation is stable.
3. While playback continues, click on an empty grid position to add a new node.
4. Observe: the new node should appear without disrupting playback.
5. Try to connect the new node to an existing node by shift-dragging an edge.
6. Observe: the edge should appear and playback should continue smoothly.
7. Wait 3 seconds and observe the animation pattern:
   - If the new node is in the circuit path, the highlight should visit it
   - If not connected to the loop, the existing loop should continue unchanged
8. Click the Stop button.
9. Verify the circuit reflects all edits made during playback.

## Expected Outcomes
- Adding nodes during playback does not cause glitches or freezes
- New edges are drawn correctly while animation continues
- Playback animation adapts to circuit changes
- No visual artifacts, stuttering, or rendering corruption
- Stop cleanly halts playback after live edits
- All edits persist after stopping
