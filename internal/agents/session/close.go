package session

// Close flushes the rebuildable conversation index and makes this Session
// handle unavailable for further mutations. The canonical JSONL has already
// been fsynced by each successful append.
func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journal == nil {
		return nil
	}
	err := s.journal.Close()
	s.journal = nil
	return err
}
