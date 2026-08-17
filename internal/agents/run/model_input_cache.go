package agentrun

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"

	"denova/internal/agents/prompts"
)

const modelInputCacheScopes = 128

var (
	modelInputCacheMu    sync.Mutex
	modelInputCacheState = make(map[string]modelInputCacheBaseline)
	modelInputCacheOrder []string
)

type modelInputLogCache struct {
	MessageFingerprint      string                                  `json:"message_fingerprint,omitempty"`
	SystemPromptFingerprint string                                  `json:"system_prompt_fingerprint,omitempty"`
	ModelConfigFingerprint  string                                  `json:"model_config_fingerprint,omitempty"`
	ToolSchemaFingerprint   string                                  `json:"tool_schema_fingerprint,omitempty"`
	ToolNames               []string                                `json:"tool_names,omitempty"`
	ToolFingerprints        []modelInputLogToolFingerprint          `json:"tool_fingerprints,omitempty"`
	MessageCount            int                                     `json:"message_count"`
	ToolCount               int                                     `json:"tool_count"`
	CacheScopeFingerprint   string                                  `json:"cache_scope_fingerprint,omitempty"`
	MessageFingerprints     []modelInputLogMessageFingerprint       `json:"message_fingerprints,omitempty"`
	SystemSections          []modelInputLogSystemSectionFingerprint `json:"system_sections,omitempty"`
	FirstDivergence         *modelInputLogFirstDivergence           `json:"first_divergence,omitempty"`
}

type modelInputLogMessageFingerprint struct {
	Index       int            `json:"index"`
	Role        agent.RoleType `json:"role,omitempty"`
	Component   string         `json:"component"`
	Fingerprint string         `json:"fingerprint"`
}

type modelInputLogSystemSectionFingerprint struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Purpose     string `json:"purpose"`
	Fingerprint string `json:"fingerprint"`
	Bytes       int    `json:"bytes"`
}

type modelInputLogFirstDivergence struct {
	PreviousCallID        string         `json:"previous_call_id"`
	Component             string         `json:"component"`
	Index                 int            `json:"index,omitempty"`
	Role                  agent.RoleType `json:"role,omitempty"`
	SectionID             string         `json:"section_id,omitempty"`
	MatchingMessagePrefix int            `json:"matching_message_prefix"`
	PreviousFingerprint   string         `json:"previous_fingerprint,omitempty"`
	CurrentFingerprint    string         `json:"current_fingerprint,omitempty"`
}

type modelInputCacheBaseline struct {
	CallID          string
	Messages        []modelInputLogMessageFingerprint
	Tools           []modelInputLogToolFingerprint
	ToolHash        string
	ModelConfigHash string
	SystemSections  []modelInputLogSystemSectionFingerprint
}

type modelInputLogToolFingerprint struct {
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

type modelInputSystemSectionsContextKey struct{}

func contextWithModelInputSystemSections(ctx context.Context, sections []modelInputLogSystemSectionFingerprint) context.Context {
	if len(sections) == 0 {
		return ctx
	}
	return context.WithValue(ctx, modelInputSystemSectionsContextKey{}, append([]modelInputLogSystemSectionFingerprint(nil), sections...))
}

func modelInputSystemSectionsFromContext(ctx context.Context) []modelInputLogSystemSectionFingerprint {
	if ctx == nil {
		return nil
	}
	sections, _ := ctx.Value(modelInputSystemSectionsContextKey{}).([]modelInputLogSystemSectionFingerprint)
	return append([]modelInputLogSystemSectionFingerprint(nil), sections...)
}

func modelInputLogCacheAttribution(messages []*agent.Message, tools []modelInputLogTool) modelInputLogCache {
	return modelInputLogCache{
		MessageFingerprint:      modelInputLogFingerprint(messages),
		SystemPromptFingerprint: modelInputLogFingerprint(modelInputLogSystemMessages(messages)),
		ToolSchemaFingerprint:   modelInputLogFingerprint(tools),
		ToolNames:               modelInputLogToolNames(tools),
		ToolFingerprints:        modelInputLogToolFingerprints(tools),
		MessageFingerprints:     modelInputLogMessageFingerprints(messages),
		MessageCount:            len(messages),
		ToolCount:               len(tools),
	}
}

func modelInputLogCacheObservation(callID, scope string, cfg providers.ModelConfig, messages []*agent.Message, tools []modelInputLogTool, systemSections []modelInputLogSystemSectionFingerprint) modelInputLogCache {
	observation := modelInputLogCacheAttribution(messages, tools)
	observation.ModelConfigFingerprint = modelInputLogFingerprint(modelInputLogConfig(cfg))
	observation.SystemSections = append([]modelInputLogSystemSectionFingerprint(nil), systemSections...)
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return observation
	}
	observation.CacheScopeFingerprint = modelInputLogFingerprint(scope)
	baseline := modelInputCacheBaseline{
		CallID: callID, Messages: append([]modelInputLogMessageFingerprint(nil), observation.MessageFingerprints...),
		Tools:           append([]modelInputLogToolFingerprint(nil), observation.ToolFingerprints...),
		ToolHash:        observation.ToolSchemaFingerprint,
		ModelConfigHash: observation.ModelConfigFingerprint,
		SystemSections:  append([]modelInputLogSystemSectionFingerprint(nil), systemSections...),
	}
	modelInputCacheMu.Lock()
	previous, exists := modelInputCacheState[scope]
	if !exists {
		if len(modelInputCacheState) >= modelInputCacheScopes && len(modelInputCacheOrder) > 0 {
			oldest := modelInputCacheOrder[0]
			modelInputCacheOrder = modelInputCacheOrder[1:]
			delete(modelInputCacheState, oldest)
		}
		modelInputCacheOrder = append(modelInputCacheOrder, scope)
	}
	modelInputCacheState[scope] = baseline
	modelInputCacheMu.Unlock()
	if exists {
		observation.FirstDivergence = firstModelInputDivergence(previous, baseline)
	}
	return observation
}

