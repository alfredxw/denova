// Package compat contains compatibility repairs for non-standard providers
// that expose an OpenAI-compatible Chat Completions endpoint.
package compat

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

// Config contains only the endpoint traits needed by compatibility repairs.
type Config struct {
	Provider      providers.ProviderID
	BaseURL       string
	Model         string
	ThinkingLevel providers.ThinkingLevel
}

// Wrap returns a possibly-decorated chat model that hides provider-specific
// quirks. If the model needs no polyfill, the original is returned untouched.
func Wrap(cm agent.ToolCallingChatModel, cfg Config) agent.ToolCallingChatModel {
	if polyfills := detect(cfg); len(polyfills) > 0 {
		slog.InfoContext(context.Background(), fmt.Sprintf("[providercompat] Wrap called: applying %d polyfill(s) model=%q", len(polyfills), cfg.Model))
		cm = chain(cm, polyfills)
	}
	return cm
}

// ExtraRequestFields returns provider-specific fields that should be merged
// into the request body (e.g. reasoning_split to ask the API to return
// thinking via the standard reasoning_content field). Called once when
// building the chat model config, before any request is sent.
func ExtraRequestFields(cfg Config) map[string]any {
	out := map[string]any{}
	if needsRepair(cfg) {
		// Ask the provider to return thinking via the standard
		// reasoning_content field, instead of embedding it in content.
		out["reasoning_split"] = true
	}
	return out
}

// ThinkingExtraFields maps Denova's unified level to provider-specific
// thinking toggles that are not represented by the OpenAI SDK. DeepSeek V4
// uses the nested thinking object; legacy compatible endpoints continue to use
// enable_thinking. Official OpenAI and Gemini endpoints need neither field.
func ThinkingExtraFields(cfg Config) map[string]any {
	if cfg.ThinkingLevel == "" || cfg.ThinkingLevel == providers.ThinkingLevelDefault {
		return nil
	}
	if cfg.Provider == providers.ProviderDeepSeek {
		mode := "enabled"
		if cfg.ThinkingLevel == providers.ThinkingLevelOff {
			mode = "disabled"
		}
		return map[string]any{"thinking": map[string]any{"type": mode}}
	}
	if cfg.Provider == providers.ProviderOpenAI || usesGeminiOpenAICompatibility(cfg) {
		return nil
	}
	return map[string]any{"enable_thinking": cfg.ThinkingLevel != providers.ThinkingLevelOff}
}

type polyfill interface {
	apply(agent.ToolCallingChatModel) agent.ToolCallingChatModel
}

// detect inspects the config and returns the polyfill chain to apply.
// Order matters: later polyfills see output of earlier ones.
func detect(cfg Config) []polyfill {
	var out []polyfill
	if needsRepair(cfg) {
		// Both polyfills needed: tool-call text-to-struct, then think-tag cleanup
		// (in case reasoning_split is ignored or falls back to inline tags).
		out = append(out, toolCallTextPolyfill{})
		out = append(out, inlineThinkPolyfill{})
	}
	return out
}

func chain(cm agent.ToolCallingChatModel, ps []polyfill) agent.ToolCallingChatModel {
	for _, p := range ps {
		cm = p.apply(cm)
	}
	return cm
}

// needsRepair returns true when the provider's OpenAI-compatible endpoint
// does not return standard tool_calls or wraps thinking in <think> tags.
// Detection is by base URL or model name matching a known non-standard
// marker. "minimax" is a known host keyword of an OpenAI-compatible
// provider that exhibits these quirks; "non-standard" and
// "incompatible" are generic markers users can opt into via their
// base URL or model name. Cheap, called once per Wrap.
func needsRepair(cfg Config) bool {
	base := strings.ToLower(cfg.BaseURL)
	model := strings.ToLower(cfg.Model)
	for _, marker := range []string{"minimax", "non-standard", "incompatible"} {
		if strings.Contains(base, marker) || strings.Contains(model, marker) {
			return true
		}
	}
	return false
}

func usesGeminiOpenAICompatibility(cfg Config) bool {
	base := strings.ToLower(strings.TrimSpace(cfg.BaseURL))
	model := strings.ToLower(strings.TrimSpace(cfg.Model))
	return strings.Contains(base, "generativelanguage.googleapis.com") ||
		strings.Contains(base, "aiplatform.googleapis.com") ||
		strings.HasPrefix(model, "gemini-")
}

// -----------------------------------------------------------------------------
// Polyfill 1: tool calls delivered as inline text instead of structured
// tool_calls. We parse the antml-style <tool_call><invoke name="...">…</invoke> </tool_call>
// XML and promote them to agent.ToolCall so the framework actually executes
// the tools.
// -----------------------------------------------------------------------------

type toolCallTextPolyfill struct{}

