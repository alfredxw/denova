package openaiclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alfredxw/denova/agent/providers"
)

func TestListModelsUsesConfiguredRouteAndKeepsPartialResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/v1/models" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer secret" || request.Header.Get("X-Tenant") != "tenant" {
			t.Errorf("headers = %#v", request.Header)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"object":"list","data":[{"id":"model-z","owned_by":"vendor"},{"id":""},{"id":"model-a"},{"id":"model-z"}]}`)
	}))
	defer server.Close()

	models, err := ListModels(context.Background(), providers.ModelConfig{
		APIKey:     "secret",
		BaseURL:    server.URL + "/v1",
		Headers:    map[string]string{"X-Tenant": "tenant"},
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 || models[0].ID != "model-a" || models[1].ID != "model-z" || models[1].OwnedBy != "vendor" {
		t.Fatalf("models = %#v", models)
	}
}
