package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"nova/internal/book"
	"nova/internal/prompts"
	"nova/internal/session"
)

const (
	maxReferenceFileBytes       = 80 * 1024
	maxReferenceTotalBytes      = 200 * 1024
	maxStyleReferenceFileBytes  = 80 * 1024
	maxStyleReferenceTotalBytes = 200 * 1024
)

// Event 表示 Agent 输出的传输无关事件。
type Event struct {
	Type string
	Data interface{}
}

// ChatRequest 表示一次聊天请求的传输无关参数。
type ChatRequest struct {
	Message         string             `json:"message"`
	References      []string           `json:"references"`
	LoreReferences  []string           `json:"lore_references"`
	StyleReferences []string           `json:"style_references"`
	Selections      []TextSelectionRef `json:"selections"`
	PlanMode        bool               `json:"plan_mode"`

	// StyleRules 由后端按当前讲述者配置注入（场景 → 风格文件）。
	// 仅当 StyleReferences 为空时才会作为"默认场景化建议"参与本轮上下文，
	// 由 Agent 基于本轮章节内容自动匹配最相近的场景并 read_file 对应文件。
	StyleRules []StyleRule `json:"-"`
}

// StyleRule 是 prompts.StyleRule 的镜像，避免调用方直接依赖 prompts 包。
type StyleRule = prompts.StyleRule

