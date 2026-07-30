package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"denova/config"
	agents "denova/internal/agents"
	novaskills "denova/internal/agents/skills"
	"denova/internal/book"
	"denova/internal/imagepreset"
	"denova/internal/interactive"
	"denova/internal/narrativestyle"
	"denova/internal/styleref"
)

func (s *ChatAppService) prepareIDEChatRuntime(ctx context.Context, req agents.ChatRequest) (ideChatRuntime, agents.ChatRequest, error) {
	a := s.app
	a.mu.Lock()
	if a.session == nil || a.bookState == nil || a.cfg == nil {
		a.mu.Unlock()
		return ideChatRuntime{}, req, ErrNoWorkspace
	}

	runtime := ideChatRuntime{
		app:            a,
		projectID:      a.cfg.ProjectID,
		projectType:    ProjectTypeBook,
		projectState:   a.cfg.ProjectStateDir,
		agentKind:      agents.AgentKindIDE,
		sess:           a.session,
		state:          a.bookState,
		bookService:    a.bookService,
		chatService:    a.chatService,
		workspace:      a.workspace,
		versionService: a.versionService,
		cfg:            *a.cfg,
	}
	runtime.cfg.Workspace = runtime.workspace
	runtime.ideTeller = ideStoryTellerForConfig(&runtime.cfg)
	a.mu.Unlock()
	return s.prepareIDEChatRuntimeSnapshot(ctx, runtime, req)
}

func (s *ChatAppService) prepareProjectChatRuntimeSnapshot(
	ctx context.Context,
	runtime ideChatRuntime,
	req agents.ChatRequest,
) (ideChatRuntime, agents.ChatRequest, error) {
	if runtime.projectType == ProjectTypeGeneral {
		return s.prepareGeneralChatRuntimeSnapshot(ctx, runtime, req)
	}
	return s.prepareIDEChatRuntimeSnapshot(ctx, runtime, req)
}

func (s *ChatAppService) prepareGeneralChatRuntimeSnapshot(
	ctx context.Context,
	runtime ideChatRuntime,
	req agents.ChatRequest,
) (ideChatRuntime, agents.ChatRequest, error) {
	if err := ctx.Err(); err != nil {
		return ideChatRuntime{}, req, err
	}
	runtime.cfg.Workspace = runtime.workspace
	runtime.cfg.ProjectID = runtime.projectID
	runtime.cfg.ProjectStateDir = runtime.projectState
	projectConfigPath := config.ProjectConfigPath(runtime.projectState)
	layered, err := config.LoadLayeredWithStartupConfigAt(runtime.cfg.DataDir(), runtime.workspace, projectConfigPath)
	if err != nil {
		return ideChatRuntime{}, req, fmt.Errorf("load General Agent project settings: %w", err)
	}
	applyLayeredSettingsToConfig(&runtime.cfg, layered)
	runtime.cfg.Workspace = runtime.workspace
	runtime.cfg.ProjectID = runtime.projectID
	runtime.cfg.ProjectStateDir = runtime.projectState
	applyRequestLocaleToConfig(&runtime.cfg, req.Locale)
	if err := s.resolveReviewFeedback(ctx, runtime, &req); err != nil {
		return ideChatRuntime{}, req, err
	}
	return runtime, req, nil
}

