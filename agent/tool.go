package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/invopop/jsonschema"
)

// DataType is a JSON Schema primitive type.
type DataType string

const (
	Object  DataType = "object"
	Number  DataType = "number"
	Integer DataType = "integer"
	String  DataType = "string"
	Array   DataType = "array"
	Null    DataType = "null"
	Boolean DataType = "boolean"
)

// ParameterInfo is the compact form of a tool parameter definition.
type ParameterInfo struct {
	Type      DataType
	ElemInfo  *ParameterInfo
	SubParams map[string]*ParameterInfo
	Desc      string
	Enum      []string
	Required  bool
}

// ParamsOneOf contains either compact parameters or a full JSON Schema.
type ParamsOneOf struct {
	params map[string]*ParameterInfo
	schema *jsonschema.Schema
}

// NewParamsOneOfByParams constructs a compact parameter schema.
func NewParamsOneOfByParams(params map[string]*ParameterInfo) *ParamsOneOf {
	return &ParamsOneOf{params: cloneParameterMap(params)}
}

// NewParamsOneOfByJSONSchema preserves a full schema, including oneOf.
func NewParamsOneOfByJSONSchema(schema *jsonschema.Schema) *ParamsOneOf {
	return &ParamsOneOf{schema: cloneJSONSchema(schema)}
}

// ToJSONSchema returns a provider-visible, inline JSON Schema.
func (params *ParamsOneOf) ToJSONSchema() (*jsonschema.Schema, error) {
	if params == nil {
		return nil, nil
	}
	if params.schema != nil {
		return cloneJSONSchema(params.schema), nil
	}
	properties := make(map[string]any, len(params.params))
	required := make([]string, 0, len(params.params))
	keys := make([]string, 0, len(params.params))
	for key := range params.params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		properties[key] = parameterSchemaMap(params.params[key])
		if params.params[key] != nil && params.params[key].Required {
			required = append(required, key)
		}
	}
	value := map[string]any{
		"type":       string(Object),
		"properties": properties,
	}
	if len(required) != 0 {
		value["required"] = required
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal compact parameter schema: %w", err)
	}
	result := &jsonschema.Schema{}
	if err := json.Unmarshal(data, result); err != nil {
		return nil, fmt.Errorf("decode compact parameter schema: %w", err)
	}
	return result, nil
}

func parameterSchemaMap(parameter *ParameterInfo) map[string]any {
	if parameter == nil {
		return map[string]any{}
	}
	result := map[string]any{"type": string(parameter.Type)}
	if parameter.Desc != "" {
		result["description"] = parameter.Desc
	}
	if len(parameter.Enum) != 0 {
		result["enum"] = append([]string(nil), parameter.Enum...)
	}
	if parameter.ElemInfo != nil {
		result["items"] = parameterSchemaMap(parameter.ElemInfo)
	}
	if len(parameter.SubParams) != 0 {
		properties := make(map[string]any, len(parameter.SubParams))
		required := make([]string, 0, len(parameter.SubParams))
		keys := make([]string, 0, len(parameter.SubParams))
		for key := range parameter.SubParams {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			properties[key] = parameterSchemaMap(parameter.SubParams[key])
			if parameter.SubParams[key] != nil && parameter.SubParams[key].Required {
				required = append(required, key)
			}
		}
		result["properties"] = properties
		if len(required) != 0 {
			result["required"] = required
		}
	}
	return result
}

// ToolInfo is the stable provider-neutral description of a tool.
type ToolInfo struct {
	Name  string
	Desc  string
	Extra map[string]any
	*ParamsOneOf
}

type toolInfoJSON struct {
	Name           string                    `json:"name,omitempty"`
	Desc           string                    `json:"desc,omitempty"`
	Extra          map[string]any            `json:"extra,omitempty"`
	HasParamsOneOf bool                      `json:"has_params_one_of,omitempty"`
	Params         map[string]*ParameterInfo `json:"params,omitempty"`
	JSONSchema     *jsonschema.Schema        `json:"json_schema,omitempty"`
}