// TextSelectionRef 表示用户在编辑器中选中的一段文本引用。
type TextSelectionRef struct {
	FileName  string `json:"file_name"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Content   string `json:"content"`
}

// ChatService 编排会话历史、文件引用和 Agent 流式响应。
type ChatService struct{}

// NewChatService 创建聊天服务。
func NewChatService() *ChatService {
	return &ChatService{}
}

// Run 运行一次聊天请求，并通过 emit 输出流式事件。
func (s *ChatService) Run(
	ctx context.Context,
	runner *adk.Runner,
	conversation Conversation,
	bookService *book.Service,
	req ChatRequest,
	emit func(Event),
) {
	recorder := newDisplayEventRecorder(conversation)
	rawEmit := emit
	emit = func(ev Event) {
		recorder.Record(ev)
		rawEmit(ev)
	}
	originalMessage := req.Message
	var resumeInterruption *session.Interruption
	if shouldResumeInterruptedRequest(req.Message) {
		resumeInterruption = conversation.PendingInterruption()
		if resumeInterruption != nil {
			req.Message = buildInterruptedResumeMessage(req.Message, resumeInterruption)
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[agent-run] panic recovered err=%v", recovered)
			markInterruptionIfNeeded(conversation, resumeInterruption, originalMessage, "", fmt.Sprint(recovered))
			emit(Event{Type: "error", Data: map[string]string{"message": "Agent 异常中断"}})
		}
	}()

	agentMessage := req.Message
	contextLog := newContextBuildLog()
	contextLog.add("用户输入", "本轮原始请求", originalMessage, "")
	if resumeInterruption != nil {
		contextLog.add("运行时恢复", "异常中断恢复上下文", req.Message, "包含上一轮原始请求、已生成助手内容和中断原因")
	}
	if req.PlanMode {
		agentMessage = appendPlanModeInstruction(agentMessage)
		contextLog.add("注入规则", "规划模式", "[规划模式] 请你先制定计划，不要执行任何写操作。", "")
	}
	if len(req.References) > 0 {
		agentMessage = appendReferenceContext(bookService, agentMessage, req.References, contextLog)
	}
	if len(req.LoreReferences) > 0 {
		agentMessage = appendLoreReferenceContext(bookService, agentMessage, req.LoreReferences, contextLog)
	}
	if len(req.StyleReferences) > 0 {
		agentMessage = appendStyleReferenceContext(bookService, agentMessage, req.StyleReferences, contextLog)
	} else if len(req.StyleRules) > 0 {
		agentMessage = appendStyleRulesHint(agentMessage, req.StyleRules)
		contextLog.addStyleRules(req.StyleRules)
	}
	if len(req.Selections) > 0 {
		agentMessage = appendSelectionContext(agentMessage, req.Selections)
		contextLog.addSelections(req.Selections)
	}
	agentMessage = appendContextBoundaryInstruction(agentMessage)
	contextLog.add("注入规则", "上下文边界", "[上下文边界] 当前用户请求是“这次要做什么”", "")

	history, err := conversation.PrepareMessages(originalMessage, agentMessage)
	if err != nil {
		log.Printf("[agent-run] prepare messages failed err=%v", err)
		emit(Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		return
	}
	log.Printf(
		"[agent-run] context composition history=%s original=%s agent_message=%s references=%s lore_references=%s style_references=%s style_rules=%d selections=%s plan_mode=%v resumed=%v",
		messageListSummary(history),
		promptPartSummary(originalMessage),
		promptPartSummary(agentMessage),
		stringListSummary(req.References),
		stringListSummary(req.LoreReferences),
		stringListSummary(req.StyleReferences),
		len(req.StyleRules),
		selectionListSummary(req.Selections),
		req.PlanMode,
		resumeInterruption != nil,
	)
	log.Printf("[agent-run] context sources %s", contextLog.String())
	if reporter, ok := conversation.(ContextSourceReporter); ok {
		if sources := strings.TrimSpace(reporter.ContextSourceSummary()); sources != "" {
			log.Printf("[agent-run] conversation context sources %s", sources)
		}
	}

	events := runner.Run(ctx, history)
	var fullContent strings.Builder
	var fullThinking strings.Builder
	log.Printf("[agent-run] started history=%d message_len=%d agent_message_len=%d plan_mode=%v style_references=%d style_rules=%d", len(history), len(req.Message), len(agentMessage), req.PlanMode, len(req.StyleReferences), len(req.StyleRules))

	for {
		if err := ctx.Err(); err != nil {
			log.Printf("[agent-run] interrupted reason=context err=%v generated_bytes=%d", err, fullContent.Len())
			appendAssistantIfAny(conversation, &fullContent, &fullThinking)
			emit(Event{Type: "aborted", Data: map[string]string{}})
			return
		}
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			log.Printf("[agent-run] interrupted reason=runner_error err=%v generated_bytes=%d", event.Err, fullContent.Len())
			generated := appendAssistantIfAny(conversation, &fullContent, &fullThinking)
			markInterruptionIfNeeded(conversation, resumeInterruption, originalMessage, generated, event.Err.Error())
			emit(Event{Type: "error", Data: map[string]string{"message": event.Err.Error()}})
			return
		}

		if event.Output == nil || event.Output.MessageOutput == nil {
			log.Printf("[agent-run] skip invalid_output output_nil=%v message_output_nil=%v", event.Output == nil, event.Output != nil && event.Output.MessageOutput == nil)
			continue
		}

		mv := event.Output.MessageOutput
		if mv.Role == schema.Tool {
			if mv.Message == nil {
				continue
			}
			content := drainContent(mv)
			fullToolContent := content
			if content == "" {
				content = "(无返回内容)"
			}
			if len(content) > 300 {
				content = content[:300] + "..."
			}
			logToolResult(mv.Message.ToolName, mv.Message.ToolCallID, content)
			data := map[string]interface{}{
				"id":      mv.Message.ToolCallID,
				"name":    mv.Message.ToolName,
				"content": content,
			}
			if itemIDs, deletedIDs := parseWriteLoreItemsToolResult(mv.Message.ToolName, fullToolContent); len(itemIDs) > 0 || len(deletedIDs) > 0 {
				data["item_ids"] = itemIDs
				data["deleted_ids"] = deletedIDs
			}
			emit(Event{Type: "tool_result", Data: data})
			continue
		}

		if mv.Role != schema.Assistant && mv.Role != "" {
			continue
		}
		if mv.IsStreaming && mv.MessageStream != nil {
			if !processStreamingEvent(mv, &fullContent, &fullThinking, emit) {
				generated := appendAssistantIfAny(conversation, &fullContent, &fullThinking)
				markInterruptionIfNeeded(conversation, resumeInterruption, originalMessage, generated, "stream recv error")
				return
			}
			continue
		}
		if mv.Message != nil {
			processNonStreamingEvent(mv, &fullContent, &fullThinking, emit)
		}
	}

	appendAssistantIfAny(conversation, &fullContent, &fullThinking)
	if resumeInterruption != nil {
		if err := conversation.ResolveInterruption(resumeInterruption.ID); err != nil {
			log.Printf("[agent-run] resolve interruption failed id=%s err=%v", resumeInterruption.ID, err)
		}
	}
	log.Printf("[agent-run] completed")
	emit(Event{Type: "done", Data: map[string]string{}})
}

// appendAssistantIfAny 将已生成的正文持久化，避免异常中断后刷新丢失输出。
func appendAssistantIfAny(conversation Conversation, content, thinking *strings.Builder) string {
	if content == nil || content.Len() == 0 {
		return ""
	}
	generated := content.String()
	reasoning := ""
	if thinking != nil && thinking.Len() > 0 {
		reasoning = thinking.String()
	}
	if appender, ok := conversation.(interface {
		AppendAssistantWithThinking(content, thinking string) error
	}); ok {
		if err := appender.AppendAssistantWithThinking(generated, reasoning); err != nil {
			log.Printf("[agent-run] persist assistant message failed err=%v", err)
		}
	} else if err := conversation.AppendAssistant(generated); err != nil {
		log.Printf("[agent-run] persist assistant message failed err=%v", err)
	}
	log.Printf("[agent-run] persisted assistant message bytes=%d thinking_bytes=%d", len(generated), len(reasoning))
	content.Reset()
	if thinking != nil {
		thinking.Reset()
	}
	return generated
}

type displayEventAppender interface {
	AppendDisplayEvent(event session.DisplayEvent) error
	UpdateDisplayToolStatus(id, name, status string) error
}

type displayEventRecorder struct {
	appender       displayEventAppender
	thinking       strings.Builder
	pendingToolIDs map[string]string
}

func newDisplayEventRecorder(conversation Conversation) *displayEventRecorder {
	appender, _ := conversation.(displayEventAppender)
	return &displayEventRecorder{
		appender:       appender,
		pendingToolIDs: make(map[string]string),
	}
}

func (r *displayEventRecorder) Record(ev Event) {
	if r == nil || r.appender == nil {
		return
	}
	switch ev.Type {
	case "thinking":
		r.thinking.WriteString(eventDataString(ev.Data, "content"))
	case "chunk":
		r.flushThinking()
	case "tool_call":
		r.flushThinking()
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		if strings.TrimSpace(name) == "" {
			name = "unknown_tool"
		}
		if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
			ID:      id,
			Role:    "tool_call",
			Content: name,
			Name:    name,
			Status:  "running",
		}); err != nil {
			log.Printf("[agent-run] persist display tool_call failed name=%s id=%s err=%v", name, id, err)
			return
		}
		if id != "" {
			r.pendingToolIDs[id] = name
		}
	case "tool_result":
		r.flushThinking()
		id := eventDataString(ev.Data, "id")
		name := eventDataString(ev.Data, "name")
		if err := r.appender.UpdateDisplayToolStatus(id, name, "success"); err != nil {
			log.Printf("[agent-run] persist display tool_result failed name=%s id=%s err=%v", name, id, err)
		}
		if id != "" {
			delete(r.pendingToolIDs, id)
		}
	case "error", "aborted":
		r.flushThinking()
		for id, name := range r.pendingToolIDs {
			if err := r.appender.UpdateDisplayToolStatus(id, name, "error"); err != nil {
				log.Printf("[agent-run] persist display tool_error failed name=%s id=%s err=%v", name, id, err)
			}
		}
		r.pendingToolIDs = make(map[string]string)
	case "done":
		r.flushThinking()
	}
}

func (r *displayEventRecorder) flushThinking() {
	if r == nil || r.appender == nil || r.thinking.Len() == 0 {
		return
	}
	content := r.thinking.String()
	r.thinking.Reset()
	if strings.TrimSpace(content) == "" {
		return
	}
	if err := r.appender.AppendDisplayEvent(session.DisplayEvent{
		Role:    "thinking",
		Content: content,
	}); err != nil {
		log.Printf("[agent-run] persist display thinking failed bytes=%d err=%v", len(content), err)
	}
}

func eventDataString(data interface{}, key string) string {
	switch typed := data.(type) {
	case map[string]string:
		return typed[key]
	case map[string]interface{}:
		if value, ok := typed[key]; ok {
			return fmt.Sprint(value)
		}
	}
	return ""
}

func parseWriteLoreItemsToolResult(toolName, content string) ([]string, []string) {
	if toolName != "write_lore_items" {
		return nil, nil
	}
	var itemIDs []string
	var deletedIDs []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if raw, ok := strings.CutPrefix(line, "item_ids:"); ok {
			_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &itemIDs)
			continue
		}
		if raw, ok := strings.CutPrefix(line, "deleted_ids:"); ok {
			_ = json.Unmarshal([]byte(strings.TrimSpace(raw)), &deletedIDs)
		}
	}
	return itemIDs, deletedIDs
}

func markInterruptionIfNeeded(conversation Conversation, resumed *session.Interruption, userMessage, assistantContent, reason string) {
	if resumed != nil {
		return
	}
	if err := conversation.MarkInterrupted(userMessage, assistantContent, reason); err != nil {
		log.Printf("[agent-run] mark interruption failed err=%v", err)
	}
}

func shouldResumeInterruptedRequest(message string) bool {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return false
	}
	switch trimmed {
	case "继续", "继续。", "继续！", "接着来", "接着写", "续上", "继续刚才":
		return true
	}
	return strings.HasPrefix(trimmed, "继续刚才") || strings.HasPrefix(trimmed, "继续之前") || strings.HasPrefix(trimmed, "从中断的地方继续")
}

func buildInterruptedResumeMessage(current string, interrupted *session.Interruption) string {
	if interrupted == nil {
		return current
	}
	return prompts.ResumeFromInterruption(current, prompts.InterruptedResume{
		UserMessage:      interrupted.UserMessage,
		AssistantContent: interrupted.AssistantContent,
		Reason:           interrupted.Reason,
	})
}

// appendReferenceContext 将用户引用的文件内容追加到本次 Agent 输入。
func appendReferenceContext(bookService *book.Service, message string, references []string, logs ...*contextBuildLog) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString(prompts.ReferenceHeader)

	total := 0
	seen := make(map[string]bool)
	for _, ref := range references {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true

		sb.WriteString("\n## @")
		sb.WriteString(ref)
		sb.WriteString("\n")

		if total >= maxReferenceTotalBytes {
			sb.WriteString(prompts.ReferenceOverflowHint)
			addContextLog(logs, "文件引用", "@"+ref, prompts.ReferenceOverflowHint, "未读取：引用内容总量已超过限制")
			continue
		}

		content, n, err := readReferencedFile(bookService, ref, maxReferenceFileBytes, maxReferenceTotalBytes-total)
		total += n
		if err != nil {
			sb.WriteString("读取失败：")
			sb.WriteString(err.Error())
			sb.WriteString("\n")
			addContextLog(logs, "文件引用", "@"+ref, err.Error(), "读取失败")
			continue
		}
		addContextLog(logs, "文件引用", "@"+ref, content, "")

		sb.WriteString("```markdown\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}

