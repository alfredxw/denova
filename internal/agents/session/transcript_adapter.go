package session

import agent "github.com/alfredxw/denova/agent"

func (s *Session) effectiveTranscriptMessagesLocked() []*agent.Message {
	start := s.clearAfterIndex - s.messageBaseIndex
	if start < 0 {
		start = 0
	}
	if start > len(s.messages) {
		start = len(s.messages)
	}
	result := make([]*agent.Message, len(s.messages)-start)
	for index, message := range s.messages[start:] {
		result[index] = agent.CloneMessage(message)
	}
	return result
}
