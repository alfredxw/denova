package context

import agent "github.com/alfredxw/denova/agent"

// CloneMessages returns a deep-enough model-message snapshot using the Agent
// library's canonical clone semantics.
func CloneMessages(messages []*agent.Message) []*agent.Message {
	if messages == nil {
		return nil
	}
	cloned := make([]*agent.Message, len(messages))
	for index, message := range messages {
		cloned[index] = agent.CloneMessage(message)
	}
	return cloned
}
