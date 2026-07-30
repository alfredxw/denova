package agent

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// RoleType is the stable wire role of a Message.
type RoleType string

const (
	System    RoleType = "system"
	User      RoleType = "user"
	Assistant RoleType = "assistant"
	ToolRole  RoleType = "tool"
)

// FunctionCall describes a function-style tool invocation.
type FunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ToolCall uses the stable tool_calls wire shape persisted by Agent sessions.
type ToolCall struct {
	Index    *int           `json:"index,omitempty"`
	ID       string         `json:"id"`
	Type     string         `json:"type"`
	Function FunctionCall   `json:"function"`
	Extra    map[string]any `json:"extra,omitempty"`
}

// TopLogProb describes an alternative token and its log probability.
type TopLogProb struct {
	Token   string  `json:"token"`
	LogProb float64 `json:"logprob"`
	Bytes   []int64 `json:"bytes,omitempty"`
}

// LogProb describes the probability metadata for a generated token.
type LogProb struct {
	Token       string       `json:"token"`
	LogProb     float64      `json:"logprob"`
	Bytes       []int64      `json:"bytes,omitempty"`
	TopLogProbs []TopLogProb `json:"top_logprobs"`
}

// LogProbs contains token-level model response metadata.
type LogProbs struct {
	Content []LogProb `json:"content"`
}

// PromptTokenDetails contains the provider's prompt-token breakdown.
type PromptTokenDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// CompletionTokensDetails contains the provider's completion-token breakdown.
type CompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// TokenUsage is the provider-neutral usage object persisted by Agent sessions.
type TokenUsage struct {
	PromptTokens            int                     `json:"prompt_tokens"`
	PromptTokenDetails      PromptTokenDetails      `json:"prompt_token_details"`
	CompletionTokens        int                     `json:"completion_tokens"`
	TotalTokens             int                     `json:"total_tokens"`
	CompletionTokensDetails CompletionTokensDetails `json:"completion_token_details"`
}

// ResponseMeta carries provider-neutral response metadata.
type ResponseMeta struct {
	FinishReason string      `json:"finish_reason,omitempty"`
	Usage        *TokenUsage `json:"usage,omitempty"`
	LogProbs     *LogProbs   `json:"logprobs,omitempty"`
}

// ToolResultSummary preserves transcript pairing and bounded context policy
// metadata without persisting display content or the full durability payload.
type ToolResultSummary struct {
	Status              ToolResultStatus            `json:"status"`
	SyntheticReason     ToolSyntheticReason         `json:"synthetic_reason,omitempty"`
	ModelTruncated      bool                        `json:"model_truncated,omitempty"`
	DisplayTruncated    bool                        `json:"display_truncated,omitempty"`
	ResultRetention     ToolResultRetentionMode     `json:"result_retention,omitempty"`
	ContextHints        *ToolResultContextHints     `json:"context_hints,omitempty"`
	ArtifactPersistence *ToolArtifactPersistence    `json:"artifact_persistence,omitempty"`
	ProtectedReceipt    *ToolResultProtectedReceipt `json:"protected_receipt,omitempty"`
	// Deprecated replay fields.
	ContextRetention  ToolContextRetention `json:"context_retention,omitempty"`
	RetainedContent   string               `json:"retained_content,omitempty"`
	RetainedArguments string               `json:"retained_arguments,omitempty"`
	Artifacts         []ToolArtifactRef    `json:"artifacts,omitempty"`
}

// Message is the stable session and model wire type.
//
// Multimodal values stay as individual raw JSON array elements. This preserves
// fields unknown to the core while still allowing stream chunks to be appended.
type Message struct {
	Role    RoleType `json:"role"`
	Content string   `json:"content"`

	MultiContent             []json.RawMessage `json:"multi_content,omitempty"`
	UserInputMultiContent    []json.RawMessage `json:"user_input_multi_content,omitempty"`
	AssistantGenMultiContent []json.RawMessage `json:"assistant_output_multi_content,omitempty"`

	Name string `json:"name,omitempty"`

	ToolCalls  []ToolCall         `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
	ToolName   string             `json:"tool_name,omitempty"`
	ToolResult *ToolResultSummary `json:"tool_result,omitempty"`

	ResponseMeta     *ResponseMeta  `json:"response_meta,omitempty"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	Extra            map[string]any `json:"extra,omitempty"`
}

