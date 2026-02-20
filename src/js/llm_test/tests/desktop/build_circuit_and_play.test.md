---
name: build_circuit_and_play
platform: desktop
tags: [regression, workflow]
fixture: null
max_iterations: 40
---

# Build a Circuit and Play It

Build a simple beat pattern from scratch, play it, and verify the full workflow.

## Steps
1. The grid should be mostly empty. Click on 4 empty positions in the grid to create
   nodes. Space them apart to form a rectangle shape.
2. For each pair of adjacent nodes, hold Shift and drag from one node to the next to
   create edges. Connect all 4 nodes in a loop (A->B->C->D->A).
3. Verify all 4 nodes are visible and connected with edge lines.
4. Click the Play button.
5. Wait 3 seconds and observe:
   - Highlights should animate along the edges, visiting each node in sequence
   - You should see the highlight move in the loop pattern you created
6. Click the BPM + button 3 times to increase tempo.
7. Observe: the highlight animation should speed up noticeably.
8. Click the Stop button.
9. Verify playback stopped cleanly with no lingering highlights or animation.

## Expected Outcomes
- All 4 nodes render with distinct colors
- Edges display as lines with directional arrows
- Playback highlight visits nodes in the correct loop order
- BPM increase visibly accelerates the animation
- Stop cleanly halts all animation
- No visual corruption at any point during the workflow
