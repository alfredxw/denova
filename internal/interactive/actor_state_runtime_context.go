package interactive

import (
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
)

const actorStateRuntimeTruncatedNotice = "> 内容已按上下文上限截断；不要猜测未展示的 Actor、字段或模板。"

// ActorStateRuntimeContext compiles the effective story schema and replayed
// values into a bounded Markdown write guide. JSON remains the backend
// contract; the model receives readable semantics, exact stable IDs, current
// values, and examples.
func ActorStateRuntimeContext(system StoryDirectorActorStateSystem, state map[string]any, limitBytes int, configuredChoiceCount ...int) string {
	if limitBytes <= 0 || limitBytes > DirectorContextMaxBytes {
		limitBytes = DirectorContextMaxBytes
	}
	system = normalizeActorStateSystem(system)
	projectedState := cloneActorStateRoot(state)
	if err := applyMissingInitialActors(projectedState, system, "运行时 schema 初始状态"); err != nil {
		log.Printf("[interactive-state] project initial Actors into runtime guide failed err=%v location=internal/interactive/actor_state_runtime_context.go", err)
	} else {
		state = projectedState
	}
	choiceCount := DefaultStoryChoiceCount
	if len(configuredChoiceCount) > 0 {
		choiceCount = normalizeStoryChoiceCount(configuredChoiceCount[0])
	}
	if validateStoryChoiceCount(choiceCount) != nil {
		choiceCount = DefaultStoryChoiceCount
	}
	blocks := actorStateRuntimeMarkdownBlocks(system, state, choiceCount)
	return joinBoundedActorStateRuntimeBlocks(blocks, limitBytes)
}

func actorStateRuntimeMarkdownBlocks(system StoryDirectorActorStateSystem, state map[string]any, choiceCount int) []string {
	blocks := []string{strings.Join([]string{
		"# Actor 状态手册",
		"",
		"> 来源：`effective_actor_state_schema` + `Snapshot.State.actors`；缺失的 schema 初始 Actor 仅作运行时投影，不改写事件历史。",
		"",
		"- 只提交本轮正文中已经发生的变化；未变化字段不要重复提交，也不要用空值清除。",
		"- 引用已有 Actor 时，`actor_id` 必须逐字使用下文反引号中的现有 ID；`template_id`、`field_id` 同样逐字复用。",
		"- 新建 Actor 时 actor_id 与 name 必须完全相同，直接使用故事语言中的角色名称，不要生成英文、拼音或 slug ID。",
		"- `description` 解释字段含义；`update_instruction` 决定何时、如何更新。两者都必须遵守。",
		"- “当前状态”只列本轮可写值；字段语义和更新规则统一见后面的“新 Actor 可用模板”，避免重复上下文。",
		"- `replace` 写入本轮结束后的完整值；`delta` 只用于已有数值；规则检定已消费的字段不要重复提交。",
	}, "\n")}

	rawActors, _ := state[actorStateRoot].(map[string]any)
	actorIDs := make([]string, 0, len(rawActors))
	for actorID := range rawActors {
		actorIDs = append(actorIDs, actorID)
	}
	sort.Strings(actorIDs)
	if len(actorIDs) > maxInteractiveListItems {
		actorIDs = actorIDs[:maxInteractiveListItems]
	}
	blocks = append(blocks, "## 当前状态")
	for _, actorID := range actorIDs {
		if block := actorStateRuntimeActorMarkdown(system, state, rawActors, actorID); block != "" {
			blocks = append(blocks, block)
		}
	}
	if len(actorIDs) == 0 {
		blocks = append(blocks, "> 当前没有可写 Actor；仅可按“新 Actor 可用模板”创建确有必要长期追踪的角色。")
	}

	blocks = append(blocks, actorStateRuntimeSubmissionTemplate(system, rawActors, actorIDs, choiceCount))
	blocks = append(blocks, "## 新 Actor 可用模板")
	for index, template := range system.Templates {
		if index >= maxInteractiveListItems {
			break
		}
		blocks = append(blocks, actorStateRuntimeTemplateMarkdown(template))
	}
	return blocks
}

