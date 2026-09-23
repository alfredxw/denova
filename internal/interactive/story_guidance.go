package interactive

import (
	agent "github.com/alfredxw/denova/agent"
	"strings"
)

const userGuidanceKey = "denova.user_guidance"

// UserGuidanceMessage is an additional accepted player instruction for an
// unfinished Turn. It belongs to the product context, not Native Context State.
func UserGuidanceMessage(commandID, text string, attachments []agent.Attachment) *agent.Message {
	message := agent.UserMessage(text)
	message.Attachments = attachments
	message.Extra = map[string]any{userGuidanceKey: commandID}
	return message
}

func UserGuidanceCommand(message *agent.Message) string {
	if message == nil || message.Role != agent.User {
		return ""
	}
	id, _ := message.Extra[userGuidanceKey].(string)
	if strings.TrimSpace(id) == "" || agent.ValidateIdempotencyKey(id) != nil {
		return ""
	}
	return id
}
