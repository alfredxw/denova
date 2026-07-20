package interactive

import (
	"fmt"
	"sort"
)

var statePanelRecordNameFields = map[string]string{
	"技能与能力": "名称",
	"重要物品":  "名称",
	"当前任务":  "任务名称",
	"地点记录":  "地点名称",
	"势力记录":  "势力名称",
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