// appendLoreReferenceContext 将用户本轮明确引用的结构化资料条目追加到 Agent 输入。
func appendLoreReferenceContext(bookService *book.Service, message string, references []string, logs ...*contextBuildLog) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString("\n\n# 本轮明确引用的资料库条目\n\n以下资料来自结构化资料库，优先级高于泛化摘要；请在本轮创作或判断中优先遵守这些条目的已确认设定。\n")

	if bookService == nil || bookService.Workspace() == "" {
		sb.WriteString("\n资料库读取失败：当前 workspace 不可用。\n")
		addContextLog(logs, "资料库引用", "workspace", "当前 workspace 不可用", "读取失败")
		return sb.String()
	}

	items, err := book.NewLoreStore(bookService.Workspace()).List()
	if err != nil {
		sb.WriteString("\n资料库读取失败：")
		sb.WriteString(err.Error())
		sb.WriteString("\n")
		addContextLog(logs, "资料库引用", ".nova/lore/items.json", err.Error(), "读取失败")
		return sb.String()
	}

	byID := make(map[string]book.LoreItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	seen := make(map[string]bool)
	for _, ref := range references {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		item, ok := byID[ref]
		if !ok {
			sb.WriteString("\n## @资料:")
			sb.WriteString(ref)
			sb.WriteString("\n读取失败：资料条目不存在\n")
			addContextLog(logs, "资料库引用", "@资料:"+ref, "资料条目不存在", "读取失败")
			continue
		}
		content := formatLoreReference(item)
		addContextLog(logs, "资料库引用", "@资料:"+item.Name, content, item.ID)
		sb.WriteString("\n")
		sb.WriteString(content)
		sb.WriteString("\n")
	}

	return sb.String()
}

