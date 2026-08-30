package conversationapp

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	"denova/internal/agents/prompts"
	agentrun "denova/internal/agents/run"
	novaskills "denova/internal/agents/skills"
	appagentruntime "denova/internal/app/agentruntime"
	interactiveapp "denova/internal/app/interactive"
	reviewapp "denova/internal/app/review"
	appsettings "denova/internal/app/settings"
	booklore "denova/internal/book/lore"
	imagepreset "denova/internal/image/preset"
	projectdomain "denova/internal/project"
	"denova/internal/style"
)

func Prepare(ctx context.Context, runtime Runtime, request agentchat.ChatRequest) (Runtime, agentchat.ChatRequest, error) {
	switch runtime.ProjectType {
	case projectdomain.TypeGeneral, projectdomain.TypeHarness:
		return prepareGeneral(ctx, runtime, request)
	case projectdomain.TypeBook:
		return prepareWriting(ctx, runtime, request)
	default:
		return Runtime{}, request, fmt.Errorf("unsupported project type %q", runtime.ProjectType)
	}
}

func prepareGeneral(ctx context.Context, runtime Runtime, request agentchat.ChatRequest) (Runtime, agentchat.ChatRequest, error) {
	if err := ctx.Err(); err != nil {
		return Runtime{}, request, err
	}
	runtime.Config.Workspace = runtime.Workspace
	runtime.Config.ProjectID = runtime.ProjectID
	runtime.Config.ProjectStateDir = runtime.ProjectState
	layered, err := config.LoadLayeredWithStartupConfigAt(runtime.Config.DataDir(), runtime.Workspace, config.ProjectConfigPath(runtime.ProjectState))
	if err != nil {
		return Runtime{}, request, fmt.Errorf("load General Agent project settings: %w", err)
	}
	appsettings.ApplyLayered(&runtime.Config, layered)
	runtime.Config.Workspace = runtime.Workspace
	runtime.Config.ProjectID = runtime.ProjectID
	runtime.Config.ProjectStateDir = runtime.ProjectState
	appsettings.ApplyLocale(&runtime.Config, request.Locale)
	if err := reviewapp.Resolve(ctx, ReviewRuntime(runtime), &request); err != nil {
		return Runtime{}, request, err
	}
	if _, err := agentconversation.ApplySession(runtime.Session, &runtime.Config, runtime.AgentKind); err != nil {
		return Runtime{}, request, err
	}
	return runtime, request, nil
}

func prepareWriting(ctx context.Context, runtime Runtime, request agentchat.ChatRequest) (Runtime, agentchat.ChatRequest, error) {
	runtime.Config.Workspace = runtime.Workspace
	runtime.IDETeller = appagentruntime.WritingTellerForConfig(&runtime.Config)
	dataDir := runtime.Config.DataDir()
	projectConfigPath := config.ProjectConfigPath(runtime.Config.ProjectStateDir)
	if layered, err := config.LoadLayeredWithStartupConfigAt(dataDir, runtime.Workspace, projectConfigPath); err == nil {
		appsettings.ApplyLayered(&runtime.Config, layered)
		appsettings.ApplyLocale(&runtime.Config, request.Locale)
		runtime.Config.IDEStoryTellerID = layered.Effective.IDEStoryTellerID
		if requestTellerID := strings.TrimSpace(request.TellerID); requestTellerID != "" {
			runtime.Config.IDEStoryTellerID = requestTellerID
		}
		if runtime.Config.IDEStoryTellerID == "" {
			runtime.Config.IDEStoryTellerID = style.DefaultID
		}
		teller := interactiveapp.LoadWritingTeller(dataDir, runtime.Config.IDEStoryTellerID)
		if teller.ID != "" {
			runtime.Config.IDEStoryTellerID = teller.ID
		}
		request.TellerID = runtime.Config.IDEStoryTellerID
		slog.InfoContext(ctx, fmt.Sprintf("[agent-task] load ide narrative style id=%s workspace=%s", runtime.Config.IDEStoryTellerID, runtime.Workspace))
		if len(teller.StyleRefs) > 0 || len(teller.StyleRules) > 0 {
			converted := appagentruntime.StyleRules(dataDir, teller.StyleRefs, teller.StyleRules, request.StyleScenes)
			request.StyleRules = converted
			slog.InfoContext(ctx, fmt.Sprintf("[agent-task] inject teller style rules teller_id=%s scenes=%q count=%d rules=%q", teller.ID, request.StyleScenes, len(converted), appagentruntime.StyleRuleNames(converted)))
		}
		runtime.IDETeller = appagentruntime.WritingTeller(teller, request.StyleRules)
	} else {
		slog.ErrorContext(ctx, fmt.Sprintf("[agent-task] load layered settings failed workspace=%s err=%v", runtime.Workspace, err))
		appsettings.ApplyLocale(&runtime.Config, request.Locale)
	}
	applyImagePresetPolicy(&runtime, &request)
	if err := ApplyWritingSkillPolicy(ctx, &runtime, &request); err != nil {
		return Runtime{}, request, err
	}
	if err := reviewapp.Resolve(ctx, ReviewRuntime(runtime), &request); err != nil {
		return Runtime{}, request, err
	}
	residentBytes, err := booklore.NewStore(runtime.Workspace).ResidentContentBytes()
	if err != nil {
		return Runtime{}, request, fmt.Errorf("read resident lore budget / 读取常驻资料预算失败: %w", err)
	}
	if residentBytes > booklore.ResidentLoreSafetyMaxBytes {
		return Runtime{}, request, fmt.Errorf("resident lore is unexpectedly large (%d KB); check for oversized resident files / 常驻资料正文异常过大（%d KB），请检查是否误将大型文件设为常驻资料", (residentBytes+1023)/1024, (residentBytes+1023)/1024)
	}
	if _, err := agentconversation.ApplySession(runtime.Session, &runtime.Config, runtime.AgentKind); err != nil {
		return Runtime{}, request, err
	}
	return runtime, request, nil
}

