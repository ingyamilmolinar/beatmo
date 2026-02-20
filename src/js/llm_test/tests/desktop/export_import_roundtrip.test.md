---
name: export_import_roundtrip
platform: desktop
tags: [regression, workflow]
fixture: multi_row.json
max_iterations: 30
---

# Export/Import Roundtrip

Verify that the circuit state survives an export/import cycle visually.

## Steps
1. Observe the initial state: there should be multiple nodes connected by edges, and
   multiple drum rows in the drum view.
2. Count the approximate number of visible nodes and drum rows.
3. Click the Play button and let it play for 2 seconds to confirm the circuit works.
4. Click Stop.
5. Take a careful mental note of the layout: node positions, edge connections, drum row
   count, and BPM display.
6. Look for an export or save option in the UI (may be a button or menu).
7. If no export button is visible, note this as a finding and stop.
8. After export, clear or reset the circuit if possible.
9. Import the previously exported data.
10. Compare: the node layout, edges, drum rows, and BPM should match the original.
11. Click Play again and verify the circuit plays the same pattern.

## Expected Outcomes
- The exported circuit imports back to the same visual state
- Node positions and edge connections match before/after
- Drum row count and instruments match
- BPM value is preserved
- Playback produces the same animation pattern
- No visual artifacts from the import process
