package ui

import (
    "fmt"
    "github.com/ingyamilmolinar/tunkul/core/model"
)

// BuildPath validates orthogonal connectivity for pts and then creates nodes
// and edges in order under the given drum row. If the last point equals the
// first a loop is formed.
func (g *Game) BuildPath(row int, pts [][2]int) error {
    if len(pts) == 0 { return nil }
    for i := 0; i+1 < len(pts); i++ {
        ax, ay := pts[i][0], pts[i][1]
        bx, by := pts[i+1][0], pts[i+1][1]
        if !(ax == bx || ay == by) {
            return fmt.Errorf("invalid segment %d-%d: not orthogonal (%d,%d)->(%d,%d)", i, i+1, ax, ay, bx, by)
        }
    }
    nodes := make([]*uiNode, 0, len(pts))
    g.pendingStartRow = row
    for _, p := range pts {
        n := g.tryAddNode(p[0], p[1], model.NodeTypeRegular)
        nodes = append(nodes, n)
    }
    g.pendingStartRow = -1
    for i := 0; i+1 < len(nodes); i++ {
        if nodes[i] != nodes[i+1] {
            g.addEdge(nodes[i], nodes[i+1])
        }
    }
    return nil
}

