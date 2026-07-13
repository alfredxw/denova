package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/session"
)

const maxInteractiveStateSchemaPromptBytes = interactive.DirectorContextMaxBytes
const maxStateSchemaResidentLoreRosterBytes = 8 * 1024
const stateSchemaAdaptationInstructionPrefix = "以下 JSON 是本次唯一可用的有界上下文，每个片段均标明来源字段；不要假设未提供的故事设定。\n"

func generateInteractiveStateSchema(ctx context.Context, cfg *config.Config, state *book.State, toolContext agent.InteractiveStoryToolContext, instruction string) (string, error) {
	return agent.GenerateInteractiveDirectorWithTools(ctx, cfg, state, toolContext, instruction)
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
	ResidentLoreRoster   string `json:"resident_lore_roster,omitempty"`
	LoreRevision         string `json:"lore_revision,omitempty"`
	OpeningTurnID        string `json:"opening_turn_id,omitempty"`
	OpeningUserAction    string `json:"opening_user_action,omitempty"`
	OpeningNarrative     string `json:"opening_narrative,omitempty"`
	OpeningTurnBrief     string `json:"opening_turn_brief,omitempty"`
	OpeningTurnResult    string `json:"opening_turn_result,omitempty"`
	CurrentActorIndex    string `json:"current_actor_index,omitempty"`
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

type stateSchemaAdaptationWorkspaceSources struct {
	CreativeBrief      string
	ResidentLoreRoster string
	LoreRevision       string
}

func runInteractiveStateSchemaInitialization(ctx context.Context, cfg *config.Config, state *book.State, conversation *interactiveConversation, turn interactive.TurnEvent, sessionStore *session.Store) error {
	if conversation == nil || conversation.store == nil || cfg == nil {
		return fmt.Errorf("状态结构初始化上下文不完整")
	}
	status, claimed, err := conversation.store.ClaimStateSchemaInitialization(conversation.storyID, turn.ID)
	if err != nil || !claimed {
		return err
	}
	storyCtx, err := conversation.store.StoryContext(conversation.storyID, turn.BranchID)
	if err != nil {
		_ = conversation.store.MarkStateSchemaInitializationFailed(conversation.storyID, turn.ID, err)
		return err
	}
	if storyCtx.Meta.ActorStateSchema == nil {
		err = fmt.Errorf("故事状态结构不存在")
		_ = conversation.store.MarkStateSchemaInitializationFailed(conversation.storyID, turn.ID, err)
		return err
	}
	director := conversation.storyDirectorForMeta(storyCtx.Meta)
	director.ActorState = storyCtx.Meta.ActorStateSchema.System
	director.TRPGSystem = storyCtx.Meta.ActorStateSchema.TRPGSystem
	req := interactive.CreateStoryRequest{
		Title:           storyCtx.Meta.Title,
		Origin:          storyCtx.Meta.Origin,
		StoryTellerID:   storyCtx.Meta.StoryTellerID,
		StoryDirectorID: storyCtx.Meta.StoryDirectorID,
		Opening:         storyCtx.Meta.Opening,
		ActorState:      &director.ActorState,
		TRPGSystem:      &director.TRPGSystem,
	}
	workspaceSources, err := stateSchemaAdaptationWorkspaceContext(state)
	if err != nil {
		_ = conversation.store.MarkStateSchemaInitializationFailed(conversation.storyID, turn.ID, err)
		return err
	}
	instruction, err := buildStateSchemaAdaptationInstructionAfterOpeningWithSources(req, director, &turn, storyCtx.Snapshot.State, workspaceSources)
	if err != nil {
		_ = conversation.store.MarkStateSchemaInitializationFailed(conversation.storyID, turn.ID, err)
		return err
	}
	log.Printf("[interactive-state-schema] initialization begin story_id=%s branch_id=%s turn_id=%s base_revision=%d target_revision=%d", conversation.storyID, turn.BranchID, turn.ID, status.BaseRevision, status.TargetRevision)
	generator := interactiveDirectorGenerator(generateInteractiveStateSchema)
	if conversation.customDirectorGenerator && conversation.directorGenerator != nil {
		generator = conversation.directorGenerator
	}
	var submittedProposal *interactive.ActorStateSchemaProposal
	var reviewMu sync.Mutex
	reviewedLoreIDs := map[string]bool{}
	output, err := generator(ctx, cfg, state, agent.InteractiveStoryToolContext{
		Store:               conversation.store,
		StoryID:             conversation.storyID,
		BranchID:            turn.BranchID,
		TurnID:              turn.ID,
		MaintenanceTask:     "state_schema_initialization",
		DisplayConversation: conversation,
		OnLoreItemsRead: func(ids []string) {
			reviewMu.Lock()
			defer reviewMu.Unlock()
			for _, id := range ids {
				if id = strings.TrimSpace(id); id != "" {
					reviewedLoreIDs[id] = true
				}
			}
		},
		SubmitStateSchemaProposal: func(_ context.Context, proposal interactive.ActorStateSchemaProposal) (interactive.ActorStateSchemaProposalPreview, error) {
			reviewMu.Lock()
			defer reviewMu.Unlock()
			// A later failed attempt must not leave an older valid proposal staged.
			submittedProposal = nil
			// The backend derives reviewed IDs from successful read_lore_items
			// results; model-supplied claims are never trusted as audit evidence.
			proposal.ReviewedLoreIDs = nil
			normalized, preview, validateErr := interactive.ValidateActorStateSchemaProposal(storyCtx.Meta.ActorStateSchema.System, storyCtx.Meta.ActorStateSchema.TRPGSystem, proposal)
			if validateErr != nil {
				return interactive.ActorStateSchemaProposalPreview{}, validateErr
			}
			actualReviewedLoreIDs := make([]string, 0, len(reviewedLoreIDs))
			for id := range reviewedLoreIDs {
				actualReviewedLoreIDs = append(actualReviewedLoreIDs, id)
			}
			sort.Strings(actualReviewedLoreIDs)
			actualReviewed := make(map[string]bool, len(actualReviewedLoreIDs))
			for _, id := range actualReviewedLoreIDs {
				actualReviewed[id] = true
			}
			for _, requirement := range normalized.Requirements {
				if requirement.Source.Kind == "lore" && !actualReviewed[requirement.Source.ID] {
					return interactive.ActorStateSchemaProposalPreview{}, fmt.Errorf("状态需求引用了未通过 read_lore_items 读取的资料: %s", requirement.Source.ID)
				}
			}
			normalized.ReviewedLoreIDs = actualReviewedLoreIDs
			normalized.SourceLoreRevision = workspaceSources.LoreRevision
			submittedProposal = &normalized
			return preview, nil
		},
	}, instruction)
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, "执行失败："+err.Error())
		_ = conversation.store.MarkStateSchemaInitializationFailed(conversation.storyID, turn.ID, err)
		return fmt.Errorf("生成故事状态结构适配失败: %w", err)
	}
	persistAgentCallWithStore(sessionStore, config.AgentKindInteractiveDirector, instruction, output)
	reviewMu.Lock()
	proposalToApply := submittedProposal
	reviewMu.Unlock()
	if proposalToApply == nil {
		err = fmt.Errorf("状态结构 Director 未通过 submit_state_schema_adaptation 提交提案")
		_ = conversation.store.MarkStateSchemaInitializationFailed(conversation.storyID, turn.ID, err)
		return err
	}
	if workspaceSources.LoreRevision != "" {
		currentLoreRevision, revisionErr := stateSchemaLoreRevision(state)
		if revisionErr != nil {
			err = fmt.Errorf("读取状态结构审查后的资料库 revision 失败: %w", revisionErr)
			_ = conversation.store.MarkStateSchemaInitializationFailed(conversation.storyID, turn.ID, err)
			return err
		}
		if currentLoreRevision != workspaceSources.LoreRevision {
			err = fmt.Errorf("资料库在状态结构审查期间发生变化，请重新审查: expected=%s current=%s", workspaceSources.LoreRevision, currentLoreRevision)
			_ = conversation.store.MarkStateSchemaInitializationFailed(conversation.storyID, turn.ID, err)
			return err
		}
	}
	completed, err := conversation.store.ApplyStateSchemaProposal(conversation.storyID, turn.BranchID, turn.ID, *proposalToApply)
	if err != nil {
		_ = conversation.store.MarkStateSchemaInitializationFailed(conversation.storyID, turn.ID, err)
		return err
	}
	log.Printf("[interactive-state-schema] initialization done story_id=%s branch_id=%s turn_id=%s revision=%d changes=%d warnings=%d summary=%q", conversation.storyID, turn.BranchID, turn.ID, completed.TargetRevision, len(completed.Changes), len(completed.Warnings), completed.Summary)
	return nil
}

