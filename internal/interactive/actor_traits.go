package interactive

import (
	"encoding/json"
	"fmt"
	mathrand "math/rand"
	"sort"
	"strings"
	"time"
)

const (
	StateOpSourceActorTrait = "actor_trait"
	maxActorTraitsPerActor  = maxInteractiveListItems
	maxActorTraitSummary    = 512
)

type ActorTraitSelection struct {
	PoolID   string   `json:"pool_id"`
	TraitIDs []string `json:"trait_ids,omitempty"`
}

type ActorTraitRollRequest struct {
	StoryDirectorID string                `json:"story_director_id,omitempty"`
	ActorID         string                `json:"actor_id"`
	TemplateID      string                `json:"template_id"`
	Selections      []ActorTraitSelection `json:"selections,omitempty"`
	Seed            int64                 `json:"seed,omitempty"`
}

type ActorTraitRollResult struct {
	StoryDirectorID string               `json:"story_director_id,omitempty"`
	ActorID         string               `json:"actor_id"`
	TemplateID      string               `json:"template_id"`
	Seed            int64                `json:"seed"`
	Traits          []ActorTraitInstance `json:"traits"`
}

type InitialActorTraitRoll struct {
	ActorID    string                `json:"actor_id"`
	Selections []ActorTraitSelection `json:"selections,omitempty"`
	Seed       int64                 `json:"seed,omitempty"`
}

func normalizeActorTraitPools(pools []ActorTraitPool) []ActorTraitPool {
	if pools == nil {
		return []ActorTraitPool{}
	}
	if len(pools) > maxInteractiveListItems {
		pools = pools[:maxInteractiveListItems]
	}
	out := make([]ActorTraitPool, 0, len(pools))
	seen := map[string]bool{}
	for _, pool := range pools {
		pool.ID = normalizeActorStateID(pool.ID)
		if pool.ID == "" || seen[pool.ID] {
			continue
		}
		seen[pool.ID] = true
		pool.Name = trimBytes(firstNonEmptyString(pool.Name, pool.ID), 128)
		pool.Description = trimBytes(pool.Description, maxInteractiveTextBytes)
		pool.Traits = normalizeActorTraitDefinitions(pool.Traits)
		out = append(out, pool)
	}
	return out
}

func normalizeActorTraitDefinitions(traits []ActorTraitDefinition) []ActorTraitDefinition {
	if traits == nil {
		return []ActorTraitDefinition{}
	}
	if len(traits) > maxInteractiveListItems {
		traits = traits[:maxInteractiveListItems]
	}
	out := make([]ActorTraitDefinition, 0, len(traits))
	seen := map[string]bool{}
	for _, trait := range traits {
		trait.ID = normalizeActorStateID(trait.ID)
		if trait.ID == "" || seen[trait.ID] {
			continue
		}
		seen[trait.ID] = true
		trait.Name = trimBytes(firstNonEmptyString(trait.Name, trait.ID), 128)
		trait.Summary = trimBytes(trait.Summary, maxActorTraitSummary)
		if trait.Weight <= 0 {
			trait.Weight = 1
		}
		trait.Visibility = normalizeStoryDirectorVisibility(trait.Visibility)
		out = append(out, trait)
	}
	return out
}

func normalizeActorTraitRules(rules []ActorTraitRule) []ActorTraitRule {
	if rules == nil {
		return []ActorTraitRule{}
	}
	if len(rules) > maxInteractiveListItems {
		rules = rules[:maxInteractiveListItems]
	}
	out := make([]ActorTraitRule, 0, len(rules))
	seen := map[string]bool{}
	for _, rule := range rules {
		rule.PoolID = normalizeActorStateID(rule.PoolID)
		if rule.PoolID == "" || seen[rule.PoolID] {
			continue
		}
		seen[rule.PoolID] = true
		if rule.DrawCount <= 0 {
			rule.DrawCount = 1
		}
		if rule.DrawCount > maxActorTraitsPerActor {
			rule.DrawCount = maxActorTraitsPerActor
		}
		out = append(out, rule)
	}
	return out
}

