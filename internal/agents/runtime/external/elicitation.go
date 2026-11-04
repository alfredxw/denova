package external

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	interaction "github.com/alfredxw/denova/agent/lifecycle/interaction"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// NativeElicitation carries an MCP request after the adapter decodes its wire
// format. Adapters retain request identity and own the response envelope.
type NativeElicitation struct {
	Mode    string          `json:"mode"`
	Message string          `json:"message"`
	URL     string          `json:"url"`
	Schema  json.RawMessage `json:"requestedSchema"`
}

// ElicitNative supports MCP URL confirmations and scalar form fields. Secret
// inputs and richer app-specific forms are declined, never stored as chat text.
func ElicitNative(ctx context.Context, host Host, input Input, id string, request NativeElicitation) (json.RawMessage, error) {
	decline := json.RawMessage(`{"action":"decline"}`)
	if !input.Interactive() {
		return decline, nil
	}
	if request.Mode == "url" {
		allowed, err := ApproveNative(ctx, host, input, id, request.Message+"\n"+request.URL)
		if err != nil || !allowed {
			return decline, err
		}
		return json.RawMessage(`{"action":"accept"}`), nil
	}
	var schema struct {
		Properties map[string]struct {
			Type        string   `json:"type"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Format      string   `json:"format"`
			Enum        []string `json:"enum"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(request.Schema, &schema); err != nil {
		return decline, nil
	}
	keys := make([]string, 0, len(schema.Properties))
	for key := range schema.Properties {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	values := map[string]any{}
	for _, key := range keys {
		field := schema.Properties[key]
		if field.Format == "password" || field.Format == "secret" {
			return decline, nil
		}
		if !slices.Contains([]string{"string", "boolean", "number", "integer"}, field.Type) {
			return decline, nil
		}
		prompt := field.Title
		if prompt == "" {
			prompt = key
		}
		question := NativeQuestion{ID: key, Prompt: request.Message + "\n\n" + prompt + "\n" + field.Description}
		options := field.Enum
		if field.Type == "boolean" {
			options = []string{"true", "false"}
		}
		if len(options) >= 2 && len(options) <= 8 {
			for _, label := range options {
				question.Options = append(question.Options, interaction.InteractionOption{Label: label})
			}
		}
		answers, err := AskNative(ctx, host, input, id+"-"+key, []NativeQuestion{question})
		if err != nil || answers == nil {
			return decline, err
		}
		text := strings.Join(answers[key], ", ")
		if text == "" && !slices.Contains(schema.Required, key) {
			continue
		}
		switch field.Type {
		case "string":
			values[key] = text
		case "boolean":
			value, err := strconv.ParseBool(text)
			if err != nil {
				return decline, nil
			}
			values[key] = value
		case "number", "integer":
			value, err := strconv.ParseFloat(text, 64)
			if err != nil {
				return decline, nil
			}
			values[key] = value
		}
	}
	var document any
	if err := json.Unmarshal(request.Schema, &document); err != nil {
		return decline, nil
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("urn:runtime:elicitation", document); err != nil {
		return nil, err
	}
	compiled, err := compiler.Compile("urn:runtime:elicitation")
	if err != nil {
		return decline, fmt.Errorf("compile native elicitation: %w", err)
	}
	if err := compiled.Validate(values); err != nil {
		return decline, nil
	}
	if len(keys) == 0 {
		allowed, err := ApproveNative(ctx, host, input, id, request.Message)
		if err != nil || !allowed {
			return decline, err
		}
	}
	return json.Marshal(map[string]any{"action": "accept", "content": values})
}
