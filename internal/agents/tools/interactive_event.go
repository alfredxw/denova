package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/config"
	"denova/internal/interactive"
)

type eventCardReadInput struct {
	Path string `json:"path" jsonschema_description:"Frozen event-card URI from the current Director opportunity index, in the form event://package/card."`
}

type eventCardReadScope struct {
	storyID string
	turnID  string
	cards   map[string]interactive.DirectorEvent
}

// newEventCardReadAdapter freezes the due/new opportunity and selected Story
// Director catalog for one Agent construction. The shared read-tool byte
// budget bounds each result; the catalog itself has no arbitrary card count.
func newEventCardReadAdapter(ctx InteractiveContext) (agenttools.ReadAdapter, error) {
	ctx.StoryID = strings.TrimSpace(ctx.StoryID)
	if ctx.Store == nil || ctx.StoryID == "" {
		return nil, nil
	}
	ctx.TurnID = strings.TrimSpace(ctx.TurnID)
	cards, err := ctx.Store.DirectorEventCardReadScope(ctx.StoryID, strings.TrimSpace(ctx.BranchID), ctx.TurnID)
	if err != nil {
		return nil, fmt.Errorf("freeze Director event-card read scope: %w", err)
	}
	if len(cards) == 0 {
		return nil, nil
	}
	scope := &eventCardReadScope{
		storyID: ctx.StoryID,
		turnID:  ctx.TurnID,
		cards:   make(map[string]interactive.DirectorEvent, len(cards)),
	}
	for _, card := range cards {
		if ref := strings.Trim(strings.TrimSpace(card.ID), "/"); ref != "" && card.Enabled {
			scope.cards[ref] = card
		}
	}
	if len(scope.cards) == 0 {
		return nil, nil
	}
	return agenttools.NewReadAdapter(
		agent.CapabilityIdentity{Kind: "denova.read.event_card", Version: 1},
		"event_card", matchEventCardURI, scope.readCard,
	)
}

func interactiveDirectorReadAdapterFactory(toolContext InteractiveContext) ReadAdapterFactory {
	return func(settings config.ResolvedAgentToolSettings) ([]ReadAdapterBinding, error) {
		if !settings.Allows(config.AgentToolEventRead) {
			return nil, nil
		}
		switch strings.TrimSpace(toolContext.MaintenanceTask) {
		case "director_plan_update", "opening_plan":
		default:
			return nil, nil
		}
		adapter, err := newEventCardReadAdapter(toolContext)
		if err != nil {
			return nil, err
		}
		if adapter == nil {
			return nil, nil
		}
		binding, err := newReadAdapterBinding(config.AgentToolEventRead, adapter)
		if err != nil {
			return nil, err
		}
		return []ReadAdapterBinding{binding}, nil
	}
}

func matchEventCardURI(_ context.Context, path string) (bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(path))
	if err != nil {
		return false, err
	}
	return strings.EqualFold(parsed.Scheme, "event"), nil
}

func (scope *eventCardReadScope) readCard(_ context.Context, input eventCardReadInput) (agenttools.ReadResult, error) {
	ref, canonical, err := parseEventCardURI(input.Path)
	if err != nil {
		return agenttools.ReadResult{}, err
	}
	card, allowed := scope.cards[ref]
	if !allowed {
		return agenttools.ReadResult{}, fmt.Errorf("event card is outside the frozen Director opportunity: %s", ref)
	}

	payload := struct {
		Schema string                    `json:"schema"`
		Source map[string]string         `json:"source"`
		Card   interactive.DirectorEvent `json:"card"`
	}{
		Schema: "interactive.event_card.read.v1",
		Source: map[string]string{
			"kind": "selected_story_director_event_card", "story_id": scope.storyID,
			"source_turn_id": scope.turnID, "event_ref": ref,
		},
		Card: card,
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return agenttools.ReadResult{}, fmt.Errorf("encode event card: %w", err)
	}
	return agenttools.ReadResult{Path: canonical, Kind: "event_card", Content: string(encoded)}, nil
}

func parseEventCardURI(value string) (ref, canonical string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", "", err
	}
	if !strings.EqualFold(parsed.Scheme, "event") || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", "", errors.New("event card path must be event://package/card without query, fragment, or user info")
	}
	packageID := strings.Trim(strings.TrimSpace(parsed.Host), "/")
	cardID := strings.Trim(strings.TrimSpace(parsed.Path), "/")
	if packageID == "" || cardID == "" || strings.Contains(cardID, "/") {
		return "", "", errors.New("event card path must contain exactly one package and one card segment")
	}
	ref = packageID + "/" + cardID
	return ref, "event://" + ref, nil
}
