package interactive

import "encoding/json"

// These decoders are the v1 replay adapter. New JSON only emits structured
// actor_id + field_id references; old path fields are accepted in memory.
func (value *TurnCheckAdjudication) UnmarshalJSON(data []byte) error {
	type stored TurnCheckAdjudication
	var payload struct {
		stored
		StatePaths []string `json:"state_paths,omitempty"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	*value = TurnCheckAdjudication(payload.stored)
	value.StatePaths = payload.StatePaths
	return nil
}

func (value *TurnCheckBonus) UnmarshalJSON(data []byte) error {
	type stored TurnCheckBonus
	var payload struct {
		stored
		SourcePath string `json:"source_path,omitempty"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	*value = TurnCheckBonus(payload.stored)
	value.SourcePath = payload.SourcePath
	return nil
}

func (value *TurnStateChange) UnmarshalJSON(data []byte) error {
	type stored TurnStateChange
	var payload struct {
		stored
		Path string `json:"path,omitempty"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	*value = TurnStateChange(payload.stored)
	value.Path = payload.Path
	return nil
}

func (value *RuleStateBindingModifier) UnmarshalJSON(data []byte) error {
	type stored RuleStateBindingModifier
	var payload struct {
		stored
		LegacyFieldPath string `json:"field_path,omitempty"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	*value = RuleStateBindingModifier(payload.stored)
	if value.FieldID == "" {
		value.FieldID = payload.LegacyFieldPath
	}
	return nil
}

func (value *RuleNarrativeStateRef) UnmarshalJSON(data []byte) error {
	type stored RuleNarrativeStateRef
	var payload struct {
		stored
		LegacyFieldPath string `json:"field_path,omitempty"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	*value = RuleNarrativeStateRef(payload.stored)
	if value.FieldID == "" {
		value.FieldID = payload.LegacyFieldPath
	}
	return nil
}

func (value *RuleComputedStateChange) UnmarshalJSON(data []byte) error {
	type stored RuleComputedStateChange
	var payload struct {
		stored
		LegacyFieldPath string `json:"field_path,omitempty"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	*value = RuleComputedStateChange(payload.stored)
	if value.FieldID == "" {
		value.FieldID = payload.LegacyFieldPath
	}
	return nil
}

func (value *RuleStateFormulaTerm) UnmarshalJSON(data []byte) error {
	type stored RuleStateFormulaTerm
	var payload struct {
		stored
		LegacyFieldPath string `json:"field_path,omitempty"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	*value = RuleStateFormulaTerm(payload.stored)
	if value.FieldID == "" {
		value.FieldID = payload.LegacyFieldPath
	}
	return nil
}