// appendStyleReferenceContext 将本轮指定的风格参考追加到 Agent 输入。
func appendStyleReferenceContext(bookService *book.Service, message string, styleReferences []string, logs ...*contextBuildLog) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString(prompts.StyleReferenceHeader)

	total := 0
	seen := make(map[string]bool)
	for _, ref := range styleReferences {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true

		sb.WriteString("\n## #")
		sb.WriteString(ref)
		sb.WriteString("\n")

		if total >= maxStyleReferenceTotalBytes {
			sb.WriteString(prompts.StyleReferenceOverflowHint)
			addContextLog(logs, "风格参考", "#"+ref, prompts.StyleReferenceOverflowHint, "未读取：风格参考内容总量已超过限制")
			continue
		}

		content, n, err := readStyleReferencedFile(bookService, ref, maxStyleReferenceFileBytes, maxStyleReferenceTotalBytes-total)
		total += n
		if err != nil {
			sb.WriteString("读取失败：")
			sb.WriteString(err.Error())
			sb.WriteString("\n")
			addContextLog(logs, "风格参考", "#"+ref, err.Error(), "读取失败")
			continue
		}
		addContextLog(logs, "风格参考", "#"+ref, content, "")

		sb.WriteString("```markdown\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}

// appendStyleRulesHint 在用户本轮未通过 # 指定风格时，
// 把工作区配置的「场景 → 风格文件」规则集作为建议附加到上下文。
// 不直接读取文件内容，由 Agent 基于本轮章节内容自行判断。
func appendStyleRulesHint(message string, rules []StyleRule) string {
	return prompts.StyleRulesHint(message, rules)
}

// appendSelectionContext 将用户在编辑器中选中的文本片段追加到消息上下文。
func appendSelectionContext(message string, selections []TextSelectionRef) string {
	var sb strings.Builder
	sb.WriteString(message)
	sb.WriteString(prompts.SelectionHeader)

	for _, sel := range selections {
		sb.WriteString("\n## 选中内容来自 ")
		sb.WriteString(sel.FileName)
		sb.WriteString(fmt.Sprintf(":L%d-L%d\n", sel.StartLine, sel.EndLine))
		sb.WriteString("```\n")
		sb.WriteString(sel.Content)
		sb.WriteString("\n```\n")
	}

	return sb.String()
}

// readReferencedFile 安全读取引用文件，并按单文件和总大小限制截断。
func readReferencedFile(bookService *book.Service, relPath string, fileLimit, remainLimit int) (string, int, error) {
	limit := fileLimit
	if remainLimit < limit {
		limit = remainLimit
	}
	if limit <= 0 {
		return "", 0, errors.New("引用内容总量已超过限制")
	}

	content, err := bookService.ReadFile(relPath)
	if err != nil {
		return "", 0, err
	}

	data := []byte(content)
	truncated := false
	if len(data) > limit {
		data = data[:limit]
		truncated = true
	}

	result := string(data)
	if truncated {
		result += "\n\n[内容已截断]"
	}
	return result, len(data), nil
}

func formatLoreReference(item book.LoreItem) string {
	var sb strings.Builder
	sb.WriteString("## ")
	sb.WriteString(item.Name)
	sb.WriteString("（")
	sb.WriteString(item.Type)
	sb.WriteString(" / ")
	sb.WriteString(item.Importance)
	sb.WriteString(" / ")
	sb.WriteString(item.LoadMode)
	sb.WriteString("）\n")
	sb.WriteString("ID：")
	sb.WriteString(item.ID)
	sb.WriteString("\n")
	if len(item.Tags) > 0 {
		sb.WriteString("标签：")
		sb.WriteString(strings.Join(item.Tags, "、"))
		sb.WriteString("\n")
	}
	if item.BriefDescription != "" {
		sb.WriteString("简介：")
		sb.WriteString(item.BriefDescription)
		sb.WriteString("\n")
	}
	content := strings.TrimSpace(item.Content)
	if content != "" {
		sb.WriteString("\n```markdown\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n")
	}
	return strings.TrimSpace(sb.String())
}

// readStyleReferencedFile 安全读取风格参考文件，并按单文件和总大小限制截断。
func readStyleReferencedFile(bookService *book.Service, stylePath string, fileLimit, remainLimit int) (string, int, error) {
	limit := fileLimit
	if remainLimit < limit {
		limit = remainLimit
	}
	if limit <= 0 {
		return "", 0, errors.New("风格参考内容总量已超过限制")
	}

	content, err := bookService.ReadStyleFile(stylePath)
	if err != nil {
		return "", 0, err
	}

	data := []byte(content)
	truncated := false
	if len(data) > limit {
		data = data[:limit]
		truncated = true
	}

	result := string(data)
	if truncated {
		result += "\n\n[内容已截断]"
	}
	return result, len(data), nil
}

// processStreamingEvent 处理流式助手消息，输出领域事件。
// 工具调用在流中一检测到名称就立即 emit，让前端尽早展示 running 卡片。
// 参数在流中逐帧 emit tool_args_delta，前端可实时展示 write_file 内容。
func processStreamingEvent(mv *adk.MessageVariant, fullContent, fullThinking *strings.Builder, emit func(Event)) bool {
	mv.MessageStream.SetAutomaticClose()
	var accumulatedToolCalls []schema.ToolCall
	emittedTools := make(map[int]bool) // 按 index 记录已 emit tool_call 的工具
	lastArgsLen := make(map[int]int)   // 记录上次已发送的参数长度

	for {
		frame, err := mv.MessageStream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			log.Printf("[agent-run] interrupted reason=stream_recv_error err=%v generated_bytes=%d", err, fullContent.Len())
			emit(Event{Type: "error", Data: map[string]string{"message": err.Error()}})
			return false
		}
		if frame == nil {
			continue
		}
		if frame.ReasoningContent != "" {
			if fullThinking != nil {
				fullThinking.WriteString(frame.ReasoningContent)
			}
			emit(Event{Type: "thinking", Data: map[string]string{"content": frame.ReasoningContent}})
		}
		if frame.Content != "" {
			fullContent.WriteString(frame.Content)
			emit(Event{Type: "chunk", Data: map[string]string{"content": frame.Content}})
		}
		if len(frame.ToolCalls) > 0 {
			accumulatedToolCalls = mergeToolCalls(accumulatedToolCalls, frame.ToolCalls)
			for i, tc := range accumulatedToolCalls {
				if tc.Function.Name == "" {
					continue
				}
				// 首次检测到工具名称，emit tool_call
				if !emittedTools[i] {
					emittedTools[i] = true
					lastArgsLen[i] = 0
					logToolCall(tc.Function.Name, tc.ID, len(tc.Function.Arguments), "streaming")
					data := map[string]interface{}{
						"id":   tc.ID,
						"name": tc.Function.Name,
						"args": "",
					}
					if tc.Index != nil {
						data["index"] = *tc.Index
					}
					emit(Event{Type: "tool_call", Data: data})
				}
				// 参数有增量时 emit tool_args_delta
				currentLen := len(tc.Function.Arguments)
				if currentLen > lastArgsLen[i] {
					delta := tc.Function.Arguments[lastArgsLen[i]:currentLen]
					lastArgsLen[i] = currentLen
					data := map[string]interface{}{
						"id":    tc.ID,
						"name":  tc.Function.Name,
						"delta": delta,
					}
					if tc.Index != nil {
						data["index"] = *tc.Index
					}
					emit(Event{Type: "tool_args_delta", Data: data})
				}
			}
		}
	}
	return true
}

// processNonStreamingEvent 处理非流式助手消息，输出领域事件。
func processNonStreamingEvent(mv *adk.MessageVariant, fullContent, fullThinking *strings.Builder, emit func(Event)) {
	if mv.Message.ReasoningContent != "" {
		if fullThinking != nil {
			fullThinking.WriteString(mv.Message.ReasoningContent)
		}
		emit(Event{Type: "thinking", Data: map[string]string{"content": mv.Message.ReasoningContent}})
	}
	if mv.Message.Content != "" {
		fullContent.WriteString(mv.Message.Content)
		emit(Event{Type: "chunk", Data: map[string]string{"content": mv.Message.Content}})
	}
	for _, tc := range mv.Message.ToolCalls {
		name := tc.Function.Name
		if name == "" {
			continue
		}
		args := tc.Function.Arguments
		logToolCall(name, tc.ID, len(args), "non_streaming")
		if len(args) > 200 {
			args = args[:200] + "..."
		}
		data := map[string]interface{}{
			"id":   tc.ID,
			"name": name,
			"args": args,
		}
		if tc.Index != nil {
			data["index"] = *tc.Index
		}
		emit(Event{Type: "tool_call", Data: data})
	}
}

// drainContent 从 MessageVariant 中提取完整内容。
func drainContent(mv *adk.MessageVariant) string {
	if mv.IsStreaming && mv.MessageStream != nil {
		mv.MessageStream.SetAutomaticClose()
		var sb strings.Builder
		for {
			chunk, err := mv.MessageStream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				break
			}
			if chunk != nil && chunk.Content != "" {
				sb.WriteString(chunk.Content)
			}
		}
		return sb.String()
	}
	if mv.Message != nil {
		return mv.Message.Content
	}
	return ""
}

