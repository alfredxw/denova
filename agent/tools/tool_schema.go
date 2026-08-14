package tools

import (
	"fmt"

	agent "github.com/alfredxw/denova/agent"
	"github.com/invopop/jsonschema"
)

type toolSchemaFactory func() (*jsonschema.Schema, error)

func reflectedToolSchema[T any]() (*jsonschema.Schema, error) {
	params, err := agent.GoStruct2ParamsOneOf[T]()
	if err != nil {
		return nil, err
	}
	return params.ToJSONSchema()
}

func toolSchemaFor[T any]() toolSchemaFactory {
	return func() (*jsonschema.Schema, error) { return reflectedToolSchema[T]() }
}

func newSchemaTool[T, D any](name, description string, schema *jsonschema.Schema, invoke agent.InvokeFunc[T, D]) (agent.Tool, error) {
	if schema == nil {
		return nil, fmt.Errorf("build %s tool: schema is nil", name)
	}
	info := &agent.ToolInfo{
		Name: name, Desc: description, ParamsOneOf: agent.NewParamsOneOfByJSONSchema(schema),
	}
	return agent.NewTool(info, invoke), nil
}

func newUnionTool[T, D any](name, description string, invoke agent.InvokeFunc[T, D], factories ...toolSchemaFactory) (agent.Tool, error) {
	variants := make([]*jsonschema.Schema, 0, len(factories))
	for index, factory := range factories {
		if factory == nil {
			return nil, fmt.Errorf("build %s tool variant %d: schema factory is nil", name, index)
		}
		variant, err := factory()
		if err != nil {
			return nil, fmt.Errorf("build %s tool variant %d: %w", name, index, err)
		}
		variants = append(variants, variant)
	}
	// Function-calling providers require the root parameters schema to declare an
	// object even when the accepted shapes are expressed as a oneOf union.
	return newSchemaTool(name, description, &jsonschema.Schema{Type: "object", OneOf: variants}, invoke)
}
