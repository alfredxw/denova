package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	"denova/internal/book/lore"
)

type readLoreItemsInput struct {
	IDs   []string `json:"ids,omitempty" jsonschema:"description=Lore item IDs. Prefer names when reading by unique name."`
	Names []string `json:"names,omitempty" jsonschema:"description=Unique lore item names. Director and creative Agents should prefer reading by name."`
}

type listLoreItemsInput struct {
	Keywords  []string `json:"keywords,omitempty" jsonschema_description:"Optional search terms. Each item independently matches ID, name, aliases, tags, description, and body. Do not combine several keywords into one string."`
	Match     string   `json:"match,omitempty" jsonschema:"enum=any,enum=all" jsonschema_description:"Relationship among keywords: any matches any keyword (OR, default); all requires every keyword (AND)."`
	Types     []string `json:"types,omitempty" jsonschema_description:"Optional lore types: character, world, location, faction, rule, item, or other."`
	LoadModes []string `json:"load_modes,omitempty" jsonschema_description:"Optional load modes: resident, auto, or manual. Prefer resident for state-schema review."`
	Detail    string   `json:"detail,omitempty" jsonschema:"enum=index,enum=full" jsonschema_description:"Result detail: index (default) returns catalog entries and descriptions; full returns complete bodies when filters are present, avoiding a separate read_lore_items call."`
	Limit     int      `json:"limit,omitempty" jsonschema_description:"Number of filtered results on this page, default 10. Unfiltered catalogs paginate automatically by index byte budget."`
	Offset    int      `json:"offset,omitempty" jsonschema_description:"Pagination offset, default 0. Continue from the returned next-page offset."`
}

type writeLoreItemsInput struct {
	Message   string               `json:"message,omitempty" jsonschema:"description=Optional change summary for this lore update; summarize briefly in Chinese."`
	Items     []writeLoreItemInput `json:"items,omitempty" jsonschema:"description=Lore items to create or partially update. Each array element is one independently retrievable entity or coherent topic; use separate elements for distinct characters, locations, factions, items, or world rules. Do not put the entire library or several unrelated entities into one element. Creation requires at least name. Updates require an existing id and only changed fields; omitted fields retain their values."`
	DeleteIDs []string             `json:"delete_ids,omitempty" jsonschema:"description=Lore item IDs to delete. Use only when the author explicitly requests deletion."`
}

type writeLoreItemInput struct {
	ID               string   `json:"id,omitempty" jsonschema:"description=Lore ID. An update requires the exact existing ID; creation may omit it for automatic generation."`
	Enabled          *bool    `json:"enabled,omitempty" jsonschema:"description=Whether the lore item is enabled. A disabled item remains stored but is excluded from the lore index, read tools, and model context. Omit when uncertain."`
	Type             string   `json:"type,omitempty" jsonschema:"description=Lore type: character, world, location, faction, rule, item, or other. Creation defaults to other; omission during update retains the current value."`
	Name             string   `json:"name,omitempty" jsonschema:"description=Lore name. Required for creation; omission during update retains the current value."`
	Importance       string   `json:"importance,omitempty" jsonschema:"description=Importance: major, important, or minor. Creation defaults to important; omission during update retains the current value."`
	Tags             []string `json:"tags,omitempty" jsonschema:"description=Tags. Omission during update retains the current value; an empty array clears it."`
	BriefDescription string   `json:"brief_description,omitempty" jsonschema:"description=Index description. Start with type and name, then use 3-5 sentences for identity, aliases, key facts, use cases, and trigger terms. Omission on creation generates it from the body; omission on update retains the current value."`
	Keywords         []string `json:"keywords,omitempty" jsonschema:"description=Aliases, keywords, or trigger terms. Omission during update retains the current value; an empty array clears it."`
	LoadMode         string   `json:"load_mode,omitempty" jsonschema:"description=Load mode: resident, auto, or manual. Creation infers it automatically; omission during update retains the current value."`
	Content          string   `json:"content,omitempty" jsonschema:"description=Markdown body for this entity or coherent topic only, in the author's language. Include its stable canon and relevant relationships; put other independently retrievable entities in separate items. On update this replaces the entire body: read the existing item first and preserve still-valid facts. Omission retains the current value. Put per-chapter current location, injuries, psychology, and goals in setting/character-states.md instead of lore."`
}

type loreToolsOptions struct {
	ReadPolicy *loreReadPolicy
}

