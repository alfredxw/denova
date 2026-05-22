package config

import "testing"

func TestDefaultSettingsValues(t *testing.T) {
	s := DefaultSettings()
	if s.OpenAIBaseURL != "https://api.deepseek.com" {
		t.Fatalf("BaseURL: %s", s.OpenAIBaseURL)
	}
	if s.OpenAIModel != "deepseek-v4-pro" {
		t.Fatalf("Model: %s", s.OpenAIModel)
	}
	if s.AutoSaveEnabled == nil || *s.AutoSaveEnabled != true {
		t.Fatalf("AutoSaveEnabled default")
	}
	if s.MaxIteration == nil || *s.MaxIteration != 50 {
		t.Fatalf("MaxIteration default")
	}
}
