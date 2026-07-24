package agents

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// canonicalContextStructuralMutation preserves JSON numbers without float64
// loss while normalizing insignificant whitespace and object-key order.
func canonicalContextStructuralMutation(mutation json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(mutation)) == 0 {
		return nil, fmt.Errorf("structural context mutation is required")
	}
	if err := rejectDuplicateContextStructuralJSONKeys(mutation); err != nil {
		return nil, fmt.Errorf("decode structural context mutation: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(mutation))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode structural context mutation: %w", err)
	}
	if err := requireContextStructuralJSONEOF(decoder); err != nil {
		return nil, fmt.Errorf("decode structural context mutation: %w", err)
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, fmt.Errorf("structural context mutation must be a JSON object")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode structural context mutation: %w", err)
	}
	return canonical, nil
}

func decodeStrictContextStructuralJSON(data []byte, target any) error {
	if err := rejectDuplicateContextStructuralJSONKeys(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return requireContextStructuralJSONEOF(decoder)
}

// encoding/json normally accepts duplicate object keys with last-value-wins
// semantics. Durable authorization descriptors reject that ambiguity at every
// nesting level before either decoding or hashing.
func rejectDuplicateContextStructuralJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumeContextStructuralJSONValue(decoder); err != nil {
		return err
	}
	return requireContextStructuralJSONEOF(decoder)
}

func consumeContextStructuralJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, composite := token.(json.Delim)
	if !composite {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("JSON object key is not a string")
			}
			if _, duplicate := keys[key]; duplicate {
				return fmt.Errorf("duplicate JSON object field %q", key)
			}
			keys[key] = struct{}{}
			if err := consumeContextStructuralJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("JSON object is not closed")
		}
		return nil
	case '[':
		for decoder.More() {
			if err := consumeContextStructuralJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("JSON array is not closed")
		}
		return nil
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}

func requireContextStructuralJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}
