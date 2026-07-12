package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"unicode/utf8"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
)

const maxInteractiveStateSchemaPromptBytes = 32 * 1024
const stateSchemaAdaptationInstructionPrefix = "以下 JSON 是本次唯一可用的有界上下文，每个片段均标明来源字段；不要假设未提供的故事设定。\n"

func generateInteractiveStateSchema(ctx context.Context, cfg *config.Config, _ *book.State, _ agent.InteractiveStoryToolContext, instruction string) (string, error) {
	return agent.GenerateInteractiveStateSchemaAdaptation(ctx, cfg, instruction)
}

type stateSchemaAdaptationPrompt struct {
	Task         string                       `json:"task"`
	Sources      stateSchemaAdaptationSources `json:"sources"`
	StatePreset  stateSchemaAdaptationPreset  `json:"state_preset"`
	TRPGBindings []stateSchemaAdaptationRule  `json:"trpg_bindings"`
	Limits       map[string]int               `json:"limits"`
}

type stateSchemaAdaptationSources struct {
	StoryTitle           string `json:"story_title"`
	StoryOrigin          string `json:"story_origin,omitempty"`
	OpeningMode          string `json:"opening_mode,omitempty"`
	OpeningText          string `json:"opening_text,omitempty"`
	StoryDirectorID      string `json:"story_director_id"`
	StoryDirectorName    string `json:"story_director_name"`
	StoryDirectorSummary string `json:"story_director_summary,omitempty"`
	DirectorStrategy     string `json:"director_strategy,omitempty"`
	CreativeBrief        string `json:"creative_brief,omitempty"`
	LoreIndex            string `json:"lore_index,omitempty"`
}

type stateSchemaAdaptationPreset struct {
	Templates     []stateSchemaAdaptationTemplate      `json:"templates"`
	InitialActors []interactive.ActorStateInitialActor `json:"initial_actors,omitempty"`
	TraitPools    []stateSchemaAdaptationTraitPool     `json:"trait_pools,omitempty"`
}

type stateSchemaAdaptationTemplate struct {
	ID          string                        `json:"id"`
	Name        string                        `json:"name"`
	Description string                        `json:"description,omitempty"`
	Fields      []interactive.ActorStateField `json:"fields,omitempty"`
	TraitRules  []interactive.ActorTraitRule  `json:"trait_rules,omitempty"`
}

type stateSchemaAdaptationTraitPool struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Traits      []string `json:"traits,omitempty"`
}

type stateSchemaAdaptationRule struct {
	ID            string                         `json:"id"`
	Label         string                         `json:"label,omitempty"`
	StateBindings []interactive.RuleStateBinding `json:"state_bindings,omitempty"`
}

func (s *InteractiveAppService) adaptStoryStateSchema(ctx context.Context, req interactive.CreateStoryRequest) (interactive.CreateStoryRequest, error) {
	if req.ActorState == nil || len(req.ActorState.Templates) == 0 {
		return req, nil
	}
	cfg := s.cfg()
	if cfg == nil || strings.TrimSpace(cfg.NovaDir) == "" {
		return req, nil
	}
	director, err := interactive.NewStoryDirectorLibrary(cfg.NovaDir).Get(req.StoryDirectorID)
	if err != nil {
		return req, fmt.Errorf("读取故事导演以初始化状态结构失败: %w", err)
	}
	if director.Strategy.StateSchemaAdaptationMode == interactive.StateSchemaAdaptationModeOff {
		return req, nil
	}
	s.app.mu.RLock()
	state := s.app.bookState
	s.app.mu.RUnlock()
	instruction, err := buildStateSchemaAdaptationInstruction(req, director, state)
	if err != nil {
		return req, err
	}
	generator := s.app.interactiveDirectorGenerator()
	if generator == nil {
		generator = generateInteractiveStateSchema
	}
	output, err := generator(ctx, cfg, state, agent.InteractiveStoryToolContext{MaintenanceTask: "state_schema_initialization"}, instruction)
	if err != nil {
		return req, fmt.Errorf("初始化故事状态结构失败: %w", err)
	}
	adaptation, err := interactive.ParseActorStateSchemaAdaptation(output)
	if err != nil {
		return req, fmt.Errorf("初始化故事状态结构失败: %w", err)
	}
	trpgSystem := director.TRPGSystem
	if req.TRPGSystem != nil {
		trpgSystem = *req.TRPGSystem
	}
	system, record, err := interactive.ApplyActorStateSchemaAdaptation(*req.ActorState, trpgSystem, adaptation)
	if err != nil {
		return req, fmt.Errorf("初始化故事状态结构校验失败: %w", err)
	}
	req.ActorState = &system
	req.ActorStateAdaptation = &record
	log.Printf("[interactive-state-schema] adapted story title=%q director_id=%s template_ops=%d field_ops=%d initial_actor_ops=%d summary=%q", req.Title, req.StoryDirectorID, record.TemplateOps, record.FieldOps, record.InitialActorOps, record.Summary)
	return req, nil
}

