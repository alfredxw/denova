package agent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/invopop/jsonschema"
)

// ToolArgumentIssue is one machine-readable reason a tool call could not be
// normalized safely. The model can correct these issues in its next turn.
type ToolArgumentIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ToolArgumentsError identifies an invalid_arguments outcome without turning
// recoverable model input mistakes into runtime failures.
type ToolArgumentsError struct {
	Issues []ToolArgumentIssue `json:"issues"`
}

func (e *ToolArgumentsError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "invalid tool arguments"
	}
	issue := e.Issues[0]
	return fmt.Sprintf("%s: %s", issue.Path, issue.Message)
}

func toolArgumentError(code, path, format string, values ...any) error {
	return &ToolArgumentsError{Issues: []ToolArgumentIssue{{
		Code: code, Path: path, Message: fmt.Sprintf(format, values...),
	}}}
}

// NormalizeToolArguments applies only stable, unambiguous schema corrections
// and returns canonical JSON. Domain-specific validation remains owned by the
// concrete tool implementation.
func NormalizeToolArguments(info *ToolInfo, arguments string) (string, error) {
	if info == nil {
		return "", toolArgumentError("schema_unavailable", "$", "tool info is nil")
	}
	schema, err := info.ToJSONSchema()
	if err != nil {
		return "", toolArgumentError("schema_unavailable", "$", "%v", err)
	}
	return normalizeToolArgumentsWithSchema(arguments, schema)
}

func normalizeToolArgumentsWithSchema(arguments string, schema *jsonschema.Schema) (string, error) {
	if strings.TrimSpace(arguments) == "" {
		return "", toolArgumentError("invalid_json", "$", "empty JSON input")
	}
	value, err := decodeJSONValue(arguments)
	if err != nil {
		return "", toolArgumentError("invalid_json", "$", "%v", err)
	}
	normalized, err := normalizeJSONValue("$", value, schema)
	if err != nil {
		return "", err
	}
	if err := validateJSONValue("$", normalized, schema); err != nil {
		return "", toolArgumentError("constraint_violation", "$", "%v", err)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", toolArgumentError("invalid_json", "$", "encode normalized arguments: %v", err)
	}
	return string(encoded), nil
}

func decodeJSONValue(arguments string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return nil, err
	}
	return value, nil
}

func normalizeJSONValue(path string, value any, schema *jsonschema.Schema) (any, error) {
	if schema == nil {
		return cloneJSONValue(value), nil
	}
	if allowed, boolean := jsonSchemaBoolean(schema); boolean {
		if allowed {
			return cloneJSONValue(value), nil
		}
		return nil, toolArgumentError("schema_rejected", path, "value is not allowed by schema")
	}

	current := cloneJSONValue(value)
	for _, candidate := range schema.AllOf {
		normalized, err := normalizeJSONValue(path, current, candidate)
		if err != nil {
			return nil, err
		}
		current = normalized
	}
	if len(schema.OneOf) != 0 {
		normalized, err := normalizeSchemaBranch(path, current, schema.OneOf, true)
		if err != nil {
			return nil, err
		}
		current = normalized
	}
	if len(schema.AnyOf) != 0 {
		normalized, err := normalizeSchemaBranch(path, current, schema.AnyOf, false)
		if err != nil {
			return nil, err
		}
		current = normalized
	}

	coerced, err := coerceJSONPrimitive(path, current, schema)
	if err != nil {
		return nil, err
	}
	current = coerced

	switch typed := current.(type) {
	case map[string]any:
		return normalizeJSONObject(path, typed, schema)
	case []any:
		if schema.Type != "" && schema.Type != "array" {
			return nil, toolArgumentError("type_conflict", path, "has type array, want %s", schema.Type)
		}
		result := make([]any, len(typed))
		for index, item := range typed {
			itemSchema := schema.Items
			if index < len(schema.PrefixItems) {
				itemSchema = schema.PrefixItems[index]
			}
			normalized, err := normalizeJSONValue(fmt.Sprintf("%s[%d]", path, index), item, itemSchema)
			if err != nil {
				return nil, err
			}
			result[index] = normalized
		}
		return result, nil
	default:
		if schema.Type != "" && !matchesJSONType(current, schema.Type) {
			return nil, toolArgumentError("type_conflict", path, "has type %T, want %s", current, schema.Type)
		}
		return current, nil
	}
}