func ProjectConversation(runtime Runtime, request agentchat.ChatRequest) *agentconversation.SessionConversation {
	if runtime.AgentKind == agentrun.AgentKindGeneral || runtime.AgentKind == agentrun.AgentKindHarness {
		return agentconversation.NewSessionConversationForAgent(runtime.Session, &runtime.Config, runtime.AgentKind).
			WithInputVisibility(request.InputVisibility).
			WithInputDisplayContent(request.DisplayMessage)
	}
	runtimeContexts := prompts.IDEWorkspaceRuntimeContextsForContext(runtime.State, request.IDEContext)
	return agentconversation.NewSessionConversationForAgentWithRuntimeContexts(
		runtime.Session, &runtime.Config, config.AgentKindIDE,
		runtimeContexts.StableTitle, runtimeContexts.Stable,
		runtimeContexts.DynamicTitle, runtimeContexts.Dynamic,
	).WithInputVisibility(request.InputVisibility).WithInputDisplayContent(request.DisplayMessage)
}

func BindReviewFeedback(options agentrun.Options, runtime Runtime, request agentchat.ChatRequest) agentrun.Options {
	return reviewapp.BindInputCommit(options, ReviewRuntime(runtime), request)
}

func ReviewRuntime(runtime Runtime) reviewapp.Runtime {
	sessionID := ""
	if runtime.Session != nil {
		sessionID = runtime.Session.ID
	}
	return reviewapp.Runtime{
		Workspace: runtime.Workspace, StateRoot: runtime.ProjectState, SessionID: sessionID,
		DocumentsEnabled: runtime.State != nil, BookService: runtime.BookService,
	}
}

func applyImagePresetPolicy(runtime *Runtime, request *agentchat.ChatRequest) {
	if runtime == nil || request == nil {
		return
	}
	presetID := imagepreset.NormalizeID(request.ImagePresetID)
	if presetID == "" {
		presetID = imagepreset.NormalizeID(runtime.Config.IDEImagePresetID)
	}
	if presetID == "" {
		presetID = imagepreset.DefaultID
	}
	request.ImagePresetID = presetID
	preset := imagepreset.DefaultPreset()
	if strings.TrimSpace(runtime.Config.DataDir()) != "" {
		loaded, err := imagepreset.NewLibrary(runtime.Config.DataDir()).Get(presetID)
		if err != nil {
			slog.ErrorContext(context.Background(), fmt.Sprintf("[agent-task] load image preset failed id=%s workspace=%s err=%v; fallback=%s", presetID, runtime.Workspace, err, imagepreset.DefaultID))
		} else {
			preset = loaded
		}
	}
	agentSystemPrompt := preset.PromptForTargets(imagepreset.TargetAgentSystem)
	toolRequestPrompt := preset.PromptForTargets(imagepreset.TargetToolRequest)
	request.ImagePreset = agentchat.ImagePresetContext{
		ID: preset.ID, Name: preset.Name, AgentSystemPrompt: agentSystemPrompt, ToolRequestPrompt: toolRequestPrompt,
	}
	runtime.Config.ImagePresetToolPrompt = toolRequestPrompt
	runtime.IDETeller.ImagePresetID = preset.ID
	runtime.IDETeller.ImagePresetName = preset.Name
	runtime.IDETeller.ImagePresetSystemPrompt = agentSystemPrompt
	slog.InfoContext(context.Background(), fmt.Sprintf("[agent-task] selected image preset id=%s name=%q workspace=%s agent_system_chars=%d tool_request_chars=%d", request.ImagePreset.ID, request.ImagePreset.Name, runtime.Workspace, len([]rune(agentSystemPrompt)), len([]rune(toolRequestPrompt))))
}

func ApplyWritingSkillPolicy(ctx context.Context, runtime *Runtime, request *agentchat.ChatRequest) error {
	if runtime == nil || request == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	selected := novaskills.ResolveWritingSkillName(&runtime.Config, request.WritingSkill)
	backend := novaskills.NewAgentBackend(
		novaskills.NewDirectories(runtime.Config.SkillsDir, runtime.Config.DataDir(), runtime.Workspace),
		config.AgentKindIDE,
		config.ResolveAgentSkillOverrides(&runtime.Config, config.AgentKindIDE),
	)
	available, err := backend.List(ctx)
	if err != nil {
		return fmt.Errorf("list available writing Skills: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	availableNames := make(map[string]bool, len(available))
	for _, skill := range available {
		if skill.HasCapability(novaskills.CapabilityWritingWorkflow) {
			availableNames[skill.Name] = true
		}
	}
	if availableNames[selected] {
		request.WritingSkill = selected
		slog.InfoContext(ctx, fmt.Sprintf("[agent-task] selected writing skill name=%s workspace=%s", request.WritingSkill, runtime.Workspace))
		return nil
	}
	fallback := config.DefaultWritingSkillName
	if selected != fallback && availableNames[fallback] {
		request.WritingSkill = fallback
		slog.WarnContext(ctx, fmt.Sprintf("[agent-task] writing skill unavailable name=%s workspace=%s; fallback=%s", selected, runtime.Workspace, fallback))
		return nil
	}
	request.WritingSkill = ""
	slog.InfoContext(ctx, fmt.Sprintf("[agent-task] no active writing skill available requested=%s workspace=%s; continue without writing skill", selected, runtime.Workspace))
	return nil
}
