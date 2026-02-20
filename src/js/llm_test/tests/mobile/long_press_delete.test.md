---
name: long_press_delete
platform: mobile
device: iPhone 12 landscape
tags: [touch, gesture, editing]
fixture: simple_loop.json
max_iterations: 25
---

# Long Press to Delete Node (Mobile)

Test the long-press context menu for deleting nodes on mobile.

## Steps
1. Observe the grid with nodes. Count the number of visible nodes.
2. Find a node in the grid area.
3. Use the touch_gesture tool to long-press on that node:
   - gesture: long_press, at the node's position, duration: 700
4. A popup/context menu should appear near the pressed location.
5. Look for a "Delete" option in the popup.
6. Tap the Delete option.
7. Verify:
   - The node has been removed from the grid
   - The popup/menu has closed
   - Surrounding nodes and edges adjusted properly

## Expected Outcomes
- Long press triggers a visible popup/context menu
- Delete option is clearly labeled and tappable
- Node disappears after deletion
- Connected edges are properly removed or rerouted
- No visual artifacts after deletion