// AssistantOutputMultiContent returns the assistant multimodal wire elements.
// The persisted field name remains assistant_output_multi_content.
func (m *Message) AssistantOutputMultiContent() []json.RawMessage {
	if m == nil {
		return nil
	}
	return m.AssistantGenMultiContent
}

// SystemMessage constructs a system message.
func SystemMessage(content string) *Message {
	return &Message{Role: System, Content: content}
}

// UserMessage constructs a user message.
func UserMessage(content string) *Message {
	return &Message{Role: User, Content: content}
}

// AssistantMessage constructs an assistant message.
func AssistantMessage(content string, toolCalls []ToolCall) *Message {
	return &Message{Role: Assistant, Content: content, ToolCalls: toolCalls}
}

type toolMessageOptions struct {
	toolName string
}

// ToolMessageOption configures a ToolMessage constructor.
type ToolMessageOption func(*toolMessageOptions)

// WithToolName records the called tool's stable name on a tool result message.
func WithToolName(name string) ToolMessageOption {
	return func(options *toolMessageOptions) {
		options.toolName = name
	}
}

// ToolMessage constructs the model-context projection of a structured result.
func ToolMessage(result ToolResult, toolCallID string, opts ...ToolMessageOption) *Message {
	options := &toolMessageOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}
	return &Message{
		Role:       ToolRole,
		Content:    result.ModelContent,
		ToolCallID: toolCallID,
		ToolName:   options.toolName,
		ToolResult: &ToolResultSummary{
			Status: result.Status, SyntheticReason: result.SyntheticReason,
			ModelTruncated:      result.Metadata.ModelTruncated,
			DisplayTruncated:    result.Metadata.DisplayTruncated,
			ResultRetention:     result.ResultRetention,
			ContextHints:        cloneToolResultContextHints(result.ContextHints),
			ArtifactPersistence: cloneToolArtifactPersistence(result.Metadata.ArtifactPersistence),
			ProtectedReceipt:    cloneToolResultProtectedReceipt(result.ProtectedReceipt),
			ContextRetention:    result.ContextRetention,
			RetainedContent:     result.RetainedContent,
			RetainedArguments:   result.RetainedArguments,
			Artifacts:           append([]ToolArtifactRef(nil), result.Artifacts...),
		},
	}
}

// EffectiveToolResult reconstructs a structured result from a transcript. Old
// histories without a summary remain readable as successful text results.
func (m *Message) EffectiveToolResult() ToolResult {
	if m == nil || m.Role != ToolRole {
		return ToolResult{}
	}
	result := TextToolResult(m.Content)
	if m.ToolResult != nil {
		if m.ToolResult.Status != "" {
			result.Status = m.ToolResult.Status
		}
		result.SyntheticReason = m.ToolResult.SyntheticReason
		result.Metadata.ModelTruncated = m.ToolResult.ModelTruncated
		result.Metadata.DisplayTruncated = m.ToolResult.DisplayTruncated
		result.ResultRetention = m.ToolResult.ResultRetention
		result.ContextHints = cloneToolResultContextHints(m.ToolResult.ContextHints)
		result.Metadata.ArtifactPersistence = cloneToolArtifactPersistence(m.ToolResult.ArtifactPersistence)
		result.ProtectedReceipt = cloneToolResultProtectedReceipt(m.ToolResult.ProtectedReceipt)
		result.ContextRetention = m.ToolResult.ContextRetention
		result.RetainedContent = m.ToolResult.RetainedContent
		result.RetainedArguments = m.ToolResult.RetainedArguments
		result.Artifacts = append([]ToolArtifactRef(nil), m.ToolResult.Artifacts...)
	}
	return result
}

