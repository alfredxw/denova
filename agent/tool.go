package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"

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
	normalizedArguments, err := normalizeToolArgumentsWithSchema(arguments, tool.schema)
	if err != nil {
		return ToolResult{}, fmt.Errorf("normalize arguments for tool %q: %w", toolName(tool.info), err)
	}
	if tool.options != nil && tool.options.unmarshalArguments != nil {
		decoded, err := tool.options.unmarshalArguments(ctx, normalizedArguments)
		if err != nil {
			return ToolResult{}, fmt.Errorf("decode arguments for tool %q: %w", toolName(tool.info), err)
		}
		value, ok := decoded.(T)
		if !ok {
			return ToolResult{}, fmt.Errorf("decode arguments for tool %q: got %T, want %T", toolName(tool.info), decoded, input)
		}
		input = value
	} else if err := json.Unmarshal([]byte(normalizedArguments), &input); err != nil {
		return ToolResult{}, fmt.Errorf("decode arguments for tool %q: %w", toolName(tool.info), err)
	}

	output, err := tool.invoke(ctx, input)
	if err != nil {
		wrapped := fmt.Errorf("invoke tool %q: %w", toolName(tool.info), err)
		// A tool can commit its domain effect and then fail while reporting or
		// finalizing it. Preserve a structured terminal receipt for lifecycle
		// middleware instead of replacing it with an empty result.
		if value, ok := any(output).(ToolResult); ok {
			return value, wrapped
		}
		if value, ok := any(output).(*ToolResult); ok && value != nil {
			return *value, wrapped
		}
		return ToolResult{}, wrapped
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
