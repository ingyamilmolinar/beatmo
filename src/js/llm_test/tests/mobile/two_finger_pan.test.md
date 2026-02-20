---
name: two_finger_pan
platform: mobile
device: iPhone 12 landscape
tags: [touch, gesture, navigation]
fixture: simple_loop.json
max_iterations: 20
---

# Two-Finger Pan Camera (Mobile)

Test two-finger pan gesture to move the camera/viewport.

## Steps
1. Observe the grid pane. Note the position of nodes relative to the screen edges.
2. Use the touch_gesture tool to perform a two-finger pan:
   - gesture: two_finger_pan, starting at the center of the grid
   - delta_x: 100, delta_y: 0 (pan right)
3. Observe: all nodes should have shifted LEFT relative to the screen (camera moved right).
4. Use the touch_gesture tool to pan back:
   - gesture: two_finger_pan, starting at the center of the grid
   - delta_x: -100, delta_y: 0 (pan left)
5. Observe: nodes should return to approximately their original positions.

## Expected Outcomes
- Two-finger pan moves the viewport smoothly
- All nodes and edges shift together (no tearing)
- No rendering artifacts during or after panning
- Pan in opposite direction reverses the movement
