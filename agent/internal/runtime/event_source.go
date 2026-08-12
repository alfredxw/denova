package runtime

import (
	"errors"
	"fmt"
	"strings"
)

const (
	maxEventSourceDepth = 32
	maxEventSourceBytes = 8 << 10
)

func validateEventSource(source EventSource) error {
	if len(source.Path) > maxEventSourceDepth {
		return fmt.Errorf("event source path exceeds %d entries", maxEventSourceDepth)
	}
	total := len(source.Name) + len(source.InvocationID) + len(source.InvocationType)
	if strings.TrimSpace(source.Name) != source.Name {
		return errors.New("event source name cannot contain surrounding whitespace")
	}
	if strings.TrimSpace(source.InvocationID) != source.InvocationID {
		return errors.New("event source invocation ID cannot contain surrounding whitespace")
	}
	if strings.TrimSpace(source.InvocationType) != source.InvocationType {
		return errors.New("event source invocation type cannot contain surrounding whitespace")
	}
	if source.InvocationType != "" && source.InvocationID == "" {
		return errors.New("event source invocation type requires an invocation ID")
	}
	for _, part := range source.Path {
		if strings.TrimSpace(part) == "" || strings.TrimSpace(part) != part {
			return errors.New("event source path requires non-empty normalized entries")
		}
		total += len(part)
	}
	if total > maxEventSourceBytes {
		return fmt.Errorf("event source exceeds %d bytes", maxEventSourceBytes)
	}
	if len(source.Path) > 0 && source.Name != "" && source.Path[len(source.Path)-1] != source.Name {
		return errors.New("event source name must match the final path entry")
	}
	return nil
}

func cloneEventSource(source EventSource) EventSource {
	source.Path = append([]string(nil), source.Path...)
	return source
}

func eventSourcesEqual(left, right EventSource) bool {
	if left.Name != right.Name || left.InvocationID != right.InvocationID ||
		left.InvocationType != right.InvocationType || len(left.Path) != len(right.Path) {
		return false
	}
	for index := range left.Path {
		if left.Path[index] != right.Path[index] {
			return false
		}
	}
	return true
}

func eventSourceEmpty(source EventSource) bool {
	return source.Name == "" && len(source.Path) == 0 && source.InvocationID == "" && source.InvocationType == ""
}