func buildStateSchemaAdaptationInstruction(req interactive.CreateStoryRequest, director interactive.StoryDirector, state *book.State) (string, error) {
	creativeBrief, loreIndex := stateSchemaAdaptationWorkspaceContext(state)
	trpgSystem := director.TRPGSystem
	if req.TRPGSystem != nil {
		trpgSystem = *req.TRPGSystem
	}
	prompt := stateSchemaAdaptationPrompt{
		Task: "基于故事设定对状态预设执行创建前适配，输出最小且充分的 schema diff。",
		Sources: stateSchemaAdaptationSources{
			StoryTitle:           trimStateSchemaPromptText(req.Title, 256),
			StoryOrigin:          trimStateSchemaPromptText(req.Origin, 4000),
			OpeningMode:          trimStateSchemaPromptText(req.Opening.Mode, 32),
			OpeningText:          trimStateSchemaPromptText(firstNonEmptyApp(req.Opening.CustomText, req.Opening.PresetText), 4000),
			StoryDirectorID:      trimStateSchemaPromptText(director.ID, 128),
			StoryDirectorName:    trimStateSchemaPromptText(director.Name, 256),
			StoryDirectorSummary: trimStateSchemaPromptText(director.Description, 1000),
			DirectorStrategy:     trimStateSchemaPromptText(director.Strategy.PromptMarkdown, 4000),
			CreativeBrief:        creativeBrief,
			LoreIndex:            loreIndex,
		},
		StatePreset:  compactStateSchemaAdaptationPreset(*req.ActorState),
		TRPGBindings: compactStateSchemaAdaptationRules(trpgSystem),
		Limits: map[string]int{
			"max_prompt_bytes":      maxInteractiveStateSchemaPromptBytes,
			"max_template_ops":      64,
			"max_field_ops":         64,
			"max_initial_actor_ops": 64,
		},
	}
	data, err := json.Marshal(prompt)
	if err != nil {
		return "", fmt.Errorf("序列化状态结构初始化上下文失败: %w", err)
	}
	maxPayloadBytes := maxInteractiveStateSchemaPromptBytes - len(stateSchemaAdaptationInstructionPrefix)
	if len(data) > maxPayloadBytes {
		for index := range prompt.StatePreset.Templates {
			prompt.StatePreset.Templates[index].Description = ""
			for fieldIndex := range prompt.StatePreset.Templates[index].Fields {
				prompt.StatePreset.Templates[index].Fields[fieldIndex].Description = ""
				prompt.StatePreset.Templates[index].Fields[fieldIndex].UpdateInstruction = ""
			}
		}
		for index := range prompt.StatePreset.TraitPools {
			prompt.StatePreset.TraitPools[index].Description = ""
			prompt.StatePreset.TraitPools[index].Traits = nil
		}
		data, err = json.Marshal(prompt)
		if err != nil {
			return "", fmt.Errorf("压缩状态结构初始化上下文失败: %w", err)
		}
	}
	if len(data) > maxPayloadBytes {
		return "", fmt.Errorf("状态结构初始化上下文超过上限: %d > %d bytes", len(data)+len(stateSchemaAdaptationInstructionPrefix), maxInteractiveStateSchemaPromptBytes)
	}
	return stateSchemaAdaptationInstructionPrefix + string(data), nil
}

func stateSchemaAdaptationWorkspaceContext(state *book.State) (string, string) {
	if state == nil || strings.TrimSpace(state.Workspace()) == "" {
		return "", ""
	}
	creativeBrief := trimStateSchemaPromptText(state.IdeasContext(), 2000)
	loreIndex, err := book.NewLoreStore(state.Workspace()).LoreIndexMarkdown(book.LoreIndexOptions{Limit: 50, MaxBytes: 2 * 1024})
	if err != nil {
		log.Printf("[interactive-state-schema] load bounded lore index failed workspace=%s err=%v", state.Workspace(), err)
		return creativeBrief, ""
	}
	return creativeBrief, trimStateSchemaPromptText(loreIndex, 2000)
}

func compactStateSchemaAdaptationPreset(system interactive.StoryDirectorActorStateSystem) stateSchemaAdaptationPreset {
	preset := stateSchemaAdaptationPreset{InitialActors: append([]interactive.ActorStateInitialActor(nil), system.InitialActors...)}
	for _, template := range system.Templates {
		fields := append([]interactive.ActorStateField(nil), template.Fields...)
		for index := range fields {
			fields[index].Description = trimStateSchemaPromptText(fields[index].Description, 320)
			fields[index].UpdateInstruction = trimStateSchemaPromptText(fields[index].UpdateInstruction, 320)
		}
		preset.Templates = append(preset.Templates, stateSchemaAdaptationTemplate{
			ID:          template.ID,
			Name:        template.Name,
			Description: trimStateSchemaPromptText(template.Description, 480),
			Fields:      fields,
			TraitRules:  append([]interactive.ActorTraitRule(nil), template.TraitRules...),
		})
	}
	for _, pool := range system.TraitPools {
		item := stateSchemaAdaptationTraitPool{ID: pool.ID, Name: pool.Name, Description: trimStateSchemaPromptText(pool.Description, 320)}
		for _, trait := range pool.Traits {
			item.Traits = append(item.Traits, trimStateSchemaPromptText(trait.Name, 128))
		}
		preset.TraitPools = append(preset.TraitPools, item)
	}
	return preset
}

func compactStateSchemaAdaptationRules(system interactive.StoryDirectorTRPGSystem) []stateSchemaAdaptationRule {
	var rules []stateSchemaAdaptationRule
	for _, rule := range system.RuleTemplates {
		if len(rule.StateBindings) == 0 {
			continue
		}
		rules = append(rules, stateSchemaAdaptationRule{ID: rule.ID, Label: rule.Label, StateBindings: rule.StateBindings})
	}
	return rules
}

func trimStateSchemaPromptText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}
