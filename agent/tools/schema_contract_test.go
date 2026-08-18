package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/invopop/jsonschema"
)

type schemaTaskExecutor struct{}

func (schemaTaskExecutor) Identity() agent.CapabilityIdentity {
	return agent.CapabilityIdentity{Kind: "tasks.schema-test", Version: 1}
}
func (schemaTaskExecutor) Start(context.Context, TaskRequest) (Task, error) { return Task{}, nil }
func (schemaTaskExecutor) Observe(context.Context, TaskRef, string) (TaskObservation, error) {
	return TaskObservation{}, nil
}
func (schemaTaskExecutor) Steer(context.Context, TaskRef, agent.Input) error { return nil }
func (schemaTaskExecutor) Respond(context.Context, TaskRef, string, agent.InteractionResponse) error {
	return nil
}
func (schemaTaskExecutor) Abort(context.Context, TaskRef, agent.AbortRequest) error { return nil }

func TestActionToolsExposeDisjointOperationSchemas(t *testing.T) {
	tests := []struct {
		name       string
		build      func() agent.Toolset
		actions    []string
		properties map[string][]string
	}{
		{
			name: "task", build: func() agent.Toolset { return Tasks(schemaTaskExecutor{}) },
			actions: []string{"start", "observe", "steer", "respond", "abort"},
			properties: map[string][]string{
				"start": {"action", "starts"}, "observe": {"action", "cursor", "refs"},
				"steer": {"action", "input", "refs"}, "respond": {"action", "responses"},
				"abort": {"action", "reason", "refs"},
			},
		},
		{
			name: "skill", build: func() agent.Toolset { return Skills(testSkillSource{}) },
			actions: []string{"list", "read"},
			properties: map[string][]string{
				"list": {"action", "limit", "query"}, "read": {"action", "refs"},
			},
		},
		{
			name: "todo", build: func() agent.Toolset { return Todo() },
			actions: []string{"read", "update", "replace", "clear"},
			properties: map[string][]string{
				"read": {"action"}, "update": {"action", "expected_revision", "mutations"},
				"replace": {"action", "expected_revision", "items"}, "clear": {"action", "expected_revision"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			schema := preparedToolSchema(t, test.build)
			if schema.Type != "object" {
				t.Fatalf("root schema type = %q, want object", schema.Type)
			}
			if len(schema.OneOf) != len(test.actions) {
				t.Fatalf("oneOf variants = %d, want %d", len(schema.OneOf), len(test.actions))
			}
			for index, variant := range schema.OneOf {
				action := test.actions[index]
				actionSchema, ok := variant.Properties.Get("action")
				if !ok || len(actionSchema.Enum) != 1 || actionSchema.Enum[0] != action {
					t.Fatalf("variant %d action = %#v", index, actionSchema)
				}
				if !containsString(variant.Required, "action") {
					t.Fatalf("variant %s does not require action: %#v", action, variant.Required)
				}
				got := schemaPropertyNames(variant)
				want := append([]string(nil), test.properties[action]...)
				sort.Strings(want)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("variant %s properties = %#v, want %#v", action, got, want)
				}
				encoded, err := json.Marshal(variant)
				if err != nil || !strings.Contains(string(encoded), `"additionalProperties":false`) {
					t.Fatalf("variant %s is not closed: %s, %v", action, encoded, err)
				}
			}
		})
	}
}

func TestAskSchemaSeparatesFreeTextAndChoiceQuestions(t *testing.T) {
	schema := preparedToolSchema(t, Ask)
	questions, ok := schema.Properties.Get("questions")
	if !ok || questions.Items == nil || len(questions.Items.OneOf) != 2 {
		t.Fatalf("ask question variants = %#v", questions)
	}
	freeText := questions.Items.OneOf[0]
	choice := questions.Items.OneOf[1]
	if got := schemaPropertyNames(freeText); !reflect.DeepEqual(got, []string{"allow_free_text", "id", "prompt"}) {
		t.Fatalf("free-text properties = %#v", got)
	}
	allow, _ := freeText.Properties.Get("allow_free_text")
	if allow == nil || allow.Const != true {
		t.Fatalf("allow_free_text schema = %#v", allow)
	}
	if got := schemaPropertyNames(choice); !reflect.DeepEqual(got, []string{"id", "multiple", "options", "prompt"}) {
		t.Fatalf("choice properties = %#v", got)
	}
	for name, variant := range map[string]*jsonschema.Schema{"free text": freeText, "choice": choice} {
		prompt, _ := variant.Properties.Get("prompt")
		if prompt == nil || prompt.Type != "string" || prompt.Properties.Len() != 0 {
			t.Fatalf("%s prompt must be one language-specific string: %#v", name, prompt)
		}
	}
	options, _ := choice.Properties.Get("options")
	if options == nil || options.MinItems == nil || *options.MinItems != 2 || options.MaxItems == nil || *options.MaxItems != 4 ||
		options.Contains == nil || options.MinContains == nil || *options.MinContains != 1 || options.MaxContains == nil || *options.MaxContains != 1 {
		t.Fatalf("choice options schema = %#v", options)
	}
	recommended, _ := options.Contains.Properties.Get("recommended")
	if recommended == nil || recommended.Const != true {
		t.Fatalf("recommended marker schema = %#v", recommended)
	}
	label, _ := options.Items.Properties.Get("label")
	description, _ := options.Items.Properties.Get("description")
	if label == nil || label.Type != "string" || description == nil || description.Type != "string" {
		t.Fatalf("Ask option copy must use single-language strings: label=%#v description=%#v", label, description)
	}
}

func preparedToolSchema(t *testing.T, build func() agent.Toolset) *jsonschema.Schema {
	t.Helper()
	toolset := build()
	definitions, err := toolset.PrepareTools(context.Background(), agent.ToolRequest{})
	if err != nil || len(definitions) != 1 {
		t.Fatalf("definitions = %d, error = %v", len(definitions), err)
	}
	info, err := definitions[0].Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func schemaPropertyNames(schema *jsonschema.Schema) []string {
	result := make([]string, 0)
	if schema != nil && schema.Properties != nil {
		for pair := schema.Properties.Oldest(); pair != nil; pair = pair.Next() {
			result = append(result, pair.Key)
		}
	}
	sort.Strings(result)
	return result
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