func logToolCall(name, id string, argsBytes int, source string) {
	log.Printf("[agent-tool] call source=%s name=%s id=%s args_bytes=%d", source, name, id, argsBytes)
}

func logToolResult(name, id, content string) {
	if looksLikeToolFailure(content) {
		log.Printf("[agent-tool] result suspected_failure=true name=%s id=%s bytes=%d preview=%q", name, id, len(content), safeLogPreview(content, 300))
		return
	}
	log.Printf("[agent-tool] result name=%s id=%s bytes=%d", name, id, len(content))
}

func looksLikeToolFailure(content string) bool {
	text := strings.ToLower(content)
	failureKeywords := []string{
		"error", "failed", "failure", "panic", "exception", "traceback",
		"permission denied", "not found", "timeout", "timed out",
		"失败", "错误", "异常", "拒绝", "超时", "不存在",
	}
	for _, keyword := range failureKeywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func safeLogPreview(content string, limit int) string {
	content = strings.ReplaceAll(content, "\n", "\\n")
	content = strings.ReplaceAll(content, "\r", "\\r")
	if len(content) <= limit {
		return content
	}
	for limit > 0 && !utf8.RuneStart(content[limit]) {
		limit--
	}
	return content[:limit] + "..."
}

// mergeToolCalls 合并流式 frame 中分散的 tool call 信息。
func mergeToolCalls(existing []schema.ToolCall, incoming []schema.ToolCall) []schema.ToolCall {
	for _, tc := range incoming {
		idx := tc.Index
		if idx == nil {
			if tc.Function.Name != "" {
				existing = append(existing, tc)
			}
			continue
		}

		i := *idx
		for len(existing) <= i {
			existing = append(existing, schema.ToolCall{})
		}
		if tc.Function.Name != "" {
			existing[i].Function.Name = tc.Function.Name
		}
		existing[i].Function.Arguments += tc.Function.Arguments
		if tc.ID != "" {
			existing[i].ID = tc.ID
		}
		existing[i].Index = tc.Index
	}
	return existing
}

// appendPlanModeInstruction 在用户消息前追加规划模式指令，允许读取文件但禁止写操作，只输出结构化计划。
func appendPlanModeInstruction(message string) string {
	return prompts.PlanMode(message)
}

// appendContextBoundaryInstruction 在用户消息前追加上下文边界说明，
// 强调当前请求才是"这次要做什么"，工作区/已确认小说状态是"背景是什么"，
// 历史对话只能用于辅助理解，不能直接成为本轮执行依据。
func appendContextBoundaryInstruction(message string) string {
	return prompts.ContextBoundary(message)
}

type contextBuildLog struct {
	parts []contextLogPart
}

type contextLogPart struct {
	Source  string
	Title   string
	Content string
	Note    string
}

func newContextBuildLog() *contextBuildLog {
	return &contextBuildLog{parts: []contextLogPart{}}
}

func (l *contextBuildLog) add(source, title, content, note string) {
	if l == nil {
		return
	}
	source = strings.TrimSpace(source)
	title = strings.TrimSpace(title)
	if source == "" && title == "" && strings.TrimSpace(content) == "" {
		return
	}
	l.parts = append(l.parts, contextLogPart{
		Source:  source,
		Title:   title,
		Content: content,
		Note:    strings.TrimSpace(note),
	})
}

func (l *contextBuildLog) addStyleRules(rules []StyleRule) {
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if scene == "" || len(rule.Styles) == 0 {
			continue
		}
		styles := trimmedNonEmpty(rule.Styles)
		if len(styles) == 0 {
			continue
		}
		l.add("注入规则", "场景化默认风格规则："+scene, strings.Join(styles, "、"), "Agent 将按场景自行判断是否 read_file")
	}
}

