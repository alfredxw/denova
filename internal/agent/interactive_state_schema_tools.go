package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"denova/internal/interactive"
)

const initializeStoryStateSchemaToolName = "initialize_story_state_schema"

type openingStateSchemaBatchToolInput struct {
	Summary  string                                 `json:"summary,omitempty" jsonschema_description:"本次开局状态结构审查的简短摘要。"`
	Items    []openingStateSchemaBatchItemToolInput `json:"items" jsonschema:"maxItems=16" jsonschema_description:"本次新增或重试的独立提案项。只重传 rejected/blocked 项；已 accepted 的 item_id 不要重传。"`
	Finalize bool                                   `json:"finalize" jsonschema_description:"本批成功后是否完成草稿。存在 rejected/blocked 项时后端不会 finalize。"`
}

type openingStateSchemaBatchItemToolInput struct {
	ItemID       string                                         `json:"item_id" jsonschema_description:"稳定且唯一的幂等 ID，仅使用字母、数字、点、下划线、冒号或短横线。"`
	DependsOn    []string                                       `json:"depends_on,omitempty" jsonschema:"maxItems=16" jsonschema_description:"本项依赖的其他 item_id。"`
	Summary      string                                         `json:"summary,omitempty" jsonschema_description:"本项审查或结构调整的简短摘要。"`
	Requirements []openingStateSchemaRequirementReviewToolInput `json:"requirements" jsonschema:"minItems=1,maxItems=64" jsonschema_description:"本项自包含的来源化字段审查。covered/add/replace 必须填写 template_id、field_id 和 expected_type。"`
	Adaptation   openingStateSchemaAdaptationToolInput          `json:"adaptation" jsonschema_description:"只允许 template_ops；Actor 创建和值必须稍后通过 submit_interactive_turn.state_changes 提交。"`
}

type openingStateSchemaRequirementSourceToolInput struct {
	Kind string `json:"kind" jsonschema:"enum=opening,enum=lore,enum=trpg" jsonschema_description:"来源类型。开局草案固定使用 opening；仅已读取资料可使用 lore；规则模板使用 trpg。"`
	ID   string `json:"id" jsonschema_description:"来源稳定 ID。kind=opening 时必须逐字填写 opening-draft。"`
}

type openingStateSchemaRequirementReviewToolInput struct {
	Source       openingStateSchemaRequirementSourceToolInput `json:"source"`
	Requirement  string                                       `json:"requirement" jsonschema_description:"为什么本故事需要这个长期状态字段，以及它将承接什么信息。"`
	EvidenceKind string                                       `json:"evidence_kind" jsonschema:"enum=confirmed,enum=inferred,enum=default" jsonschema_description:"confirmed=来源明确确认；inferred=合理推断；default=规则级默认。"`
	ValuePolicy  string                                       `json:"value_policy" jsonschema:"enum=schema_only" jsonschema_description:"开局结构工具固定为 schema_only；不得在此写 Actor 值。"`
	ExpectedType string                                       `json:"expected_type,omitempty" jsonschema:"enum=number,enum=string,enum=bool,enum=enum,enum=object,enum=list" jsonschema_description:"covered/add/replace 必填，且必须与目标字段类型一致。ignored 时省略。"`
	Min          *float64                                     `json:"min,omitempty" jsonschema_description:"仅当来源明确要求数值下界时填写，并与目标字段一致。"`
	Max          *float64                                     `json:"max,omitempty" jsonschema_description:"仅当来源明确要求数值上界时填写，并与目标字段一致。"`
	Decision     string                                       `json:"decision" jsonschema:"enum=covered,enum=add,enum=replace,enum=ignored" jsonschema_description:"covered=现有字段足够；add/replace=需要对应 template_ops；ignored=明确不进入长期状态并填写 reason。"`
	TemplateID   string                                       `json:"template_id,omitempty" jsonschema_description:"covered/add/replace 必填，逐字使用 Actor 状态手册中的 Template ID。"`
	FieldID      string                                       `json:"field_id,omitempty" jsonschema_description:"covered/add/replace 必填，逐字使用目标字段 ID；add 时填写要新增的字段 ID。"`
	Reason       string                                       `json:"reason,omitempty" jsonschema_description:"ignored 必填；其它决策仅在需要补充取舍时填写。"`
}

type openingStateSchemaAdaptationToolInput struct {
	Summary     string                                   `json:"summary,omitempty" jsonschema_description:"结构 diff 的简短摘要。"`
	TemplateOps []interactive.ActorStateTemplateSchemaOp `json:"template_ops,omitempty" jsonschema:"maxItems=64" jsonschema_description:"满足本项 requirements 所需的最小模板/字段 diff；covered 时为空数组。"`
}