// Clone returns a deep, independently mutable copy of m.
func (m *Message) Clone() *Message {
	if m == nil {
		return nil
	}
	clone := *m
	clone.MultiContent = cloneRawMessages(m.MultiContent)
	clone.UserInputMultiContent = cloneRawMessages(m.UserInputMultiContent)
	clone.AssistantGenMultiContent = cloneRawMessages(m.AssistantGenMultiContent)
	clone.ToolCalls = cloneToolCalls(m.ToolCalls)
	if m.ToolResult != nil {
		result := *m.ToolResult
		result.Artifacts = append([]ToolArtifactRef(nil), m.ToolResult.Artifacts...)
		result.ContextHints = cloneToolResultContextHints(m.ToolResult.ContextHints)
		result.ArtifactPersistence = cloneToolArtifactPersistence(m.ToolResult.ArtifactPersistence)
		result.ProtectedReceipt = cloneToolResultProtectedReceipt(m.ToolResult.ProtectedReceipt)
		clone.ToolResult = &result
	}
	clone.Extra = cloneStringAnyMap(m.Extra)
	clone.ResponseMeta = cloneResponseMeta(m.ResponseMeta)
	return &clone
}

func cloneToolResultContextHints(hints *ToolResultContextHints) *ToolResultContextHints {
	if hints == nil {
		return nil
	}
	clone := *hints
	clone.Recovery.Reference = cloneStringAnyMap(hints.Recovery.Reference)
	return &clone
}

func cloneToolArtifactPersistence(persistence *ToolArtifactPersistence) *ToolArtifactPersistence {
	if persistence == nil {
		return nil
	}
	clone := *persistence
	return &clone
}

func cloneToolResultProtectedReceipt(receipt *ToolResultProtectedReceipt) *ToolResultProtectedReceipt {
	if receipt == nil {
		return nil
	}
	clone := *receipt
	return &clone
}

// CloneMessage is the function form of Message.Clone.
func CloneMessage(message *Message) *Message {
	return message.Clone()
}

func cloneRawMessages(values []json.RawMessage) []json.RawMessage {
	if values == nil {
		return nil
	}
	result := make([]json.RawMessage, len(values))
	for index, value := range values {
		result[index] = append(json.RawMessage(nil), value...)
	}
	return result
}

func cloneToolCalls(values []ToolCall) []ToolCall {
	if values == nil {
		return nil
	}
	result := make([]ToolCall, len(values))
	for index, value := range values {
		result[index] = value
		if value.Index != nil {
			callIndex := *value.Index
			result[index].Index = &callIndex
		}
		result[index].Extra = cloneStringAnyMap(value.Extra)
	}
	return result
}

func cloneResponseMeta(meta *ResponseMeta) *ResponseMeta {
	if meta == nil {
		return nil
	}
	clone := *meta
	if meta.Usage != nil {
		usage := *meta.Usage
		clone.Usage = &usage
	}
	if meta.LogProbs != nil {
		logProbs := &LogProbs{Content: append([]LogProb(nil), meta.LogProbs.Content...)}
		for index := range logProbs.Content {
			logProbs.Content[index].Bytes = append([]int64(nil), meta.LogProbs.Content[index].Bytes...)
			logProbs.Content[index].TopLogProbs = append([]TopLogProb(nil), meta.LogProbs.Content[index].TopLogProbs...)
			for alternativeIndex := range logProbs.Content[index].TopLogProbs {
				logProbs.Content[index].TopLogProbs[alternativeIndex].Bytes = append(
					[]int64(nil),
					meta.LogProbs.Content[index].TopLogProbs[alternativeIndex].Bytes...,
				)
			}
		}
		clone.LogProbs = logProbs
	}
	return &clone
}

func cloneStringAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = cloneAny(value)
	}
	return result
}

func cloneAny(value any) any {
	if value == nil {
		return nil
	}
	return cloneReflectValue(reflect.ValueOf(value)).Interface()
}

func cloneReflectValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		clone := cloneReflectValue(value.Elem())
		result := reflect.New(value.Type()).Elem()
		result.Set(clone)
		return result
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.New(value.Type().Elem())
		result.Elem().Set(cloneReflectValue(value.Elem()))
		return result
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			result.SetMapIndex(iterator.Key(), cloneReflectValue(iterator.Value()))
		}
		return result
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		result := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			result.Index(index).Set(cloneReflectValue(value.Index(index)))
		}
		return result
	case reflect.Array:
		result := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			result.Index(index).Set(cloneReflectValue(value.Index(index)))
		}
		return result
	default:
		return value
	}
}

// MessageAssembler incrementally applies the same strict rules as
// ConcatMessages. A failed append does not mutate the accepted prefix.
type MessageAssembler struct {
	chunks []*Message
}

// NewMessageAssembler returns an empty message assembler.
func NewMessageAssembler() *MessageAssembler {
	return &MessageAssembler{}
}

