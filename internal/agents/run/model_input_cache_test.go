package agentrun

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

func TestModelInputLogCacheAttributionFingerprintsToolSchema(t *testing.T) {
	messages := []*agent.Message{
		agent.SystemMessage("system"),
		agent.UserMessage("hello"),
	}
	tools := []*agent.ToolInfo{{
		Name: "read",
		Desc: "Read a file",
		ParamsOneOf: agent.NewParamsOneOfByParams(map[string]*agent.ParameterInfo{
			"path": {Type: agent.String, Desc: "File path", Required: true},
		}),
	}}
	firstTools := modelInputLogTools(tools)
	first := modelInputLogCacheAttribution(messages, firstTools)
	second := modelInputLogCacheAttribution(messages, modelInputLogTools(tools))
	if first.MessageFingerprint == "" || first.ToolSchemaFingerprint == "" {
		t.Fatalf("fingerprints should be populated: %#v", first)
	}
	if len(first.ToolFingerprints) != 1 || first.ToolFingerprints[0].Name != "read" || first.ToolFingerprints[0].Fingerprint == "" {
		t.Fatalf("per-tool fingerprints should be populated without schema details: %#v", first.ToolFingerprints)
	}
	cachePayload, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(cachePayload), "File path") || strings.Contains(string(cachePayload), "parameters") {
		t.Fatalf("cache attribution must not expose full tool schema: %s", string(cachePayload))
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same input should produce stable attribution: first=%#v second=%#v", first, second)
	}

	changedTools := modelInputLogTools([]*agent.ToolInfo{{
		Name: "read",
		Desc: "Read a file with line offsets",
		ParamsOneOf: agent.NewParamsOneOfByParams(map[string]*agent.ParameterInfo{
			"path":   {Type: agent.String, Desc: "File path", Required: true},
			"offset": {Type: agent.Number, Desc: "Line offset"},
		}),
	}})
	changed := modelInputLogCacheAttribution(messages, changedTools)
	if changed.ToolSchemaFingerprint == first.ToolSchemaFingerprint {
		t.Fatalf("tool schema fingerprint should change when schema changes: before=%#v after=%#v", first, changed)
	}
	if changed.MessageFingerprint != first.MessageFingerprint || changed.SystemPromptFingerprint != first.SystemPromptFingerprint {
		t.Fatalf("tool-only changes should not alter message/system fingerprints: before=%#v after=%#v", first, changed)
	}
}

func TestModelInputCacheObservationLocatesFirstDivergence(t *testing.T) {
	scope := t.Name()
	messages := []*agent.Message{agent.SystemMessage("stable system"), agent.UserMessage("first turn")}
	tools := modelInputLogTools([]*agent.ToolInfo{{Name: "read", Desc: "Read files"}})
	modelConfig := providers.ModelConfig{Model: "test-model"}
	first := modelInputLogCacheObservation("call-1", scope, modelConfig, messages, tools, nil)
	if first.FirstDivergence != nil || first.CacheScopeFingerprint == "" || len(first.MessageFingerprints) != 2 {
		t.Fatalf("initial cache observation = %#v", first)
	}

	same := modelInputLogCacheObservation("call-2", scope, modelConfig, messages, tools, nil)
	if same.FirstDivergence == nil || same.FirstDivergence.Component != "none" || same.FirstDivergence.MatchingMessagePrefix != 2 {
		t.Fatalf("unchanged cache observation = %#v", same.FirstDivergence)
	}

	changedMessages := []*agent.Message{agent.SystemMessage("stable system"), agent.UserMessage("second turn")}
	changed := modelInputLogCacheObservation("call-3", scope, modelConfig, changedMessages, tools, nil)
	if changed.FirstDivergence == nil || changed.FirstDivergence.Component != "message" ||
		changed.FirstDivergence.Index != 1 || changed.FirstDivergence.MatchingMessagePrefix != 1 ||
		changed.FirstDivergence.PreviousCallID != "call-2" {
		t.Fatalf("message cache divergence = %#v", changed.FirstDivergence)
	}

	changedTools := modelInputLogTools([]*agent.ToolInfo{{Name: "read", Desc: "Read files with offsets"}})
	toolChanged := modelInputLogCacheObservation("call-4", scope, modelConfig, changedMessages, changedTools, nil)
	if toolChanged.FirstDivergence == nil || toolChanged.FirstDivergence.Component != "tool_schema" ||
		toolChanged.FirstDivergence.MatchingMessagePrefix != len(changedMessages) {
		t.Fatalf("tool cache divergence = %#v", toolChanged.FirstDivergence)
	}
}

func TestModelInputLogMessageFingerprintsIdentifyContextState(t *testing.T) {
	state := agent.UserMessage("state")
	state.Extra = map[string]any{"agent.context_state": "v1"}
	fingerprints := modelInputLogMessageFingerprints([]*agent.Message{agent.SystemMessage("system"), state})
	if len(fingerprints) != 2 || fingerprints[0].Component != "system_message" || fingerprints[1].Component != "context_state" {
		t.Fatalf("message components = %#v", fingerprints)
	}
}

func TestFirstModelInputDivergenceIdentifiesSystemSection(t *testing.T) {
	previousMessages := modelInputLogMessageFingerprints([]*agent.Message{agent.SystemMessage("alpha"), agent.UserMessage("turn")})
	currentMessages := modelInputLogMessageFingerprints([]*agent.Message{agent.SystemMessage("beta"), agent.UserMessage("turn")})
	previous := modelInputCacheBaseline{
		CallID: "call-alpha", Messages: previousMessages,
		SystemSections: []modelInputLogSystemSectionFingerprint{{ID: "workflow", Fingerprint: "alpha", Bytes: 5}},
	}
	current := modelInputCacheBaseline{
		CallID: "call-beta", Messages: currentMessages,
		SystemSections: []modelInputLogSystemSectionFingerprint{{ID: "workflow", Fingerprint: "beta", Bytes: 4}},
	}
	divergence := firstModelInputDivergence(previous, current)
	if divergence == nil || divergence.Component != "system_section" || divergence.SectionID != "workflow" ||
		divergence.Index != 0 || divergence.PreviousCallID != "call-alpha" {
		t.Fatalf("system section divergence = %#v", divergence)
	}
}