// MarshalJSON preserves the stable ToolInfo persistence shape.
func (info *ToolInfo) MarshalJSON() ([]byte, error) {
	if info == nil {
		return []byte("null"), nil
	}
	wire := toolInfoJSON{Name: info.Name, Desc: info.Desc, Extra: info.Extra}
	if info.ParamsOneOf != nil {
		wire.HasParamsOneOf = true
		wire.Params = info.ParamsOneOf.params
		wire.JSONSchema = info.ParamsOneOf.schema
	}
	return json.Marshal(wire)
}

// UnmarshalJSON accepts the stable ToolInfo persistence shape.
func (info *ToolInfo) UnmarshalJSON(data []byte) error {
	wire := toolInfoJSON{}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	info.Name = wire.Name
	info.Desc = wire.Desc
	info.Extra = wire.Extra
	info.ParamsOneOf = nil
	if wire.HasParamsOneOf {
		info.ParamsOneOf = &ParamsOneOf{params: wire.Params, schema: wire.JSONSchema}
		if info.ParamsOneOf.params == nil && info.ParamsOneOf.schema == nil {
			info.ParamsOneOf.params = map[string]*ParameterInfo{}
		}
	}
	return nil
}

// ToolOption is reserved for per-invocation, provider-neutral tool settings.
// Its named type prevents provider SDK options from leaking into this seam.
type ToolOption struct {
	Key   string
	Value any
}

// InvokeFunc is a typed tool implementation.
type InvokeFunc[T, D any] func(ctx context.Context, input T) (D, error)

// SchemaModifierFn maps application-specific struct tags into JSON Schema.
type SchemaModifierFn func(jsonTagName string, fieldType reflect.Type, tag reflect.StructTag, schema *jsonschema.Schema)

// UnmarshalArguments customizes typed argument decoding.
type UnmarshalArguments func(ctx context.Context, arguments string) (any, error)

// MarshalOutput customizes the structured result produced by a typed tool.
type MarshalOutput func(ctx context.Context, output any) (ToolResult, error)

type inferOptions struct {
	schemaModifier     SchemaModifierFn
	unmarshalArguments UnmarshalArguments
	marshalOutput      MarshalOutput
}

// InferOption configures InferTool and GoStruct2 helpers.
type InferOption func(*inferOptions)

// WithSchemaModifier applies a custom modifier after reflection.
func WithSchemaModifier(modifier SchemaModifierFn) InferOption {
	return func(options *inferOptions) {
		options.schemaModifier = modifier
	}
}

// WithUnmarshalArguments opts into application-defined argument decoding.
func WithUnmarshalArguments(unmarshal UnmarshalArguments) InferOption {
	return func(options *inferOptions) {
		options.unmarshalArguments = unmarshal
	}
}

// WithMarshalOutput opts into application-defined result encoding.
func WithMarshalOutput(marshal MarshalOutput) InferOption {
	return func(options *inferOptions) {
		options.marshalOutput = marshal
	}
}

func collectInferOptions(opts []InferOption) *inferOptions {
	options := &inferOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}
	return options
}

// GoStruct2ParamsOneOf reflects T into an inline, provider-visible schema.
func GoStruct2ParamsOneOf[T any](opts ...InferOption) (*ParamsOneOf, error) {
	options := collectInferOptions(opts)
	typeOfT := reflect.TypeFor[T]()
	reflector := &jsonschema.Reflector{Anonymous: true, DoNotReference: true}
	schema := reflector.ReflectFromType(typeOfT)
	if schema == nil {
		return nil, fmt.Errorf("reflect tool schema for %v: empty schema", typeOfT)
	}
	schema.Version = ""
	schema.ID = ""
	schema.Ref = ""
	schema.Definitions = nil
	if options.schemaModifier != nil {
		applySchemaModifier(typeOfT, reflect.StructTag(""), "_root", schema, options.schemaModifier)
	}
	return NewParamsOneOfByJSONSchema(schema), nil
}

// GoStruct2ToolInfo reflects T and attaches the supplied stable tool identity.
func GoStruct2ToolInfo[T any](name, description string, opts ...InferOption) (*ToolInfo, error) {
	params, err := GoStruct2ParamsOneOf[T](opts...)
	if err != nil {
		return nil, err
	}
	return &ToolInfo{Name: name, Desc: description, ParamsOneOf: params}, nil
}