// loreReadPolicy observes only lore bodies successfully returned to the model.
// Output sizing belongs to the shared tool-result projection boundary; Lore
// must not reject an otherwise valid batch using a second, stricter budget.
type loreReadPolicy struct {
	OnRead func([]string)
}

func defaultLoreReadPolicy() *loreReadPolicy {
	return &loreReadPolicy{}
}

func (p *loreReadPolicy) validateBatch(input readLoreItemsInput) error {
	if len(input.IDs) > 0 && len(input.Names) > 0 {
		return fmt.Errorf("provide either ids or names, not both")
	}
	count := len(input.IDs) + len(input.Names)
	if count == 0 {
		return fmt.Errorf("provide at least one lore ID or unique name")
	}
	return nil
}

func (p *loreReadPolicy) observe(items []lore.Item) {
	if p == nil || p.OnRead == nil {
		return
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if id := strings.TrimSpace(item.ID); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		p.OnRead(ids)
	}
}

func newLoreTools(workspace string, allowWrite bool, options ...loreToolsOptions) ([]agent.ToolDefinition, error) {
	workspace = strings.TrimSpace(workspace)
	var readPolicy *loreReadPolicy
	if len(options) > 0 {
		readPolicy = options[0].ReadPolicy
	}
	if readPolicy == nil {
		readPolicy = defaultLoreReadPolicy()
	}
	readTool, err := agent.InferTool("read_lore_items", "Batch-read complete lore bodies by item ID or unique name. When a name already appears in the context catalog, read it directly without first calling list_lore_items.", func(ctx context.Context, input readLoreItemsInput) (string, error) {
		_ = ctx
		if workspace == "" {
			return "", fmt.Errorf("cannot read lore because the current workspace is unavailable")
		}
		if err := readPolicy.validateBatch(input); err != nil {
			return "", err
		}
		store := lore.NewStore(workspace)
		var result lore.ReadResult
		var err error
		missingField := "ids"
		if len(input.Names) > 0 {
			result, err = store.ReadManyNames(input.Names)
			missingField = "names"
		} else {
			result, err = store.ReadMany(input.IDs)
		}
		if err != nil {
			return "", err
		}
		if len(result.Items) == 0 {
			missing, _ := json.Marshal(result.Missing)
			return "", fmt.Errorf("no lore items matched; missing %s: %s", missingField, missing)
		}
		output := formatLoreReadResult(result, missingField)
		readPolicy.observe(result.Items)
		return output, nil
	})
	if err != nil {
		return nil, err
	}
	listTool, err := agent.InferTool("list_lore_items", "Browse or search enabled lore. An empty filter returns a name catalog of at most 256 KiB. With filters, detail=index returns descriptions and detail=full can return complete bodies in the same call. Use read_lore_items directly for a known unique name.", func(ctx context.Context, input listLoreItemsInput) (string, error) {
		_ = ctx
		if workspace == "" {
			return "", fmt.Errorf("cannot list lore because the current workspace is unavailable")
		}
		if err := validateListLoreItemsInput(input); err != nil {
			return "", err
		}
		store := lore.NewStore(workspace)
		if !hasLoreListFilters(input) {
			catalog, err := store.NameCatalogMarkdown(lore.NameCatalogOptions{
				Offset:   input.Offset,
				MaxBytes: lore.IndexDefaultMaxBytes,
			})
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(catalog), nil
		}
		options := lore.IndexOptions{
			Keywords:  input.Keywords,
			Match:     input.Match,
			Types:     input.Types,
			LoadModes: input.LoadModes,
			Limit:     input.Limit,
			Offset:    input.Offset,
			Paginate:  true,
		}
		if strings.EqualFold(strings.TrimSpace(input.Detail), "full") {
			items, err := store.QueryLoreItems(options)
			if err != nil {
				return "", err
			}
			output := formatLoreItems(items)
			readPolicy.observe(items)
			return output, nil
		}
		index, err := store.SearchIndexMarkdown(options)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(index) == "" {
			return "The lore store is empty.", nil
		}
		return strings.TrimSpace(index), nil
	})
	if err != nil {
		return nil, err
	}
	readDescriptor := boundedReadDescriptor(ToolSourceLore, config.AgentToolLoreRead, agent.ToolResultRecoveryRerun)
	definedReadTool, err := defineTool(readTool, readDescriptor)
	if err != nil {
		return nil, err
	}
	definedListTool, err := defineTool(listTool, readDescriptor)
	if err != nil {
		return nil, err
	}
	tools := []agent.ToolDefinition{definedListTool, definedReadTool}
	if !allowWrite {
		return tools, nil
	}
	writeTool, err := agent.InferTool("write_lore_items", "Batch-create, partially update, or delete lore items. Each item is one independently retrievable entity or coherent topic, such as a character, location, faction, item, or world rule. Organize library-wide updates into separate items in the same batch, not one omnibus entry. Keep related facts about the same entity together. Find matching existing items with list_lore_items and read their bodies before updating; reuse their exact IDs instead of creating duplicates. Creation requires at least name; updates send only changed fields, while omitted fields retain their values. A supplied content replaces the whole body, so preserve still-valid canon. The backend may generate brief_description on creation. Put post-chapter current location, injuries, psychology, goals, and possessions in setting/character-states.md instead of lore. Do not store chapter planning or future plot in lore.", func(ctx context.Context, input writeLoreItemsInput) (agent.ToolResult, error) {
		_ = ctx
		if workspace == "" {
			return agent.ToolResult{}, fmt.Errorf("cannot write lore because the current workspace is unavailable")
		}
		store := lore.NewStore(workspace)
		ops, err := buildWriteLoreOperations(store, input)
		if err != nil {
			return agent.ToolResult{}, err
		}
		result, err := store.ApplyOperations(input.Message, ops)
		if err != nil {
			return agent.ToolResult{}, err
		}
		details, err := json.Marshal(map[string]any{
			"schema": "lore.write.v1", "item_ids": writeLoreChangedItemIDs(result),
			"deleted_ids": result.DeletedIDs,
		})
		if err != nil {
			return agent.ToolResult{}, err
		}
		toolResult := agent.TextToolResult(formatWriteLoreItemsResult(result))
		toolResult.Details = details
		return toolResult, nil
	})
	if err != nil {
		return nil, err
	}
	definedWriteTool, err := defineTool(writeTool, workspaceWriteDescriptor(ToolSourceLore, config.AgentToolLoreWrite, agent.ToolRecoveryReconcilable))
	if err != nil {
		return nil, err
	}
	return append(tools, definedWriteTool), nil
}