func normalizeSchemaBranch(path string, value any, candidates []*jsonschema.Schema, exactlyOne bool) (any, error) {
	type branchMatch struct {
		value any
	}
	matches := make([]branchMatch, 0, len(candidates))
	for _, candidate := range candidates {
		normalized, err := normalizeJSONValue(path, cloneJSONValue(value), candidate)
		if err != nil || validateJSONValue(path, normalized, candidate) != nil {
			continue
		}
		matches = append(matches, branchMatch{value: normalized})
	}
	if len(matches) == 0 {
		return nil, toolArgumentError("branch_no_match", path, "does not match any allowed schema branch")
	}
	if exactlyOne && len(matches) != 1 {
		return nil, toolArgumentError("branch_ambiguity", path, "matches %d oneOf branches", len(matches))
	}
	if len(matches) == 1 {
		return matches[0].value, nil
	}
	first := matches[0].value
	for _, match := range matches[1:] {
		if !jsonValuesEqual(first, match.value) {
			// anyOf does not require a branch choice. Keep an already-valid value
			// unchanged; otherwise normalization would have to guess semantics.
			for _, candidate := range candidates {
				if validateJSONValue(path, value, candidate) == nil {
					return cloneJSONValue(value), nil
				}
			}
			return nil, toolArgumentError("branch_ambiguity", path, "normalization differs across matching anyOf branches")
		}
	}
	return first, nil
}

func coerceJSONPrimitive(path string, value any, schema *jsonschema.Schema) (any, error) {
	if schema == nil || schema.Type == "" || len(schema.Enum) != 0 || schema.Const != nil {
		return value, nil
	}
	if number, ok := value.(json.Number); ok && schema.Type == "integer" {
		parsed, valid := new(big.Rat).SetString(number.String())
		if !valid || parsed.Denom().Cmp(big.NewInt(1)) != 0 {
			return nil, toolArgumentError("type_conflict", path, "number %q is not an integer", number)
		}
		return json.Number(parsed.Num().String()), nil
	}
	text, ok := value.(string)
	if !ok {
		return value, nil
	}
	trimmed := strings.TrimSpace(text)
	switch schema.Type {
	case "boolean":
		switch trimmed {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	case "number", "integer":
		if !validJSONNumber(trimmed) {
			break
		}
		parsed, valid := new(big.Rat).SetString(trimmed)
		if !valid {
			break
		}
		if schema.Type == "integer" {
			if parsed.Denom().Cmp(big.NewInt(1)) != 0 {
				break
			}
			return json.Number(parsed.Num().String()), nil
		}
		return json.Number(trimmed), nil
	}
	return value, nil
}

func normalizeJSONObject(path string, value map[string]any, schema *jsonschema.Schema) (map[string]any, error) {
	if schema.Type != "" && schema.Type != "object" {
		return nil, toolArgumentError("type_conflict", path, "has type object, want %s", schema.Type)
	}
	result := make(map[string]any, len(value))
	recognizedInputs := 0
	droppedInputs := make([]string, 0)
	for key, child := range value {
		property, matched := schemaProperty(schema, key)
		if !matched && schema.AdditionalProperties != nil {
			if allowed, boolean := jsonSchemaBoolean(schema.AdditionalProperties); boolean && !allowed {
				droppedInputs = append(droppedInputs, key)
				continue
			}
			property = schema.AdditionalProperties
			matched = true
		}
		if !matched {
			result[key] = cloneJSONValue(child)
			continue
		}
		recognizedInputs++
		normalized, err := normalizeJSONValue(path+"."+key, child, property)
		if err != nil {
			return nil, err
		}
		result[key] = normalized
	}
	if len(value) > 0 && recognizedInputs == 0 && len(droppedInputs) > 0 {
		sort.Strings(droppedInputs)
		return nil, toolArgumentError("unsupported_arguments", path, "no supported properties were provided; unsupported: %s", strings.Join(droppedInputs, ", "))
	}
	if schema.Properties != nil {
		for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
			if _, exists := result[pair.Key]; exists || pair.Value == nil || pair.Value.Default == nil {
				continue
			}
			defaultValue, err := canonicalJSONValue(pair.Value.Default)
			if err != nil {
				return nil, toolArgumentError("invalid_default", path+"."+pair.Key, "%v", err)
			}
			normalized, err := normalizeJSONValue(path+"."+pair.Key, defaultValue, pair.Value)
			if err != nil {
				return nil, toolArgumentError("invalid_default", path+"."+pair.Key, "%v", err)
			}
			result[pair.Key] = normalized
		}
	}
	for _, required := range schema.Required {
		if _, exists := result[required]; !exists {
			return nil, toolArgumentError("missing_required", path+"."+required, "required property is missing")
		}
	}
	return result, nil
}

