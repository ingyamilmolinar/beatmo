package timeline

// TierForTest reports which read tier currently holds abs for row: "immutables",
// "ring", "archive", or "none". Test-only diagnostic mirroring valueLocked's
// tier precedence.
func (s *Service) TierForTest(row, abs int) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m := s.immutables[row]; m != nil {
		if _, ok := m[abs]; ok {
			return "immutables"
		}
	}
	if row >= 0 && row < len(s.rows) {
		if _, _, _, ok := s.rows[row].commits.value(abs); ok {
			return "ring"
		}
	}
	if a := s.archives[row]; a != nil {
		if _, _, _, ok := a.lookup(abs); ok {
			return "archive"
		}
	}
	return "none"
}
