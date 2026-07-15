package interactive

import (
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const StateOpSourceTurnResult = "turn_result"

const (
	TurnStateUpdateReplace = "replace"
	TurnStateUpdateDelta   = "delta"
	TurnStateUpdateCreate  = "create"

	maxDirectorUpdateReasonBytes = 1024
)

// StateUpdate is the small, model-facing state mutation contract. Path is a
// schema-bound JSON Pointer whose first segment is a stable Actor ID.
type StateUpdate struct {
	Op    string `json:"op" jsonschema:"enum=replace,enum=delta,enum=create" jsonschema_description:"状态操作，只能使用 replace、delta 或 create。"`
	Path  string `json:"path" jsonschema_description:"以稳定 actor_id 开头的 schema-bound JSON Pointer，例如 /protagonist/生命值。"`
	Value any    `json:"value" jsonschema_description:"replace/create 的目标值，或 delta 的数值变化量。"`
}

// DirectorUpdateHint is a lightweight post-narrative signal from the Game
// Agent. It only reports that committed facts materially affect future
// planning; the Director remains responsible for deciding patch versus replan
// and which Markdown documents actually need edits.
type DirectorUpdateHint struct {
	Needed bool   `json:"needed" jsonschema_description:"仅当本回合让当前目标、阶段、关键关系、重大线索或规划前提发生实质变化时为 true；普通承接必须为 false。"`
	Reason string `json:"reason,omitempty" jsonschema_description:"needed=true 时简短说明哪些已发生事实影响后续规划；不要提出具体 director.md 改写方案。"`
}

// TurnResult is the complete hidden result produced by the Game Agent. The
// backend compiles StateUpdates into replayable StateDelta operations.
type TurnResult struct {
	StateUpdates   []StateUpdate       `json:"state_updates"`
	Choices        []string            `json:"choices"`
	DirectorUpdate *DirectorUpdateHint `json:"director_update,omitempty"`
}

func NormalizeTurnResult(result TurnResult) TurnResult {
	result.StateUpdates = normalizeTurnStateUpdates(result.StateUpdates)
	result.Choices = normalizeChoiceListLimit(result.Choices, MaxStoryChoiceCount+1)
	result.DirectorUpdate = normalizeDirectorUpdateHint(result.DirectorUpdate)
	return result
}

func ValidateTurnResult(result TurnResult, configuredChoiceCount ...int) error {
	choiceCount := DefaultStoryChoiceCount
	if len(configuredChoiceCount) > 0 {
		choiceCount = normalizeStoryChoiceCount(configuredChoiceCount[0])
	}
	return validateTurnResult(result, choiceCount, false)
}

func validateTerminalTurnResult(result TurnResult, configuredChoiceCount int) error {
	return validateTurnResult(result, configuredChoiceCount, true)
}

func validateTurnResult(result TurnResult, configuredChoiceCount int, terminal bool) error {
	choiceCount := normalizeStoryChoiceCount(configuredChoiceCount)
	if err := validateStoryChoiceCount(choiceCount); err != nil {
		return err
	}
	if len(result.StateUpdates) > maxInteractiveListItems {
		return fmt.Errorf("TurnResult state_updates 不能超过 %d 项", maxInteractiveListItems)
	}
	for index, update := range result.StateUpdates {
		if err := validateStateUpdateShape(update); err != nil {
			return fmt.Errorf("TurnResult state_updates[%d] 无效: %w", index, err)
		}
	}
	if err := validateDirectorUpdateHint(result.DirectorUpdate); err != nil {
		return fmt.Errorf("TurnResult director_update 无效: %w", err)
	}
	if terminal {
		if len(result.Choices) != 0 {
			return fmt.Errorf("明确终局的 TurnResult choices 必须为空")
		}
		return nil
	}
	if len(result.Choices) != choiceCount {
		return fmt.Errorf("TurnResult choices 必须提供恰好 %d 个不同的行动建议", choiceCount)
	}
	return nil
}

func normalizeDirectorUpdateHint(hint *DirectorUpdateHint) *DirectorUpdateHint {
	if hint == nil {
		return nil
	}
	normalized := &DirectorUpdateHint{
		Needed: hint.Needed,
		Reason: strings.TrimSpace(trimBytes(hint.Reason, maxDirectorUpdateReasonBytes)),
	}
	if !normalized.Needed {
		return nil
	}
	return normalized
}

func validateDirectorUpdateHint(hint *DirectorUpdateHint) error {
	if hint == nil {
		return nil
	}
	if !hint.Needed {
		return fmt.Errorf("needed=false 时应省略 director_update")
	}
	if strings.TrimSpace(hint.Reason) == "" {
		return fmt.Errorf("needed=true 时 reason 不能为空")
	}
	if len([]byte(hint.Reason)) > maxDirectorUpdateReasonBytes {
		return fmt.Errorf("reason 超过 %d bytes", maxDirectorUpdateReasonBytes)
	}
	return nil
}

func normalizeTurnResultPointer(result *TurnResult, configuredChoiceCount int, terminal bool) *TurnResult {
	if result == nil {
		return nil
	}
	normalized := NormalizeTurnResult(*result)
	var err error
	if terminal {
		err = validateTerminalTurnResult(normalized, configuredChoiceCount)
	} else {
		err = ValidateTurnResult(normalized, configuredChoiceCount)
	}
	if err != nil {
		return nil
	}
	return &normalized
}

func normalizeTurnStateUpdates(updates []StateUpdate) []StateUpdate {
	if updates == nil {
		return []StateUpdate{}
	}
	result := make([]StateUpdate, len(updates))
	for index, update := range updates {
		update.Op = strings.ToLower(strings.TrimSpace(update.Op))
		update.Path = strings.TrimSpace(update.Path)
		result[index] = update
	}
	return result
}

func validateStateUpdateShape(update StateUpdate) error {
	switch update.Op {
	case TurnStateUpdateReplace, TurnStateUpdateDelta, TurnStateUpdateCreate:
	default:
		return fmt.Errorf("op 必须是 replace、delta 或 create")
	}
	if !strings.HasPrefix(update.Path, "/") || update.Path == "/" {
		return fmt.Errorf("path 必须是以 / 开头的非空 JSON Pointer")
	}
	if update.Value == nil {
		return fmt.Errorf("value 不能为空")
	}
	if update.Op == TurnStateUpdateDelta {
		if _, ok := actorStateNumber(update.Value); !ok {
			return fmt.Errorf("delta 的 value 必须是 number")
		}
	}
	return nil
}

func normalizedChoiceKey(value string) string {
	return cases.Fold().String(norm.NFKC.String(strings.TrimSpace(value)))
}

// normalizeEnum remains shared by Director decision normalization.
func normalizeEnum(value string, allowed ...string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	if len(allowed) > 0 {
		return allowed[0]
	}
	return ""
}