// Append validates and appends one message chunk.
func (assembler *MessageAssembler) Append(chunk *Message) error {
	if chunk == nil {
		return fmt.Errorf("append message chunk: nil chunk")
	}
	candidate := append(append([]*Message(nil), assembler.chunks...), chunk)
	if _, err := ConcatMessages(candidate); err != nil {
		return err
	}
	assembler.chunks = candidate
	return nil
}

// Message returns the merged accepted chunks.
func (assembler *MessageAssembler) Message() (*Message, error) {
	return ConcatMessages(assembler.chunks)
}

// Reset discards all accepted chunks.
func (assembler *MessageAssembler) Reset() {
	assembler.chunks = nil
}

// ConcatMessages merges streaming message chunks without silently resolving
// identity conflicts.
func ConcatMessages(messages []*Message) (*Message, error) {
	result := &Message{}
	var content strings.Builder
	var reasoning strings.Builder
	var calls []ToolCall

	for index, message := range messages {
		if message == nil {
			return nil, fmt.Errorf("concat messages: nil chunk at index %d", index)
		}
		if err := mergeIdentity("role", string(message.Role), (*stringRole)(&result.Role)); err != nil {
			return nil, err
		}
		if err := mergeStringIdentity("name", message.Name, &result.Name); err != nil {
			return nil, err
		}
		if err := mergeStringIdentity("tool call ID", message.ToolCallID, &result.ToolCallID); err != nil {
			return nil, err
		}
		if err := mergeStringIdentity("tool name", message.ToolName, &result.ToolName); err != nil {
			return nil, err
		}

		content.WriteString(message.Content)
		reasoning.WriteString(message.ReasoningContent)
		calls = append(calls, message.ToolCalls...)
		result.MultiContent = append(result.MultiContent, cloneRawMessages(message.MultiContent)...)
		result.UserInputMultiContent = append(result.UserInputMultiContent, cloneRawMessages(message.UserInputMultiContent)...)
		result.AssistantGenMultiContent = append(result.AssistantGenMultiContent, cloneRawMessages(message.AssistantGenMultiContent)...)

		var err error
		result.Extra, err = mergeExtraMaps(result.Extra, message.Extra)
		if err != nil {
			return nil, fmt.Errorf("concat message extra: %w", err)
		}
		mergeResponseMeta(&result.ResponseMeta, message.ResponseMeta)
	}

	result.Content = content.String()
	result.ReasoningContent = reasoning.String()
	if len(calls) != 0 {
		merged, err := concatToolCalls(calls)
		if err != nil {
			return nil, err
		}
		result.ToolCalls = merged
	}
	return result, nil
}

type stringRole RoleType

func mergeIdentity(label, incoming string, target *stringRole) error {
	current := string(*target)
	if incoming == "" {
		return nil
	}
	if current != "" && current != incoming {
		return fmt.Errorf("concat messages: conflicting %s %q and %q", label, current, incoming)
	}
	*target = stringRole(incoming)
	return nil
}

func mergeStringIdentity(label, incoming string, target *string) error {
	if incoming == "" {
		return nil
	}
	if *target != "" && *target != incoming {
		return fmt.Errorf("concat messages: conflicting %s %q and %q", label, *target, incoming)
	}
	*target = incoming
	return nil
}

func concatToolCalls(chunks []ToolCall) ([]ToolCall, error) {
	withoutIndex := make([]ToolCall, 0)
	byIndex := make(map[int][]ToolCall)
	indices := make([]int, 0)
	seen := make(map[int]struct{})
	for _, chunk := range chunks {
		if chunk.Index == nil {
			withoutIndex = append(withoutIndex, cloneToolCalls([]ToolCall{chunk})[0])
			continue
		}
		index := *chunk.Index
		if _, exists := seen[index]; !exists {
			seen[index] = struct{}{}
			indices = append(indices, index)
		}
		byIndex[index] = append(byIndex[index], chunk)
	}
	sort.Ints(indices)
	result := withoutIndex
	for _, index := range indices {
		merged, err := mergeIndexedToolCall(index, byIndex[index])
		if err != nil {
			return nil, err
		}
		result = append(result, merged)
	}
	return result, nil
}

