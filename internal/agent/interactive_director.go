package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/book"
	"denova/internal/prompts"
	"denova/internal/session"
)

const interactiveDirectorAgentLabel = "interactive-director-agent"

const (
	directorPlanHiddenNotice = "chapter_body_hidden"
	directorPlanHiddenReason = "director_plan_body"
	directorPlanProgressStep = 100
)

func GenerateInteractiveDirector(ctx context.Context, cfg *config.Config, instruction string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	var runErr error
	traceCtx, finishTrace := withStandaloneRunTrace(ctx, cfg, config.AgentKindInteractiveDirector, "interactive_director", "generate", map[string]any{
		"instruction_chars": len([]rune(instruction)),
	})
	defer func() { finishTrace(runErr) }()
	modelCfg := chatModelConfigForAgent(cfg, config.AgentKindInteractiveDirector)
	log.Printf("[%s] generate begin instruction=%s", interactiveDirectorAgentLabel, promptPartSummary(instruction))
	messages := []*schema.Message{
		schema.SystemMessage(protectedSystemInstruction(cfg, config.AgentKindInteractiveDirector, prompts.BuildInteractiveDirectorSystemInstruction())),
		schema.UserMessage(instruction),
	}
	content, err := generateWithJSONFallback(traceCtx, modelCfg, messages, config.AgentKindInteractiveDirector, "interactive_director", interactiveDirectorAgentLabel)
	if err != nil {
		runErr = err
		return "", fmt.Errorf("生成互动导演状态失败: %w", err)
	}
	log.Printf("[%s] generate done output=%s", interactiveDirectorAgentLabel, promptPartSummary(content))
	return content, nil
}

func GenerateInteractiveDirectorWithTools(ctx context.Context, cfg *config.Config, state *book.State, toolContext InteractiveStoryToolContext, instruction string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("配置不存在")
	}
	if state == nil {
		return GenerateInteractiveDirector(ctx, cfg, instruction)
	}
	builtAgent, err := BuildInteractiveDirector(ctx, cfg, state, toolContext)
	if err != nil {
		return "", fmt.Errorf("构建互动导演 Agent 失败: %w", err)
	}
	runner := NewRunnerWithOptions(ctx, builtAgent, RunOptions{AgentKind: config.AgentKindInteractiveDirector, Workspace: cfg.Workspace})
	conversation := &singleInstructionConversation{
		instruction:           instruction,
		display:               toolContext.DisplayConversation,
		hideDirectorToolInput: cfg.HideChapterBodyLiveOutput,
		directorTools:         map[string]*directorToolDisplayState{},
	}
	bookService := book.NewService(state.Workspace())
	var runErr error
	NewChatService().RunWithOptions(ctx, runner, conversation, bookService, ChatRequest{Message: instruction}, RunOptions{
		AgentKind:          config.AgentKindInteractiveDirector,
		Workspace:          cfg.Workspace,
		ToolResultMaxBytes: 16 * 1024,
	}, func(event Event) {
		if event.Type != "error" {
			return
		}
		if data, ok := event.Data.(map[string]string); ok {
			runErr = fmt.Errorf("%s", data["message"])
		}
	})
	if runErr != nil {
		return "", runErr
	}
	output := strings.TrimSpace(conversation.output)
	if output == "" {
		return "", fmt.Errorf("互动导演 Agent 返回为空")
	}
	return output, nil
}

type singleInstructionConversation struct {
	instruction           string
	output                string
	display               Conversation
	hideDirectorToolInput bool
	mu                    sync.Mutex
	directorTools         map[string]*directorToolDisplayState
}

func (c *singleInstructionConversation) PrepareMessages(_, agentMessage string) ([]*schema.Message, error) {
	message := strings.TrimSpace(agentMessage)
	if message == "" {
		message = c.instruction
	}
	return []*schema.Message{schema.UserMessage(message)}, nil
}

func (c *singleInstructionConversation) AppendAssistant(content string) error {
	c.output = content
	return nil
}

func (c *singleInstructionConversation) MarkInterrupted(_, assistantContent, _ string) error {
	c.output = assistantContent
	return nil
}

func (c *singleInstructionConversation) PendingInterruption() *session.Interruption {
	return nil
}

func (c *singleInstructionConversation) ResolveInterruption(string) error {
	return nil
}

func (c *singleInstructionConversation) AppendDisplayEvent(event session.DisplayEvent) error {
	if c == nil {
		return nil
	}
	event = decorateDirectorDisplayEvent(event)
	if c.hideDirectorToolInput && directorPlanWriteTool(event.Name) {
		event = c.recordDirectorToolEvent(event)
	}
	return c.forwardDisplayEvent(event)
}

