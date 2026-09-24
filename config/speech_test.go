package config

import (
	"encoding/json"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

func TestSpeechSettingsPersistClearAndStayUserScoped(t *testing.T) {
	settings := Settings{OpenAIModel: "text-model", Speech: &SpeechSettings{Endpoint: "http://localhost:8000/v1/audio/speech", APIKey: "secret", Model: "custom", Voice: "voice"}}
	data, err := toml.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	var restored Settings
	if err := toml.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Speech == nil || *restored.Speech != *settings.Speech {
		t.Fatalf("speech not restored: %#v", restored.Speech)
	}
	cleared, err := ApplySettingsMergePatch(restored, json.RawMessage(`{"speech":{"endpoint":"","api_key":"","model":"","voice":""}}`))
	if err != nil {
		t.Fatal(err)
	}
	effective := Merge(settings, cleared)
	if effective.Speech == nil || *effective.Speech != (SpeechSettings{}) || effective.OpenAIModel != "text-model" {
		t.Fatal("clearing speech affected inherited speech or language model settings")
	}
	if err := ValidateWorkspaceSettingsPatch(json.RawMessage(`{"speech":{"api_key":"workspace"}}`)); err == nil {
		t.Fatal("workspace accepted speech credentials")
	}
}
