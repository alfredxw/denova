package external

import (
	"encoding/json"
	"fmt"

	agentmodel "github.com/alfredxw/denova/agent/model"
	agentschema "github.com/alfredxw/denova/agent/schema"
)

// Adapters may report the resolved model's existing visual estimator. Unknown
// models use Agent's shared image fallback; no Native execution is involved.
type modelInputEstimator interface {
	InputEstimator(Input) agentmodel.InputEstimator
}

func estimatorFor(adapter Adapter, input Input) agentmodel.InputEstimator {
	if provider, ok := adapter.(modelInputEstimator); ok {
		return provider.InputEstimator(input)
	}
	return agentmodel.InputEstimator{}
}

func messageCost(estimator agentmodel.InputEstimator, message Message) (agentmodel.InputSize, error) {
	// Tool images can accompany any projected public role. The neutral User
	// envelope ensures all of them receive the same visual estimate.
	files := append(append([]agentschema.Attachment(nil), message.Attachments...), message.ToolImages...)
	size, err := estimator.Estimate([]*agentschema.Message{agentschema.UserMessageWithAttachments(message.Text, files)}, nil)
	size.Bytes = messageBytes(message)
	return size, err
}

func inputTokens(estimator agentmodel.InputEstimator, input Input) (int, error) {
	// Maintenance has no tools; this estimate includes its complete instructions,
	// rolling summary, ordered text records and native image parts.
	tokens := agentmodel.EstimateTextTokens(input.Instructions) + agentmodel.EstimateTextTokens(input.Text)
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
	input.Text = agentschema.ModelUserContent(&agentschema.Message{Content: input.Text, Attachments: input.Attachments})
	for index := range input.History {
		message := &input.History[index]
		message.Text = agentschema.ModelUserContent(&agentschema.Message{Content: message.Text, Attachments: message.Attachments})
	}
	if err := checkInputBytes(input, limit); err != nil {
		return Input{}, err
	}
	return input, nil
}

// carriesImages reports whether the rebuilt conversation will send media to the
// engine: historical user/tool attachments or the current request's files.
// Only image media has an engine block channel, so non-image attachments are
// plain text and never consume the media reserve.
func carriesImages(input Input) bool {
	image := func(files []agentschema.Attachment) bool {
		for _, file := range files {
			if agentschema.IsNativeImageMediaType(file.MediaType) {
				return true
			}
		}
		return false
	}
	if image(input.Attachments) {
		return true
	}
	for _, message := range input.History {
		if image(message.Attachments) || image(message.ToolImages) {
			return true
		}
	}
	return false
}

// The replay estimate mirrors the adapter's cold-rebuild renderer so history
// preparation reserves the media space before images are encoded. For typical
// text every value stays conservative (over) against the real renderer: JSONL
// escaping and the .json block envelope add several percent over semantic
// bytes. Attachment description blocks carry a small fixed overhead that the
// escape margin covers; an underestimation would hand the transport a prompt
// that still exceeds its payload once images are added.
const (
	// replayEscapePercent applies to all semantic text: quotes, backslashes and
	// newlines can inflate serialized records well past raw byte counts.
	replayEscapePercent = 115
	// replayMessageEnvelopeBytes is the fixed JSONL wrapper around one record
	// ({"content":"...","role":"..."}\n) minus the role bytes already counted
	// by messageBytes. replayPrefixBytes and replayRequestPrefixBytes mirror the
	// renderer's fixed leading text and the current-request separator.
	// replayCheckpointLabelBytes precedes a checkpoint summary record.
	replayMessageEnvelopeBytes = 15
	replayPrefixBytes          = 260
	replayRequestPrefixBytes   = 22
	replayCheckpointLabelBytes = 31
)

// estimateReplayText estimates the bytes a cold rebuild will render for the
// engine prompt: semantic history, the checkpoint summary, the current request
// and their serialization overhead. summaryBytes is the reserve to plan for
// (the bounded summary ceiling during trimming, the actual summary otherwise).
// historyMessages is the number of history records that will be rendered.
func estimateReplayText(historyBytes, summaryBytes, requestBytes, historyMessages int) int {
	text := historyBytes + replayMessageEnvelopeBytes*historyMessages + replayPrefixBytes
	if summaryBytes > 0 {
		text += replayCheckpointLabelBytes + replayMessageEnvelopeBytes + summaryBytes
	}
	text += replayRequestPrefixBytes + requestBytes
	return text * replayEscapePercent / 100
}