func actorStateRuntimeActorMarkdown(system StoryDirectorActorStateSystem, state map[string]any, rawActors map[string]any, actorID string) string {
	record, _ := rawActors[actorID].(map[string]any)
	if record == nil {
		return ""
	}
	templateID := normalizeActorStateID(fmt.Sprint(record["template_id"]))
	template := actorStateTemplateByID(system, templateID)
	if template.ID == "" {
		return ""
	}
	name, _ := record["name"].(string)
	role, _ := record["role"].(string)
	var sb strings.Builder
	fmt.Fprintf(&sb, "### %s\n\n", actorStateRuntimeText(firstNonEmptyString(name, actorID)))
	fmt.Fprintf(&sb, "- Actor ID：%s\n", actorStateRuntimeCode(actorID))
	fmt.Fprintf(&sb, "- Template ID：%s\n", actorStateRuntimeCode(templateID))
	if strings.TrimSpace(role) != "" {
		fmt.Fprintf(&sb, "- 角色：%s\n", actorStateRuntimeText(role))
	}
	if description := actorStateRuntimeText(fmt.Sprint(record["description"])); description != "" && description != "<nil>" {
		fmt.Fprintf(&sb, "- 角色说明：%s\n", description)
	}

	rawState, _ := record["state"].(map[string]any)
	for _, field := range template.Fields {
		if field.Visibility == "hidden" {
			continue
		}
		fieldID := actorStateFieldID(field)
		value := rawState[fieldID]
		if value == nil && strings.TrimSpace(field.LegacyPath) != "" {
			value = getPathExact(rawState, field.LegacyPath)
		}
		fmt.Fprintf(&sb, "\n#### %s\n\n", actorStateRuntimeText(firstNonEmptyString(field.Name, fieldID)))
		fmt.Fprintf(&sb, "- 字段 ID：%s\n", actorStateRuntimeCode(fieldID))
		fmt.Fprintf(&sb, "- 当前值：%s\n", actorStateRuntimeValue(value))
		fmt.Fprintf(&sb, "- 类型：%s%s\n", actorStateRuntimeCode(field.Type), actorStateRuntimeConstraints(field))
	}

	traits := actorTraitInstancesFromState(state, actorID)
	visibleTraits := make([]ActorTraitInstance, 0, len(traits))
	for _, trait := range traits {
		if trait.Visibility != "hidden" {
			visibleTraits = append(visibleTraits, trait)
		}
	}
	if len(visibleTraits) > 0 {
		sb.WriteString("\n#### 已分配词条（只读）\n")
		for _, trait := range visibleTraits {
			fmt.Fprintf(&sb, "\n- %s（Trait ID：%s）", actorStateRuntimeText(trait.Name), actorStateRuntimeCode(trait.TraitID))
			if summary := actorStateRuntimeText(trait.Summary); summary != "" {
				fmt.Fprintf(&sb, "：%s", summary)
			}
		}
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String())
}

func actorStateRuntimeTemplateMarkdown(template ActorStateTemplate) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "### %s\n\n", actorStateRuntimeText(firstNonEmptyString(template.Name, template.ID)))
	fmt.Fprintf(&sb, "- Template ID：%s\n", actorStateRuntimeCode(template.ID))
	if description := actorStateRuntimeText(template.Description); description != "" {
		fmt.Fprintf(&sb, "- 模板说明：%s\n", description)
	}
	if len(template.TraitRules) > 0 {
		sb.WriteString("- 词条由后端按模板规则自动分配；创建参数中不要伪造词条。\n")
	}
	for _, field := range template.Fields {
		if field.Visibility == "hidden" {
			continue
		}
		fieldID := actorStateFieldID(field)
		fmt.Fprintf(&sb, "\n#### %s\n\n", actorStateRuntimeText(firstNonEmptyString(field.Name, fieldID)))
		fmt.Fprintf(&sb, "- 字段 ID：%s\n", actorStateRuntimeCode(fieldID))
		fmt.Fprintf(&sb, "- 类型：%s%s\n", actorStateRuntimeCode(field.Type), actorStateRuntimeConstraints(field))
		if field.Default != nil {
			fmt.Fprintf(&sb, "- 默认值：%s\n", actorStateRuntimeValue(field.Default))
		}
		if description := actorStateRuntimeText(field.Description); description != "" {
			fmt.Fprintf(&sb, "- 字段说明：%s\n", description)
		}
		if instruction := actorStateRuntimeText(field.UpdateInstruction); instruction != "" {
			fmt.Fprintf(&sb, "- 更新指引：%s\n", instruction)
		}
	}
	return strings.TrimSpace(sb.String())
}