func validateListLoreItemsInput(input listLoreItemsInput) error {
	match := strings.TrimSpace(input.Match)
	if match != "" && match != lore.IndexMatchAny && match != lore.IndexMatchAll {
		return fmt.Errorf("match must be any or all")
	}
	validTypes := map[string]bool{"character": true, "world": true, "location": true, "faction": true, "rule": true, "item": true, "other": true}
	for _, itemType := range input.Types {
		if !validTypes[strings.TrimSpace(itemType)] {
			return fmt.Errorf("invalid lore type: %s", strings.TrimSpace(itemType))
		}
	}
	validLoadModes := map[string]bool{lore.LoadModeResident: true, lore.LoadModeAuto: true, lore.LoadModeManual: true}
	for _, loadMode := range input.LoadModes {
		if !validLoadModes[strings.TrimSpace(loadMode)] {
			return fmt.Errorf("invalid lore load mode: %s", strings.TrimSpace(loadMode))
		}
	}
	if input.Limit < 0 {
		return fmt.Errorf("limit cannot be negative; omission defaults to %d", lore.IndexDefaultLimit)
	}
	if input.Offset < 0 {
		return fmt.Errorf("offset cannot be negative")
	}
	detail := strings.ToLower(strings.TrimSpace(input.Detail))
	if detail != "" && detail != "index" && detail != "full" {
		return fmt.Errorf("detail must be index or full")
	}
	if detail == "full" && !hasLoreListFilters(input) {
		return fmt.Errorf("detail=full requires keywords, types, or load_modes; an unbounded read of all lore bodies is not allowed")
	}
	return nil
}

func hasLoreListFilters(input listLoreItemsInput) bool {
	return len(input.Keywords) > 0 || len(input.Types) > 0 || len(input.LoadModes) > 0
}

func formatLoreItems(items []lore.Item) string {
	if len(items) == 0 {
		return "No lore items were read."
	}
	var sb strings.Builder
	fmt.Fprintln(&sb, "# Lore Items")
	fmt.Fprintln(&sb)
	for _, item := range items {
		fmt.Fprintln(&sb, lore.ReferenceMarkdown(item))
		fmt.Fprintln(&sb)
	}
	return strings.TrimSpace(sb.String())
}