func (c *singleInstructionConversation) AppendDisplayToolArgs(id, name, delta string) error {
	if c == nil || delta == "" {
		return nil
	}
	if c.hideDirectorToolInput && c.shouldHideDirectorToolArgs(id, name) {
		event, ok := c.recordDirectorToolArgs(id, name, delta)
		if !ok {
			return nil
		}
		return c.forwardDisplayEvent(event)
	}
	if appender, ok := c.display.(displayToolArgsAppender); ok {
		return appender.AppendDisplayToolArgs(id, name, delta)
	}
	return nil
}

func (c *singleInstructionConversation) UpdateDisplayToolStatus(id, name, status string) error {
	if c == nil {
		return nil
	}
	if c.hideDirectorToolInput {
		if event, ok := c.finishDirectorToolEvent(id, name, status, ""); ok {
			if err := c.forwardDisplayEvent(event); err != nil {
				return err
			}
		}
	}
	if updater, ok := c.display.(displayEventAppender); ok {
		return updater.UpdateDisplayToolStatus(id, name, status)
	}
	return nil
}

func (c *singleInstructionConversation) UpdateDisplayToolResult(id, name, status, result string) error {
	if c == nil {
		return nil
	}
	if c.hideDirectorToolInput {
		if event, ok := c.finishDirectorToolEvent(id, name, status, result); ok {
			if err := c.forwardDisplayEvent(event); err != nil {
				return err
			}
		}
	}
	if updater, ok := c.display.(displayToolResultUpdater); ok {
		return updater.UpdateDisplayToolResult(id, name, status, result)
	}
	if updater, ok := c.display.(displayEventAppender); ok {
		return updater.UpdateDisplayToolStatus(id, name, status)
	}
	return nil
}

func (c *singleInstructionConversation) forwardDisplayEvent(event session.DisplayEvent) error {
	if appender, ok := c.display.(displayEventAppender); ok {
		return appender.AppendDisplayEvent(event)
	}
	return nil
}

func (c *singleInstructionConversation) recordDirectorToolEvent(event session.DisplayEvent) session.DisplayEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.directorToolStateLocked(event.ID, event.Name)
	if state == nil {
		return event
	}
	state.event = event
	state.appendArgs(event.Args)
	projected, ok := state.projectEvent(event)
	if !ok {
		projected.Args = ""
		return projected
	}
	state.sentChars = state.generatedChars
	return projected
}

func (c *singleInstructionConversation) shouldHideDirectorToolArgs(id, name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if directorPlanWriteTool(name) {
		return true
	}
	state := c.findDirectorToolStateLocked(id, name)
	return state != nil && directorPlanWriteTool(state.name)
}

func (c *singleInstructionConversation) recordDirectorToolArgs(id, name, delta string) (session.DisplayEvent, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.directorToolStateLocked(id, name)
	if state == nil {
		return session.DisplayEvent{}, false
	}
	if strings.TrimSpace(state.event.Role) == "" {
		state.event = decorateDirectorDisplayEvent(session.DisplayEvent{
			ID:      strings.TrimSpace(id),
			Role:    "tool_call",
			Content: strings.TrimSpace(state.name),
			Name:    strings.TrimSpace(state.name),
			Status:  "running",
		})
	}
	state.appendArgs(delta)
	event, ok := state.progressEvent()
	if !ok {
		return session.DisplayEvent{}, false
	}
	return event, true
}

func (c *singleInstructionConversation) finishDirectorToolEvent(id, name, status, result string) (session.DisplayEvent, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	state := c.findDirectorToolStateLocked(id, name)
	if state == nil {
		return session.DisplayEvent{}, false
	}
	state.syncDecodedGeneratedChars()
	event, ok := state.projectEvent(state.event)
	if !ok {
		delete(c.directorTools, directorToolStateKey(state.id, state.name))
		return session.DisplayEvent{}, false
	}
	if strings.TrimSpace(status) != "" {
		event.Status = strings.TrimSpace(status)
	}
	event.Result = result
	delete(c.directorTools, directorToolStateKey(state.id, state.name))
	return event, true
}

func (c *singleInstructionConversation) directorToolStateLocked(id, name string) *directorToolDisplayState {
	if c.directorTools == nil {
		c.directorTools = map[string]*directorToolDisplayState{}
	}
	if existing := c.findDirectorToolStateLocked(id, name); existing != nil {
		if strings.TrimSpace(name) != "" {
			existing.name = strings.TrimSpace(name)
		}
		return existing
	}
	name = strings.TrimSpace(name)
	if !directorPlanWriteTool(name) {
		return nil
	}
	id = strings.TrimSpace(id)
	key := directorToolStateKey(id, name)
	state := &directorToolDisplayState{id: id, name: name}
	c.directorTools[key] = state
	return state
}

