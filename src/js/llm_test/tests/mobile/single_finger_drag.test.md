---
name: single_finger_drag
platform: mobile
device: iPhone 12 landscape
tags: [touch, gesture, navigation]
fixture: simple_loop.json
max_iterations: 20
---

# Single Finger Drag (Mobile)

Test single-finger drag to pan the grid camera on mobile.

## Steps
1. Observe the grid pane. Note the position of nodes relative to the screen edges.
2. Use the touch_gesture tool to swipe across the grid area:
   - gesture: swipe, starting at the center of the grid
   - delta_x: 150, delta_y: 0 (swipe right)
3. Observe: the grid viewport should shift. Nodes should move LEFT relative to the screen.
4. Use the touch_gesture tool to swipe back:
   - gesture: swipe, starting at the center of the grid
   - delta_x: -150, delta_y: 0 (swipe left)
5. Observe: nodes should return to approximately their original positions.
6. Try a vertical swipe:
   - gesture: swipe, starting at the center of the grid
   - delta_x: 0, delta_y: 100 (swipe down)
7. Observe: the viewport should shift vertically.

## Expected Outcomes
- Single-finger drag moves the grid camera smoothly
- Horizontal and vertical swiping both work
- No rendering artifacts during dragging
- Drag in opposite direction reverses the movement
- Nodes and edges remain properly connected during pan
