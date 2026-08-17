package terminal

// scrollback keeps the most recent terminal output so a re-attaching client can restore
// the screen. It is a fixed-capacity buffer: the limit comes from configuration and the
// oldest bytes are dropped once it is reached, so a long-lived session cannot exhaust memory.
type scrollback struct {
	buf   []byte
	limit int
}

func newScrollback(limit int) *scrollback {
	if limit <= 0 {
		limit = defaultScrollbackBytes
	}
	return &scrollback{buf: make([]byte, 0, minInt(limit, 32*1024)), limit: limit}
}

// append stores new output, discarding from the front so the retained bytes are the latest screen.
func (s *scrollback) append(chunk []byte) {
	if len(chunk) == 0 {
		return
	}
	if len(chunk) >= s.limit {
		s.buf = append(s.buf[:0], chunk[len(chunk)-s.limit:]...)
		return
	}
	if len(s.buf)+len(chunk) > s.limit {
		drop := len(s.buf) + len(chunk) - s.limit
		s.buf = append(s.buf[:0], s.buf[drop:]...)
	}
	s.buf = append(s.buf, chunk...)
}

// snapshot returns a copy of the buffer that callers may use outside the lock.
func (s *scrollback) snapshot() []byte {
	if len(s.buf) == 0 {
		return nil
	}
	out := make([]byte, len(s.buf))
	copy(out, s.buf)
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