func schemaProperty(schema *jsonschema.Schema, key string) (*jsonschema.Schema, bool) {
	if schema.Properties != nil {
		if property, exists := schema.Properties.Get(key); exists {
			return property, true
		}
	}
	for pattern, property := range schema.PatternProperties {
		compiled, err := regexp.Compile(pattern)
		if err == nil && compiled.MatchString(key) {
			return property, true
		}
	}
	return nil, false
}

func canonicalJSONValue(value any) (any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return decodeJSONValue(string(encoded))
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			result[key] = cloneJSONValue(child)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = cloneJSONValue(child)
		}
		return result
	default:
		return typed
	}
}

func validJSONNumber(value string) bool {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var number json.Number
	return decoder.Decode(&number) == nil && requireJSONEOF(decoder) == nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values are not allowed")
	}
	return fmt.Errorf("invalid trailing JSON: %w", err)
}

func validateJSONValue(path string, value any, schema *jsonschema.Schema) error {
	if schema == nil {
		return nil
	}
	if allowed, boolean := jsonSchemaBoolean(schema); boolean {
		if allowed {
			return nil
		}
		return fmt.Errorf("%s is not allowed by schema", path)
	}
	for _, candidate := range schema.AllOf {
		if err := validateJSONValue(path, value, candidate); err != nil {
			return err
		}
	}
	if len(schema.OneOf) != 0 {
		matches := 0
		for _, candidate := range schema.OneOf {
			if validateJSONValue(path, value, candidate) == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("%s must match exactly one schema (matched %d)", path, matches)
		}
	}
	if len(schema.AnyOf) != 0 {
		matches := false
		for _, candidate := range schema.AnyOf {
			if validateJSONValue(path, value, candidate) == nil {
				matches = true
				break
			}
		}
		if !matches {
			return fmt.Errorf("%s does not match any allowed schema", path)
		}
	}
	if len(schema.Enum) != 0 && !jsonValueIn(value, schema.Enum) {
		return fmt.Errorf("%s is not one of the allowed enum values", path)
	}
	if schema.Const != nil && !jsonValuesEqual(value, schema.Const) {
		return fmt.Errorf("%s does not equal the required constant", path)
	}
	if schema.Type != "" && !matchesJSONType(value, schema.Type) {
		return fmt.Errorf("%s has type %T, want %s", path, value, schema.Type)
	}
	switch typed := value.(type) {
	case map[string]any:
		if schema.MinProperties != nil && uint64(len(typed)) < *schema.MinProperties {
			return fmt.Errorf("%s has %d properties, want at least %d", path, len(typed), *schema.MinProperties)
		}
		if schema.MaxProperties != nil && uint64(len(typed)) > *schema.MaxProperties {
			return fmt.Errorf("%s has %d properties, want at most %d", path, len(typed), *schema.MaxProperties)
		}
		for _, required := range schema.Required {
			if _, exists := typed[required]; !exists {
				return fmt.Errorf("%s.%s is required", path, required)
			}
		}
		for key, child := range typed {
			matched := false
			if schema.Properties != nil {
				if property, exists := schema.Properties.Get(key); exists {
					matched = true
					if err := validateJSONValue(path+"."+key, child, property); err != nil {
						return err
					}
				}
			}
			for pattern, property := range schema.PatternProperties {
				compiled, err := regexp.Compile(pattern)
				if err != nil {
					return fmt.Errorf("%s has invalid property pattern %q: %w", path, pattern, err)
				}
				if compiled.MatchString(key) {
					matched = true
					if err := validateJSONValue(path+"."+key, child, property); err != nil {
						return err
					}
				}
			}
			if !matched && schema.AdditionalProperties != nil {
				if err := validateJSONValue(path+"."+key, child, schema.AdditionalProperties); err != nil {
					return fmt.Errorf("%s contains unsupported property %q: %w", path, key, err)
				}
			}
		}
	case []any:
		if schema.MinItems != nil && uint64(len(typed)) < *schema.MinItems {
			return fmt.Errorf("%s has %d items, want at least %d", path, len(typed), *schema.MinItems)
		}
		if schema.MaxItems != nil && uint64(len(typed)) > *schema.MaxItems {
			return fmt.Errorf("%s has %d items, want at most %d", path, len(typed), *schema.MaxItems)
		}
		for index, child := range typed {
			itemSchema := schema.Items
			if index < len(schema.PrefixItems) {
				itemSchema = schema.PrefixItems[index]
			}
			if err := validateJSONValue(fmt.Sprintf("%s[%d]", path, index), child, itemSchema); err != nil {
				return err
			}
		}
	case string:
		length := uint64(utf8.RuneCountInString(typed))
		if schema.MinLength != nil && length < *schema.MinLength {
			return fmt.Errorf("%s has length %d, want at least %d", path, length, *schema.MinLength)
		}
		if schema.MaxLength != nil && length > *schema.MaxLength {
			return fmt.Errorf("%s has length %d, want at most %d", path, length, *schema.MaxLength)
		}
		if schema.Pattern != "" {
			pattern, err := regexp.Compile(schema.Pattern)
			if err != nil {
				return fmt.Errorf("%s has invalid string pattern %q: %w", path, schema.Pattern, err)
			}
			if !pattern.MatchString(typed) {
				return fmt.Errorf("%s does not match required pattern %q", path, schema.Pattern)
			}
		}
	case json.Number:
		if err := validateJSONNumber(path, typed, schema); err != nil {
			return err
		}
	}
	return nil
}