// InferTool creates a strict typed tool.
func InferTool[T, D any](name, description string, invoke InvokeFunc[T, D], opts ...InferOption) (Tool, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("infer tool: name is required")
	}
	if invoke == nil {
		return nil, fmt.Errorf("infer tool %q: invoke function is required", name)
	}
	info, err := GoStruct2ToolInfo[T](name, description, opts...)
	if err != nil {
		return nil, fmt.Errorf("infer tool %q: %w", name, err)
	}
	schema, err := info.ToJSONSchema()
	if err != nil {
		return nil, fmt.Errorf("infer tool %q schema: %w", name, err)
	}
	return &inferredTool[T, D]{
		info:    info,
		schema:  schema,
		invoke:  invoke,
		options: collectInferOptions(opts),
	}, nil
}

// NewTool binds a typed function to an explicit ToolInfo.
func NewTool[T, D any](info *ToolInfo, invoke InvokeFunc[T, D], opts ...InferOption) Tool {
	var schema *jsonschema.Schema
	if info != nil {
		schema, _ = info.ToJSONSchema()
	}
	return &inferredTool[T, D]{info: cloneToolInfo(info), schema: schema, invoke: invoke, options: collectInferOptions(opts)}
}

type inferredTool[T, D any] struct {
	info    *ToolInfo
	schema  *jsonschema.Schema
	invoke  InvokeFunc[T, D]
	options *inferOptions
}

func (tool *inferredTool[T, D]) Info(context.Context) (*ToolInfo, error) {
	return cloneToolInfo(tool.info), nil
}

func (tool *inferredTool[T, D]) Run(ctx context.Context, arguments string, _ ...ToolOption) (ToolResult, error) {
	var input T
	if tool.options != nil && tool.options.unmarshalArguments != nil {
		decoded, err := tool.options.unmarshalArguments(ctx, arguments)
		if err != nil {
			return ToolResult{}, fmt.Errorf("decode arguments for tool %q: %w", toolName(tool.info), err)
		}
		value, ok := decoded.(T)
		if !ok {
			return ToolResult{}, fmt.Errorf("decode arguments for tool %q: got %T, want %T", toolName(tool.info), decoded, input)
		}
		input = value
	} else if err := strictDecodeArguments(arguments, &input, tool.schema); err != nil {
		return ToolResult{}, fmt.Errorf("decode arguments for tool %q: %w", toolName(tool.info), err)
	}

	output, err := tool.invoke(ctx, input)
	if err != nil {
		return ToolResult{}, fmt.Errorf("invoke tool %q: %w", toolName(tool.info), err)
	}
	if tool.options != nil && tool.options.marshalOutput != nil {
		result, err := tool.options.marshalOutput(ctx, output)
		if err != nil {
			return ToolResult{}, fmt.Errorf("encode result for tool %q: %w", toolName(tool.info), err)
		}
		return result, nil
	}
	if value, ok := any(output).(ToolResult); ok {
		return value, nil
	}
	if value, ok := any(output).(*ToolResult); ok {
		if value == nil {
			return ToolResult{}, fmt.Errorf("encode result for tool %q: nil ToolResult", toolName(tool.info))
		}
		return *value, nil
	}
	if value, ok := any(output).(string); ok {
		return TextToolResult(value), nil
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return ToolResult{}, fmt.Errorf("encode result for tool %q: %w", toolName(tool.info), err)
	}
	return TextToolResult(string(encoded)), nil
}

// ValidateToolArguments checks raw arguments against the exact schema exposed
// to the provider. It is repeated immediately before Tool.Run, so middleware
// cannot bypass validation by rewriting arguments.
func ValidateToolArguments(info *ToolInfo, arguments string) error {
	if info == nil {
		return errors.New("tool info is nil")
	}
	schema, err := info.ToJSONSchema()
	if err != nil {
		return err
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}
	if err := validateJSONValue("$", value, schema); err != nil {
		return err
	}
	return nil
}

