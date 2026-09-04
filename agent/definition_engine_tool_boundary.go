package agent

import "errors"

// canonicalToolBatchAssistant accepts the finalized Agent-owned response while
// retaining lifecycle metadata attached when its raw stream crossed the
// Definition Engine. Tool preparation and middleware rewrites are deliberately
// not reconstructed from lower-level execution events.
func canonicalToolBatchAssistant(current, candidate *Message) (*Message, error) {
	if current == nil || current.Role != Assistant || len(current.ToolCalls) == 0 {
		return nil, errors.New("canonical tool batch has no pending assistant owner")
	}
	if _, err := validateCanonicalToolCallMessage(candidate); err != nil {
		return nil, err
	}
	canonical := candidate.Clone()
	if canonical.AgentMeta == nil {
		canonical.AgentMeta = current.Clone().AgentMeta
	} else if current.AgentMeta != nil && canonical.AgentMeta.ModelResponseOrdinal != current.AgentMeta.ModelResponseOrdinal {
		return nil, errors.New("canonical tool batch changed its model response identity")
	}
	return canonical, nil
}

func completedCanonicalToolBatch(current *Message, messages []*Message) ([]*Message, error) {
	if err := ValidateContextCommitMessages(messages); err != nil {
		return nil, err
	}
	result := cloneMessages(messages)
	assistant, err := canonicalToolBatchAssistant(current, result[0])
	if err != nil {
		return nil, err
	}
	result[0] = assistant
	return result, nil
}
