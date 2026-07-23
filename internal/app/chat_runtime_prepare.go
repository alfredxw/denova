package app

import (
	"context"
	"fmt"
	"log"
	"strings"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/imagepreset"
	"denova/internal/interactive"
	"denova/internal/styleref"
)

func (s *ChatAppService) prepareIDEChatRuntime(ctx context.Context, req agent.ChatRequest) (ideChatRuntime, agent.ChatRequest, error) {
	a := s.app
	a.mu.Lock()
	if a.session == nil || a.bookState == nil || a.cfg == nil {
		a.mu.Unlock()
		return ideChatRuntime{}, req, ErrNoWorkspace
	}

	runtime := ideChatRuntime{
		app:            a,
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
	novaDir := runtime.cfg.DataDir()
	a.mu.Unlock()

	if layered, err := config.LoadLayeredWithStartupConfig(novaDir, runtime.workspace); err == nil {
		applyLayeredSettingsToConfig(&runtime.cfg, layered)
		applyRequestLocaleToConfig(&runtime.cfg, req.Locale)
		runtime.cfg.IDEStoryTellerID = layered.Effective.IDEStoryTellerID
		if requestTellerID := strings.TrimSpace(req.TellerID); requestTellerID != "" {
			runtime.cfg.IDEStoryTellerID = requestTellerID
		}
		if runtime.cfg.IDEStoryTellerID == "" {
			runtime.cfg.IDEStoryTellerID = "classic"
		}
		req.TellerID = runtime.cfg.IDEStoryTellerID
		log.Printf("[agent-task] load ide teller id=%s workspace=%s", runtime.cfg.IDEStoryTellerID, runtime.workspace)

		teller := loadInteractiveTeller(novaDir, runtime.cfg.IDEStoryTellerID)
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
	if err := applyWritingSkillRuntimePolicy(&runtime, &req); err != nil {
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

func applyImagePresetRuntimePolicy(runtime *ideChatRuntime, req *agent.ChatRequest) {
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
	req.ImagePreset = agent.ImagePresetContext{
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

func applyWritingSkillRuntimePolicy(runtime *ideChatRuntime, req *agent.ChatRequest) error {
	if runtime == nil || req == nil {
		return nil
	}
	req.WritingSkill = agent.ResolveWritingSkillName(&runtime.cfg, req.WritingSkill)
	log.Printf("[agent-task] selected writing skill name=%s workspace=%s", req.WritingSkill, runtime.workspace)
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

func appStyleRuleNames(rules []agent.StyleRule) []string {
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

func convertTellerStyleRules(novaDir string, globalRefs []string, rules []interactive.StyleRule, scenes []string) []agent.StyleRule {
	converted := make([]agent.StyleRule, 0, len(rules)+1)
	allowed := styleSceneSet(scenes)
	styleRefs := styleref.NewLibrary(novaDir)
	if len(globalRefs) > 0 {
		converted = append(converted, agent.StyleRule{
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
			converted = append(converted, agent.StyleRule{
				Global:          true,
				StyleReferences: styleReferencesForPrompt(styleRefs.Resolve(r.StyleRefs)),
				StyleContents:   r.StyleContents,
			})
			continue
		}
		if len(allowed) > 0 && !allowed[scene] {
			continue
		}
		converted = append(converted, agent.StyleRule{
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

func styleReferencesForPrompt(refs []styleref.Reference) []agent.StyleReference {
	result := make([]agent.StyleReference, 0, len(refs))
	for _, ref := range refs {
		result = append(result, agent.StyleReference{
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
