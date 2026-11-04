package external

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	conversation "denova/internal/agents/conversation"
	"denova/internal/i18n"
	interaction "github.com/alfredxw/denova/agent/lifecycle/interaction"
)

// ErrInteractionWithdrawn cancels one native prompt while its operation remains
// active. A user stop or process loss still uses the operation's recovery path.
var ErrInteractionWithdrawn = errors.New("native interaction withdrawn")

// NativeQuestion maps the engine's user-facing question to the product's durable
// interaction. Provider labels and IDs stay separate from validated option IDs.
type NativeQuestion struct {
	ID       string
	Prompt   string
	Options  []interaction.InteractionOption
	Multiple bool
}

func (input Input) Interactive() bool {
	if input.Mode != OperationTurn {
		return false
	}
	for _, tool := range input.Tools {
		if tool.Name == "ask" {
			return true
		}
	}
	return false
}

func AskNative(ctx context.Context, host Host, input Input, id string, questions []NativeQuestion) (map[string][]string, error) {
	if !input.Interactive() {
		return nil, nil
	}
	if len(questions) > 3 {
		answers := map[string][]string{}
		for offset := 0; offset < len(questions); offset += 3 {
			batch, err := AskNative(ctx, host, input, fmt.Sprintf("%s-%d", id, offset), questions[offset:min(offset+3, len(questions))])
			if err != nil || batch == nil {
				return nil, err
			}
			for key, value := range batch {
				answers[key] = value
			}
		}
		return answers, nil
	}
	items := make([]interaction.InteractionQuestion, len(questions))
	for index, question := range questions {
		options := append([]interaction.InteractionOption(nil), question.Options...)
		for i := range options {
			options[i].Value = fmt.Sprintf("option_%d", i)
		}
		items[index] = interaction.InteractionQuestion{ID: fmt.Sprintf("question_%d", index), Prompt: input.PortableText(question.Prompt), Options: options, Multiple: question.Multiple, AllowFreeText: true}
	}
	body, err := json.Marshal(map[string]any{"questions": items})
	if err != nil {
		return nil, err
	}
	result, err := host.CallTool(ctx, ToolCall{ID: "native-question-" + id, Name: "ask", Arguments: body})
	if err != nil {
		return nil, err
	}
	var resolution conversation.HostAskResolution
	if !result.Success || json.Unmarshal([]byte(result.Text), &resolution) != nil || resolution.Status != "answered" {
		return nil, nil
	}
	answers := map[string][]string{}
	for i, question := range questions {
		for _, answer := range resolution.Answers {
			if answer.QuestionID != items[i].ID {
				continue
			}
			for _, selected := range answer.SelectedOptions {
				answers[question.ID] = append(answers[question.ID], selected.Label)
			}
			if answer.CustomInput != "" {
				answers[question.ID] = append(answers[question.ID], answer.CustomInput)
			}
		}
	}
	return answers, nil
}

// ApproveNative requires an explicit option selection. Free text, cancellation,
// missing interaction support and unrecognized answers never grant permission.
func ApproveNative(ctx context.Context, host Host, input Input, id, description string) (bool, error) {
	if !input.Interactive() {
		return false, nil
	}
	locale := i18n.New(input.Locale)
	question := interaction.InteractionQuestion{ID: "approval", Prompt: locale.T("agentRuntime.nativeApproval") + "\n\n" + input.PortableText(description), Options: []interaction.InteractionOption{
		{Value: "allow", Label: locale.T("agentRuntime.allowOnce")},
		{Value: "deny", Label: locale.T("agentRuntime.deny")},
	}}
	body, _ := json.Marshal(map[string]any{"questions": []interaction.InteractionQuestion{question}})
	result, err := host.CallTool(ctx, ToolCall{ID: "native-approval-" + id, Name: "ask", Arguments: body})
	if err != nil {
		return false, err
	}
	var resolution conversation.HostAskResolution
	if !result.Success || json.Unmarshal([]byte(result.Text), &resolution) != nil || resolution.Status != "answered" {
		return false, nil
	}
	for _, answer := range resolution.Answers {
		if answer.QuestionID == "approval" && strings.TrimSpace(answer.CustomInput) == "" && len(answer.SelectedOptions) == 1 && answer.SelectedOptions[0].ID == "allow" {
			return true, nil
		}
	}
	return false, nil
}
