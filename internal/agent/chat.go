package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"

	"nova/internal/book"
	"nova/internal/observability"
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

	// StyleRules 由后端按当前导演配置注入（场景 → 风格文件）。
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
type ChatService struct {
	policy  LoopPolicy
	runtime *Runtime
}

// Runtime owns the task-level Agent loop: context assembly, tool observation,
// durable run state, post-run verification, and final lifecycle events.
type Runtime struct {
	policy LoopPolicy
}

// NewChatService 创建聊天服务。
func NewChatService() *ChatService {
	return NewChatServiceWithPolicy(DefaultLoopPolicy())
}

// NewChatServiceWithPolicy 创建带显式 loop policy 的聊天服务，主要用于测试和后续分 Agent 配置。
func NewChatServiceWithPolicy(policy LoopPolicy) *ChatService {
	policy = policy.normalized()
	return &ChatService{policy: policy, runtime: NewRuntime(policy)}
}

func NewRuntime(policy LoopPolicy) *Runtime {
	return &Runtime{policy: policy.normalized()}
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
	s.RunWithOptions(ctx, runner, conversation, bookService, req, RunOptions{}, emit)
}

func (s *ChatService) RunWithOptions(
	ctx context.Context,
	runner *adk.Runner,
	conversation Conversation,
	bookService *book.Service,
	req ChatRequest,
	options RunOptions,
	emit func(Event),
) {
	runtime := NewRuntime(DefaultLoopPolicy())
	if s != nil {
		if s.runtime != nil {
			runtime = s.runtime
		} else {
			runtime = NewRuntime(s.policy)
		}
	}
	runtime.Run(ctx, runner, conversation, bookService, req, options, emit)
}