func (c *singleInstructionConversation) findDirectorToolStateLocked(id, name string) *directorToolDisplayState {
	if len(c.directorTools) == 0 {
		return nil
	}
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id != "" {
		for _, state := range c.directorTools {
			if state.id == id {
				return state
			}
		}
	}
	if name == "" {
		return nil
	}
	for _, state := range c.directorTools {
		if state.name == name {
			return state
		}
	}
	return nil
}

type directorToolDisplayState struct {
	id             string
	name           string
	rawArgs        string
	displayArgs    string
	generatedChars int
	sentChars      int
	event          session.DisplayEvent
	counter        directorToolTextCounter
}

func (s *directorToolDisplayState) appendArgs(delta string) {
	if s == nil || delta == "" {
		return
	}
	s.rawArgs += delta
	s.generatedChars += s.counter.countDelta(delta, directorToolGeneratedTextKeys(s.name))
}

func (s *directorToolDisplayState) projectEvent(event session.DisplayEvent) (session.DisplayEvent, bool) {
	if s == nil {
		return event, false
	}
	s.syncDecodedGeneratedChars()
	displayArgs, ok := s.projectDisplayArgs()
	if !ok {
		return event, false
	}
	event.Args = displayArgs
	markDirectorPlanInputHidden(&event, s.generatedChars)
	return event, true
}

func (s *directorToolDisplayState) progressEvent() (session.DisplayEvent, bool) {
	if s == nil {
		return session.DisplayEvent{}, false
	}
	event, ok := s.projectEvent(s.event)
	if !ok {
		return session.DisplayEvent{}, false
	}
	charsChanged := s.generatedChars-s.sentChars >= directorPlanProgressStep
	if event.Args == s.displayArgs && !charsChanged {
		return session.DisplayEvent{}, false
	}
	s.displayArgs = event.Args
	s.sentChars = s.generatedChars
	return event, true
}

func (s *directorToolDisplayState) projectDisplayArgs() (string, bool) {
	preview, ok := directorToolPathArgPreviewFromArgs(s.rawArgs)
	if !ok || !isDirectorPlanPath(preview.path) {
		return "", false
	}
	return `{"file_path":"director.md"}`, true
}

func (s *directorToolDisplayState) syncDecodedGeneratedChars() {
	if s == nil {
		return
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(s.rawArgs)), &payload); err != nil {
		return
	}
	for _, key := range directorToolGeneratedTextKeys(s.name) {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			continue
		}
		s.generatedChars = utf8.RuneCountInString(text)
		return
	}
}

type directorToolTextCounter struct {
	scanBuffer        string
	countingValue     bool
	valueDone         bool
	escapedValue      bool
	unicodeEscapeLeft int
}

func (c *directorToolTextCounter) countDelta(delta string, keys []string) int {
	if c == nil || c.valueDone || delta == "" {
		return 0
	}
	input := delta
	if !c.countingValue {
		c.scanBuffer += delta
		offset, ok := jsonStringValueOffsetAny(c.scanBuffer, keys)
		if !ok {
			c.trimScanBuffer()
			return 0
		}
		input = c.scanBuffer[offset:]
		c.scanBuffer = ""
		c.countingValue = true
	}
	return c.countValue(input)
}

func (c *directorToolTextCounter) trimScanBuffer() {
	const maxScanBuffer = 256
	if len(c.scanBuffer) > maxScanBuffer {
		c.scanBuffer = c.scanBuffer[len(c.scanBuffer)-maxScanBuffer:]
	}
}

func (c *directorToolTextCounter) countValue(input string) int {
	count := 0
	for input != "" {
		r, size := utf8.DecodeRuneInString(input)
		if size <= 0 {
			size = 1
		}
		input = input[size:]
		if c.unicodeEscapeLeft > 0 {
			c.unicodeEscapeLeft--
			if c.unicodeEscapeLeft == 0 {
				count++
			}
			continue
		}
		if c.escapedValue {
			c.escapedValue = false
			if r == 'u' {
				c.unicodeEscapeLeft = 4
				continue
			}
			count++
			continue
		}
		switch r {
		case '\\':
			c.escapedValue = true
		case '"':
			c.valueDone = true
			return count
		default:
			count++
		}
	}
	return count
}

type directorToolPathArgPreview struct {
	key  string
	path string
}