// prepareIDEChatRuntimeSnapshot applies the same request/runtime policy to an
// explicitly captured project runtime. The foreground Writing workspace and
// user-level AgentChat both use this path, so prompt, Skills, teller, image,
// review and resident-lore behavior cannot silently drift between surfaces.
func (s *ChatAppService) prepareIDEChatRuntimeSnapshot(
	ctx context.Context,
	runtime ideChatRuntime,
	req agents.ChatRequest,
) (ideChatRuntime, agents.ChatRequest, error) {
	runtime.cfg.Workspace = runtime.workspace
	runtime.ideTeller = ideStoryTellerForConfig(&runtime.cfg)
	novaDir := runtime.cfg.DataDir()
	projectConfigPath := config.ProjectConfigPath(runtime.cfg.ProjectStateDir)
	if layered, err := config.LoadLayeredWithStartupConfigAt(novaDir, runtime.workspace, projectConfigPath); err == nil {
		applyLayeredSettingsToConfig(&runtime.cfg, layered)
		applyRequestLocaleToConfig(&runtime.cfg, req.Locale)
		runtime.cfg.IDEStoryTellerID = layered.Effective.IDEStoryTellerID
		if requestTellerID := strings.TrimSpace(req.TellerID); requestTellerID != "" {
			runtime.cfg.IDEStoryTellerID = requestTellerID
		}
		if runtime.cfg.IDEStoryTellerID == "" {
			runtime.cfg.IDEStoryTellerID = narrativestyle.DefaultID
		}
		teller := loadWritingTeller(novaDir, runtime.cfg.IDEStoryTellerID)
		if teller.ID != "" {
			runtime.cfg.IDEStoryTellerID = teller.ID
		}
		req.TellerID = runtime.cfg.IDEStoryTellerID
		log.Printf("[agent-task] load ide narrative style id=%s workspace=%s", runtime.cfg.IDEStoryTellerID, runtime.workspace)
		if len(teller.StyleRefs) > 0 || len(teller.StyleRules) > 0 {
			converted := convertTellerStyleRules(novaDir, teller.StyleRefs, teller.StyleRules, req.StyleScenes)
			req.StyleRules = converted
			log.Printf("[agent-task] inject teller style rules teller_id=%s scenes=%q count=%d rules=%q", teller.ID, req.StyleScenes, len(converted), appStyleRuleNames(converted))
		}
		runtime.ideTeller = ideStoryTellerFromInteractive(teller, req.StyleRules)
	} else {
		log.Printf("[agent-task] load layered settings failed workspace=%s err=%v", runtime.workspace, err)
		applyRequestLocaleToConfig(&runtime.cfg, req.Locale)
	}
	applyImagePresetRuntimePolicy(&runtime, &req)
	if err := applyWritingSkillRuntimePolicy(ctx, &runtime, &req); err != nil {
		return ideChatRuntime{}, req, err
	}
	if err := s.resolveReviewFeedback(ctx, runtime, &req); err != nil {
		return ideChatRuntime{}, req, err
	}
	residentBytes, err := book.NewLoreStore(runtime.workspace).ResidentContentBytes()
	if err != nil {
		return ideChatRuntime{}, req, fmt.Errorf("读取常驻资料预算失败: %w", err)
	}
	if residentBytes > book.ResidentLoreSafetyMaxBytes {
		return ideChatRuntime{}, req, fmt.Errorf("常驻资料正文异常过大（%d KB）；请检查是否误将大型文件设为常驻资料", (residentBytes+1023)/1024)
	}
	return runtime, req, nil
}

func applyImagePresetRuntimePolicy(runtime *ideChatRuntime, req *agents.ChatRequest) {
	if runtime == nil || req == nil {
		return
	}
	presetID := imagepreset.NormalizeID(req.ImagePresetID)
	if presetID == "" {
		presetID = imagepreset.NormalizeID(runtime.cfg.IDEImagePresetID)
	}
	if presetID == "" {
		presetID = imagepreset.DefaultID
	}
	req.ImagePresetID = presetID
	preset := imagepreset.DefaultPreset()
	if strings.TrimSpace(runtime.cfg.DataDir()) != "" {
		loaded, err := imagepreset.NewLibrary(runtime.cfg.DataDir()).Get(presetID)
		if err != nil {
			log.Printf("[agent-task] load image preset failed id=%s workspace=%s err=%v; fallback=%s", presetID, runtime.workspace, err, imagepreset.DefaultID)
		} else {
			preset = loaded
		}
	}
	agentSystemPrompt := preset.PromptForTargets(imagepreset.TargetAgentSystem)
	toolRequestPrompt := preset.PromptForTargets(imagepreset.TargetToolRequest)
	req.ImagePreset = agents.ImagePresetContext{
		ID:                preset.ID,
		Name:              preset.Name,
		AgentSystemPrompt: agentSystemPrompt,
		ToolRequestPrompt: toolRequestPrompt,
	}
	runtime.cfg.ImagePresetToolPrompt = toolRequestPrompt
	runtime.ideTeller.ImagePresetID = preset.ID
	runtime.ideTeller.ImagePresetName = preset.Name
	runtime.ideTeller.ImagePresetSystemPrompt = agentSystemPrompt
	log.Printf("[agent-task] selected image preset id=%s name=%q workspace=%s agent_system_chars=%d tool_request_chars=%d", req.ImagePreset.ID, req.ImagePreset.Name, runtime.workspace, len([]rune(agentSystemPrompt)), len([]rune(toolRequestPrompt)))
}