func modelInputLogMessageFingerprints(messages []*agent.Message) []modelInputLogMessageFingerprint {
	result := make([]modelInputLogMessageFingerprint, 0, len(messages))
	for index, message := range messages {
		component := "message"
		role := agent.RoleType("")
		if message != nil {
			role = message.Role
			if role == agent.System {
				component = "system_message"
			} else if agent.IsContextStateMessage(message) {
				component = "context_state"
			}
		}
		result = append(result, modelInputLogMessageFingerprint{
			Index: index, Role: role, Component: component, Fingerprint: modelInputLogFingerprint(message),
		})
	}
	return result
}

func firstModelInputDivergence(previous, current modelInputCacheBaseline) *modelInputLogFirstDivergence {
	if previous.ModelConfigHash != current.ModelConfigHash {
		return &modelInputLogFirstDivergence{
			PreviousCallID: previous.CallID, Component: "model_config",
			PreviousFingerprint: previous.ModelConfigHash, CurrentFingerprint: current.ModelConfigHash,
		}
	}
	if section := firstSystemSectionDivergence(previous.SystemSections, current.SystemSections); section != nil {
		section.PreviousCallID = previous.CallID
		return section
	}
	if previous.ToolHash != current.ToolHash {
		index := firstToolFingerprintDivergence(previous.Tools, current.Tools)
		var before, after string
		if index < len(previous.Tools) {
			before = previous.Tools[index].Fingerprint
		}
		if index < len(current.Tools) {
			after = current.Tools[index].Fingerprint
		}
		return &modelInputLogFirstDivergence{
			PreviousCallID: previous.CallID, Component: "tool_schema", Index: index,
			MatchingMessagePrefix: matchingMessageFingerprintPrefix(previous.Messages, current.Messages),
			PreviousFingerprint:   before, CurrentFingerprint: after,
		}
	}
	common := min(len(previous.Messages), len(current.Messages))
	for index := 0; index < common; index++ {
		before, after := previous.Messages[index], current.Messages[index]
		if before.Fingerprint == after.Fingerprint {
			continue
		}
		return &modelInputLogFirstDivergence{
			PreviousCallID: previous.CallID, Component: after.Component, Index: index, Role: after.Role,
			MatchingMessagePrefix: index, PreviousFingerprint: before.Fingerprint, CurrentFingerprint: after.Fingerprint,
		}
	}
	if len(previous.Messages) != len(current.Messages) {
		var before, after modelInputLogMessageFingerprint
		if common < len(previous.Messages) {
			before = previous.Messages[common]
		}
		if common < len(current.Messages) {
			after = current.Messages[common]
		}
		component := after.Component
		role := after.Role
		if component == "" {
			component = "message_removed"
			role = before.Role
		}
		return &modelInputLogFirstDivergence{
			PreviousCallID: previous.CallID, Component: component, Index: common, Role: role,
			MatchingMessagePrefix: common, PreviousFingerprint: before.Fingerprint, CurrentFingerprint: after.Fingerprint,
		}
	}
	return &modelInputLogFirstDivergence{
		PreviousCallID: previous.CallID, Component: "none", MatchingMessagePrefix: len(current.Messages),
	}
}

