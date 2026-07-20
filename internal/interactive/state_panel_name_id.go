package interactive

import (
	"fmt"
	"log"
	"sort"
)

var statePanelRecordNameFields = map[string]string{
	"技能与能力": "名称",
	"重要物品":  "名称",
	"当前任务":  "任务名称",
	"地点记录":  "地点名称",
	"势力记录":  "势力名称",
}

// normalizeStatePanelRecordNameIDs derives a missing canonical name only from
// the record's own story-language ID. A conflicting explicit name is left
// untouched so the strict validator can reject the ambiguous input.
func normalizeStatePanelRecordNameIDs(fieldID string, value any) (any, bool) {
	nameField, enforced := statePanelRecordNameFields[normalizeActorStateFieldName(fieldID)]
	if !enforced {
		return value, false
	}
	records, ok := value.(map[string]any)
	if !ok {
		return value, false
	}
	normalizedRecords := make(map[string]any, len(records))
	changed := false
	for id, rawRecord := range records {
		record, ok := rawRecord.(map[string]any)
		if !ok {
			normalizedRecords[id] = rawRecord
			continue
		}
		normalizedRecord := make(map[string]any, len(record)+1)
		for key, item := range record {
			normalizedRecord[key] = item
		}
		if nameField != "名称" {
			if alias, aliasExists := normalizedRecord["名称"]; aliasExists {
				aliasName, aliasOK := alias.(string)
				if !aliasOK || normalizeActorStateFieldName(aliasName) != normalizeActorStateFieldName(id) {
					normalizedRecords[id] = normalizedRecord
					continue
				}
				delete(normalizedRecord, "名称")
				changed = true
			}
		}
		if _, exists := normalizedRecord[nameField]; !exists {
			normalizedRecord[nameField] = id
			changed = true
			log.Printf("[interactive-turn-submission] derived named-record field field_id=%q record_id=%q name_field=%q location=internal/interactive/state_panel_name_id.go", fieldID, id, nameField)
		}
		normalizedRecords[id] = normalizedRecord
	}
	if !changed {
		return value, false
	}
	return normalizedRecords, true
}

// validateStatePanelRecordNameIDs keeps the object key shown as a record ID in
// the state panel identical to that record's visible name.
func validateStatePanelRecordNameIDs(fieldID string, value any) error {
	nameField, enforced := statePanelRecordNameFields[normalizeActorStateFieldName(fieldID)]
	if !enforced {
		return nil
	}
	records, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(records))
	for id := range records {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		record, ok := records[id].(map[string]any)
		if !ok {
			return fmt.Errorf("状态面板字段 %s 的记录 %q 必须是包含 %s 的 object", fieldID, id, nameField)
		}
		if nameField != "名称" {
			if alias, exists := record["名称"]; exists {
				aliasName, aliasOK := alias.(string)
				if !aliasOK || normalizeActorStateFieldName(aliasName) != normalizeActorStateFieldName(id) {
					return fmt.Errorf("状态面板字段 %s 的记录包含冲突的 名称: id=%q name=%v", fieldID, id, alias)
				}
			}
		}
		name, ok := record[nameField].(string)
		if !ok || normalizeActorStateFieldName(name) == "" {
			return fmt.Errorf("状态面板字段 %s 的记录 %q 缺少非空 %s", fieldID, id, nameField)
		}
		if normalizeActorStateFieldName(id) != normalizeActorStateFieldName(name) {
			return fmt.Errorf("状态面板字段 %s 的记录 ID 必须与 %s 完全相同（名称即 ID）: id=%q name=%q", fieldID, nameField, id, name)
		}
	}
	return nil
}

func validateActorStateRecordNameIDs(template ActorStateTemplate, state map[string]any) error {
	for _, field := range template.Fields {
		fieldID := actorStateFieldID(field)
		value, exists := state[fieldID]
		if !exists {
			continue
		}
		if err := validateStatePanelRecordNameIDs(fieldID, value); err != nil {
			return err
		}
	}
	return nil
}
