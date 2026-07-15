package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
	"unicode/utf8"

	"denova/config"
	"denova/internal/agent"
	"denova/internal/book"
	"denova/internal/interactive"
	"denova/internal/session"
)

const maxInteractiveStateSchemaNonStatePromptBytes = interactive.DirectorContextMaxBytes
const maxInteractiveStateSchemaCurrentActorStateBytes = 1024 * 1024
const maxInteractiveStateSchemaTotalPromptBytes = 2 * 1024 * 1024
const maxInteractiveStateSchemaResidentLoreContextBytes = book.ResidentLoreSafetyMaxBytes + interactive.DirectorContextMaxBytes
const stateSchemaAdaptationInstructionPrefix = "以下 JSON 是本次动态上下文；current_actor_state 在声明的 1 MiB 安全上限内完整保留，超过上限会明确拒绝而不会静默截断；其他动态片段有界；完整常驻资料由独立稳定前缀提供。每个片段均标明来源字段；不要假设未提供的故事设定。\n"

const (
	stateSchemaStoryOriginSourceID = "story-origin"
	stateSchemaOpeningTextSourceID = "opening-text"
)

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
	StoryTitle                string                        `json:"story_title"`
	StoryOrigin               string                        `json:"story_origin,omitempty"`
	StoryOriginSourceID       string                        `json:"story_origin_source_id,omitempty"`
	OpeningMode               string                        `json:"opening_mode,omitempty"`
	OpeningText               string                        `json:"opening_text,omitempty"`
	OpeningTextSourceID       string                        `json:"opening_text_source_id,omitempty"`
	StoryDirectorID           string                        `json:"story_director_id"`
	StoryDirectorName         string                        `json:"story_director_name"`
	StoryDirectorSummary      string                        `json:"story_director_summary,omitempty"`
	DirectorStrategy          string                        `json:"director_strategy,omitempty"`
	CreativeBrief             string                        `json:"creative_brief,omitempty"`
	ResidentLore              stateSchemaResidentLoreSource `json:"resident_lore"`
	LoreRevision              string                        `json:"lore_revision,omitempty"`
	OpeningTurnID             string                        `json:"opening_turn_id,omitempty"`
	OpeningUserAction         string                        `json:"opening_user_action,omitempty"`
	OpeningNarrative          string                        `json:"opening_narrative,omitempty"`
	OpeningTurnResult         string                        `json:"opening_turn_result,omitempty"`
	OpeningTurnResultSourceID string                        `json:"opening_turn_result_source_id,omitempty"`
	CurrentActorState         *stateSchemaCurrentActorState `json:"current_actor_state,omitempty"`
}

type stateSchemaCurrentActorState struct {
	Source map[string]string `json:"source"`
	Actors map[string]any    `json:"actors"`
}

