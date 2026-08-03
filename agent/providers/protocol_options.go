package providers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// EncodeProtocolOptions serializes one adapter-owned compatibility value for a
// provider preset. Keeping the payload opaque prevents the provider registry
// from accumulating protocol-specific fields.
func EncodeProtocolOptions(value any) (json.RawMessage, error) {
	if value == nil {
		return nil, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode protocol options: %w", err)
	}
	return data, nil
}

// DecodeProtocolOptions strictly decodes the selected adapter's options. An
// empty payload is equivalent to an empty object, so adapters own their safe
// defaults without requiring callers to know their schema.
func DecodeProtocolOptions(data json.RawMessage, target any) error {
	if target == nil {
		return fmt.Errorf("decode protocol options: target is required")
	}
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode protocol options: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode protocol options: %w", err)
	} else if err == nil {
		return fmt.Errorf("decode protocol options: unexpected trailing value")
	}
	return nil
}

func mergeProtocolOptions(defaults, overrides json.RawMessage) (json.RawMessage, error) {
	left, err := decodeJSONObject(defaults)
	if err != nil {
		return nil, fmt.Errorf("decode preset protocol options: %w", err)
	}
	right, err := decodeJSONObject(overrides)
	if err != nil {
		return nil, fmt.Errorf("decode model protocol options: %w", err)
	}
	if len(left) == 0 && len(right) == 0 {
		return nil, nil
	}
	merged := mergeJSONObjects(left, right)
	data, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encode merged protocol options: %w", err)
	}
	return data, nil
}

func decodeJSONObject(data json.RawMessage) (map[string]any, error) {
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return map[string]any{}, nil
	}
	return value, nil
}

func mergeJSONObjects(base, override map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(override))
	for key, value := range base {
		result[key] = value
	}
	for key, value := range override {
		if childOverride, ok := value.(map[string]any); ok {
			if childBase, ok := result[key].(map[string]any); ok {
				result[key] = mergeJSONObjects(childBase, childOverride)
				continue
			}
		}
		result[key] = value
	}
	return result
}
