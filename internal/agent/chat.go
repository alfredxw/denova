package agent

import (
	"context"
	"errors"

	"github.com/alfredxw/denova/adk"

	runstate "denova/internal/agent/runtime"
	"denova/internal/book"
	"denova/internal/prompts"
)

const (
	maxReferenceFileBytes = 256 * 1024
)

// Event 表示 Agent 输出的传输无关事件。
type Event struct {
	Type string
	Data interface{}
}

// ChatRequest 表示一次聊天请求的传输无关参数。
type ChatRequest struct {
	// CommandID is the caller-owned idempotency key for a root turn. HTTP
	// clients must retain it while acceptance is uncertain; server-side retry
	// code must never replace it with a newly generated identity.
	CommandID      string             `json:"command_id"`
	Message        string             `json:"message"`
	References     []string           `json:"references"`
	LoreReferences []string           `json:"lore_references"`
	StyleScenes    []string           `json:"style_scenes"`
	Selections     []TextSelectionRef `json:"selections"`
	IDEContext     IDEContextRef      `json:"ide_context,omitempty"`
	ReviewFeedback ReviewFeedbackRefs `json:"review_feedback,omitempty"`
	PlanMode       bool               `json:"plan_mode"`
	WritingSkill   string             `json:"writing_skill"`
	ImagePresetID  string             `json:"image_preset_id"`
	TellerID       string             `json:"teller_id"`
	Locale         string             `json:"-"`

	// StyleRules 由后端按当前导演配置注入（场景 → 共享文风参考索引）。
	// StyleScenes 非空时只注入用户本轮通过 # 指定的场景；为空时作为场景化建议参与本轮上下文。
	StyleRules []StyleRule `json:"-"`

	// ImagePreset is resolved by the app layer from ImagePresetID or workspace settings.
	ImagePreset ImagePresetContext `json:"-"`

	// ResolvedReviewFeedback is populated by the app layer from a canonical
	// workspace review ledger. Clients may submit IDs only, never comment text.
	ResolvedReviewFeedback ReviewFeedbackContexts `json:"-"`

	// callerInput freezes the transport payload before the app resolves mutable
	// defaults and canonical context. Durable command identity must describe what
	// the caller retried, while HarnessTurnSpec.Request keeps the resolved values
	// used by the first accepted execution.
	callerInput *chatRequestCallerInput
}

// chatRequestCallerInput is deliberately private: it is command identity, not
// another model-visible context surface. Keep this list aligned with public
// caller-controlled ChatRequest fields.
type chatRequestCallerInput struct {
	CommandID      string             `json:"command_id"`
	Message        string             `json:"message"`
	References     []string           `json:"references,omitempty"`
	LoreReferences []string           `json:"lore_references,omitempty"`
	StyleScenes    []string           `json:"style_scenes,omitempty"`
	Selections     []TextSelectionRef `json:"selections,omitempty"`
	IDEContext     IDEContextRef      `json:"ide_context,omitempty"`
	ReviewFeedback ReviewFeedbackRefs `json:"review_feedback,omitempty"`
	PlanMode       bool               `json:"plan_mode"`
	WritingSkill   string             `json:"writing_skill,omitempty"`
	ImagePresetID  string             `json:"image_preset_id,omitempty"`
	TellerID       string             `json:"teller_id,omitempty"`
	Locale         string             `json:"locale,omitempty"`
}

// CaptureChatRequestCallerInput freezes a deep copy of caller-controlled
// request fields exactly once. App preparation may safely resolve defaults on
// the returned request without changing its durable retry identity.
func CaptureChatRequestCallerInput(req ChatRequest) ChatRequest {
	if req.callerInput != nil {
		return req
	}
	req.callerInput = &chatRequestCallerInput{
		CommandID: req.CommandID, Message: req.Message,
		References: cloneStrings(req.References), LoreReferences: cloneStrings(req.LoreReferences),
		StyleScenes: cloneStrings(req.StyleScenes), Selections: cloneTextSelectionRefs(req.Selections),
		IDEContext:     IDEContextRef{CurrentFile: req.IDEContext.CurrentFile, OpenFiles: cloneStrings(req.IDEContext.OpenFiles)},
		ReviewFeedback: cloneReviewFeedbackRefs(req.ReviewFeedback),
		PlanMode:       req.PlanMode, WritingSkill: req.WritingSkill,
		ImagePresetID: req.ImagePresetID, TellerID: req.TellerID, Locale: req.Locale,
	}
	return req
}