func matchingMessageFingerprintPrefix(previous, current []modelInputLogMessageFingerprint) int {
	common := min(len(previous), len(current))
	for index := 0; index < common; index++ {
		if previous[index].Fingerprint != current[index].Fingerprint {
			return index
		}
	}
	return common
}

func firstSystemSectionDivergence(previous, current []modelInputLogSystemSectionFingerprint) *modelInputLogFirstDivergence {
	common := min(len(previous), len(current))
	for index := 0; index < common; index++ {
		before, after := previous[index], current[index]
		if before == after {
			continue
		}
		return &modelInputLogFirstDivergence{
			Component: "system_section", SectionID: after.ID,
			PreviousFingerprint: before.Fingerprint, CurrentFingerprint: after.Fingerprint,
		}
	}
	if len(previous) == len(current) {
		return nil
	}
	var before, after modelInputLogSystemSectionFingerprint
	if common < len(previous) {
		before = previous[common]
	}
	if common < len(current) {
		after = current[common]
	}
	sectionID := after.ID
	if sectionID == "" {
		sectionID = before.ID
	}
	return &modelInputLogFirstDivergence{
		Component: "system_section", SectionID: sectionID,
		PreviousFingerprint: before.Fingerprint, CurrentFingerprint: after.Fingerprint,
	}
}

func modelInputLogSystemSections(composition prompts.SystemPromptComposition) []modelInputLogSystemSectionFingerprint {
	fragments := composition.Fragments()
	if len(fragments) == 0 {
		return nil
	}
	sections := make([]modelInputLogSystemSectionFingerprint, 0, len(fragments))
	for _, fragment := range fragments {
		rendered := fragment.Prefix + fragment.Content + fragment.Suffix
		sections = append(sections, modelInputLogSystemSectionFingerprint{
			ID: fragment.ID, Source: fragment.Source, Purpose: fragment.Purpose,
			Fingerprint: modelInputLogFingerprint(rendered), Bytes: len(rendered),
		})
	}
	return sections
}

func firstToolFingerprintDivergence(previous, current []modelInputLogToolFingerprint) int {
	common := min(len(previous), len(current))
	for index := 0; index < common; index++ {
		if previous[index] != current[index] {
			return index
		}
	}
	return common
}

func logModelInputCacheResult(callID, runID string, usage *agent.TokenUsage) {
	if !modelInputLogEnabled.Load() || strings.TrimSpace(callID) == "" || usage == nil || usage.PromptTokens <= 0 {
		return
	}
	cached := max(0, min(usage.PromptTokenDetails.CachedTokens, usage.PromptTokens))
	record := &modelInputLogCacheResultRecord{
		Type: "llm_cache_result", Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		CallID: strings.TrimSpace(callID), RunID: strings.TrimSpace(runID),
		PromptTokens: usage.PromptTokens, CacheReadTokens: cached,
		UncachedPromptTokens:  usage.PromptTokens - cached,
		CacheReadRatio:        float64(cached) / float64(usage.PromptTokens),
		CacheWriteTokensKnown: false,
	}
	if !enqueueModelInputLogJob(modelInputLogJob{cacheResult: record}) {
		slog.InfoContext(context.Background(), fmt.Sprintf("[llm-input-log] cache result dropped call_id=%s reason=queue_full", callID))
	}
}

func modelInputLogSystemMessages(messages []*agent.Message) []*agent.Message {
	if len(messages) == 0 {
		return nil
	}
	var result []*agent.Message
	for _, message := range messages {
		if message == nil || message.Role != agent.System {
			continue
		}
		result = append(result, message)
	}
	return result
}

func modelInputLogToolNames(tools []modelInputLogTool) []string {
	if len(tools) == 0 {
		return nil
	}
	names := make([]string, 0, len(tools))
	for _, item := range tools {
		name := strings.TrimSpace(item.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func modelInputLogToolFingerprints(tools []modelInputLogTool) []modelInputLogToolFingerprint {
	if len(tools) == 0 {
		return nil
	}
	result := make([]modelInputLogToolFingerprint, 0, len(tools))
	for _, item := range tools {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		result = append(result, modelInputLogToolFingerprint{Name: name, Fingerprint: modelInputLogFingerprint(item)})
	}
	return result
}

func modelInputLogFingerprint(value any) string {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) == 0 || bytes.Equal(payload, []byte("null")) {
		return ""
	}
	sum := sha256.Sum256(payload)
	return fmt.Sprintf("sha256:%x", sum[:8])
}
