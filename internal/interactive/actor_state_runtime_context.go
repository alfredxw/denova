package interactive

import (
	interactivestate "denova/internal/interactive/state"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ActorStateRuntimeContext compiles the effective story schema and replayed
// values into a complete Markdown write guide. The caller must reject an
// oversized context before model execution, never truncate this write contract.
// Shared definitions affect presentation only; persisted schemas stay unchanged.
func ActorStateRuntimeContext(system StoryDirectorActorStateSystem, state map[string]any, choiceCount int) string {
	system = normalizeActorStateSystem(system)
	state = ActorStateRuntimeProjection(system, state)
	choiceCount = normalizeStoryChoiceCount(choiceCount)
	if validateStoryChoiceCount(choiceCount) != nil {
		choiceCount = DefaultStoryChoiceCount
	}
	blocks := actorStateRuntimeMarkdownBlocks(system, state, choiceCount)
	return strings.Join(blocks, "\n\n")
}

func actorStateRuntimeMarkdownBlocks(system StoryDirectorActorStateSystem, state map[string]any, choiceCount int) []string {
	blocks := []string{strings.Join([]string{
		"# Actor State Handbook",
		"",
		"> Source: `effective_actor_state_schema` + `Snapshot.State.actors` + `Snapshot.State.actor_archives`. Missing schema-defined initial Actors are projected at runtime only; event history is not rewritten.",
		"",
		"- Submit facts established by this turn's narrative and required story-continuity updates. Do not repeat other unchanged fields or clear them with empty values.",
		"- When referencing an existing Actor, copy the `actor_id` shown below in backticks exactly. Reuse `template_id` and `field_id` exactly as well.",
		"- When an Actor has no initial definition, choose a template that fits its confirmed role and state needs, then create it in state_changes. Use the established IDs story and world for those system Actors. For new characters, including the player character, use their story-language name as both actor_id and name.",
		"- Templates list every allowed Field ID. Field Definitions provide each field's type, constraints, default, description, and update instruction; a shared definition applies only to its listed Template IDs. Follow both Field description and Update instruction.",
		"- Current State maps exact Field IDs to complete JSON values. `_unset_` means no current value, not zero, false, an empty string, or a template default. Template defaults describe creation, not current state.",
		"- When the story Actor defines 当前事件, set it every turn. Initialize 当前详细地点 when unset and update it when prose changes location. On creation, put both values in initial_state. Opening initialization_guide.required_state_changes must also be filled; an empty state_changes array cannot bypass required updates.",
		"- `replace` writes the complete end-of-turn value. Use `delta` only for an existing number. Do not resubmit fields already consumed by a rule check.",
		"- Use `archive` only for a non-system Actor whose death or permanent departure is confirmed in the narrative. The full historical state is preserved, but the Actor cannot be written or used in checks next turn.",
		"- Use `restore` when an archived Actor actually returns. Both archive and restore require a fact-based `reason`; never infer them from health values or wording alone.",
	}, "\n")}
	blocks = append(blocks, actorStateRuntimeSchemaMarkdown(system))

	rawActors, _ := state[actorStateRoot].(map[string]any)
	actorIDs := make([]string, 0, len(rawActors))
	for actorID := range rawActors {
		actorIDs = append(actorIDs, actorID)
	}
	sort.Strings(actorIDs)
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
		fmt.Fprintf(&sb, "- %s: %s\n", actorStateRuntimeCode(fieldID), actorStateRuntimeValue(value))
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

func actorStateRuntimeSchemaMarkdown(system StoryDirectorActorStateSystem) string {
	// The full rendered contract is the deduplication key: fields differing in
	// any model-visible rule remain separate, even when their IDs match.
	type definition struct {
		markdown  string
		templates []string
	}
	definitions := []definition{}
	definitionIndex := map[string]int{}
	var sb strings.Builder
	sb.WriteString("## Templates Available to New Actors\n")
	for _, template := range system.Templates {
		fmt.Fprintf(&sb, "\n### %s (Template ID: %s)\n", actorStateRuntimeText(template.Name), actorStateRuntimeCode(template.ID))
		if description := actorStateRuntimeText(template.Description); description != "" {
			fmt.Fprintf(&sb, "\n%s\n", description)
		}
		if len(template.TraitRules) > 0 {
			sb.WriteString("\nTraits are assigned automatically by the backend according to template rules. Do not fabricate traits in create parameters.\n")
		}
		fieldIDs := make([]string, 0, len(template.Fields))
		for _, field := range template.Fields {
			fieldIDs = append(fieldIDs, actorStateRuntimeCode(actorStateFieldID(field)))
			markdown := actorStateRuntimeFieldMarkdown(field)
			index, exists := definitionIndex[markdown]
			if !exists {
				index = len(definitions)
				definitionIndex[markdown] = index
				definitions = append(definitions, definition{markdown: markdown})
			}
			definitions[index].templates = append(definitions[index].templates, actorStateRuntimeCode(template.ID))
		}
		fmt.Fprintf(&sb, "\nField IDs: %s\n", strings.Join(fieldIDs, ", "))
	}
	// Keep template-set and field ordering deterministic without creating IDs
	// or aliases that the model could accidentally submit as real Field IDs.
	groups := []string{}
	groupFields := map[string][]string{}
	for _, definition := range definitions {
		group := strings.Join(definition.templates, ", ")
		if _, exists := groupFields[group]; !exists {
			groups = append(groups, group)
		}
		groupFields[group] = append(groupFields[group], definition.markdown)
	}
	sb.WriteString("\n## Field Definitions\n")
	for _, group := range groups {
		fmt.Fprintf(&sb, "\n### Applies to Template IDs: %s\n\n%s\n", group, strings.Join(groupFields[group], "\n\n"))
	}
	return strings.TrimSpace(sb.String())
}

func actorStateRuntimeFieldMarkdown(field ActorStateField) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "- %s: %s%s", actorStateRuntimeCode(actorStateFieldID(field)), field.Type, actorStateRuntimeConstraints(field))
	if field.Default != nil {
		fmt.Fprintf(&sb, "; default=%s", actorStateRuntimeValue(field.Default))
	}
	if field.MaxField != "" {
		fmt.Fprintf(&sb, "; capacity=the same Actor's %s field (initialize both; current must not exceed capacity)", actorStateRuntimeCode(field.MaxField))
	}
	if description := actorStateRuntimeText(field.Description); description != "" {
		fmt.Fprintf(&sb, "\n  Field description: %s", description)
	}
	if instruction := actorStateRuntimeText(field.UpdateInstruction); instruction != "" {
		fmt.Fprintf(&sb, "\n  Update instruction: %s", instruction)
	}
	return sb.String()
}