var (
	pcInvokeRe    = regexp.MustCompile(`(?s)<invoke\s+name="([^"]+)"\s*>(.*?)</invoke>`)
	pcToolCallRe  = regexp.MustCompile(`(?s)<tool_call>(.*?)</tool_call>`)
	pcParamNamedR = regexp.MustCompile(`(?s)<parameter\s+name="([^"]+)"\s*>(.*?)</parameter>`)
	pcParamTagR   = regexp.MustCompile(`(?s)<([a-zA-Z_][\w.-]*)>(.*?)</[a-zA-Z_][\w.-]*>`)
)

func (toolCallTextPolyfill) apply(inner agent.ToolCallingChatModel) agent.ToolCallingChatModel {
	return &toolCallTextModel{inner: inner}
}

type toolCallTextModel struct{ inner agent.ToolCallingChatModel }

func (m *toolCallTextModel) Generate(ctx context.Context, in []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	msg, err := m.inner.Generate(ctx, in, opts...)
	if err != nil || msg == nil {
		return msg, err
	}
	extractTextToolCalls(msg)
	return msg, nil
}

func (m *toolCallTextModel) Stream(ctx context.Context, in []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	sr, err := m.inner.Stream(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	repair := &textToolCallStreamRepair{}
	return transformMessageStream(sr, repair.push, repair.flush), nil
}

func (m *toolCallTextModel) WithTools(tools []*agent.ToolInfo) (agent.ToolCallingChatModel, error) {
	inner, err := m.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &toolCallTextModel{inner: inner}, nil
}

func extractTextToolCalls(msg *agent.Message) {
	if msg == nil || len(msg.ToolCalls) > 0 || msg.Content == "" {
		return
	}
	matches := pcInvokeRe.FindAllStringSubmatch(msg.Content, -1)
	if len(matches) == 0 {
		return
	}
	calls := make([]agent.ToolCall, 0, len(matches))
	for i, m := range matches {
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		params := parseInvokeParams(m[2])
		args, _ := json.Marshal(params)
		idx := i
		calls = append(calls, agent.ToolCall{
			Index: &idx,
			ID:    newTextToolCallID(),
			Type:  "function",
			Function: agent.FunctionCall{
				Name:      name,
				Arguments: string(args),
			},
		})
	}
	if len(calls) == 0 {
		return
	}
	msg.ToolCalls = calls
	msg.Content = pcToolCallRe.ReplaceAllString(msg.Content, "")
	msg.Content = pcInvokeRe.ReplaceAllString(msg.Content, "")
}

func newTextToolCallID() string {
	// The synthetic ID becomes part of the durable transcript. Randomness keeps
	// repeated textual calls distinct across turns, model instances, and process
	// restarts instead of resetting an in-memory counter on every response.
	return "text_tool_call_" + strings.ToLower(rand.Text())
}

func parseInvokeParams(body string) map[string]string {
	out := map[string]string{}
	if named := pcParamNamedR.FindAllStringSubmatch(body, -1); len(named) > 0 {
		for _, m := range named {
			if k := strings.TrimSpace(m[1]); k != "" {
				out[k] = strings.TrimSpace(m[2])
			}
		}
		return out
	}
	for _, m := range pcParamTagR.FindAllStringSubmatch(body, -1) {
		k := strings.TrimSpace(m[1])
		if k == "" || strings.EqualFold(k, "parameter") {
			continue
		}
		out[k] = strings.TrimSpace(m[2])
	}
	return out
}

// -----------------------------------------------------------------------------
// Polyfill 2: some providers (or fallback paths) still emit <think>…</think>
// inline. Strip them from content and surface as ReasoningContent if missing.
// -----------------------------------------------------------------------------

type inlineThinkPolyfill struct{}

func (inlineThinkPolyfill) apply(inner agent.ToolCallingChatModel) agent.ToolCallingChatModel {
	return &inlineThinkModel{inner: inner}
}

type inlineThinkModel struct{ inner agent.ToolCallingChatModel }

func (m *inlineThinkModel) Generate(ctx context.Context, in []*agent.Message, opts ...agent.ModelOption) (*agent.Message, error) {
	msg, err := m.inner.Generate(ctx, in, opts...)
	if err != nil || msg == nil {
		return msg, err
	}
	stripInlineThink(msg)
	return msg, nil
}

func (m *inlineThinkModel) Stream(ctx context.Context, in []*agent.Message, opts ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	sr, err := m.inner.Stream(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	repair := &inlineThinkStreamRepair{}
	return transformMessageStream(sr, repair.push, repair.flush), nil
}

func (m *inlineThinkModel) WithTools(tools []*agent.ToolInfo) (agent.ToolCallingChatModel, error) {
	inner, err := m.inner.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &inlineThinkModel{inner: inner}, nil
}

func stripInlineThink(msg *agent.Message) {
	if msg == nil || msg.Content == "" {
		return
	}
	clean, thinking := stripThinkTagsSimple(msg.Content)
	if thinking != "" && strings.TrimSpace(msg.ReasoningContent) == "" {
		msg.ReasoningContent = thinking
	}
	msg.Content = clean
}

// stripThinkTagsSimple removes paired/unclosed <think>…</think> and orphan </think>
// prelude in one shot. Used on whole-message content (post-stream concat), so
// regex is fine — no cross-chunk state to maintain. The agent package's
// thinkTagExtractor handles the streaming variant separately.
func stripThinkTagsSimple(s string) (content, thinking string) {
	// paired <think>…</think> (lazy, may not find anything if unclosed)
	paired := regexp.MustCompile(`(?is)<think>(.*?)(?:</think>|$)`)
	matches := paired.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		// no <think> opener: maybe an orphan </think> prelude
		if idx := strings.Index(strings.ToLower(s), "</think>"); idx >= 0 {
			prelude := strings.TrimSpace(s[:idx])
			if prelude != "" {
				thinking = prelude
			}
			content = strings.TrimLeft(s[idx+len("</think>"):], " \t\r\n")
		} else {
			content = s
		}
		return content, thinking
	}
	var contentBuilder, thinkBuilder strings.Builder
	last := 0
	for _, m := range matches {
		if m[0] > last {
			contentBuilder.WriteString(s[last:m[0]])
		}
		thinkBuilder.WriteString(s[m[2]:m[3]])
		last = m[1]
	}
	contentBuilder.WriteString(s[last:])
	// also strip any orphan </think> remaining in the content tail
	content = paired.ReplaceAllString(contentBuilder.String(), "")
	// and any orphan </think> fragments
	content = regexp.MustCompile(`(?i)\n?</think>\s*`).ReplaceAllString(content, "")
	content = strings.TrimLeft(content, " \t\r\n")
	return content, thinkBuilder.String()
}

type streamEmitFunc func(*agent.Message)

func transformMessageStream(
	input *agent.StreamReader[*agent.Message],
	push func(*agent.Message, streamEmitFunc),
	flush func(streamEmitFunc),
) *agent.StreamReader[*agent.Message] {
	output, writer := agent.Pipe[*agent.Message](1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				writer.Send(nil, fmt.Errorf("providercompat stream transform panic: %v", recovered))
			}
			writer.Close()
			input.Close()
		}()
		emit := func(message *agent.Message) {
			if message == nil || emptyMessage(message) {
				return
			}
			writer.Send(message, nil)
		}
		for {
			frame, err := input.Recv()
			if errors.Is(err, io.EOF) {
				if flush != nil {
					flush(emit)
				}
				return
			}
			if err != nil {
				writer.Send(nil, err)
				return
			}
			if push != nil {
				push(frame, emit)
			} else {
				emit(frame)
			}
		}
	}()
	return output
}