func (input openingStateSchemaBatchToolInput) batch() interactive.ActorStateSchemaBatch {
	batch := interactive.ActorStateSchemaBatch{
		Summary:  input.Summary,
		Items:    make([]interactive.ActorStateSchemaBatchItem, 0, len(input.Items)),
		Finalize: input.Finalize,
	}
	for _, item := range input.Items {
		converted := interactive.ActorStateSchemaBatchItem{
			ItemID:       item.ItemID,
			DependsOn:    append([]string(nil), item.DependsOn...),
			Summary:      item.Summary,
			Requirements: make([]interactive.ActorStateSchemaRequirementReview, 0, len(item.Requirements)),
			Adaptation: interactive.ActorStateSchemaAdaptation{
				Summary:     item.Adaptation.Summary,
				TemplateOps: append([]interactive.ActorStateTemplateSchemaOp(nil), item.Adaptation.TemplateOps...),
			},
		}
		for _, requirement := range item.Requirements {
			converted.Requirements = append(converted.Requirements, interactive.ActorStateSchemaRequirementReview{
				Source: interactive.ActorStateSchemaRequirementSource{
					Kind: requirement.Source.Kind,
					ID:   requirement.Source.ID,
				},
				Requirement:  requirement.Requirement,
				EvidenceKind: requirement.EvidenceKind,
				ValuePolicy:  requirement.ValuePolicy,
				ExpectedType: requirement.ExpectedType,
				Min:          requirement.Min,
				Max:          requirement.Max,
				Decision:     requirement.Decision,
				TemplateID:   requirement.TemplateID,
				FieldID:      requirement.FieldID,
				Reason:       requirement.Reason,
			})
		}
		batch.Items = append(batch.Items, converted)
	}
	return batch
}

func newInteractiveOpeningStateSchemaTools(ctx InteractiveStoryToolContext) ([]tool.BaseTool, error) {
	if ctx.SubmitStateSchemaBatch == nil {
		return nil, nil
	}
	description := strings.Join([]string{
		"仅在故事首回合正文之前，增量暂存本故事的状态模板与字段结构。模型可见参数是开局专用的 structure-only 契约；不要提交 Actor、initial_actor_ops 或 actor_ops。",
		"开局草案来源必须精确写为 source={\"kind\":\"opening\",\"id\":\"opening-draft\"}。evidence_kind 只能是 confirmed/inferred/default，value_policy 固定为 schema_only；covered/add/replace 必须填写现有或目标 template_id、field_id 与合法 expected_type。",
		"结构 requirement 与 template_ops 必须使用状态手册中的 Template ID，不能使用 Actor ID；例如 story 是 actor_id，对应的 template_id 是 story_context。后端只会将能由初始 Actor 唯一确定的误用归一化，并始终保存规范 Template ID。",
		"按状态的变化边界而不是文字能否勉强容纳来判断 covered：会独立消耗、恢复、触发阈值、参与检定或单独展示的资源/倒计时必须有专用字段，不能塞进当前处境、当前事件、世界局势或物品描述。例如氧气与站体完整度应各自使用有 min/max 的 number 字段。",
		"只有确实没有独立结构需求时才使用具体字段的 covered 审查和空 template_ops。工具分别返回 accepted、rejected、blocked；只重试失败项，finalized=true 后再输出开局正文。",
		"finalized 回执包含 initialization_guide：auto_initialized_fields 已由模板默认值或初始 Actor 值覆盖；required_state_changes 列出首次 submit_interactive_turn 必须一次填写的精确 actor_id、template_id、field_id 和 type。不得用空字符串、未设置、未知或待定占位。",
		"草稿不会单独写入；只有结构、正文、所有初始字段和 choices 全部通过时才原子落盘。Actor 创建与所有初始值稍后通过 submit_interactive_turn.state_changes 提交。",
	}, "\n")
	submitTool, err := utils.InferTool(
		initializeStoryStateSchemaToolName,
		description,
		func(callCtx context.Context, input openingStateSchemaBatchToolInput) (string, error) {
			result, err := ctx.SubmitStateSchemaBatch(callCtx, input.batch())
			if err != nil {
				return "", err
			}
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", fmt.Errorf("序列化开局状态结构 Batch 结果失败: %w", err)
			}
			return string(data), nil
		},
	)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{submitTool}, nil
}
