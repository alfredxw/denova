package compaction

import (
	"reflect"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

func cloneContextMessages(messages []*agent.Message) []*agent.Message {
	if messages == nil {
		return nil
	}
	cloned := make([]*agent.Message, len(messages))
	for index, message := range messages {
		cloned[index] = agent.CloneMessage(message)
	}
	return cloned
}

func contextMessagesEqual(left, right []*agent.Message) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !reflect.DeepEqual(left[index], right[index]) {
			return false
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
