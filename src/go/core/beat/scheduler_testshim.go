package beat

import (
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// GraphInterface defines the methods of model.Graph that Scheduler uses.
type GraphInterface interface {
	CalculateBeatRow() ([]bool, map[int]model.NodeID)
	BeatLength() int
}

// MockGraph is a mock implementation of GraphInterface for testing purposes.
type MockGraph struct {
	BeatRowToReturn []bool
	BeatLengthVal   int // To mock BeatLength
}

// CalculateBeatRow returns the predefined BeatRowToReturn.
func (m *MockGraph) CalculateBeatRow() ([]bool, map[int]model.NodeID) {
	activeNodes := make(map[int]model.NodeID)
	for i, active := range m.BeatRowToReturn {
		if active {
			activeNodes[i] = model.NodeID(i) // Use index as dummy NodeID
		}
	}
	return m.BeatRowToReturn, activeNodes
}

// BeatLength returns the predefined BeatLengthVal.
func (m *MockGraph) BeatLength() int {
	return m.BeatLengthVal
}

// AddNode, RemoveNode, GetNodeByID, ToggleStep are not used by the scheduler,
// so we can provide dummy implementations to satisfy the interface.
func (m *MockGraph) AddNode(i, j int) model.NodeID { return 0 }
func (m *MockGraph) RemoveNode(id model.NodeID)     {}
func (m *MockGraph) GetNodeByID(id model.NodeID) (model.Node, bool) {
	return model.Node{}, false
}
func (m *MockGraph) ToggleStep(i int) {}

// SetNowFunc is **only** compiled when the “test” build-tag is active.
// The UI test-suite uses it to deterministically advance the scheduler’s clock.
func (s *Scheduler) SetNowFunc(f func() time.Time) {
	s.now = f
}