func emptyMessage(message *agent.Message) bool {
	return message.Content == "" &&
		message.ReasoningContent == "" &&
		len(message.ToolCalls) == 0 &&
		len(message.MultiContent) == 0 &&
		len(message.UserInputMultiContent) == 0 &&
		len(message.AssistantGenMultiContent) == 0 &&
		message.ResponseMeta == nil
}

type textToolCallStreamRepair struct {
	buffer     string
	inToolCall bool
}

func (r *textToolCallStreamRepair) push(frame *agent.Message, emit streamEmitFunc) {
	if frame == nil {
		return
	}
	if frame.Content == "" || len(frame.ToolCalls) > 0 {
		emit(frame)
		return
	}
	r.buffer += frame.Content
	r.drain(frame, emit)
}

func (r *textToolCallStreamRepair) flush(emit streamEmitFunc) {
	if r.buffer == "" {
		return
	}
	r.emitToolAwareContent(&agent.Message{Role: agent.Assistant}, r.buffer, emit)
	r.buffer = ""
	r.inToolCall = false
}

func (r *textToolCallStreamRepair) drain(base *agent.Message, emit streamEmitFunc) {
	const openTag = "<tool_call>"
	const closeTag = "</tool_call>"
	for r.buffer != "" {
		if r.inToolCall {
			closeIndex := indexFold(r.buffer, closeTag)
			if closeIndex < 0 {
				return
			}
			end := closeIndex + len(closeTag)
			r.emitToolAwareContent(base, r.buffer[:end], emit)
			r.buffer = r.buffer[end:]
			r.inToolCall = false
			continue
		}

		openIndex := indexFold(r.buffer, openTag)
		if openIndex >= 0 {
			if openIndex > 0 {
				emit(messageWithContent(base, r.buffer[:openIndex]))
			}
			r.buffer = r.buffer[openIndex:]
			r.inToolCall = true
			continue
		}
		keep := partialTagSuffixLength(r.buffer, openTag)
		if keep > 0 {
			if len(r.buffer) > keep {
				emit(messageWithContent(base, r.buffer[:len(r.buffer)-keep]))
				r.buffer = r.buffer[len(r.buffer)-keep:]
			}
			return
		}
		emit(messageWithContent(base, r.buffer))
		r.buffer = ""
	}
}