func (l *contextBuildLog) addSelections(selections []TextSelectionRef) {
	for _, sel := range selections {
		title := strings.TrimSpace(sel.FileName)
		if title == "" {
			title = "未命名选区"
		}
		if sel.StartLine > 0 || sel.EndLine > 0 {
			title = fmt.Sprintf("%s:L%d-L%d", title, sel.StartLine, sel.EndLine)
		}
		l.add("编辑器选区", title, sel.Content, "")
	}
}

func (l *contextBuildLog) String() string {
	if l == nil || len(l.parts) == 0 {
		return "count=0"
	}
	parts := make([]string, 0, len(l.parts))
	for i, part := range l.parts {
		content := strings.TrimSpace(part.Content)
		fields := []string{
			fmt.Sprintf("%d:source=%q", i, part.Source),
			fmt.Sprintf("title=%q", part.Title),
			"bytes=" + intString(len(content)),
			"chars=" + intString(utf8.RuneCountInString(content)),
			"preview=" + strconv.Quote(safeLogPreview(content, 100)),
		}
		if part.Note != "" {
			fields = append(fields, "note="+strconv.Quote(part.Note))
		}
		parts = append(parts, strings.Join(fields, ","))
	}
	return fmt.Sprintf("count=%d parts=[%s]", len(l.parts), strings.Join(parts, "; "))
}

