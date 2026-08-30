package session

import (
	"fmt"
	"strings"
)

// Channel classifies creator-owned conversations for presentation. It never
// changes runtime identity, capabilities, or the Project that owns a Session.
type Channel string

const (
	ChannelAgent         Channel = "agent"
	ChannelConfiguration Channel = "configuration"
)

// ParseChannel validates a transport or persisted value. An omitted channel
// belongs to the ordinary Agent list so journals written before this field
// remain readable without migration.
func ParseChannel(value string) (Channel, error) {
	switch Channel(strings.TrimSpace(value)) {
	case "", ChannelAgent:
		return ChannelAgent, nil
	case ChannelConfiguration:
		return ChannelConfiguration, nil
	default:
		return "", fmt.Errorf("unsupported session channel %q", value)
	}
}

// ParseOptionalChannel preserves omission for callers that only want to assert
// a channel when they are creating or deliberately reopening a typed Session.
func ParseOptionalChannel(value string) (Channel, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil
	}
	return ParseChannel(value)
}