func mergeIndexedToolCall(index int, chunks []ToolCall) (ToolCall, error) {
	merged := ToolCall{Index: &index}
	var arguments strings.Builder
	for _, chunk := range chunks {
		if err := mergeStringIdentity(fmt.Sprintf("tool call[%d] ID", index), chunk.ID, &merged.ID); err != nil {
			return ToolCall{}, err
		}
		if err := mergeStringIdentity(fmt.Sprintf("tool call[%d] type", index), chunk.Type, &merged.Type); err != nil {
			return ToolCall{}, err
		}
		if err := mergeStringIdentity(fmt.Sprintf("tool call[%d] name", index), chunk.Function.Name, &merged.Function.Name); err != nil {
			return ToolCall{}, err
		}
		arguments.WriteString(chunk.Function.Arguments)
		var err error
		merged.Extra, err = mergeExtraMaps(merged.Extra, chunk.Extra)
		if err != nil {
			return ToolCall{}, fmt.Errorf("concat tool call[%d] extra: %w", index, err)
		}
	}
	merged.Function.Arguments = arguments.String()
	return merged, nil
}

func mergeResponseMeta(target **ResponseMeta, incoming *ResponseMeta) {
	if incoming == nil {
		return
	}
	if *target == nil {
		*target = &ResponseMeta{}
	}
	if incoming.FinishReason != "" {
		(*target).FinishReason = incoming.FinishReason
	}
	if incoming.Usage != nil {
		if (*target).Usage == nil {
			(*target).Usage = &TokenUsage{}
		}
		mergeUsage((*target).Usage, incoming.Usage)
	}
	if incoming.LogProbs != nil {
		if (*target).LogProbs == nil {
			(*target).LogProbs = &LogProbs{}
		}
		copy := cloneResponseMeta(&ResponseMeta{LogProbs: incoming.LogProbs}).LogProbs
		(*target).LogProbs.Content = append((*target).LogProbs.Content, copy.Content...)
	}
}

func mergeUsage(target, incoming *TokenUsage) {
	target.PromptTokens = max(target.PromptTokens, incoming.PromptTokens)
	target.CompletionTokens = max(target.CompletionTokens, incoming.CompletionTokens)
	target.TotalTokens = max(target.TotalTokens, incoming.TotalTokens)
	target.PromptTokenDetails.CachedTokens = max(
		target.PromptTokenDetails.CachedTokens,
		incoming.PromptTokenDetails.CachedTokens,
	)
	target.CompletionTokensDetails.ReasoningTokens = max(
		target.CompletionTokensDetails.ReasoningTokens,
		incoming.CompletionTokensDetails.ReasoningTokens,
	)
}

func mergeExtraMaps(current, incoming map[string]any) (map[string]any, error) {
	if incoming == nil {
		return current, nil
	}
	if current == nil {
		return cloneStringAnyMap(incoming), nil
	}
	result := cloneStringAnyMap(current)
	for key, incomingValue := range incoming {
		currentValue, exists := result[key]
		if !exists {
			result[key] = cloneAny(incomingValue)
			continue
		}
		merged, err := mergeExtraValue(currentValue, incomingValue)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", key, err)
		}
		result[key] = merged
	}
	return result, nil
}

func mergeExtraValue(current, incoming any) (any, error) {
	if currentMap, ok := current.(map[string]any); ok {
		incomingMap, compatible := incoming.(map[string]any)
		if !compatible {
			return nil, fmt.Errorf("conflicting extra types %T and %T", current, incoming)
		}
		return mergeExtraMaps(currentMap, incomingMap)
	}
	if currentString, ok := current.(string); ok {
		incomingString, compatible := incoming.(string)
		if !compatible {
			return nil, fmt.Errorf("conflicting extra types %T and %T", current, incoming)
		}
		return currentString + incomingString, nil
	}
	if reflect.TypeOf(current) != reflect.TypeOf(incoming) {
		return nil, fmt.Errorf("conflicting extra types %T and %T", current, incoming)
	}
	if reflect.DeepEqual(current, incoming) {
		return cloneAny(incoming), nil
	}
	value := reflect.ValueOf(incoming)
	if value.IsValid() {
		switch value.Kind() {
		case reflect.Bool,
			reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			return cloneAny(incoming), nil
		}
	}
	return nil, fmt.Errorf("cannot merge multiple non-zero values of type %T", incoming)
}