// validateToolSchema rejects malformed schemas at the registry boundary,
// before a provider can see a tool that the local final-argument validator
// cannot interpret consistently.
func validateToolSchema(schema *jsonschema.Schema) error {
	if schema == nil {
		return nil
	}
	if _, err := json.Marshal(schema); err != nil {
		return fmt.Errorf("encode JSON schema: %w", err)
	}
	return validateToolSchemaAt("$", schema)
}

func validateToolSchemaAt(path string, schema *jsonschema.Schema) error {
	if schema == nil {
		return fmt.Errorf("%s contains a nil schema", path)
	}
	switch schema.Type {
	case "", "object", "array", "string", "boolean", "number", "integer", "null":
	default:
		return fmt.Errorf("%s has unsupported type %q", path, schema.Type)
	}
	if schema.Pattern != "" {
		if _, err := regexp.Compile(schema.Pattern); err != nil {
			return fmt.Errorf("%s has invalid pattern: %w", path, err)
		}
	}
	if schema.MinLength != nil && schema.MaxLength != nil && *schema.MinLength > *schema.MaxLength {
		return fmt.Errorf("%s has minLength greater than maxLength", path)
	}
	if schema.MinItems != nil && schema.MaxItems != nil && *schema.MinItems > *schema.MaxItems {
		return fmt.Errorf("%s has minItems greater than maxItems", path)
	}
	if schema.MinProperties != nil && schema.MaxProperties != nil && *schema.MinProperties > *schema.MaxProperties {
		return fmt.Errorf("%s has minProperties greater than maxProperties", path)
	}
	children := []struct {
		name   string
		values []*jsonschema.Schema
	}{
		{name: "allOf", values: schema.AllOf},
		{name: "anyOf", values: schema.AnyOf},
		{name: "oneOf", values: schema.OneOf},
		{name: "prefixItems", values: schema.PrefixItems},
	}
	for _, group := range children {
		for index, child := range group.values {
			if err := validateToolSchemaAt(fmt.Sprintf("%s.%s[%d]", path, group.name, index), child); err != nil {
				return err
			}
		}
	}
	single := []struct {
		name  string
		value *jsonschema.Schema
	}{
		{name: "not", value: schema.Not}, {name: "if", value: schema.If},
		{name: "then", value: schema.Then}, {name: "else", value: schema.Else},
		{name: "items", value: schema.Items}, {name: "contains", value: schema.Contains},
		{name: "additionalProperties", value: schema.AdditionalProperties},
		{name: "propertyNames", value: schema.PropertyNames},
		{name: "contentSchema", value: schema.ContentSchema},
	}
	for _, child := range single {
		if child.value != nil {
			if err := validateToolSchemaAt(path+"."+child.name, child.value); err != nil {
				return err
			}
		}
	}
	if schema.Properties != nil {
		for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
			if err := validateToolSchemaAt(path+".properties."+pair.Key, pair.Value); err != nil {
				return err
			}
		}
	}
	maps := []struct {
		name   string
		values map[string]*jsonschema.Schema
	}{
		{name: "$defs", values: schema.Definitions},
		{name: "dependentSchemas", values: schema.DependentSchemas},
		{name: "patternProperties", values: schema.PatternProperties},
	}
	for _, group := range maps {
		for name, child := range group.values {
			if err := validateToolSchemaAt(path+"."+group.name+"."+name, child); err != nil {
				return err
			}
		}
	}
	return nil
}

func toolName(info *ToolInfo) string {
	if info == nil {
		return ""
	}
	return info.Name
}