func validateActorTraitSystem(system StoryDirectorActorStateSystem) error {
	system = normalizeActorStateSystem(system)
	pools := map[string]ActorTraitPool{}
	for _, pool := range system.TraitPools {
		pools[pool.ID] = pool
	}
	for _, template := range system.Templates {
		total := 0
		for _, rule := range template.TraitRules {
			pool, ok := pools[rule.PoolID]
			if !ok {
				return fmt.Errorf("Actor 状态模板 %s 引用了不存在的词条池: %s", template.ID, rule.PoolID)
			}
			if len(pool.Traits) == 0 {
				return fmt.Errorf("Actor 状态模板 %s 引用的词条池为空: %s", template.ID, rule.PoolID)
			}
			if rule.DrawCount > len(pool.Traits) {
				return fmt.Errorf("Actor 状态模板 %s 从词条池 %s 抽取 %d 项，但池中只有 %d 项", template.ID, rule.PoolID, rule.DrawCount, len(pool.Traits))
			}
			total += rule.DrawCount
		}
		if total > maxActorTraitsPerActor {
			return fmt.Errorf("Actor 状态模板 %s 最多只能分配 %d 个词条", template.ID, maxActorTraitsPerActor)
		}
	}
	return nil
}

func RollActorTraits(system StoryDirectorActorStateSystem, req ActorTraitRollRequest) (ActorTraitRollResult, error) {
	return rollActorTraits(system, req, "trait_roll", "")
}

func rollActorTraits(system StoryDirectorActorStateSystem, req ActorTraitRollRequest, sourceKind, sourceTurnID string) (ActorTraitRollResult, error) {
	system = normalizeActorStateSystem(system)
	if err := validateActorTraitSystem(system); err != nil {
		return ActorTraitRollResult{}, err
	}
	req.ActorID = normalizeActorStateID(req.ActorID)
	req.TemplateID = normalizeActorStateID(req.TemplateID)
	if req.ActorID == "" {
		return ActorTraitRollResult{}, fmt.Errorf("词条抽取缺少 actor_id")
	}
	template := actorStateTemplateByID(system, req.TemplateID)
	if template.ID == "" {
		return ActorTraitRollResult{}, fmt.Errorf("Actor 状态模板不存在: %s", req.TemplateID)
	}
	seed := req.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	selectionByPool, err := normalizeActorTraitSelections(req.Selections)
	if err != nil {
		return ActorTraitRollResult{}, err
	}
	rules := map[string]ActorTraitRule{}
	for _, rule := range template.TraitRules {
		rules[rule.PoolID] = rule
	}
	for poolID := range selectionByPool {
		if _, ok := rules[poolID]; !ok {
			return ActorTraitRollResult{}, fmt.Errorf("Actor 状态模板 %s 不允许使用词条池: %s", template.ID, poolID)
		}
	}
	rng := mathrand.New(mathrand.NewSource(seed))
	traits := make([]ActorTraitInstance, 0)
	for _, rule := range template.TraitRules {
		pool, _ := actorTraitPoolByID(system, rule.PoolID)
		selected := selectionByPool[rule.PoolID]
		if len(selected) > rule.DrawCount {
			return ActorTraitRollResult{}, fmt.Errorf("词条池 %s 最多选择 %d 个词条", rule.PoolID, rule.DrawCount)
		}
		picked, err := pickActorTraits(pool, rule.DrawCount, selected, rng)
		if err != nil {
			return ActorTraitRollResult{}, err
		}
		sourceID := fmt.Sprintf("trait-roll:%s:%s:%d", req.ActorID, pool.ID, seed)
		for _, trait := range picked {
			traits = append(traits, ActorTraitInstance{
				PoolID:       pool.ID,
				PoolName:     pool.Name,
				TraitID:      trait.ID,
				Name:         trait.Name,
				Summary:      trait.Summary,
				Visibility:   trait.Visibility,
				SourceKind:   sourceKind,
				SourceID:     sourceID,
				SourceTurnID: sourceTurnID,
			})
		}
	}
	return ActorTraitRollResult{
		StoryDirectorID: NormalizeStoryDirectorID(req.StoryDirectorID),
		ActorID:         req.ActorID,
		TemplateID:      template.ID,
		Seed:            seed,
		Traits:          traits,
	}, nil
}

