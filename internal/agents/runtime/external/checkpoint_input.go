package external

import (
	"encoding/json"
	"fmt"

	agent "github.com/alfredxw/denova/agent"
)

// Adapters may report the resolved model's existing visual estimator. Unknown
// models use Agent's conservative reserve; no Native execution is involved.
type modelInputEstimator interface {
	InputEstimator(Input) agent.InputEstimator
}

func estimatorFor(adapter Adapter, input Input) agent.InputEstimator {
	if provider, ok := adapter.(modelInputEstimator); ok {
		return provider.InputEstimator(input)
	}
	return agent.InputEstimator{}
}

func messageCost(estimator agent.InputEstimator, message Message) (agent.InputSize, error) {
	// Tool images can accompany any projected public role. The neutral User
	// envelope ensures all of them receive the same visual estimate.
	files := append(append([]agent.Attachment(nil), message.Attachments...), message.ToolImages...)
	size, err := estimator.Estimate([]*agent.Message{agent.UserMessageWithAttachments(message.Text, files)}, nil)
	size.Bytes = messageBytes(message)
	return size, err
}

func inputTokens(estimator agent.InputEstimator, input Input) (int, error) {
	// Maintenance has no tools; this estimate includes its complete instructions,
	// rolling summary, ordered text records and native image parts.
	tokens := agent.EstimateTextTokens(input.Instructions) + agent.EstimateTextTokens(input.Text)
	for _, message := range input.History {
		cost, err := messageCost(estimator, message)
		if err != nil {
			return 0, err
		}
		tokens += cost.Tokens
	}
	return tokens, nil
}

func checkInputBytes(input Input, limit int) error {
	if limit <= 0 {
		return nil
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if len(body) > limit {
		return fmt.Errorf("external provider input exceeds shared semantic byte budget: %d > %d", len(body), limit)
	}
	return nil
}

func prepareInput(input Input, limit int) (Input, error) {
	input.History = append([]Message(nil), input.History...)
	input.Text = agent.ModelUserContent(&agent.Message{Content: input.Text, Attachments: input.Attachments})
	for index := range input.History {
		message := &input.History[index]
		message.Text = agent.ModelUserContent(&agent.Message{Content: message.Text, Attachments: message.Attachments})
	}
	if err := checkInputBytes(input, limit); err != nil {
		return Input{}, err
	}
	return input, nil
}
