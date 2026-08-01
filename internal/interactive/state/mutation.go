// Package state defines the durable and model-facing mutation contract used by
// interactive stories. It is deliberately independent from story storage and
// Actor schema compilation so both layers can depend on one canonical format.
package state

import (
	"encoding/json"
	"fmt"
	"strings"
)

const SourceTurnResult = "turn_result"

const (
	Replace = "replace"
	Delta   = "delta"
	Create  = "create"
	Archive = "archive"
	Restore = "restore"
)

// Op is the replayable state reducer input persisted with a story event.
type Op struct {
	Op           string `json:"op"`
	Path         string `json:"path"`
	Value        any    `json:"value,omitempty"`
	Reason       string `json:"reason,omitempty"`
	SourceTurnID string `json:"source_turn_id,omitempty"`
	SourceKind   string `json:"source_kind,omitempty"`
	SourceID     string `json:"source_id,omitempty"`
}

// Update is the small, model-facing state mutation contract. Path is an RFC
// 6901 JSON Pointer whose first segment is a stable Actor ID.
type Update struct {
	Op    string `json:"op" jsonschema:"enum=replace,enum=delta,enum=create,enum=archive,enum=restore" jsonschema_description:"状态操作：replace/delta 更新字段，create 新建 Actor，archive/restore 改变 Actor 是否参与运行时状态。"`
	Path  string `json:"path" jsonschema_description:"以稳定 actor_id 开头的 schema-bound JSON Pointer，例如 /protagonist/生命值。"`
	Value any    `json:"value" jsonschema_description:"replace/create 的目标值，或 delta 的数值变化量。"`
}

// NormalizeUpdates canonicalizes operation names and pointer whitespace while
// preserving the complete submitted list and values.
func NormalizeUpdates(updates []Update) []Update {
	if updates == nil {
		return []Update{}
	}
	result := make([]Update, len(updates))
	for index, update := range updates {
		update.Op = strings.ToLower(strings.TrimSpace(update.Op))
		update.Path = strings.TrimSpace(update.Path)
		result[index] = update
	}
	return result
}

// ValidateUpdate validates the transport shape before schema-aware compiling.
func ValidateUpdate(update Update) error {
	switch update.Op {
	case Replace, Delta, Create, Archive, Restore:
	default:
		return fmt.Errorf("op 必须是 replace、delta、create、archive 或 restore")
	}
	if _, err := ParsePath(update.Path); err != nil {
		return err
	}
	if update.Value == nil {
		return fmt.Errorf("value 不能为空")
	}
	if update.Op == Delta && !number(update.Value) {
		return fmt.Errorf("delta 的 value 必须是 number")
	}
	return nil
}

func number(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, json.Number:
		return true
	default:
		return false
	}
}