func (r *Runtime) Run(
	ctx context.Context,
	runner *adk.Runner,
	conversation Conversation,
	bookService *book.Service,
	req ChatRequest,
	options RunOptions,
	emit func(Event),
) {
	if emit == nil {
		emit = func(Event) {}
	}
	runLogger := observability.Logger("agent-run")
	policy := DefaultLoopPolicy()
	if r != nil {
		policy = r.policy.normalized()
	}
	workspace := ""
	if bookService != nil {
		workspace = bookService.Workspace()
	}
	options = options.normalized(workspace)
	runLedger, ledgerErr := newRunLedgerWithOptions(workspace, policy.RunLedger, options)
	if ledgerErr != nil {
		runLogger.Warn("run_ledger_unavailable", slog.String("workspace", workspace), slog.Any("error", ledgerErr))
	}
	checkpointID := options.checkpointID(runLedger.ID())
	observer := newRunObserver(runLedger)
	if runLedger != nil {
		defer func() {
			if err := runLedger.Close(); err != nil {
				runLogger.Warn("run_ledger_close_failed", slog.String("run_id", runLedger.ID()), slog.Any("error", err))
			}
		}()
	}
	finished := false
	finishRun := func(status, reason string, generatedBytes int) {
		if finished {
			return
		}
		finished = true
		if err := runLedger.RecordFinish(status, reason, generatedBytes); err != nil {
			runLogger.Warn("run_ledger_finish_failed", slog.String("run_id", runLedger.ID()), slog.Any("error", err))
		}
	}

	recorder := newDisplayEventRecorder(conversation)
	mutations := newMutationTracker()
	rawEmit := emit
	emit = func(ev Event) {
		mutations.Observe(ev)
		recorder.Record(ev)
		if err := runLedger.RecordEvent(ev); err != nil {
			runLogger.Warn("run_ledger_event_failed", slog.String("run_id", runLedger.ID()), slog.String("event_type", ev.Type), slog.Any("error", err))
		}
		rawEmit(ev)
	}
	emit(Event{Type: "run_state", Data: map[string]string{
		"run_id":     runLedger.ID(),
		"task_id":    options.TaskID,
		"agent_kind": options.AgentKind,
		"session_id": options.SessionID,
		"phase":      "started",
	}})
	originalMessage := req.Message
	if err := runLedger.Record("run_started", map[string]any{
		"workspace":        workspace,
		"task_id":          options.TaskID,
		"agent_kind":       options.AgentKind,
		"session_id":       options.SessionID,
		"mode":             options.Mode,
		"message":          textSummary{Bytes: len(originalMessage), Chars: len([]rune(originalMessage)), Preview: safeLogPreview(originalMessage, policy.RunLedger.PreviewChars)},
		"references":       len(req.References),
		"lore_references":  len(req.LoreReferences),
		"style_references": len(req.StyleReferences),
		"selections":       len(req.Selections),
		"plan_mode":        req.PlanMode,
		"checkpoint_id":    checkpointID,
	}); err != nil {
		runLogger.Warn("run_ledger_start_failed", slog.String("run_id", runLedger.ID()), slog.Any("error", err))
	}
	var resumeInterruption *session.Interruption
	if shouldResumeInterruptedRequest(req.Message) {
		resumeInterruption = conversation.PendingInterruption()
		if resumeInterruption != nil {
			req.Message = buildInterruptedResumeMessage(req.Message, resumeInterruption)
		}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			runLogger.Error("panic_recovered", slog.Any("error", recovered))
			markInterruptionIfNeeded(conversation, resumeInterruption, originalMessage, "", fmt.Sprint(recovered))
			finishRun("panic", fmt.Sprint(recovered), 0)
			emit(Event{Type: "error", Data: map[string]string{"message": "Agent 异常中断"}})
		}
	}()

	agentMessage := req.Message
	contextLog := newContextBuildLog(policy.ContextLedger)
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
		runLogger.Error("prepare_messages_failed", slog.Any("error", err))
		finishRun("error", err.Error(), 0)
		emit(Event{Type: "error", Data: map[string]string{"message": err.Error()}})
		return
	}
	if err := runLedger.RecordContext(contextLog.Audit()); err != nil {
		runLogger.Warn("run_ledger_context_failed", slog.String("run_id", runLedger.ID()), slog.Any("error", err))
	}
	runLogger.Info(
		"context_composition",
		slog.String("history", messageListSummary(history)),
		slog.String("original", promptPartSummary(originalMessage)),
		slog.String("agent_message", promptPartSummary(agentMessage)),
		slog.String("references", stringListSummary(req.References)),
		slog.String("lore_references", stringListSummary(req.LoreReferences)),
		slog.String("style_references", stringListSummary(req.StyleReferences)),
		slog.Int("style_rules", len(req.StyleRules)),
		slog.String("selections", selectionListSummary(req.Selections)),
		slog.Bool("plan_mode", req.PlanMode),
		slog.Bool("resumed", resumeInterruption != nil),
	)
	runLogger.Info("context_sources", slog.String("summary", contextLog.String()), slog.Any("sources", contextLog.Audit()))
	if reporter, ok := conversation.(ContextSourceReporter); ok {
		if sources := strings.TrimSpace(reporter.ContextSourceSummary()); sources != "" {
			runLogger.Info("conversation_context_sources", slog.String("sources", sources))
		}
	}

	runCtx := ContextWithRunObserver(ctx, observer)
	runOptions := []adk.AgentRunOption{}
	if checkpointID != "" {
		runOptions = append(runOptions, adk.WithCheckPointID(checkpointID))
	}
	events := runner.Run(runCtx, history, runOptions...)
	var fullContent strings.Builder
	var fullThinking strings.Builder
	runLogger.Info("run_started", slog.Int("history", len(history)), slog.Int("message_len", len(req.Message)), slog.Int("agent_message_len", len(agentMessage)), slog.Bool("plan_mode", req.PlanMode), slog.Int("style_references", len(req.StyleReferences)), slog.Int("style_rules", len(req.StyleRules)))

	for {
		if err := ctx.Err(); err != nil {
			runLogger.Warn("run_interrupted", slog.String("reason", "context"), slog.Any("error", err), slog.Int("generated_bytes", fullContent.Len()))
			generatedBytes := fullContent.Len()
			appendAssistantIfAny(conversation, &fullContent, &fullThinking)
			finishRun("aborted", err.Error(), generatedBytes)
			emit(Event{Type: "aborted", Data: map[string]string{}})
			return
		}
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			runLogger.Error("run_interrupted", slog.String("reason", "runner_error"), slog.Any("error", event.Err), slog.Int("generated_bytes", fullContent.Len()))
			generated := appendAssistantIfAny(conversation, &fullContent, &fullThinking)
			markInterruptionIfNeeded(conversation, resumeInterruption, originalMessage, generated, event.Err.Error())
			finishRun("error", event.Err.Error(), len(generated))
			emit(Event{Type: "error", Data: map[string]string{"message": event.Err.Error()}})
			return
		}

		if event.Output == nil || event.Output.MessageOutput == nil {
			runLogger.Warn("invalid_output_skipped", slog.Bool("output_nil", event.Output == nil), slog.Bool("message_output_nil", event.Output != nil && event.Output.MessageOutput == nil))
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
				finishRun("error", "stream recv error", len(generated))
				return
			}
			continue
		}
		if mv.Message != nil {
			processNonStreamingEvent(mv, &fullContent, &fullThinking, emit)
		}
	}

	generatedBytes := fullContent.Len()
	appendAssistantIfAny(conversation, &fullContent, &fullThinking)
	if resumeInterruption != nil {
		if err := conversation.ResolveInterruption(resumeInterruption.ID); err != nil {
			runLogger.Error("resolve_interruption_failed", slog.String("interruption_id", resumeInterruption.ID), slog.Any("error", err))
		}
	}
	observedMutations := mutations.Mutations()
	observer.RecordMutations(observedMutations)
	verification := VerifyPostRunMutations(bookService, observedMutations)
	observer.RecordVerification(verification)
	if options.OnMutationsVerified != nil && len(observedMutations) > 0 {
		options.OnMutationsVerified(ctx, observedMutations, verification)
	}
	if verification.Mutations > 0 {
		runLogger.Info("post_run_verification", slog.String("status", verification.Status), slog.Int("mutations", verification.Mutations), slog.Int("checks", len(verification.Checks)), slog.Any("warnings", verification.Warnings))
		emit(Event{Type: "post_run_verification", Data: verification})
		emit(Event{Type: "verification", Data: verification})
	}
	runLogger.Info("run_completed")
	finishRun("success", "", generatedBytes)
	emit(Event{Type: "run_state", Data: map[string]string{
		"run_id":     runLedger.ID(),
		"task_id":    options.TaskID,
		"agent_kind": options.AgentKind,
		"session_id": options.SessionID,
		"phase":      "finished",
		"status":     "success",
	}})
	emit(Event{Type: "done", Data: map[string]string{}})
}