func actorStateRuntimeSubmissionTemplate(system StoryDirectorActorStateSystem, rawActors map[string]any, actorIDs []string, choiceCount int) string {
	actorID, fieldID, exampleValue := actorStateRuntimeExample(system, rawActors, actorIDs)
	choices := make([]string, normalizeStoryChoiceCount(choiceCount))
	for index := range choices {
		choices[index] = fmt.Sprintf("{{next_action_%d}}", index+1)
	}
	var sb strings.Builder
	sb.WriteString("## Submission Parameter Templates\n\n")
	sb.WriteString("Example update for an existing Actor field. Include required story-continuity and opening initialization updates as well:\n\n```json\n")
	example := map[string]any{
		"state_changes": []map[string]any{{"op": interactivestate.Replace, "actor_id": actorID, "field_id": fieldID, "value": exampleValue}},
		"choices":       choices,
	}
	data, _ := json.Marshal(example)
	sb.Write(data)
	sb.WriteString("\n```\n\n")
	sb.WriteString("Use an empty state_changes array only when there are no changed or required fields. To create a new Actor, use:\n\n")
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
	data, _ = json.Marshal(create)
	sb.Write(data)
	sb.WriteString("\n```\n\nUse `archive` only after an Actor's death or permanent departure is confirmed. Use `restore` only when that Actor truly returns:\n\n```json\n")
	lifecycle := []map[string]any{
		{"op": interactivestate.Archive, "actor_id": "{{existing_actor_id}}", "reason": "{{confirmed_exit_reason}}"},
		{"op": interactivestate.Restore, "actor_id": "{{archived_actor_id}}", "reason": "{{confirmed_return_reason}}"},
	}
	data, _ = json.Marshal(lifecycle)
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
		parts = append(parts, "allowed values "+actorStateRuntimeValue(field.Options))
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
	data, err := json.Marshal(value)
	if err != nil {
		return "_unreadable_"
	}
	return string(data)
}

func actorStateRuntimeText(value string) string {
	return strings.TrimSpace(value)
}

func actorStateRuntimeCode(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	fence := "`"
	for strings.Contains(value, fence) {
		fence += "`"
	}
	if len(fence) > 1 {
		value = " " + value + " "
	}
	return fence + value + fence
}
