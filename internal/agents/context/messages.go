package context

import (
	"reflect"

	agent "github.com/alfredxw/denova/agent"
)

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

// MessagesEqual compares exact provider-neutral message projections.
func MessagesEqual(left, right []*agent.Message) bool {
	return reflect.DeepEqual(left, right)
}

// MessagesHavePrefix reports whether prefix is an exact stable prefix of
// messages; it is used to preserve provider-cache identity across rewinds.
func MessagesHavePrefix(messages, prefix []*agent.Message) bool {
	return len(messages) >= len(prefix) && MessagesEqual(messages[:len(prefix)], prefix)
}
