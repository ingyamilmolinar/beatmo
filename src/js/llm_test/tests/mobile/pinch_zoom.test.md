---
name: pinch_zoom
platform: mobile
device: iPhone 12 landscape
tags: [smoke, touch, gesture]
fixture: simple_loop.json
max_iterations: 20
---

# Pinch Zoom on Mobile

Test pinch-to-zoom gesture on the grid pane.

## Steps
1. Observe the grid pane showing colored nodes connected by edges.
2. Note the current size of the nodes.
3. Use the touch_gesture tool to pinch OUT (zoom in) on the center of the grid pane:
   - gesture: pinch_out, at the center of the grid area
4. Observe: nodes should appear LARGER after zooming in.
5. Use the touch_gesture tool to pinch IN (zoom out) on the same area:
   - gesture: pinch_in, at the center of the grid area
6. Observe: nodes should return to approximately their original size.

## Expected Outcomes
- Pinch-out makes nodes visibly larger (zoom in)
- Pinch-in makes nodes visibly smaller (zoom out)
- Zoom is smooth with no flickering or rendering artifacts
- Nodes and edges remain properly drawn during zoom
- No UI elements overlap or misalign during zoom