func actorStateRuntimeSubmissionTemplate(system StoryDirectorActorStateSystem, rawActors map[string]any, actorIDs []string, choiceCount int) string {
	actorID, fieldID, exampleValue := actorStateRuntimeExample(system, rawActors, actorIDs)
	choices := make([]string, normalizeStoryChoiceCount(choiceCount))
	for index := range choices {
		choices[index] = fmt.Sprintf("{{next_action_%d}}", index+1)
	}
	var sb strings.Builder
	sb.WriteString("## 提交参数模板\n\n")
	sb.WriteString("已有 Actor 字段更新示例（只提交确实变化的字段）：\n\n```json\n")
	example := map[string]any{
		"state_changes": []map[string]any{{"op": TurnStateUpdateReplace, "actor_id": actorID, "field_id": fieldID, "value": exampleValue}},
		"choices":       choices,
	}
	data, _ := json.MarshalIndent(example, "", "  ")
	sb.Write(data)
	sb.WriteString("\n```\n\n")
	sb.WriteString("没有状态变化时提交 `\"state_changes\": []`。创建新 Actor 时使用：\n\n```json\n")
	create := map[string]any{
		"op":            TurnStateUpdateCreate,
		"actor_id":      "{{new_actor_name}}",
		"template_id":   "{{template_id}}",
		"name":          "{{new_actor_name}}",
		"initial_state": map[string]any{"{{field_id}}": "{{initial_value_matching_field_type}}"},
	}
	data, _ = json.MarshalIndent(create, "", "  ")
	sb.Write(data)
	sb.WriteString("\n```\n\n对象字段的子路径使用可选 `subpath` 字符串数组；不要自行拼接路径字符串。")
	return sb.String()
}

func actorStateRuntimeExample(system StoryDirectorActorStateSystem, rawActors map[string]any, actorIDs []string) (string, string, any) {
	for _, actorID := range actorIDs {
		record, _ := rawActors[actorID].(map[string]any)
		template := actorStateTemplateByID(system, normalizeActorStateID(fmt.Sprint(record["template_id"])))
		for _, field := range template.Fields {
			if field.Visibility != "hidden" {
				return actorID, actorStateFieldID(field), actorStateRuntimeExampleValue(field)
			}
		}
	}
	return "{{actor_id}}", "{{field_id}}", "{{new_value_matching_field_type}}"
}

func actorStateRuntimeExampleValue(field ActorStateField) any {
	if len(field.Options) > 0 {
		return field.Options[0]
	}
	switch field.Type {
	case "number":
		value := float64(1)
		if field.Min != nil {
			value = *field.Min
		}
		if field.Max != nil && value > *field.Max {
			value = *field.Max
		}
		return value
	case "bool":
		return true
	case "list":
		return []any{"{{new_item}}"}
	case "object":
		return map[string]any{"{{key}}": "{{new_value}}"}
	default:
		return "{{new_string_value}}"
	}
}

func actorStateRuntimeConstraints(field ActorStateField) string {
	parts := make([]string, 0, 3)
	if field.Min != nil {
		parts = append(parts, fmt.Sprintf("最小值 %v", *field.Min))
	}
	if field.Max != nil {
		parts = append(parts, fmt.Sprintf("最大值 %v", *field.Max))
	}
	if len(field.Options) > 0 {
		options := make([]string, 0, len(field.Options))
		for _, option := range field.Options {
			options = append(options, actorStateRuntimeCode(option))
		}
		parts = append(parts, "可选值 "+strings.Join(options, "、"))
	}
	if len(parts) == 0 {
		return ""
	}
	return "（" + strings.Join(parts, "；") + "）"
}

func actorStateRuntimeValue(value any) string {
	if value == nil {
		return "_未设置_"
	}
	if text, ok := value.(string); ok {
		return actorStateRuntimeCode(text)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "_无法读取_"
	}
	return actorStateRuntimeCode(string(data))
}

func actorStateRuntimeText(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	return trimBytes(value, maxInteractiveTextBytes)
}

func actorStateRuntimeCode(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	if strings.Contains(value, "`") {
		return "``" + value + "``"
	}
	return "`" + value + "`"
}

func joinBoundedActorStateRuntimeBlocks(blocks []string, limitBytes int) string {
	if limitBytes <= 0 {
		return ""
	}
	var sb strings.Builder
	truncated := false
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		separator := ""
		if sb.Len() > 0 {
			separator = "\n\n"
		}
		reserve := len([]byte("\n\n" + actorStateRuntimeTruncatedNotice))
		if sb.Len()+len([]byte(separator))+len([]byte(block))+reserve > limitBytes {
			truncated = true
			break
		}
		sb.WriteString(separator)
		sb.WriteString(block)
	}
	if truncated {
		if sb.Len() > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(actorStateRuntimeTruncatedNotice)
	}
	if sb.Len() == 0 {
		return trimBytes(actorStateRuntimeTruncatedNotice, limitBytes)
	}
	return sb.String()
}
