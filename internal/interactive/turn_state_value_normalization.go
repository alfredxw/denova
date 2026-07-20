package interactive

import (
	"log"
	"math"
	"strconv"
	"strings"
)

// normalizeTurnSubmissionActorStateValues repairs only lossless model-output
// encodings. The regular Actor State validator remains the source of truth and
// still rejects ambiguous or semantically invalid values after normalization.
func normalizeTurnSubmissionActorStateValues(actorID string, template ActorStateTemplate, values map[string]any) map[string]any {
	if len(values) == 0 || template.ID == "" {
		return values
	}
	fieldsByReference := actorStateFieldsByReference(template)
	normalized := make(map[string]any, len(values))
	for fieldReference, value := range values {
		field, found := fieldsByReference[actorStateFieldNameKey(fieldReference)]
		if !found {
			normalized[fieldReference] = value
			continue
		}
		converted, changed := normalizeTurnSubmissionFieldValue(field, value)
		if changed {
			log.Printf("[interactive-turn-submission] normalized lossless field value actor_id=%q field_id=%q from=string to=%s location=internal/interactive/turn_state_value_normalization.go", actorID, actorStateFieldID(field), field.Type)
			value = converted
		}
		if canonical, canonicalized := normalizeStatePanelRecordNameIDs(actorStateFieldID(field), value); canonicalized {
			value = canonical
		}
		normalized[fieldReference] = value
	}
	return normalized
}

func normalizeTurnSubmissionFieldValue(field ActorStateField, value any) (any, bool) {
	text, isString := value.(string)
	if !isString {
		return value, false
	}
	text = strings.TrimSpace(text)
	switch field.Type {
	case "number":
		number, err := strconv.ParseFloat(text, 64)
		if err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
			return number, true
		}
	case "bool":
		switch strings.ToLower(text) {
		case "true":
			return true, true
		case "false":
			return false, true
		}
	case "object":
		var object map[string]any
		if err := decodeStrictJSON([]byte(text), &object, true); err == nil && object != nil {
			return object, true
		}
	case "list":
		var list []any
		if err := decodeStrictJSON([]byte(text), &list, true); err == nil && list != nil {
			return list, true
		}
	}
	return value, false
}