func applyWritingSkillRuntimePolicy(ctx context.Context, runtime *ideChatRuntime, req *agents.ChatRequest) error {
	if runtime == nil || req == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	selected := agents.ResolveWritingSkillName(&runtime.cfg, req.WritingSkill)
	backend := novaskills.NewAgentBackend(
		novaskills.NewDirectories(runtime.cfg.SkillsDir, runtime.cfg.DataDir(), runtime.workspace),
		config.AgentKindIDE,
		config.ResolveAgentSkillOverrides(&runtime.cfg, config.AgentKindIDE),
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
		req.WritingSkill = selected
		log.Printf("[agent-task] selected writing skill name=%s workspace=%s", req.WritingSkill, runtime.workspace)
		return nil
	}

	fallback := config.DefaultWritingSkillName
	if selected != fallback && availableNames[fallback] {
		req.WritingSkill = fallback
		log.Printf("[agent-task] writing skill unavailable name=%s workspace=%s; fallback=%s", selected, runtime.workspace, fallback)
		return nil
	}
	req.WritingSkill = ""
	log.Printf("[agent-task] no active writing skill available requested=%s workspace=%s; continue without writing skill", selected, runtime.workspace)
	return nil
}

// ActiveTask 返回当前活跃任务（可能为 nil）。
func (a *App) ActiveTask() *Task {
	return a.chat().ActiveTask()
}

func (s *ChatAppService) ActiveTask() *Task {
	a := s.app
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.activeWritingRun != nil && !a.activeWritingRun.matchesCurrent(a) {
		return nil
	}
	return a.activeTask
}

func appStyleRuleNames(rules []agents.StyleRule) []string {
	names := make([]string, 0, len(rules))
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if rule.Global {
			scene = "global"
		}
		names = append(names, fmt.Sprintf("%s -> %d refs, %d legacy contents", scene, len(rule.StyleReferences), len(rule.StyleContents)))
	}
	return names
}

func convertTellerStyleRules(novaDir string, globalRefs []string, rules []interactive.StyleRule, scenes []string) []agents.StyleRule {
	converted := make([]agents.StyleRule, 0, len(rules)+1)
	allowed := styleSceneSet(scenes)
	styleRefs := styleref.NewLibrary(novaDir)
	if len(globalRefs) > 0 {
		converted = append(converted, agents.StyleRule{
			Global:          true,
			StyleReferences: styleReferencesForPrompt(styleRefs.Resolve(globalRefs)),
		})
	}
	for _, r := range rules {
		scene := strings.TrimSpace(r.Scene)
		if scene == "" || (len(r.StyleRefs) == 0 && len(r.StyleContents) == 0) {
			continue
		}
		if isGlobalStyleScene(scene) {
			converted = append(converted, agents.StyleRule{
				Global:          true,
				StyleReferences: styleReferencesForPrompt(styleRefs.Resolve(r.StyleRefs)),
				StyleContents:   r.StyleContents,
			})
			continue
		}
		if len(allowed) > 0 && !allowed[scene] {
			continue
		}
		converted = append(converted, agents.StyleRule{
			Scene:           scene,
			StyleReferences: styleReferencesForPrompt(styleRefs.Resolve(r.StyleRefs)),
			StyleContents:   r.StyleContents,
		})
	}
	return converted
}

func isGlobalStyleScene(scene string) bool {
	normalized := strings.ToLower(strings.TrimSpace(scene))
	return normalized == "全局" || normalized == "global"
}

func styleReferencesForPrompt(refs []styleref.Reference) []agents.StyleReference {
	result := make([]agents.StyleReference, 0, len(refs))
	for _, ref := range refs {
		result = append(result, agents.StyleReference{
			Name:        ref.Name,
			Description: ref.Description,
			Path:        ref.Path,
			DisplayPath: ref.DisplayPath,
			Missing:     ref.Missing,
			Error:       ref.Error,
		})
	}
	return result
}

func styleSceneSet(scenes []string) map[string]bool {
	if len(scenes) == 0 {
		return nil
	}
	set := make(map[string]bool, len(scenes))
	for _, scene := range scenes {
		scene = strings.TrimSpace(scene)
		if scene != "" {
			set[scene] = true
		}
	}
	return set
}