func addContextLog(logs []*contextBuildLog, source, title, content, note string) {
	for _, l := range logs {
		if l != nil {
			l.add(source, title, content, note)
		}
	}
}

func trimmedNonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

func messageListSummary(messages []*schema.Message) string {
	if len(messages) == 0 {
		return "count=0"
	}
	roleCounts := make(map[string]int)
	totalBytes := 0
	totalChars := 0
	for _, msg := range messages {
		if msg == nil {
			roleCounts["<nil>"]++
			continue
		}
		role := fmt.Sprint(msg.Role)
		roleCounts[role]++
		totalBytes += len(msg.Content)
		totalChars += utf8.RuneCountInString(msg.Content)
	}

	parts := make([]string, 0, len(messages))
	for i, msg := range messages {
		parts = append(parts, messageSummary(i, len(messages), msg))
	}

	return fmt.Sprintf("count=%d roles=%s total_bytes=%d total_chars=%d parts=[%s]", len(messages), roleCountSummary(roleCounts), totalBytes, totalChars, strings.Join(parts, "; "))
}

func messageSummary(index, total int, msg *schema.Message) string {
	if msg == nil {
		return fmt.Sprintf("%d:<nil>", index)
	}
	source := "会话历史"
	if index == total-1 {
		source = "本轮增强后用户输入"
	}
	return fmt.Sprintf("%d:source=%s role=%s(%s)", index, source, msg.Role, promptPartSummary(msg.Content))
}

