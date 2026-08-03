package models

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alfredxw/denova/agent/providers"

	"denova/config"
)

type testHost struct{ config config.Config }

func (host testHost) ModelConfigSnapshot() config.Config { return host.config }

func TestPingUsesStoredSecretAndRealAgentAdapter(t *testing.T) {
	requestBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer secret" || request.Header.Get("X-Custom") != "custom" {
			t.Errorf("headers = %#v", request.Header)
		}
		body, _ := io.ReadAll(request.Body)
		requestBody <- body
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{
  "id":"ping","object":"chat.completion","created":1,"model":"provider-model",
  "choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"OK"}}],
  "usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}
}`)
	}))
	defer server.Close()

	service := NewService(testHost{config: config.Config{ModelProfiles: []config.ModelProfileSettings{{
		ID: "private", Provider: string(providers.ProviderOpenAICompatible), Protocol: string(providers.ProtocolOpenAIChatCompletions),
		APIKey: "secret", BaseURL: server.URL, Model: "private-model",
		Headers: map[string]string{"X-Custom": "custom"},
	}}}})
	result, err := service.Ping(context.Background(), config.ModelProfileSettings{ID: "private"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Provider != string(providers.ProviderOpenAICompatible) || result.Protocol != string(providers.ProtocolOpenAIChatCompletions) || result.Model != "private-model" {
		t.Fatalf("result = %#v", result)
	}
	var request map[string]any
	if err := json.Unmarshal(<-requestBody, &request); err != nil {
		t.Fatal(err)
	}
	if request["model"] != "private-model" || request["max_tokens"] != float64(16) {
		t.Fatalf("request = %#v", request)
	}
}

func TestPingDoesNotForwardStoredCredentialsToChangedOrigin(t *testing.T) {
	headers := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		headers <- request.Header.Clone()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{
  "id":"ping","object":"chat.completion","created":1,"model":"provider-model",
  "choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"OK"}}]
}`)
	}))
	defer server.Close()

	service := NewService(testHost{config: config.Config{ModelProfiles: []config.ModelProfileSettings{{
		ID: "private", Provider: string(providers.ProviderOpenAICompatible), Protocol: string(providers.ProtocolOpenAIChatCompletions),
		APIKey: "stored-secret", BaseURL: "https://original.example.test/v1", Model: "private-model",
		Headers: map[string]string{"X-Private": "stored-header"},
	}}}})
	if _, err := service.Ping(context.Background(), config.ModelProfileSettings{
		ID: "private", BaseURL: server.URL,
	}); err != nil {
		t.Fatal(err)
	}
	requestHeaders := <-headers
	if requestHeaders.Get("Authorization") == "Bearer stored-secret" || requestHeaders.Get("X-Private") != "" {
		t.Fatalf("changed endpoint received stored credentials: %#v", requestHeaders)
	}
}

func TestPingPreservesProviderAPIErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, `{"error":{"message":"invalid key","type":"authentication_error","code":"invalid_api_key"}}`)
	}))
	defer server.Close()

	service := NewService(testHost{config: config.Config{}})
	_, err := service.Ping(context.Background(), config.ModelProfileSettings{
		ID: "invalid", Provider: string(providers.ProviderOpenAICompatible), Protocol: string(providers.ProtocolOpenAIChatCompletions),
		APIKey: "bad", BaseURL: server.URL, Model: "model",
	})
	if !IsProviderRequestError(err) {
		t.Fatalf("error classification = %T %v", err, err)
	}
	var apiError *providers.APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusUnauthorized {
		t.Fatalf("error = %T %v", err, err)
	}
}

func TestListModelsUsesStoredSecretAndKeepsSuggestionsIndependent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/models" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer stored-secret" || request.Header.Get("X-Custom") != "custom" {
			t.Errorf("headers = %#v", request.Header)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"object":"list","data":[{"id":"model-b"},{"id":"model-a","owned_by":"vendor"}]}`)
	}))
	defer server.Close()

	service := NewService(testHost{config: config.Config{ModelProfiles: []config.ModelProfileSettings{{
		ID: "private", Provider: string(providers.ProviderOpenAICompatible), Protocol: string(providers.ProtocolOpenAIResponses),
		APIKey: "stored-secret", BaseURL: server.URL + "/v1", Model: "custom-model-not-in-list",
		Headers: map[string]string{"X-Custom": "custom"},
	}}}})
	result, err := service.List(context.Background(), config.ModelProfileSettings{ID: "private"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Provider != string(providers.ProviderOpenAICompatible) || result.Protocol != string(providers.ProtocolOpenAIResponses) || len(result.Models) != 2 {
		t.Fatalf("result = %#v", result)
	}
	if result.Models[0].ID != "model-a" || result.Models[0].OwnedBy != "vendor" || result.Models[1].ID != "model-b" {
		t.Fatalf("models = %#v", result.Models)
	}
}

func TestListModelsDoesNotForwardStoredCredentialsToChangedOrigin(t *testing.T) {
	headers := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		headers <- request.Header.Clone()
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"object":"list","data":[]}`)
	}))
	defer server.Close()

	service := NewService(testHost{config: config.Config{ModelProfiles: []config.ModelProfileSettings{{
		ID: "private", Provider: string(providers.ProviderOpenAICompatible), Protocol: string(providers.ProtocolOpenAIChatCompletions),
		APIKey: "stored-secret", BaseURL: "https://original.example.test/v1", Model: "custom-model",
		Headers: map[string]string{"X-Private": "stored-header"},
	}}}})
	if _, err := service.List(context.Background(), config.ModelProfileSettings{
		ID: "private", BaseURL: server.URL + "/v1",
	}); err != nil {
		t.Fatal(err)
	}
	requestHeaders := <-headers
	if requestHeaders.Get("Authorization") == "Bearer stored-secret" || requestHeaders.Get("X-Private") != "" {
		t.Fatalf("changed endpoint received stored credentials: %#v", requestHeaders)
	}
}

func TestCatalogPublishesAdaptersAndDeepSeekRoutes(t *testing.T) {
	service := NewService(testHost{})
	catalog, err := service.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	wantProtocols := map[string]bool{
		string(providers.ProtocolOpenAIChatCompletions): false,
		string(providers.ProtocolOpenAIResponses):       false,
		string(providers.ProtocolAnthropicMessages):     false,
	}
	for _, protocol := range catalog.Protocols {
		if _, ok := wantProtocols[protocol]; ok {
			wantProtocols[protocol] = true
		}
	}
	for protocol, found := range wantProtocols {
		if !found {
			t.Fatalf("protocol %q missing from catalog: %#v", protocol, catalog.Protocols)
		}
	}
	for _, preset := range catalog.Providers {
		if preset.ID != string(providers.ProviderDeepSeek) {
			continue
		}
		if preset.Endpoints[string(providers.ProtocolOpenAIResponses)].BaseURL != "https://api.deepseek.com" ||
			preset.Endpoints[string(providers.ProtocolAnthropicMessages)].BaseURL != "https://api.deepseek.com/anthropic" {
			t.Fatalf("DeepSeek routes = %#v", preset.Endpoints)
		}
		return
	}
	t.Fatal("DeepSeek preset missing")
}