type stateSchemaResidentLoreSource struct {
	Source       string   `json:"source"`
	Complete     bool     `json:"complete"`
	MaxBodyBytes int      `json:"max_body_bytes"`
	BodyBytes    int      `json:"body_bytes"`
	IDs          []string `json:"ids,omitempty"`
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
	var reviewMu sync.Mutex
	reviewedLoreIDs := map[string]bool{}
	for _, id := range workspaceSources.ResidentLoreIDs {
		if id = strings.TrimSpace(id); id != "" {
			reviewedLoreIDs[id] = true
		}
	}
	draft := interactive.NewActorStateSchemaBatchDraft(storyCtx.Meta.ActorStateSchema.System, storyCtx.Meta.ActorStateSchema.TRPGSystem)
	openingSourceIDs := []string{turn.ID}
	if strings.TrimSpace(req.Origin) != "" {
		openingSourceIDs = append(openingSourceIDs, stateSchemaStoryOriginSourceID)
	}
	if strings.TrimSpace(firstNonEmptyApp(req.Opening.CustomText, req.Opening.PresetText)) != "" {
		openingSourceIDs = append(openingSourceIDs, stateSchemaOpeningTextSourceID)
	}
	turnResultSourceIDs := []string{}
	if turn.TurnResult != nil {
		turnResultSourceIDs = append(turnResultSourceIDs, turn.ID)
	}
	trpgSourceIDs := stateSchemaAdaptationRuleSourceIDs(compactStateSchemaAdaptationRules(storyCtx.Meta.ActorStateSchema.TRPGSystem))
	batchAuditLocked := func() interactive.ActorStateSchemaBatchAudit {
		ids := make([]string, 0, len(reviewedLoreIDs))
		for id := range reviewedLoreIDs {
			ids = append(ids, id)
		}
		return interactive.ActorStateSchemaBatchAudit{
			ReviewedLoreIDs: ids, OpeningSourceIDs: openingSourceIDs, TurnResultSourceIDs: turnResultSourceIDs,
			TRPGSourceIDs: trpgSourceIDs, SourceLoreRevision: workspaceSources.LoreRevision, CurrentState: storyCtx.Snapshot.State,
		}
	}
	submitBatchLocked := func(batch interactive.ActorStateSchemaBatch) interactive.ActorStateSchemaBatchResult {
		result := draft.Submit(batch, batchAuditLocked())
		log.Printf("[interactive-state-schema] batch story_id=%s turn_id=%s accepted=%d rejected=%d blocked=%d draft_items=%d finalize_requested=%t finalized=%t", conversation.storyID, turn.ID, len(result.Accepted), len(result.Rejected), len(result.Blocked), result.DraftAcceptedItems, batch.Finalize, result.Finalized)
		return result
	}
	output, err := generator(ctx, cfg, state, agent.InteractiveStoryToolContext{
		Store:           conversation.store,
		StoryID:         conversation.storyID,
		BranchID:        turn.BranchID,
		TurnID:          turn.ID,
		MaintenanceTask: "state_schema_initialization",
		StableContextTitle: fmt.Sprintf(
			"完整常驻资料（source: enabled resident lore bodies; complete=true; body_bytes=%d; max_body_bytes=%d; lore_revision=%s）",
			workspaceSources.ResidentLoreBytes, book.ResidentLoreSafetyMaxBytes, workspaceSources.LoreRevision,
		),
		StableContext:         workspaceSources.ResidentLore,
		StableContextMaxBytes: maxInteractiveStateSchemaResidentLoreContextBytes,
		DisplayConversation:   conversation,
		OnLoreItemsRead: func(ids []string) {
			reviewMu.Lock()
			defer reviewMu.Unlock()
			for _, id := range ids {
				if id = strings.TrimSpace(id); id != "" {
					reviewedLoreIDs[id] = true
				}
			}
		},
		SubmitStateSchemaBatch: func(callCtx context.Context, batch interactive.ActorStateSchemaBatch) (interactive.ActorStateSchemaBatchResult, error) {
			traceItemCount := len(batch.Items)
			if traceItemCount > interactive.StateSchemaBatchMaxItems {
				traceItemCount = interactive.StateSchemaBatchMaxItems
			}
			itemIDs := make([]string, 0, traceItemCount)
			for _, item := range batch.Items[:traceItemCount] {
				itemIDs = append(itemIDs, trimStateSchemaPromptText(item.ItemID, 128))
			}
			span, _ := agent.StartTraceSpan(callCtx, "state_schema_batch", map[string]any{
				"item_count":           len(batch.Items),
				"item_ids":             itemIDs,
				"item_ids_truncated":   len(batch.Items) > traceItemCount,
				"finalize_requested":   batch.Finalize,
				"base_schema_revision": status.BaseRevision,
				"lore_revision":        workspaceSources.LoreRevision,
			})
			if err := callCtx.Err(); err != nil {
				if span != nil {
					span.Finish("error", map[string]any{"error": err.Error()})
				}
				return interactive.ActorStateSchemaBatchResult{}, err
			}
			reviewMu.Lock()
			defer reviewMu.Unlock()
			result := submitBatchLocked(batch)
			errorCodes := make([]string, 0, len(result.Rejected)+len(result.Blocked))
			for _, issue := range result.Rejected {
				errorCodes = append(errorCodes, issue.Code)
			}
			for _, issue := range result.Blocked {
				errorCodes = append(errorCodes, issue.Code)
			}
			finishAttrs := map[string]any{
				"accepted":    len(result.Accepted),
				"rejected":    len(result.Rejected),
				"blocked":     len(result.Blocked),
				"draft_items": result.DraftAcceptedItems,
				"finalized":   result.Finalized,
				"error_codes": errorCodes,
			}
			if result.Finalized {
				if finalProposal, ok := draft.FinalProposal(); ok {
					if targetSystem, _, applyErr := interactive.ApplyActorStateSchemaAdaptation(storyCtx.Meta.ActorStateSchema.System, storyCtx.Meta.ActorStateSchema.TRPGSystem, finalProposal.Adaptation); applyErr == nil {
						finalSchemaRevision := status.BaseRevision
						if !reflect.DeepEqual(storyCtx.Meta.ActorStateSchema.System, targetSystem) {
							finalSchemaRevision++
						}
						finishAttrs["final_schema_revision"] = finalSchemaRevision
					}
				}
				if currentLoreRevision, revisionErr := stateSchemaLoreRevision(state); revisionErr == nil {
					finishAttrs["final_lore_revision"] = currentLoreRevision
					finishAttrs["lore_revision_matches"] = currentLoreRevision == workspaceSources.LoreRevision
				}
			}
			if span != nil {
				span.Finish("success", finishAttrs)
			}
			return result, nil
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
	proposalToApply, proposalReady := draft.FinalProposal()
	reviewMu.Unlock()
	if !proposalReady {
		err = fmt.Errorf("状态结构 Director 未通过 submit_state_schema_adaptation finalize Batch 草稿")
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
	completed, err := conversation.store.ApplyStateSchemaProposal(conversation.storyID, turn.BranchID, turn.ID, proposalToApply)
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
		Task: "基于已落盘首轮正文、当前 Actor 状态、完整常驻资料与规则绑定完成覆盖审查，并通过 submit_state_schema_adaptation 增量提交最小且充分的 Batch，最后 finalize。",
		Sources: stateSchemaAdaptationSources{
			StoryTitle:           trimStateSchemaPromptText(req.Title, 256),
			StoryOrigin:          trimStateSchemaPromptText(req.Origin, 4000),
			StoryOriginSourceID:  stateSchemaSourceIDIfPresent(req.Origin, stateSchemaStoryOriginSourceID),
			OpeningMode:          trimStateSchemaPromptText(req.Opening.Mode, 32),
			OpeningText:          trimStateSchemaPromptText(firstNonEmptyApp(req.Opening.CustomText, req.Opening.PresetText), 4000),
			OpeningTextSourceID:  stateSchemaSourceIDIfPresent(firstNonEmptyApp(req.Opening.CustomText, req.Opening.PresetText), stateSchemaOpeningTextSourceID),
			StoryDirectorID:      trimStateSchemaPromptText(director.ID, 128),
			StoryDirectorName:    trimStateSchemaPromptText(director.Name, 256),
			StoryDirectorSummary: trimStateSchemaPromptText(director.Description, 1000),
			DirectorStrategy:     trimStateSchemaPromptText(director.Strategy.PromptMarkdown, 4000),
			CreativeBrief:        workspaceSources.CreativeBrief,
			ResidentLore: stateSchemaResidentLoreSource{
				Source:       "enabled resident lore bodies",
				Complete:     true,
				MaxBodyBytes: book.ResidentLoreSafetyMaxBytes,
				BodyBytes:    workspaceSources.ResidentLoreBytes,
				IDs:          append([]string(nil), workspaceSources.ResidentLoreIDs...),
			},
			LoreRevision: workspaceSources.LoreRevision,
		},
		StatePreset:  compactStateSchemaAdaptationPreset(*req.ActorState),
		TRPGBindings: compactStateSchemaAdaptationRules(trpgSystem),
		Limits: map[string]int{
			"max_non_state_prompt_bytes":    maxInteractiveStateSchemaNonStatePromptBytes,
			"max_current_actor_state_bytes": maxInteractiveStateSchemaCurrentActorStateBytes,
			"max_total_prompt_bytes":        maxInteractiveStateSchemaTotalPromptBytes,
			"max_template_ops":              64,
			"max_field_ops":                 64,
			"max_initial_actor_ops":         64,
			"max_lore_read_items_per_call":  interactive.StateSchemaLoreReadMaxItemsPerCall,
			"max_lore_read_result_bytes":    interactive.StateSchemaLoreReadMaxResultBytes,
			"max_lore_read_total_bytes":     interactive.StateSchemaLoreReadMaxTotalBytes,
			"max_resident_lore_body_bytes":  book.ResidentLoreSafetyMaxBytes,
			"max_batch_items":               interactive.StateSchemaBatchMaxItems,
		},
	}
	if turn != nil {
		prompt.Sources.OpeningTurnID = trimStateSchemaPromptText(turn.ID, 128)
		prompt.Sources.OpeningUserAction = trimStateSchemaPromptText(turn.User, 1200)
		prompt.Sources.OpeningNarrative = trimStateSchemaPromptText(turn.Narrative, 6000)
		prompt.Sources.OpeningTurnResult = compactStateSchemaTurnValue(turn.TurnResult, 3000)
		if turn.TurnResult != nil {
			prompt.Sources.OpeningTurnResultSourceID = trimStateSchemaPromptText(turn.ID, 128)
		}
		if req.ActorState != nil {
			actors, _ := currentState["actors"].(map[string]any)
			if actors == nil {
				actors = map[string]any{}
			}
			prompt.Sources.CurrentActorState = &stateSchemaCurrentActorState{
				Source: map[string]string{"kind": "actor_state_snapshot", "schema": "story_frozen_actor_state"},
				Actors: actors,
			}
		}
	}
	currentActorState := prompt.Sources.CurrentActorState
	if currentActorState != nil {
		actorStateData, marshalErr := json.Marshal(currentActorState)
		if marshalErr != nil {
			return "", fmt.Errorf("序列化完整 Actor 状态快照失败 / Failed to serialize the complete Actor state snapshot: %w", marshalErr)
		}
		if len(actorStateData) > maxInteractiveStateSchemaCurrentActorStateBytes {
			return "", fmt.Errorf("当前 Actor 状态快照超过安全上限 / Current Actor state snapshot exceeds the safety limit: %d > %d bytes", len(actorStateData), maxInteractiveStateSchemaCurrentActorStateBytes)
		}
	}
	prompt.Sources.CurrentActorState = nil
	data, err := json.Marshal(prompt)
	if err != nil {
		return "", fmt.Errorf("序列化状态结构初始化上下文失败: %w", err)
	}
	maxPayloadBytes := maxInteractiveStateSchemaNonStatePromptBytes - len(stateSchemaAdaptationInstructionPrefix)
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
		return "", fmt.Errorf("状态结构初始化非状态上下文超过上限 / State-schema non-state context exceeds the limit: %d > %d bytes", len(data)+len(stateSchemaAdaptationInstructionPrefix), maxInteractiveStateSchemaNonStatePromptBytes)
	}
	prompt.Sources.CurrentActorState = currentActorState
	data, err = json.Marshal(prompt)
	if err != nil {
		return "", fmt.Errorf("序列化完整 Actor 状态快照失败 / Failed to serialize the complete Actor state snapshot: %w", err)
	}
	if len(data)+len(stateSchemaAdaptationInstructionPrefix) > maxInteractiveStateSchemaTotalPromptBytes {
		return "", fmt.Errorf("状态结构初始化总上下文超过安全上限 / State-schema context exceeds the total safety limit: %d > %d bytes", len(data)+len(stateSchemaAdaptationInstructionPrefix), maxInteractiveStateSchemaTotalPromptBytes)
	}
	return stateSchemaAdaptationInstructionPrefix + string(data), nil
}

func stateSchemaSourceIDIfPresent(content, sourceID string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	return sourceID
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

func stateSchemaAdaptationRuleSourceIDs(rules []stateSchemaAdaptationRule) []string {
	ids := make([]string, 0, len(rules))
	for _, rule := range rules {
		if id := strings.TrimSpace(rule.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func trimStateSchemaPromptText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 || utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}