func roleCountSummary(counts map[string]int) string {
	if len(counts) == 0 {
		return "{}"
	}
	roles := make([]string, 0, len(counts))
	for role := range counts {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	parts := make([]string, 0, len(roles))
	for _, role := range roles {
		parts = append(parts, fmt.Sprintf("%s:%d", role, counts[role]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func stringListSummary(values []string) string {
	if len(values) == 0 {
		return "count=0"
	}
	totalBytes := 0
	for _, value := range values {
		totalBytes += len(value)
	}
	display := values
	if len(display) > 6 {
		display = append(append([]string(nil), values[:3]...), append([]string{fmt.Sprintf("... omitted=%d ...", len(values)-6)}, values[len(values)-3:]...)...)
	}
	return fmt.Sprintf("count=%d total_bytes=%d items=%q", len(values), totalBytes, display)
}

func selectionListSummary(selections []TextSelectionRef) string {
	if len(selections) == 0 {
		return "count=0"
	}
	totalBytes := 0
	parts := make([]string, 0, minInt(len(selections), 6)+1)
	for i, sel := range selections {
		totalBytes += len(sel.Content)
		if i < 3 || i >= len(selections)-3 {
			parts = append(parts, fmt.Sprintf("%s:%d-%d(%s)", sel.FileName, sel.StartLine, sel.EndLine, promptPartSummary(sel.Content)))
		} else if i == 3 {
			parts = append(parts, fmt.Sprintf("... omitted=%d ...", len(selections)-6))
		}
	}
	return fmt.Sprintf("count=%d total_content_bytes=%d items=[%s]", len(selections), totalBytes, strings.Join(parts, "; "))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// EventError 创建标准错误事件。
func EventError(err error) Event {
	return Event{Type: "error", Data: map[string]string{"message": fmt.Sprint(err)}}
}
