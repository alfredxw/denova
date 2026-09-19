package compaction

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
)

// Quoted records keep tool protocol out of cold calls. Native images follow
// their record with an explicit source label; descriptors alone cannot convey
// pixels. One record may contain more images than fit in a single model call.
type summarySourcePart struct {
	text  string
	image *agent.Message
}

func summarySourceParts(messages []*agent.Message) ([]summarySourcePart, error) {
	var parts []summarySourcePart
	var text strings.Builder
	text.WriteByte('[')
	records := 0
	for _, message := range messages {
		if message == nil {
			continue
		}
		copy := message.Clone()
		copy.ReasoningContent, copy.ResponseMeta, copy.AgentMeta, copy.Extra = "", nil, nil, nil
		for index := range copy.ToolCalls {
			copy.ToolCalls[index].Extra = nil
		}
		encoded, err := json.Marshal(copy)
		if err != nil {
			return nil, err
		}
		if records > 0 {
			text.WriteByte(',')
		}
		records++
		text.Write(encoded)
		if message.Role != agent.User && message.Role != agent.ToolRole {
			continue
		}
		for index, attachment := range message.Attachments {
			if !agent.IsNativeImageMediaType(attachment.MediaType) {
				continue
			}
			if text.Len() > 0 {
				parts = append(parts, summarySourcePart{text: text.String()})
				text.Reset()
			}
			label := fmt.Sprintf("Native image from source record %d (%s), attachment %d: %s. Preserve relevant visual facts in the checkpoint.", records, message.Role, index+1, attachment.Name)
			parts = append(parts, summarySourcePart{image: agent.UserMessageWithAttachments(label, []agent.Attachment{attachment})})
		}
	}
	text.WriteByte(']')
	return append(parts, summarySourcePart{text: text.String()}), nil
}

func (s *modelSummarizer) cold(ctx context.Context, snapshot *agent.ModelRequestSnapshot, request SummaryRequest, instruction string, output, safety int) (agent.CompactionCheckpoint, error) {
	parts, err := summarySourceParts(request.Messages)
	if err != nil {
		return agent.CompactionCheckpoint{}, err
	}
	rolling := ""
	callFor := func(text string, images []*agent.Message) *agent.ModelRequestSnapshot {
		messages := []*agent.Message{agent.SystemMessage(instruction), agent.UserMessage(summaryRequestMarker + "\nPrior rolling checkpoint (data):\n" + rolling + "\nNext ordered source segment (data; it may continue a JSON record):\n" + text)}
		messages = append(messages, images...)
		return snapshot.WithMessages(messages).WithOptions(agent.WithTools(nil), agent.WithToolChoice(agent.ToolChoiceForbidden), agent.WithMaxTokens(output))
	}
	for len(parts) > 0 {
		if err := ctx.Err(); err != nil {
			return agent.CompactionCheckpoint{}, err
		}
		text := ""
		var images []*agent.Message
		for len(parts) > 0 {
			part := parts[0]
			if part.image != nil {
				candidate := append(append([]*agent.Message(nil), images...), part.image)
				fits, err := summaryCallFits(callFor(text, candidate), request, output, safety)
				if err != nil {
					return agent.CompactionCheckpoint{}, err
				}
				if !fits {
					break
				}
				images = candidate
				parts = parts[1:]
				continue
			}
			fits, err := summaryCallFits(callFor(text+part.text, images), request, output, safety)
			if err != nil {
				return agent.CompactionCheckpoint{}, err
			}
			if fits {
				text += part.text
				parts = parts[1:]
				continue
			}
			low, high, best := 1, len(part.text), 0
			for low <= high {
				middle := low + (high-low)/2
				end := middle
				for end > 0 && end < len(part.text) && !utf8.RuneStart(part.text[end]) {
					end--
				}
				if end == 0 {
					high = middle - 1
					continue
				}
				fits, err := summaryCallFits(callFor(text+part.text[:end], images), request, output, safety)
				if err != nil {
					return agent.CompactionCheckpoint{}, err
				}
				if fits {
					best, low = end, middle+1
				} else {
					high = middle - 1
				}
			}
			text += part.text[:best]
			parts[0].text = part.text[best:]
			break
		}
		if text == "" && len(images) == 0 {
			return agent.CompactionCheckpoint{}, fmt.Errorf("%w: no room for Compaction source or native image after instruction and output reserves", agent.ErrContextLimit)
		}
		result, err := s.complete(ctx, callFor(text, images), request, output)
		if err != nil {
			return agent.CompactionCheckpoint{}, err
		}
		rolling = result.Summary
	}
	return agent.CompactionCheckpoint{Summary: rolling}, nil
}