func (r *textToolCallStreamRepair) emitToolAwareContent(base *agent.Message, content string, emit streamEmitFunc) {
	if content == "" {
		return
	}
	message := messageWithContent(base, content)
	extractTextToolCalls(message)
	emit(message)
}

type inlineThinkStreamRepair struct {
	buffer  string
	inThink bool
}

func (r *inlineThinkStreamRepair) push(frame *agent.Message, emit streamEmitFunc) {
	if frame == nil {
		return
	}
	if frame.Content == "" {
		emit(frame)
		return
	}
	if frame.ReasoningContent != "" {
		emit(messageWithThinking(frame, frame.ReasoningContent))
	}
	r.buffer += frame.Content
	r.drain(frame, emit)
}

func (r *inlineThinkStreamRepair) flush(emit streamEmitFunc) {
	if r.buffer == "" {
		return
	}
	base := &agent.Message{Role: agent.Assistant}
	if r.inThink {
		emit(messageWithThinking(base, r.buffer))
	} else {
		clean, thinking := stripThinkTagsSimple(r.buffer)
		if thinking != "" {
			emit(messageWithThinking(base, thinking))
		}
		if clean != "" {
			emit(messageWithContent(base, clean))
		}
	}
	r.buffer = ""
	r.inThink = false
}

func (r *inlineThinkStreamRepair) drain(base *agent.Message, emit streamEmitFunc) {
	const openTag = "<think>"
	const closeTag = "</think>"
	for r.buffer != "" {
		if r.inThink {
			closeIndex := indexFold(r.buffer, closeTag)
			if closeIndex >= 0 {
				if closeIndex > 0 {
					emit(messageWithThinking(base, r.buffer[:closeIndex]))
				}
				r.buffer = strings.TrimLeft(r.buffer[closeIndex+len(closeTag):], " \t\r\n")
				r.inThink = false
				continue
			}
			keep := partialTagSuffixLength(r.buffer, closeTag)
			if keep > 0 {
				if len(r.buffer) > keep {
					emit(messageWithThinking(base, r.buffer[:len(r.buffer)-keep]))
					r.buffer = r.buffer[len(r.buffer)-keep:]
				}
				return
			}
			emit(messageWithThinking(base, r.buffer))
			r.buffer = ""
			return
		}

		openIndex := indexFold(r.buffer, openTag)
		closeIndex := indexFold(r.buffer, closeTag)
		if closeIndex >= 0 && (openIndex < 0 || closeIndex < openIndex) {
			if closeIndex > 0 {
				emit(messageWithThinking(base, r.buffer[:closeIndex]))
			}
			r.buffer = strings.TrimLeft(r.buffer[closeIndex+len(closeTag):], " \t\r\n")
			continue
		}
		if openIndex >= 0 {
			if openIndex > 0 {
				emit(messageWithContent(base, r.buffer[:openIndex]))
			}
			r.buffer = r.buffer[openIndex+len(openTag):]
			r.inThink = true
			continue
		}
		keep := partialTagSuffixLength(r.buffer, openTag, closeTag)
		if keep > 0 {
			if len(r.buffer) > keep {
				emit(messageWithContent(base, r.buffer[:len(r.buffer)-keep]))
				r.buffer = r.buffer[len(r.buffer)-keep:]
			}
			return
		}
		emit(messageWithContent(base, r.buffer))
		r.buffer = ""
	}
}

func messageWithContent(base *agent.Message, content string) *agent.Message {
	if base == nil {
		return &agent.Message{Role: agent.Assistant, Content: content}
	}
	message := *base
	message.Content = content
	message.ToolCalls = nil
	message.ReasoningContent = ""
	return &message
}

func messageWithThinking(base *agent.Message, thinking string) *agent.Message {
	if base == nil {
		return &agent.Message{Role: agent.Assistant, ReasoningContent: thinking}
	}
	message := *base
	message.Content = ""
	message.ToolCalls = nil
	message.ReasoningContent = thinking
	return &message
}

func indexFold(value, needle string) int {
	return strings.Index(strings.ToLower(value), strings.ToLower(needle))
}

func partialTagSuffixLength(value string, tags ...string) int {
	maxLength := 0
	for _, tag := range tags {
		limit := len(tag) - 1
		if len(value) < limit {
			limit = len(value)
		}
		for length := limit; length > 0; length-- {
			if strings.EqualFold(value[len(value)-length:], tag[:length]) {
				if length > maxLength {
					maxLength = length
				}
				break
			}
		}
	}
	return maxLength
}
