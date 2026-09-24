package platform

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/invopop/jsonschema"
)

func TestImageOptionsRoundTripAndFenceChangedReplay(t *testing.T) {
	m, project := testManager(t)
	host := &resourceTestHost{project: project, profile: "test-image", image: resourcePNG(t), requests: make(chan ImageRequest, 2)}
	m.ConfigureResources(host)
	release := testResourceGame(t, m, project)
	instance, opened := openResourceGame(t, m, project, release)
	for _, command := range []ImageRequest{
		{CommandID: "defaults", ModelSlot: "art", Prompt: "A portrait"},
		{CommandID: "explicit", ModelSlot: "art", Prompt: "A portrait", Size: "1536x1024", AspectRatio: "3:2", Resolution: " 4K ", Quality: "high", OutputFormat: " WEBP "},
	} {
		if status, data := testRequest(t, opened.Connection, "POST", "/images/generations", "", command); status != 202 {
			t.Fatalf("start: %d %s", status, data)
		}
		if result := awaitImage(t, opened.Connection, command.CommandID); result.Status != "completed" {
			t.Fatalf("image did not complete: %+v", result)
		}
		if got := <-host.requests; !reflect.DeepEqual(got, command) {
			t.Fatalf("public image options were transformed before the native host: got=%+v want=%+v", got, command)
		}
		if status, data := testRequest(t, opened.Connection, "POST", "/images/generations", "", command); status != 200 {
			t.Fatalf("same options did not replay: %d %s", status, data)
		}
		for _, change := range []func(*ImageRequest){func(input *ImageRequest) { input.Resolution = "2K" }, func(input *ImageRequest) { input.OutputFormat = "jpeg" }} {
			changed := command
			change(&changed)
			if status, data := testRequest(t, opened.Connection, "POST", "/images/generations", "", changed); status != 409 {
				t.Fatalf("changed options reused a paid command: %d %s", status, data)
			}
		}
	}
	for _, field := range []string{"resolution", "outputFormat"} {
		input := map[string]any{"commandId": "invalid-" + field, "modelSlot": "art", "prompt": "A portrait", field: 42}
		if status, data := testRequest(t, opened.Connection, "POST", "/images/generations", "", input); status != 400 {
			t.Fatalf("non-string %s accepted: %d %s", field, status, data)
		}
	}
	// A receipt written by the previous optional-field contract must still
	// replay without invoking the provider. Empty new fields marshal away.
	const legacyInput = `[{"commandId":"legacy","modelSlot":"art","prompt":"A portrait","size":"1024x1024","aspectRatio":"1:1","quality":"high"},"test-image"]`
	legacyResult := ImageResult{CommandID: "legacy", Status: "completed", Images: []GeneratedImage{}}
	key := imageReceiptPath(m.runtimes[instance.ID].owner, "legacy")
	if err := writeJSON(key, imageReceipt{Version: 1, InputHash: stableID(legacyInput), Result: legacyResult}); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{"commandId": "legacy", "modelSlot": "art", "prompt": "A portrait", "size": "1024x1024", "aspectRatio": "1:1", "quality": "high", "resolution": "", "outputFormat": ""}
	status, data := testRequest(t, opened.Connection, "POST", "/images/generations", "", legacy)
	var replay ImageResult
	if err := json.Unmarshal(data, &replay); err != nil || status != 200 || !reflect.DeepEqual(replay, legacyResult) {
		t.Fatalf("legacy receipt changed: %d %s %v", status, data, err)
	}
	if host.calls.Load() != 2 {
		t.Fatalf("replays or rejected options called provider: %d", host.calls.Load())
	}
	schema := OpenAPI()["components"].(map[string]any)["schemas"].(map[string]any)["ImageRequest"].(*jsonschema.Schema)
	properties := schema.Properties
	for _, field := range []string{"resolution", "outputFormat"} {
		if property, ok := properties.Get(field); !ok || property.Type != "string" {
			t.Fatalf("image option missing from public discovery: %s", field)
		}
		for _, required := range schema.Required {
			if required == field {
				t.Fatalf("optional image option required by discovery: %s", field)
			}
		}
	}
}