func buildStateSchemaAdaptationInstruction(req interactive.CreateStoryRequest, director interactive.StoryDirector, state *book.State) (string, error) {
	return buildStateSchemaAdaptationInstructionAfterOpening(req, director, state, nil, nil)
}

func buildStateSchemaAdaptationInstructionAfterOpening(req interactive.CreateStoryRequest, director interactive.StoryDirector, state *book.State, turn *interactive.TurnEvent, currentState map[string]any) (string, error) {
	workspaceSources, err := stateSchemaAdaptationWorkspaceContext(state)
	if err != nil {
		return "", err
	}
	return buildStateSchemaAdaptationInstructionAfterOpeningWithSources(req, director, turn, currentState, workspaceSources)
}

func buildStateSchemaAdaptationInstructionAfterOpeningWithSources(req interactive.CreateStoryRequest, director interactive.StoryDirector, turn *interactive.TurnEvent, currentState map[string]any, workspaceSources stateSchemaAdaptationWorkspaceSources) (string, error) {
	trpgSystem := director.TRPGSystem
	if req.TRPGSystem != nil {
		trpgSystem = *req.TRPGSystem
	}
	prompt := stateSchemaAdaptationPrompt{
		Task: "基于已落盘首轮正文、当前 Actor 状态、常驻资料与规则绑定完成覆盖审查，并通过 submit_state_schema_adaptation 提交最小且充分的提案。",
		Sources: stateSchemaAdaptationSources{
			StoryTitle:           trimStateSchemaPromptText(req.Title, 256),
			StoryOrigin:          trimStateSchemaPromptText(req.Origin, 4000),
			OpeningMode:          trimStateSchemaPromptText(req.Opening.Mode, 32),
			OpeningText:          trimStateSchemaPromptText(firstNonEmptyApp(req.Opening.CustomText, req.Opening.PresetText), 4000),
			StoryDirectorID:      trimStateSchemaPromptText(director.ID, 128),
			StoryDirectorName:    trimStateSchemaPromptText(director.Name, 256),
			StoryDirectorSummary: trimStateSchemaPromptText(director.Description, 1000),
			DirectorStrategy:     trimStateSchemaPromptText(director.Strategy.PromptMarkdown, 4000),
			CreativeBrief:        workspaceSources.CreativeBrief,
			ResidentLoreRoster:   workspaceSources.ResidentLoreRoster,
			LoreRevision:         workspaceSources.LoreRevision,
		},
		StatePreset:  compactStateSchemaAdaptationPreset(*req.ActorState),
		TRPGBindings: compactStateSchemaAdaptationRules(trpgSystem),
		Limits: map[string]int{
			"max_prompt_bytes":             maxInteractiveStateSchemaPromptBytes,
			"max_template_ops":             64,
			"max_field_ops":                64,
			"max_initial_actor_ops":        64,
			"max_lore_read_items_per_call": interactive.StateSchemaLoreReadMaxItemsPerCall,
			"max_lore_read_result_bytes":   interactive.StateSchemaLoreReadMaxResultBytes,
			"max_lore_read_total_bytes":    interactive.StateSchemaLoreReadMaxTotalBytes,
		},
	}
	if turn != nil {
		prompt.Sources.OpeningTurnID = trimStateSchemaPromptText(turn.ID, 128)
		prompt.Sources.OpeningUserAction = trimStateSchemaPromptText(turn.User, 1200)
		prompt.Sources.OpeningNarrative = trimStateSchemaPromptText(turn.Narrative, 6000)
		prompt.Sources.OpeningTurnBrief = compactStateSchemaTurnValue(turn.TurnBrief, 3000)
		prompt.Sources.OpeningTurnResult = compactStateSchemaTurnValue(turn.TurnResult, 3000)
		if req.ActorState != nil {
			prompt.Sources.CurrentActorIndex = trimStateSchemaPromptText(interactive.ActorStateRuntimeContext(*req.ActorState, currentState, 6*1024), 6000)
		}
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

func compactStateSchemaTurnValue(value any, maxRunes int) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return trimStateSchemaPromptText(string(data), maxRunes)
}

func stateSchemaAdaptationWorkspaceContext(state *book.State) (stateSchemaAdaptationWorkspaceSources, error) {
	if state == nil || strings.TrimSpace(state.Workspace()) == "" {
		return stateSchemaAdaptationWorkspaceSources{}, nil
	}
	creativeBrief := trimStateSchemaPromptText(state.IdeasContext(), 2000)
	store := book.NewLoreStore(state.Workspace())
	loreIndex, err := store.ResidentLoreIndexMarkdown(maxStateSchemaResidentLoreRosterBytes)
	if err != nil {
		return stateSchemaAdaptationWorkspaceSources{}, fmt.Errorf("读取状态结构审查的常驻资料库目录失败 workspace=%s: %w", state.Workspace(), err)
	}
	loreRevision, err := store.Revision()
	if err != nil {
		return stateSchemaAdaptationWorkspaceSources{}, fmt.Errorf("读取状态结构审查的资料库 revision 失败 workspace=%s: %w", state.Workspace(), err)
	}
	return stateSchemaAdaptationWorkspaceSources{
		CreativeBrief:      creativeBrief,
		ResidentLoreRoster: trimStateSchemaPromptText(loreIndex, maxStateSchemaResidentLoreRosterBytes),
		LoreRevision:       strings.TrimSpace(loreRevision),
	}, nil
}

func stateSchemaLoreRevision(state *book.State) (string, error) {
	if state == nil || strings.TrimSpace(state.Workspace()) == "" {
		return "", nil
	}
	return book.NewLoreStore(state.Workspace()).Revision()
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