func validateJSONNumber(path string, value json.Number, schema *jsonschema.Schema) error {
	number, ok := new(big.Rat).SetString(value.String())
	if !ok {
		return fmt.Errorf("%s contains invalid JSON number %q", path, value)
	}
	constraints := []struct {
		name      string
		value     json.Number
		acceptCmp func(int) bool
	}{
		{name: "minimum", value: schema.Minimum, acceptCmp: func(cmp int) bool { return cmp >= 0 }},
		{name: "exclusive minimum", value: schema.ExclusiveMinimum, acceptCmp: func(cmp int) bool { return cmp > 0 }},
		{name: "maximum", value: schema.Maximum, acceptCmp: func(cmp int) bool { return cmp <= 0 }},
		{name: "exclusive maximum", value: schema.ExclusiveMaximum, acceptCmp: func(cmp int) bool { return cmp < 0 }},
	}
	for _, constraint := range constraints {
		if constraint.value == "" {
			continue
		}
		boundary, valid := new(big.Rat).SetString(constraint.value.String())
		if !valid {
			return fmt.Errorf("%s schema has invalid %s %q", path, constraint.name, constraint.value)
		}
		if !constraint.acceptCmp(number.Cmp(boundary)) {
			return fmt.Errorf("%s value %s violates %s %s", path, value, constraint.name, constraint.value)
		}
	}
	if schema.MultipleOf != "" {
		multiple, valid := new(big.Rat).SetString(schema.MultipleOf.String())
		if !valid || multiple.Sign() <= 0 {
			return fmt.Errorf("%s schema has invalid multipleOf %q", path, schema.MultipleOf)
		}
		quotient := new(big.Rat).Quo(number, multiple)
		if quotient.Denom().Cmp(big.NewInt(1)) != 0 {
			return fmt.Errorf("%s value %s is not a multiple of %s", path, value, schema.MultipleOf)
		}
	}
	return nil
}

func jsonSchemaBoolean(schema *jsonschema.Schema) (bool, bool) {
	encoded, err := json.Marshal(schema)
	if err != nil {
		return false, false
	}
	switch string(encoded) {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func matchesJSONType(value any, expected string) bool {
	switch expected {
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "string":
		_, ok := value.(string)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "number":
		_, ok := value.(json.Number)
		return ok
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return false
		}
		parsed, valid := new(big.Rat).SetString(number.String())
		return valid && parsed.Denom().Cmp(big.NewInt(1)) == 0
	case "null":
		return value == nil
	default:
		return true
	}
}

func jsonValueIn(value any, candidates []any) bool {
	for _, candidate := range candidates {
		canonical, err := canonicalJSONValue(candidate)
		if err == nil && jsonValuesEqual(value, canonical) {
			return true
		}
	}
	return false
}

func jsonValuesEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}