func strictDecodeArguments(arguments string, destination any, schema *jsonschema.Schema) error {
	if strings.TrimSpace(arguments) == "" {
		return errors.New("empty JSON input")
	}
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := requireJSONEOF(decoder); err != nil {
		return err
	}

	rawDecoder := json.NewDecoder(strings.NewReader(arguments))
	rawDecoder.UseNumber()
	var raw any
	if err := rawDecoder.Decode(&raw); err != nil {
		return err
	}
	if err := validateJSONValue("$", raw, schema); err != nil {
		return err
	}
	return nil
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
		if schema.Items != nil {
			for index, child := range typed {
				if err := validateJSONValue(fmt.Sprintf("%s[%d]", path, index), child, schema.Items); err != nil {
					return err
				}
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

// jsonschema represents boolean schemas through an internal field. Marshaling
// is the package-supported way to preserve that representation across cloned
// schemas, so argument validation recognizes both `true` and `false` here.
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
		if jsonValuesEqual(value, candidate) {
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

func applySchemaModifier(t reflect.Type, tag reflect.StructTag, name string, schema *jsonschema.Schema, modifier SchemaModifierFn) {
	if schema == nil || t == nil {
		return
	}
	modifier(name, t, tag, schema)
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		applySchemaModifier(t.Elem(), tag, name, schema.Items, modifier)
		return
	}
	if t.Kind() != reflect.Struct || schema.Properties == nil {
		return
	}
	for index := 0; index < t.NumField(); index++ {
		field := t.Field(index)
		if field.PkgPath != "" {
			continue
		}
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "-" {
			continue
		}
		if jsonName == "" {
			jsonName = field.Name
		}
		property, exists := schema.Properties.Get(jsonName)
		if !exists {
			continue
		}
		applySchemaModifier(field.Type, field.Tag, jsonName, property, modifier)
	}
}

func cloneToolInfos(values []*ToolInfo) []*ToolInfo {
	if values == nil {
		return nil
	}
	result := make([]*ToolInfo, len(values))
	for index, value := range values {
		result[index] = cloneToolInfo(value)
	}
	return result
}

func cloneToolInfo(info *ToolInfo) *ToolInfo {
	if info == nil {
		return nil
	}
	clone := *info
	clone.Extra = cloneStringAnyMap(info.Extra)
	if info.ParamsOneOf != nil {
		clone.ParamsOneOf = &ParamsOneOf{
			params: cloneParameterMap(info.ParamsOneOf.params),
			schema: cloneJSONSchema(info.ParamsOneOf.schema),
		}
	}
	return &clone
}

func cloneParameterMap(values map[string]*ParameterInfo) map[string]*ParameterInfo {
	if values == nil {
		return nil
	}
	result := make(map[string]*ParameterInfo, len(values))
	for key, value := range values {
		result[key] = cloneParameterInfo(value)
	}
	return result
}

func cloneParameterInfo(info *ParameterInfo) *ParameterInfo {
	if info == nil {
		return nil
	}
	clone := *info
	clone.Enum = append([]string(nil), info.Enum...)
	clone.ElemInfo = cloneParameterInfo(info.ElemInfo)
	clone.SubParams = cloneParameterMap(info.SubParams)
	return &clone
}

func cloneJSONSchema(schema *jsonschema.Schema) *jsonschema.Schema {
	if schema == nil {
		return nil
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return schema
	}
	clone := &jsonschema.Schema{}
	if err := json.Unmarshal(data, clone); err != nil {
		return schema
	}
	return clone
}

type toolCallContextKey struct{}

type toolCallContext struct {
	providerCallID string
	executionID    string
	name           string
}

// ContextWithToolCall records provider transcript metadata for direct tool
// callers. Native Agent execution additionally binds a durable execution ID.
func ContextWithToolCall(ctx context.Context, callID, name string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, toolCallContextKey{}, toolCallContext{providerCallID: callID, name: name})
}

func contextWithToolExecution(ctx context.Context, executionID, providerCallID, name string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, toolCallContextKey{}, toolCallContext{
		executionID: executionID, providerCallID: providerCallID, name: name,
	})
}

// ToolCallID returns the provider transcript call ID. Durable lifecycle and
// host correlation must use CurrentToolExecutionID instead.
func ToolCallID(ctx context.Context) string {
	metadata, _ := toolCallMetadata(ctx)
	return metadata.providerCallID
}

// ToolName returns the current tool name, or an empty string outside a call.
func ToolName(ctx context.Context) string {
	metadata, _ := toolCallMetadata(ctx)
	return metadata.name
}

func toolCallMetadata(ctx context.Context) (toolCallContext, bool) {
	if ctx == nil {
		return toolCallContext{}, false
	}
	metadata, ok := ctx.Value(toolCallContextKey{}).(toolCallContext)
	return metadata, ok
}