func chatRequestCallerView(req ChatRequest) chatRequestCallerInput {
	if req.callerInput != nil {
		return *req.callerInput
	}
	return *CaptureChatRequestCallerInput(req).callerInput
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func cloneTextSelectionRefs(values []TextSelectionRef) []TextSelectionRef {
	return append([]TextSelectionRef(nil), values...)
}

func cloneReviewFeedbackRefs(values ReviewFeedbackRefs) ReviewFeedbackRefs {
	cloned := make(ReviewFeedbackRefs, len(values))
	for index, value := range values {
		cloned[index] = value
		cloned[index].CommentIDs = cloneStrings(value.CommentIDs)
	}
	return cloned
}

// StyleRule 是 prompts.StyleRule 的镜像，避免调用方直接依赖 prompts 包。
type StyleRule = prompts.StyleRule

// StyleReference 是 prompts.StyleReference 的镜像，避免调用方直接依赖 prompts 包。
type StyleReference = prompts.StyleReference

// IDEContextRef carries lightweight, model-visible IDE state for one turn.
// It must describe UI focus only and must not include editor file content.
type IDEContextRef struct {
	CurrentFile string   `json:"current_file,omitempty"`
	OpenFiles   []string `json:"open_files,omitempty"`
}

// ImagePresetContext is a bounded visual style preset for image generation only.
type ImagePresetContext struct {
	ID                string
	Name              string
	AgentSystemPrompt string
	ToolRequestPrompt string
}

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
	harness *chatHarness
}

// turnExecutor owns one task-level Agent loop behind the durable harness. It is
// deliberately private so production callers cannot bypass command admission.
type turnExecutor struct {
	policy LoopPolicy
}

// NewEphemeralChatService creates an in-memory ChatService for tests and
// short-lived local execution. Production wiring must use NewDurableChatService.
func NewEphemeralChatService() *ChatService {
	return NewEphemeralChatServiceWithPolicy(DefaultLoopPolicy())
}

// NewEphemeralChatServiceWithPolicy creates an ephemeral service with an
// explicit loop policy.
func NewEphemeralChatServiceWithPolicy(policy LoopPolicy) *ChatService {
	service, err := newHarnessChatService(context.Background(), policy, runstate.NewMemoryJournalStore())
	if err != nil {
		// The in-memory store and built-in factory are fully local invariants; a
		// construction error is a programming bug, not a legacy fallback signal.
		panic(err)
	}
	return service
}

func newTurnExecutor(policy LoopPolicy) *turnExecutor {
	return &turnExecutor{policy: policy.normalized()}
}

func (s *ChatService) RunWithOptions(
	ctx context.Context,
	runner *adk.Runner,
	conversation Conversation,
	bookService *book.Service,
	req ChatRequest,
	options RunOptions,
	emit func(Event),
) RunOutcome {
	if s == nil || s.harness == nil {
		err := errors.New("durable agent harness is unavailable")
		emitHarnessError(emit, err)
		return outcomeFromOutput(RunOutcomeFailed, err, err.Error(), "", "")
	}
	return s.harness.run(ctx, runner, conversation, bookService, req, options, emit)
}

// StartWithOptions durably accepts StartTurn and returns before waiting for the
// model operation to settle. API layers use this boundary before publishing a
// display task ID; legacy synchronous callers continue to use RunWithOptions.
func (s *ChatService) StartWithOptions(
	ctx context.Context,
	runner *adk.Runner,
	conversation Conversation,
	bookService *book.Service,
	req ChatRequest,
	options RunOptions,
	emit func(Event),
) (*AcceptedRun, error) {
	if s == nil || s.harness == nil {
		return nil, errors.New("durable agent harness is unavailable")
	}
	return s.harness.start(ctx, runner, conversation, bookService, req, options, emit)
}

func (r *turnExecutor) Run(
	ctx context.Context,
	runner *adk.Runner,
	conversation Conversation,
	bookService *book.Service,
	req ChatRequest,
	options RunOptions,
	emit func(Event),
) RunOutcome {
	run := newChatRun(r, ctx, runner, conversation, bookService, req, options, emit)
	return run.execute()
}

func interactiveTurnCompletedByCancel(err error, agentKind string, conversation Conversation, generatedBytes int) bool {
	if err == nil || agentKind != AgentKindInteractiveStory || generatedBytes == 0 {
		return false
	}
	reporter, ok := conversation.(InteractiveNarrativeReadinessReporter)
	if !ok || !reporter.InteractiveNarrativeReady() {
		return false
	}
	var cancelErr *adk.CancelError
	return errors.As(err, &cancelErr) && cancelErr.Info != nil && cancelErr.Info.Mode&adk.CancelAfterToolCalls != 0
}
