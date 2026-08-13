package interactive

import (
	interactivestate "denova/internal/interactive/state"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const actorStateRuntimeTruncatedNotice = "> Content was truncated at the context limit. Do not guess Actors, fields, or templates that are not shown."

// ActorStateRuntimeContext compiles the effective story schema and replayed
// values into a bounded Markdown write guide. JSON remains the backend
// contract; the model receives readable semantics, exact stable IDs, current
// values, and examples.
func ActorStateRuntimeContext(system StoryDirectorActorStateSystem, state map[string]any, limitBytes int, configuredChoiceCount ...int) string {
	if limitBytes <= 0 || limitBytes > DirectorContextMaxBytes {
		limitBytes = DirectorContextMaxBytes
	}
	system = normalizeActorStateSystem(system)
	state = ActorStateRuntimeProjection(system, state)
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
		"# Actor State Handbook",
		"",
		"> Source: `effective_actor_state_schema` + `Snapshot.State.actors` + `Snapshot.State.actor_archives`. Missing schema-defined initial Actors are projected at runtime only; event history is not rewritten.",
		"",
		"- Submit only changes that actually occurred in this turn's narrative. Do not repeat unchanged fields or clear them with empty values.",
		"- When referencing an existing Actor, copy the `actor_id` shown below in backticks exactly. Reuse `template_id` and `field_id` exactly as well.",
		"- When creating an Actor, actor_id and name must be identical and must use the character name in the story's language. Do not generate English translations, pinyin, or slug IDs.",
		"- `description` explains what a field means; `update_instruction` defines when and how to update it. Follow both.",
		"- Current State lists only values writable this turn. Field semantics and update rules appear once under Templates Available to New Actors to avoid duplicate context.",
		"- `replace` writes the complete end-of-turn value. Use `delta` only for an existing number. Do not resubmit fields already consumed by a rule check.",
		"- Use `archive` only for a non-system Actor whose death or permanent departure is confirmed in the narrative. The full historical state is preserved, but the Actor cannot be written or used in checks next turn.",
		"- Use `restore` when an archived Actor actually returns. Both archive and restore require a fact-based `reason`; never infer them from health values or wording alone.",
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
	blocks = append(blocks, "## Current State")
	for _, actorID := range actorIDs {
		if block := actorStateRuntimeActorMarkdown(system, state, rawActors, actorID); block != "" {
			blocks = append(blocks, block)
		}
	}
	if len(actorIDs) == 0 {
		blocks = append(blocks, "> There are no active Actors. Restore an archived Actor only when it truly returns, or create a character from Templates Available to New Actors only when persistent tracking is necessary.")
	}
	if archives, ok := state[actorArchiveRoot].([]ActorArchiveSummary); ok && len(archives) > 0 {
		blocks = append(blocks, "## Archived Actors (Read-only Index)")
		blocks = append(blocks, "> This section includes only identity and archive provenance needed to decide whether to restore an Actor. Full archived state is excluded from the next-turn context.")
		for _, archive := range archives {
			blocks = append(blocks, actorStateRuntimeArchiveMarkdown(archive))
		}
	}

	blocks = append(blocks, actorStateRuntimeSubmissionTemplate(system, rawActors, actorIDs, choiceCount))
	blocks = append(blocks, "## Templates Available to New Actors")
	for index, template := range system.Templates {
		if index >= maxInteractiveListItems {
			break
		}
		blocks = append(blocks, actorStateRuntimeTemplateMarkdown(template))
	}
	return blocks
}

func actorStateRuntimeArchiveMarkdown(archive ActorArchiveSummary) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "### %s\n\n", actorStateRuntimeText(firstNonEmptyString(archive.Name, archive.ActorID)))
	fmt.Fprintf(&sb, "- Actor ID: %s\n", actorStateRuntimeCode(archive.ActorID))
	if archive.TemplateID != "" {
		fmt.Fprintf(&sb, "- Template ID: %s\n", actorStateRuntimeCode(archive.TemplateID))
	}
	if archive.Reason != "" {
		fmt.Fprintf(&sb, "- Archive reason: %s\n", actorStateRuntimeText(archive.Reason))
	}
	if archive.SourceTurnID != "" {
		fmt.Fprintf(&sb, "- Source turn: %s\n", actorStateRuntimeCode(archive.SourceTurnID))
	}
	return strings.TrimSpace(sb.String())
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
	fmt.Fprintf(&sb, "- Actor ID: %s\n", actorStateRuntimeCode(actorID))
	fmt.Fprintf(&sb, "- Template ID: %s\n", actorStateRuntimeCode(templateID))
	if strings.TrimSpace(role) != "" {
		fmt.Fprintf(&sb, "- Role: %s\n", actorStateRuntimeText(role))
	}
	if description := actorStateRuntimeText(fmt.Sprint(record["description"])); description != "" && description != "<nil>" {
		fmt.Fprintf(&sb, "- Actor description: %s\n", description)
	}

	rawState, _ := record["state"].(map[string]any)
	for _, field := range template.Fields {
		fieldID := actorStateFieldID(field)
		value := rawState[fieldID]
		if value == nil && strings.TrimSpace(field.LegacyPath) != "" {
			value = getPathExact(rawState, field.LegacyPath)
		}
		fmt.Fprintf(&sb, "\n#### %s\n\n", actorStateRuntimeText(firstNonEmptyString(field.Name, fieldID)))
		fmt.Fprintf(&sb, "- Field ID: %s\n", actorStateRuntimeCode(fieldID))
		fmt.Fprintf(&sb, "- Current value: %s\n", actorStateRuntimeValue(value))
		fmt.Fprintf(&sb, "- Type: %s%s\n", actorStateRuntimeCode(field.Type), actorStateRuntimeConstraints(field))
	}

	traits := actorTraitInstancesFromState(state, actorID)
	if len(traits) > 0 {
		sb.WriteString("\n#### Assigned Traits (Read-only)\n")
		for _, trait := range traits {
			fmt.Fprintf(&sb, "\n- %s (Trait ID: %s)", actorStateRuntimeText(trait.Name), actorStateRuntimeCode(trait.TraitID))
			if summary := actorStateRuntimeText(trait.Summary); summary != "" {
				fmt.Fprintf(&sb, ": %s", summary)
			}
		}
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String())
}

