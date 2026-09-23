package agentruntime

import (
	"encoding/json"

	agentchat "denova/internal/agents/chat"
	agentexecution "denova/internal/agents/execution"
	agentrun "denova/internal/agents/run"
	agent "github.com/alfredxw/denova/agent"
)

func externalInputFingerprint(input ExternalCycleInput) string {
	return agentexecution.RequestSemanticFingerprint(input.Request) + ":" + input.RegenerateFromTurnID
}

// ChatRequest deliberately omits server-resolved fields from transport JSON.
// Durable inputs retain portable attachment descriptors, locale and visibility
// explicitly; resolved prompts, credentials and runtime paths are not stored.
type durableExternalRequest struct {
	Caller      agentchat.CallerInput    `json:"caller"`
	Visibility  agentrun.InputVisibility `json:"visibility,omitempty"`
	Attachments []agent.Attachment       `json:"attachments,omitempty"`
}
type durableExternalInput struct {
	Request              durableExternalRequest   `json:"request"`
	Delivery             agentrun.DeliveryKind    `json:"delivery"`
	Resume               bool                     `json:"resume,omitempty"`
	Guidance             []durableExternalRequest `json:"guidance,omitempty"`
	GoalID               string                   `json:"goal_id,omitempty"`
	GoalRevision         uint64                   `json:"goal_revision,omitempty"`
	RegenerateFromTurnID string                   `json:"regenerate_from_turn_id,omitempty"`
}

func storeExternalRequest(request agentchat.ChatRequest) durableExternalRequest {
	return durableExternalRequest{Caller: agentchat.CallerView(request), Visibility: request.InputVisibility, Attachments: request.AttachedFiles}
}
func (stored durableExternalRequest) restore() (agentchat.ChatRequest, error) {
	request := stored.Caller.Request()
	request.Locale, request.InputVisibility, request.AttachedFiles = stored.Caller.Locale, stored.Visibility, stored.Attachments
	return agentchat.CaptureChatRequestCallerInput(request), nil
}
func (input ExternalCycleInput) MarshalJSON() ([]byte, error) {
	stored := durableExternalInput{Request: storeExternalRequest(input.Request), Delivery: input.Delivery, Resume: input.Resume, GoalID: input.GoalID, GoalRevision: input.GoalRevision, RegenerateFromTurnID: input.RegenerateFromTurnID}
	for _, request := range input.Guidance {
		stored.Guidance = append(stored.Guidance, storeExternalRequest(request))
	}
	return json.Marshal(stored)
}
func (input *ExternalCycleInput) UnmarshalJSON(data []byte) error {
	var stored durableExternalInput
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	request, err := stored.Request.restore()
	if err != nil {
		return err
	}
	*input = ExternalCycleInput{Request: request, Delivery: stored.Delivery, Resume: stored.Resume, GoalID: stored.GoalID, GoalRevision: stored.GoalRevision, RegenerateFromTurnID: stored.RegenerateFromTurnID}
	for _, source := range stored.Guidance {
		request, err := source.restore()
		if err != nil {
			return err
		}
		input.Guidance = append(input.Guidance, request)
	}
	return nil
}