func directorToolPathArgPreviewFromArgs(args string) (directorToolPathArgPreview, bool) {
	trimmed := strings.TrimSpace(args)
	if trimmed == "" {
		return directorToolPathArgPreview{}, false
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		for _, key := range []string{"file_path", "path", "filename", "file"} {
			value, _ := payload[key].(string)
			value = strings.TrimSpace(value)
			if value != "" {
				return directorToolPathArgPreview{key: key, path: value}, true
			}
		}
	}
	for _, key := range []string{"file_path", "path", "filename", "file"} {
		value, ok := partialDirectorJSONStringField(trimmed, key)
		value = strings.TrimSpace(value)
		if ok && value != "" {
			return directorToolPathArgPreview{key: key, path: value}, true
		}
	}
	return directorToolPathArgPreview{}, false
}

func partialDirectorJSONStringField(args, key string) (string, bool) {
	needle := `"` + key + `"`
	searchFrom := 0
	for {
		index := strings.Index(args[searchFrom:], needle)
		if index < 0 {
			return "", false
		}
		index += searchFrom
		afterKey := strings.TrimLeft(args[index+len(needle):], " \n\r\t")
		if !strings.HasPrefix(afterKey, ":") {
			searchFrom = index + len(needle)
			continue
		}
		afterColon := strings.TrimLeft(afterKey[1:], " \n\r\t")
		if !strings.HasPrefix(afterColon, `"`) {
			searchFrom = index + len(needle)
			continue
		}
		value := afterColon[1:]
		escaped := false
		for i := 0; i < len(value); i++ {
			switch value[i] {
			case '\\':
				escaped = !escaped
			case '"':
				if escaped {
					escaped = false
					continue
				}
				decoded, err := strconv.Unquote(`"` + value[:i] + `"`)
				if err != nil {
					return value[:i], true
				}
				return decoded, true
			default:
				escaped = false
			}
		}
		return "", false
	}
}

func jsonStringValueOffsetAny(data string, keys []string) (int, bool) {
	for _, key := range keys {
		if offset, ok := jsonStringValueOffset(data, key); ok {
			return offset, true
		}
	}
	return 0, false
}

func jsonStringValueOffset(data, key string) (int, bool) {
	needle := `"` + key + `"`
	index := strings.Index(data, needle)
	if index < 0 {
		return 0, false
	}
	afterKey := strings.TrimLeft(data[index+len(needle):], " \n\r\t")
	if afterKey == "" || !strings.HasPrefix(afterKey, ":") {
		return 0, false
	}
	afterColon := strings.TrimLeft(afterKey[1:], " \n\r\t")
	if afterColon == "" || !strings.HasPrefix(afterColon, `"`) {
		return 0, false
	}
	return len(data) - len(afterColon) + 1, true
}

func directorToolGeneratedTextKeys(name string) []string {
	switch strings.TrimSpace(name) {
	case "edit_file":
		return []string{"new_string", "content"}
	default:
		return []string{"content", "new_string"}
	}
}

func directorToolStateKey(id, name string) string {
	id = strings.TrimSpace(id)
	if id != "" {
		return id
	}
	return "name:" + strings.TrimSpace(name)
}

func decorateDirectorDisplayEvent(event session.DisplayEvent) session.DisplayEvent {
	if strings.TrimSpace(event.AgentKind) == "" {
		event.AgentKind = config.AgentKindInteractiveDirector
	}
	if strings.TrimSpace(event.AgentName) == "" {
		event.AgentName = "interactive_director"
	}
	if strings.TrimSpace(event.RootAgentName) == "" {
		event.RootAgentName = event.AgentName
	}
	if strings.TrimSpace(event.Content) == "" && strings.TrimSpace(event.Name) != "" {
		event.Content = strings.TrimSpace(event.Name)
	}
	return event
}

func markDirectorPlanInputHidden(event *session.DisplayEvent, generatedChars int) {
	if event == nil {
		return
	}
	event.SSEHiddenFields = []string{"content", "new_string", "old_string"}
	event.SSEHiddenReason = directorPlanHiddenReason
	event.SSEDisplayNotice = directorPlanHiddenNotice
	if generatedChars > 0 {
		event.SSEGeneratedChars = generatedChars
	}
}

func directorPlanWriteTool(name string) bool {
	switch strings.TrimSpace(name) {
	case "write_file", "edit_file":
		return true
	default:
		return false
	}
}

func isDirectorPlanPath(path string) bool {
	normalized := strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
	for strings.HasSuffix(normalized, "/") {
		normalized = strings.TrimSuffix(normalized, "/")
	}
	return normalized == "director.md" || strings.HasSuffix(normalized, "/director.md")
}