func actorStateRuntimeTemplateMarkdown(template ActorStateTemplate) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "### %s\n\n", actorStateRuntimeText(firstNonEmptyString(template.Name, template.ID)))
	fmt.Fprintf(&sb, "- Template ID: %s\n", actorStateRuntimeCode(template.ID))
	if description := actorStateRuntimeText(template.Description); description != "" {
		fmt.Fprintf(&sb, "- Template description: %s\n", description)
	}
	if len(template.TraitRules) > 0 {
		sb.WriteString("- Traits are assigned automatically by the backend according to template rules. Do not fabricate traits in create parameters.\n")
	}
	for _, field := range template.Fields {
		fieldID := actorStateFieldID(field)
		fmt.Fprintf(&sb, "\n#### %s\n\n", actorStateRuntimeText(firstNonEmptyString(field.Name, fieldID)))
		fmt.Fprintf(&sb, "- Field ID: %s\n", actorStateRuntimeCode(fieldID))
		fmt.Fprintf(&sb, "- Type: %s%s\n", actorStateRuntimeCode(field.Type), actorStateRuntimeConstraints(field))
		if field.Default != nil {
			fmt.Fprintf(&sb, "- Default: %s\n", actorStateRuntimeValue(field.Default))
		}
		if description := actorStateRuntimeText(field.Description); description != "" {
			fmt.Fprintf(&sb, "- Field description: %s\n", description)
		}
		if instruction := actorStateRuntimeText(field.UpdateInstruction); instruction != "" {
			fmt.Fprintf(&sb, "- Update instruction: %s\n", instruction)
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
	sb.WriteString("## Submission Parameter Templates\n\n")
	sb.WriteString("Example update for an existing Actor field (submit only fields that actually changed):\n\n```json\n")
	example := map[string]any{
		"state_changes": []map[string]any{{"op": interactivestate.Replace, "actor_id": actorID, "field_id": fieldID, "value": exampleValue}},
		"choices":       choices,
	}
	data, _ := json.MarshalIndent(example, "", "  ")
	sb.Write(data)
	sb.WriteString("\n```\n\n")
	sb.WriteString("When no state changed, submit `\"state_changes\": []`. To create a new Actor, use:\n\n")
	sb.WriteString("`initial_state` must use real Field IDs from the target template. The keys below are type placeholders and must not be copied literally. number, bool, object, and list fields require native JSON values, not quoted strings. Omit a field that has a default when no reliable new fact overrides it.\n\n```json\n")
	create := map[string]any{
		"op":          interactivestate.Create,
		"actor_id":    "{{new_actor_name}}",
		"template_id": "{{template_id}}",
		"name":        "{{new_actor_name}}",
		"initial_state": map[string]any{
			"{{string_field_id}}": "{{text_value}}",
			"{{number_field_id}}": float64(0),
			"{{bool_field_id}}":   false,
			"{{object_field_id}}": map[string]any{},
			"{{list_field_id}}":   []any{},
		},
	}
	data, _ = json.MarshalIndent(create, "", "  ")
	sb.Write(data)
	sb.WriteString("\n```\n\nUse `archive` only after an Actor's death or permanent departure is confirmed. Use `restore` only when that Actor truly returns:\n\n```json\n")
	lifecycle := []map[string]any{
		{"op": interactivestate.Archive, "actor_id": "{{existing_actor_id}}", "reason": "{{confirmed_exit_reason}}"},
		{"op": interactivestate.Restore, "actor_id": "{{archived_actor_id}}", "reason": "{{confirmed_return_reason}}"},
	}
	data, _ = json.MarshalIndent(lifecycle, "", "  ")
	sb.Write(data)
	sb.WriteString("\n```\n\nFor nested object fields, use the optional `subpath` string array. Do not construct path strings manually.")
	return sb.String()
}

func actorStateRuntimeExample(system StoryDirectorActorStateSystem, rawActors map[string]any, actorIDs []string) (string, string, any) {
	for _, actorID := range actorIDs {
		record, _ := rawActors[actorID].(map[string]any)
		template := actorStateTemplateByID(system, normalizeActorStateID(fmt.Sprint(record["template_id"])))
		for _, field := range template.Fields {
			return actorID, actorStateFieldID(field), actorStateRuntimeExampleValue(field)
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
		parts = append(parts, fmt.Sprintf("minimum %v", *field.Min))
	}
	if field.Max != nil {
		parts = append(parts, fmt.Sprintf("maximum %v", *field.Max))
	}
	if len(field.Options) > 0 {
		options := make([]string, 0, len(field.Options))
		for _, option := range field.Options {
			options = append(options, actorStateRuntimeCode(option))
		}
		parts = append(parts, "allowed values "+strings.Join(options, ", "))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, "; ") + ")"
}

func actorStateRuntimeValue(value any) string {
	if value == nil {
		return "_unset_"
	}
	if text, ok := value.(string); ok {
		return actorStateRuntimeCode(text)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "_unreadable_"
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
