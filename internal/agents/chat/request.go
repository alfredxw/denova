package chat

import (
	agentattachment "denova/internal/agents/attachment"
	"denova/internal/agents/run"

	"denova/internal/agents/prompts"
	agentreview "denova/internal/agents/review"
	agent "github.com/alfredxw/denova/agent"
)

// ReferenceFileByteLimit bounds one caller-selected workspace reference in the
// model-visible turn context and its durable command metadata.
const ReferenceFileByteLimit = 256 * 1024

// ChatRequest 表示一次聊天请求的传输无关参数。
type ChatRequest struct {
	// agentrun.CommandID is the caller-owned idempotency key for a root turn. HTTP
	// clients must retain it while acceptance is uncertain; server-side retry
	// code must never replace it with a newly generated identity.
	CommandID string `json:"command_id"`
	Message   string `json:"message"`
	// ResumeInterruptionID binds this turn to one exact pending interruption.
	// The current Message remains authoritative and may redirect the resumed work.
	ResumeInterruptionID string `json:"resume_interruption_id,omitempty"`
	// DisplayMessage is an optional user-facing projection of Message. The
	// canonical Message remains model-visible and durable for recovery.
	DisplayMessage    string                   `json:"display_message,omitempty"`
	AttachmentUploads []agentattachment.Upload `json:"attachments,omitempty"`
	AttachmentIDs     []string                 `json:"attachment_ids,omitempty"`
	References        []string                 `json:"references"`
	LoreReferences    []string                 `json:"lore_references"`
	StyleScenes       []string                 `json:"style_scenes"`
	Selections        []TextSelectionRef       `json:"selections"`
	IDEContext        prompts.IDEContextRef    `json:"ide_context,omitempty"`
	ReviewFeedback    agentreview.Refs         `json:"review_feedback,omitempty"`
	PlanMode          bool                     `json:"plan_mode"`
	WritingSkill      string                   `json:"writing_skill"`
	ImagePresetID     string                   `json:"image_preset_id"`
	TellerID          string                   `json:"teller_id"`
	Locale            string                   `json:"-"`
	InputVisibility   agentrun.InputVisibility `json:"-"`
	AttachedFiles     []agent.Attachment       `json:"-"`

	// StyleRules 由后端按当前导演配置注入（场景 → 共享文风参考索引）。
	// StyleScenes 非空时只注入用户本轮通过 # 指定的场景；为空时作为场景化建议参与本轮上下文。
	StyleRules []prompts.StyleRule `json:"-"`

	// ImagePreset is resolved by the app layer from ImagePresetID or workspace settings.
	ImagePreset ImagePresetContext `json:"-"`

	// ResolvedReviewFeedback is populated by the app layer from a canonical
	// workspace review ledger. Clients may submit IDs only, never comment text.
	ResolvedReviewFeedback agentreview.Contexts `json:"-"`

	// callerInput freezes the transport payload before the app resolves mutable
	// defaults and canonical context. Durable command identity must describe what
	// the caller retried, while CycleSpec.Request keeps the resolved values
	// used by the first accepted execution.
	callerInput *CallerInput
}

// CallerInput is the immutable, caller-controlled command identity used by the
// durable execution. It is not model-visible context. Keep this list aligned with
// the public caller-controlled ChatRequest fields.
type CallerInput struct {
	CommandID            string                `json:"command_id"`
	Message              string                `json:"message"`
	ResumeInterruptionID string                `json:"resume_interruption_id,omitempty"`
	DisplayMessage       string                `json:"display_message,omitempty"`
	AttachmentIDs        []string              `json:"attachment_ids,omitempty"`
	References           []string              `json:"references,omitempty"`
	LoreReferences       []string              `json:"lore_references,omitempty"`
	StyleScenes          []string              `json:"style_scenes,omitempty"`
	Selections           []TextSelectionRef    `json:"selections,omitempty"`
	IDEContext           prompts.IDEContextRef `json:"ide_context,omitempty"`
	ReviewFeedback       agentreview.Refs      `json:"review_feedback,omitempty"`
	PlanMode             bool                  `json:"plan_mode"`
	WritingSkill         string                `json:"writing_skill,omitempty"`
	ImagePresetID        string                `json:"image_preset_id,omitempty"`
	TellerID             string                `json:"teller_id,omitempty"`
	Locale               string                `json:"locale,omitempty"`
}

// CaptureChatRequestCallerInput freezes a deep copy of caller-controlled
// request fields exactly once. App preparation may safely resolve defaults on
// the returned request without changing its durable retry identity.
func CaptureChatRequestCallerInput(req ChatRequest) ChatRequest {
	if req.callerInput != nil {
		return req
	}
	req.callerInput = &CallerInput{
		CommandID: req.CommandID, Message: req.Message, DisplayMessage: req.DisplayMessage,
		ResumeInterruptionID: req.ResumeInterruptionID,
		AttachmentIDs:        cloneStrings(req.AttachmentIDs),
		References:           cloneStrings(req.References), LoreReferences: cloneStrings(req.LoreReferences),
		StyleScenes: cloneStrings(req.StyleScenes), Selections: cloneTextSelectionRefs(req.Selections),
		IDEContext:     prompts.IDEContextRef{CurrentFile: req.IDEContext.CurrentFile, OpenFiles: cloneStrings(req.IDEContext.OpenFiles)},
		ReviewFeedback: req.ReviewFeedback.Clone(),
		PlanMode:       req.PlanMode, WritingSkill: req.WritingSkill,
		ImagePresetID: req.ImagePresetID, TellerID: req.TellerID, Locale: req.Locale,
	}
	return req
}

// CallerView returns the immutable caller-controlled identity for req. The
// returned value may be encoded by durable admission without exposing resolved
// defaults or canonical context as part of the caller's idempotency key.
func CallerView(req ChatRequest) CallerInput {
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
