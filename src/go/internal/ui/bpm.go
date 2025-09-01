package ui

import "strconv"

const maxBPM = 1000

// parseBPM converts txt into a BPM value and reports validity.
func parseBPM(txt string) (int, bool) {
       v, err := strconv.Atoi(txt)
       if err != nil || v < 1 || v > maxBPM {
               return 0, false
       }
       return v, true
}