func formatLoreReadResult(result lore.ReadResult, missingField string) string {
	output := formatLoreItems(result.Items)
	if len(result.Missing) == 0 {
		return output
	}
	missing, _ := json.Marshal(result.Missing)
	return output + "\n\n## Missing Lore\n\n" + missingField + ": " + string(missing)
}

func buildWriteLoreOperations(store *lore.Store, input writeLoreItemsInput) ([]lore.Operation, error) {
	itemsByID := map[string]lore.Item{}
	// Explicit write IDs may refer to disabled entries. Those entries stay out
	// of model read tools, but an author-approved review snapshot can still
	// safely drive an update without accidentally treating the ID as a create.
	existing, err := store.ListAll()
	if err != nil {
		return nil, err
	}
	for _, item := range existing {
		itemsByID[item.ID] = item
	}
	ops := make([]lore.Operation, 0, len(input.Items)+len(input.DeleteIDs))
	for _, item := range input.Items {
		item.ID = strings.TrimSpace(item.ID)
		item.Name = strings.TrimSpace(item.Name)
		loreInput := lore.ItemInput{
			ID:               item.ID,
			Enabled:          item.Enabled,
			Type:             item.Type,
			Name:             item.Name,
			Importance:       item.Importance,
			Tags:             item.Tags,
			BriefDescription: item.BriefDescription,
			Keywords:         item.Keywords,
			LoadMode:         item.LoadMode,
			Content:          item.Content,
		}
		op := "create"
		if item.ID != "" {
			if _, ok := itemsByID[item.ID]; ok {
				op = "update"
			}
		}
		if op == "create" && item.Name == "" {
			return nil, fmt.Errorf("name is required when creating lore")
		}
		if op == "update" && !hasWriteLoreItemChanges(item) {
			return nil, fmt.Errorf("updating lore item %s requires at least one changed field", item.ID)
		}
		ops = append(ops, lore.Operation{Op: op, ID: item.ID, Item: loreInput})
	}
	for _, id := range input.DeleteIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		ops = append(ops, lore.Operation{Op: "delete", ID: id})
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("there are no lore changes to write")
	}
	return ops, nil
}

func hasWriteLoreItemChanges(item writeLoreItemInput) bool {
	return item.Enabled != nil || strings.TrimSpace(item.Type) != "" || strings.TrimSpace(item.Name) != "" ||
		strings.TrimSpace(item.Importance) != "" || item.Tags != nil || strings.TrimSpace(item.BriefDescription) != "" ||
		item.Keywords != nil || strings.TrimSpace(item.LoadMode) != "" || strings.TrimSpace(item.Content) != ""
}

func formatWriteLoreItemsResult(result lore.ApplyResult) string {
	changed := []string{}
	if len(result.Created) > 0 {
		changed = append(changed, fmt.Sprintf("created %d", len(result.Created)))
	}
	if len(result.Updated) > 0 {
		changed = append(changed, fmt.Sprintf("updated %d", len(result.Updated)))
	}
	if len(result.DeletedIDs) > 0 {
		changed = append(changed, fmt.Sprintf("deleted %d", len(result.DeletedIDs)))
	}
	message := strings.TrimSpace(result.Message)
	if message == "" {
		message = "Lore updated"
	}
	if len(changed) > 0 {
		message += "（" + strings.Join(changed, "，") + "）"
	}
	itemIDs := writeLoreChangedItemIDs(result)
	itemIDsJSON, _ := json.Marshal(itemIDs)
	deletedIDsJSON, _ := json.Marshal(result.DeletedIDs)
	lines := []string{message}
	lines = append(lines, "item_ids: "+string(itemIDsJSON))
	lines = append(lines, "deleted_ids: "+string(deletedIDsJSON))
	return strings.Join(lines, "\n")
}

func writeLoreChangedItemIDs(result lore.ApplyResult) []string {
	ids := make([]string, 0, len(result.Created)+len(result.Updated)+len(result.DeletedIDs))
	seen := map[string]bool{}
	for _, item := range result.Created {
		if item.ID != "" && !seen[item.ID] {
			seen[item.ID] = true
			ids = append(ids, item.ID)
		}
	}
	for _, item := range result.Updated {
		if item.ID != "" && !seen[item.ID] {
			seen[item.ID] = true
			ids = append(ids, item.ID)
		}
	}
	for _, id := range result.DeletedIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}
