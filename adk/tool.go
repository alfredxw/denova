package adk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
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

// BaseTool supplies a name, description, and argument schema.
type BaseTool interface {
	Info(ctx context.Context) (*ToolInfo, error)
}

// InvokableTool executes a complete tool call.
type InvokableTool interface {
	BaseTool
	InvokableRun(ctx context.Context, argumentsInJSON string, opts ...ToolOption) (string, error)
}

// StreamableTool executes a tool call as a stream of string fragments.
type StreamableTool interface {
	BaseTool
	StreamableRun(ctx context.Context, argumentsInJSON string, opts ...ToolOption) (*StreamReader[string], error)
}

// InvokeFunc is a typed tool implementation.
type InvokeFunc[T, D any] func(ctx context.Context, input T) (D, error)

// SchemaModifierFn maps application-specific struct tags into JSON Schema.
type SchemaModifierFn func(jsonTagName string, fieldType reflect.Type, tag reflect.StructTag, schema *jsonschema.Schema)

// UnmarshalArguments customizes typed argument decoding.
type UnmarshalArguments func(ctx context.Context, arguments string) (any, error)

// MarshalOutput customizes typed result encoding.
type MarshalOutput func(ctx context.Context, output any) (string, error)

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

// InferTool creates a strict typed invokable tool.
func InferTool[T, D any](name, description string, invoke InvokeFunc[T, D], opts ...InferOption) (InvokableTool, error) {
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
func NewTool[T, D any](info *ToolInfo, invoke InvokeFunc[T, D], opts ...InferOption) InvokableTool {
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

func (tool *inferredTool[T, D]) InvokableRun(ctx context.Context, arguments string, _ ...ToolOption) (string, error) {
	var input T
	if tool.options != nil && tool.options.unmarshalArguments != nil {
		decoded, err := tool.options.unmarshalArguments(ctx, arguments)
		if err != nil {
			return "", fmt.Errorf("decode arguments for tool %q: %w", toolName(tool.info), err)
		}
		value, ok := decoded.(T)
		if !ok {
			return "", fmt.Errorf("decode arguments for tool %q: got %T, want %T", toolName(tool.info), decoded, input)
		}
		input = value
	} else if err := strictDecodeArguments(arguments, &input, tool.schema); err != nil {
		return "", fmt.Errorf("decode arguments for tool %q: %w", toolName(tool.info), err)
	}

	output, err := tool.invoke(ctx, input)
	if err != nil {
		return "", fmt.Errorf("invoke tool %q: %w", toolName(tool.info), err)
	}
	if tool.options != nil && tool.options.marshalOutput != nil {
		result, err := tool.options.marshalOutput(ctx, output)
		if err != nil {
			return "", fmt.Errorf("encode result for tool %q: %w", toolName(tool.info), err)
		}
		return result, nil
	}
	if value, ok := any(output).(string); ok {
		return value, nil
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("encode result for tool %q: %w", toolName(tool.info), err)
	}
	return string(encoded), nil
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
		if schema.Properties != nil {
			for key, child := range typed {
				property, exists := schema.Properties.Get(key)
				if exists {
					if err := validateJSONValue(path+"."+key, child, property); err != nil {
						return err
					}
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
	}
	return nil
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
		return ok && !strings.ContainsAny(number.String(), ".eE")
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
	id   string
	name string
}

// ContextWithToolCall records stable call metadata for tools and middleware.
func ContextWithToolCall(ctx context.Context, callID, name string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, toolCallContextKey{}, toolCallContext{id: callID, name: name})
}

// ToolCallID returns the current tool call ID, or an empty string outside a call.
func ToolCallID(ctx context.Context) string {
	metadata, _ := toolCallMetadata(ctx)
	return metadata.id
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
