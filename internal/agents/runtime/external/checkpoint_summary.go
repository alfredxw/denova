package external

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
)

// summarize preserves ordered source parts, including native user/tool images.
// Each image and its source label is indivisible; text may span UTF-8-safe batches.
func (request HistoryPreparation) summarize(ctx context.Context, input Input, source []Message, summary string, estimator agent.InputEstimator) (string, error) {
	var batch []Message
	textBytes := 0
	maintenance := func(history []Message) Input {
		return Input{Selection: input.Selection, Mode: OperationSummarize, Instructions: checkpointInstruction,
			Text: "Previous summary:\n" + summary + "\n\nSummarize the next ordered canonical source records supplied in history.", History: history}
	}
	fits := func(history []Message) (bool, error) {
		candidate := maintenance(history)
		if err := checkInputBytes(candidate, request.ProviderInputMaxBytes); err != nil {
			return false, nil
		}
		tokens, err := inputTokens(estimator, candidate)
		return tokens <= historyTokenBudget, err
	}
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		result, err := request.Adapter.Run(WithSteering(ctx, nil), maintenance(batch), maintenanceHost{})
		if request.AddUsage != nil {
			request.AddUsage(result.Usage)
		}
		if err != nil {
			return fmt.Errorf("maintain external context: %w", err)
		}
		if strings.TrimSpace(result.Text) == "" || len(result.Text) > checkpointSummaryBytes {
			return errors.New("external context summary is empty or exceeds its source budget")
		}
		summary, batch, textBytes = result.Text, nil, 0
		return nil
	}
	for index, message := range source {
		// Summary references stay portable even though image bytes resolve on this host.
		files := append(append([]agent.Attachment(nil), message.Attachments...), message.ToolImages...)
		for i := range files {
			files[i].RuntimePath = ""
		}
		label := fmt.Sprintf("Source record %d (cursor %d, %s)", index+1, message.Cursor, message.Role)
		text := label + "\n" + agent.ModelUserContent(&agent.Message{Content: message.Text, Attachments: files})
		for len(text) > 0 {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			low, high, best := 1, min(len(text), maintenanceChunkBytes-textBytes), 0
			for low <= high {
				middle := low + (high-low)/2
				end := middle
				for end > 0 && end < len(text) && !utf8.RuneStart(text[end]) {
					end--
				}
				if end == 0 {
					low = middle + 1
					continue
				}
				ok, err := fits(append(append([]Message(nil), batch...), Message{Role: "user", Text: text[:end]}))
				if err != nil {
					return "", err
				}
				if ok {
					best, low = end, middle+1
				} else {
					high = middle - 1
				}
			}
			if best == 0 {
				if len(batch) == 0 {
					return "", errors.New("external context budget cannot fit a source text fragment")
				}
				if err := flush(); err != nil {
					return "", err
				}
				continue
			}
			batch = append(batch, Message{Role: "user", Text: text[:best]})
			textBytes += best
			text = text[best:]
		}
		for _, images := range [][]agent.Attachment{message.Attachments, message.ToolImages} {
			for _, file := range images {
				if !agent.IsNativeImageMediaType(file.MediaType) {
					continue
				}
				if _, err := agent.ReadAttachmentImage(file); err != nil {
					return "", err
				}
				part := Message{Role: "user", Text: label + " image: " + file.Name + " (" + file.Path + ")", Attachments: []agent.Attachment{file}}
				candidate := append(append([]Message(nil), batch...), part)
				ok, err := fits(candidate)
				if err != nil {
					return "", err
				}
				if !ok {
					if err := flush(); err != nil {
						return "", err
					}
					candidate = []Message{part}
					ok, err = fits(candidate)
					if err != nil {
						return "", err
					}
					if !ok {
						return "", fmt.Errorf("external context budget cannot fit source image %q", file.Name)
					}
				}
				batch = candidate
			}
		}
	}
	if err := flush(); err != nil {
		return "", err
	}
	return summary, nil
}