func normalizeActorTraitSelections(values []ActorTraitSelection) (map[string][]string, error) {
	out := map[string][]string{}
	for _, value := range values {
		poolID := normalizeActorStateID(value.PoolID)
		if poolID == "" {
			return nil, fmt.Errorf("词条选择缺少 pool_id")
		}
		if _, exists := out[poolID]; exists {
			return nil, fmt.Errorf("词条池选择重复: %s", poolID)
		}
		ids := make([]string, 0, len(value.TraitIDs))
		seen := map[string]bool{}
		for _, id := range value.TraitIDs {
			id = normalizeActorStateID(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			ids = append(ids, id)
		}
		out[poolID] = ids
	}
	return out, nil
}

func pickActorTraits(pool ActorTraitPool, count int, selected []string, rng *mathrand.Rand) ([]ActorTraitDefinition, error) {
	byID := map[string]ActorTraitDefinition{}
	for _, trait := range pool.Traits {
		byID[trait.ID] = trait
	}
	picked := make([]ActorTraitDefinition, 0, count)
	used := map[string]bool{}
	for _, id := range selected {
		trait, ok := byID[id]
		if !ok {
			return nil, fmt.Errorf("词条池 %s 中不存在词条: %s", pool.ID, id)
		}
		if used[id] {
			continue
		}
		used[id] = true
		picked = append(picked, trait)
	}
	candidates := make([]ActorTraitDefinition, 0, len(pool.Traits))
	for _, trait := range pool.Traits {
		if !used[trait.ID] {
			candidates = append(candidates, trait)
		}
	}
	for len(picked) < count && len(candidates) > 0 {
		index := weightedActorTraitIndex(candidates, rng)
		picked = append(picked, candidates[index])
		candidates = append(candidates[:index], candidates[index+1:]...)
	}
	if len(picked) != count {
		return nil, fmt.Errorf("词条池 %s 无法抽取 %d 个不重复词条", pool.ID, count)
	}
	return picked, nil
}

func weightedActorTraitIndex(traits []ActorTraitDefinition, rng *mathrand.Rand) int {
	total := 0.0
	for _, trait := range traits {
		total += trait.Weight
	}
	if total <= 0 {
		return rng.Intn(len(traits))
	}
	target := rng.Float64() * total
	for index, trait := range traits {
		target -= trait.Weight
		if target <= 0 {
			return index
		}
	}
	return len(traits) - 1
}

func actorTraitPoolByID(system StoryDirectorActorStateSystem, id string) (ActorTraitPool, bool) {
	id = normalizeActorStateID(id)
	for _, pool := range system.TraitPools {
		if pool.ID == id {
			return pool, true
		}
	}
	return ActorTraitPool{}, false
}

func BuildActorStateInitialChanges(system StoryDirectorActorStateSystem, rolls []InitialActorTraitRoll) ([]StateOp, []ActorStateOp, error) {
	system = normalizeActorStateSystem(system)
	if actorStateEmpty(system) {
		return nil, nil, nil
	}
	if err := validateActorTraitSystem(system); err != nil {
		return nil, nil, err
	}
	rollByActor := map[string]InitialActorTraitRoll{}
	for _, roll := range rolls {
		roll.ActorID = normalizeActorStateID(roll.ActorID)
		if roll.ActorID == "" {
			return nil, nil, fmt.Errorf("初始词条抽取缺少 actor_id")
		}
		if _, exists := rollByActor[roll.ActorID]; exists {
			return nil, nil, fmt.Errorf("初始词条抽取重复: %s", roll.ActorID)
		}
		rollByActor[roll.ActorID] = roll
	}
	knownActors := map[string]bool{}
	ops := make([]StateOp, 0)
	actorOps := make([]ActorStateOp, 0)
	for _, actor := range system.InitialActors {
		knownActors[actor.ID] = true
		template := actorStateTemplateByID(system, actor.TemplateID)
		if template.ID == "" {
			continue
		}
		baseOps, baseActorOps, _, err := buildNewActorStateOps(template, actor.ID, actor.Name, actor.Role, actor.Description, actor.State, "", "")
		if err != nil {
			return nil, nil, err
		}
		ops = append(ops, baseOps...)
		actorOps = append(actorOps, baseActorOps...)
		roll := rollByActor[actor.ID]
		result, err := rollActorTraits(system, ActorTraitRollRequest{
			ActorID:    actor.ID,
			TemplateID: actor.TemplateID,
			Selections: roll.Selections,
			Seed:       roll.Seed,
		}, "initial_trait_roll", "story_create")
		if err != nil {
			return nil, nil, err
		}
		if len(result.Traits) > 0 {
			ops = append(ops, StateOp{
				Op:         "set",
				Path:       actorStateActorPath(actor.ID, "traits"),
				Value:      result.Traits,
				SourceKind: StateOpSourceActorTrait,
				SourceID:   fmt.Sprintf("initial-traits:%s:%d", actor.ID, result.Seed),
			})
		}
	}
	for actorID := range rollByActor {
		if !knownActors[actorID] {
			return nil, nil, fmt.Errorf("初始词条抽取目标不是初始 Actor: %s", actorID)
		}
	}
	return normalizeStateOps(ops), normalizeActorStateOps(actorOps), nil
}

func buildNewActorStateOps(template ActorStateTemplate, actorID, name, role, description string, state map[string]any, reason, sourceTurnID string) ([]StateOp, []ActorStateOp, map[string]any, error) {
	ops := []StateOp{
		{Op: "set", Path: actorStateActorPath(actorID, "id"), Value: actorID, Reason: reason, SourceTurnID: sourceTurnID},
		{Op: "set", Path: actorStateActorPath(actorID, "name"), Value: trimBytes(firstNonEmptyString(name, actorID), 128), Reason: reason, SourceTurnID: sourceTurnID},
		{Op: "set", Path: actorStateActorPath(actorID, "template_id"), Value: template.ID, Reason: reason, SourceTurnID: sourceTurnID},
		{Op: "set", Path: actorStateActorPath(actorID, "role"), Value: trimBytes(firstNonEmptyString(role, template.ID), 128), Reason: reason, SourceTurnID: sourceTurnID},
	}
	if strings.TrimSpace(description) != "" {
		ops = append(ops, StateOp{Op: "set", Path: actorStateActorPath(actorID, "description"), Value: trimBytes(description, maxInteractiveTextBytes), Reason: reason, SourceTurnID: sourceTurnID})
	}
	actorOps, normalizedState, err := buildActorStateValueOps(template, actorID, state, reason, sourceTurnID)
	if err != nil {
		return nil, nil, nil, err
	}
	return normalizeStateOps(ops), actorOps, normalizedState, nil
}

// buildActorStateValueOps resolves defaults and explicit values before
// emitting operations so each Actor field is initialized at most once.
func buildActorStateValueOps(template ActorStateTemplate, actorID string, state map[string]any, reason, sourceTurnID string) ([]ActorStateOp, map[string]any, error) {
	fieldByReference := actorStateFieldsByReference(template)
	normalizedState := map[string]any{}
	for _, field := range template.Fields {
		fieldID := actorStateFieldID(field)
		if field.Default != nil {
			normalizedState[fieldID] = field.Default
		}
	}
	keys := make([]string, 0, len(state))
	for key := range state {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		// A JSON null is an omitted value, not an instruction to erase a
		// default or materialize a null-valued field.
		if state[rawKey] == nil {
			continue
		}
		field, ok := fieldByReference[actorStateFieldNameKey(key)]
		if !ok {
			return nil, nil, fmt.Errorf("Actor 状态字段不在模板中: actor=%s template=%s field=%s", actorID, template.ID, key)
		}
		value, err := normalizeActorStateValue(field, state[rawKey])
		if err != nil {
			return nil, nil, err
		}
		fieldID := actorStateFieldID(field)
		normalizedState[fieldID] = value
	}
	actorOps := make([]ActorStateOp, 0, len(normalizedState))
	for _, field := range template.Fields {
		fieldID := actorStateFieldID(field)
		value, exists := normalizedState[fieldID]
		if !exists || value == nil {
			continue
		}
		actorOps = append(actorOps, ActorStateOp{Op: "set", ActorID: actorID, FieldID: fieldID, Value: value, Reason: reason, SourceTurnID: sourceTurnID})
	}
	return normalizeActorStateOps(actorOps), normalizedState, nil
}

func actorTraitInstancesFromState(state map[string]any, actorID string) []ActorTraitInstance {
	value := getPath(state, actorStateActorPath(actorID, "traits"))
	if value == nil {
		return []ActorTraitInstance{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return []ActorTraitInstance{}
	}
	var traits []ActorTraitInstance
	if err := json.Unmarshal(data, &traits); err != nil {
		return []ActorTraitInstance{}
	}
	if len(traits) > maxActorTraitsPerActor {
		traits = traits[:maxActorTraitsPerActor]
	}
	return traits
}

func applyActorTraitChanges(system StoryDirectorActorStateSystem, template ActorStateTemplate, actorID string, current []ActorTraitInstance, changes []ActorTraitChange, sourceTurnID string) ([]ActorTraitInstance, bool, error) {
	traits := append([]ActorTraitInstance(nil), current...)
	changed := false
	rules := map[string]ActorTraitRule{}
	for _, rule := range template.TraitRules {
		rules[rule.PoolID] = rule
	}
	for _, change := range changes {
		change.Op = strings.TrimSpace(change.Op)
		change.PoolID = normalizeActorStateID(change.PoolID)
		rule, ok := rules[change.PoolID]
		if !ok {
			return nil, false, fmt.Errorf("Actor 状态模板 %s 不允许使用词条池: %s", template.ID, change.PoolID)
		}
		currentPool := actorTraitsForPool(traits, change.PoolID)
		switch change.Op {
		case "draw":
			if len(currentPool) >= rule.DrawCount {
				continue
			}
			ids := make([]string, 0, len(currentPool))
			for _, trait := range currentPool {
				ids = append(ids, trait.TraitID)
			}
			result, err := rollActorTraits(system, ActorTraitRollRequest{ActorID: actorID, TemplateID: template.ID, Seed: change.Seed, Selections: []ActorTraitSelection{{PoolID: change.PoolID, TraitIDs: ids}}}, "actor_trait_change", sourceTurnID)
			if err != nil {
				return nil, false, err
			}
			rolled := actorTraitsForPool(result.Traits, change.PoolID)
			rolled = preserveExistingActorTraitSnapshots(rolled, currentPool)
			traits = replaceActorTraitsForPool(traits, change.PoolID, rolled)
			changed = true
		case "reroll":
			if len(change.TraitIDs) > 0 {
				return nil, false, fmt.Errorf("reroll 不接受 trait_ids")
			}
			result, err := rollActorTraits(system, ActorTraitRollRequest{ActorID: actorID, TemplateID: template.ID, Seed: change.Seed}, "actor_trait_change", sourceTurnID)
			if err != nil {
				return nil, false, err
			}
			traits = replaceActorTraitsForPool(traits, change.PoolID, actorTraitsForPool(result.Traits, change.PoolID))
			changed = true
		case "set":
			selection, err := normalizeActorTraitSelections([]ActorTraitSelection{{PoolID: change.PoolID, TraitIDs: change.TraitIDs}})
			if err != nil {
				return nil, false, err
			}
			if len(selection[change.PoolID]) != rule.DrawCount {
				return nil, false, fmt.Errorf("set 必须为词条池 %s 指定恰好 %d 个词条", change.PoolID, rule.DrawCount)
			}
			result, err := rollActorTraits(system, ActorTraitRollRequest{ActorID: actorID, TemplateID: template.ID, Seed: change.Seed, Selections: []ActorTraitSelection{{PoolID: change.PoolID, TraitIDs: selection[change.PoolID]}}}, "actor_trait_change", sourceTurnID)
			if err != nil {
				return nil, false, err
			}
			traits = replaceActorTraitsForPool(traits, change.PoolID, actorTraitsForPool(result.Traits, change.PoolID))
			changed = true
		case "remove":
			remove := map[string]bool{}
			for _, id := range change.TraitIDs {
				id = normalizeActorStateID(id)
				if id != "" {
					remove[id] = true
				}
			}
			if len(remove) == 0 {
				return nil, false, fmt.Errorf("remove 必须提供 trait_ids")
			}
			next := make([]ActorTraitInstance, 0, len(traits))
			for _, trait := range traits {
				if trait.PoolID == change.PoolID && remove[trait.TraitID] {
					changed = true
					continue
				}
				next = append(next, trait)
			}
			traits = next
		default:
			return nil, false, fmt.Errorf("不支持的词条操作: %s", change.Op)
		}
	}
	if len(traits) > maxActorTraitsPerActor {
		return nil, false, fmt.Errorf("Actor %s 最多只能持有 %d 个词条", actorID, maxActorTraitsPerActor)
	}
	return traits, changed, nil
}

func actorTraitsForPool(traits []ActorTraitInstance, poolID string) []ActorTraitInstance {
	out := make([]ActorTraitInstance, 0)
	for _, trait := range traits {
		if trait.PoolID == poolID {
			out = append(out, trait)
		}
	}
	return out
}

func replaceActorTraitsForPool(all []ActorTraitInstance, poolID string, replacement []ActorTraitInstance) []ActorTraitInstance {
	out := make([]ActorTraitInstance, 0, len(all)+len(replacement))
	for _, trait := range all {
		if trait.PoolID != poolID {
			out = append(out, trait)
		}
	}
	return append(out, replacement...)
}

func preserveExistingActorTraitSnapshots(rolled, existing []ActorTraitInstance) []ActorTraitInstance {
	byID := map[string]ActorTraitInstance{}
	for _, trait := range existing {
		byID[trait.TraitID] = trait
	}
	for index, trait := range rolled {
		if current, ok := byID[trait.TraitID]; ok {
			rolled[index] = current
		}
	}
	return rolled
}

func cloneActorStateRoot(state map[string]any) map[string]any {
	if len(state) == 0 {
		return map[string]any{}
	}
	data, err := json.Marshal(state)
	if err != nil {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]any{}
	}
	return out
}
