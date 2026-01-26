package ui

func (g *Game) resetOriginSequences() {
	for row := range g.originIdxsByRow {
		positions := g.originIdxsByRow[row]
		seq := 0
		if len(positions) > 1 {
			beat := 0
			if row < len(g.nextBeatIdxs) {
				beat = g.nextBeatIdxs[row]
			}
			for i, idx := range positions {
				if beat <= idx {
					seq = i
					break
				}
			}
			if beat > positions[len(positions)-1] {
				seq = 0
			}
		}
		if row < len(g.nextOriginIdxByRow) {
			g.nextOriginIdxByRow[row] = seq
		}
	}
}
