package anthropicmessages

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alfredxw/denova/agent/providers"
)

func TestListModelsUsesConfiguredRouteAndPagination(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestCount++
		if request.Method != http.MethodGet || request.URL.Path != "/v1/models" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("X-Api-Key") != "secret" || request.Header.Get("X-Tenant") != "tenant" {
			t.Errorf("headers = %#v", request.Header)
		}
		if request.URL.Query().Get("limit") != "1000" {
			t.Errorf("limit = %q", request.URL.Query().Get("limit"))
		}

		writer.Header().Set("Content-Type", "application/json")
		if request.URL.Query().Get("after_id") == "model-b" {
			_, _ = io.WriteString(writer, `{"data":[{"id":"model-a","display_name":"Model A","type":"model"},{"id":"model-b","display_name":"Duplicate","type":"model"}],"first_id":"model-a","last_id":"model-a","has_more":false}`)
			return
		}
		_, _ = io.WriteString(writer, `{"data":[{"id":"model-b","display_name":"Model B","type":"model"},{"id":"","display_name":"Invalid","type":"model"}],"first_id":"model-b","last_id":"model-b","has_more":true}`)
	}))
	defer server.Close()

	models, err := (&Adapter{}).ListModels(context.Background(), providers.ModelConfig{
		Provider:   providers.ProviderAnthropic,
		Protocol:   providers.ProtocolAnthropicMessages,
		APIKey:     "secret",
		BaseURL:    server.URL,
		Headers:    map[string]string{"X-Tenant": "tenant"},
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 {
		t.Fatalf("request count = %d, want 2", requestCount)
	}
	if len(models) != 2 || models[0].ID != "model-a" || models[0].DisplayName != "Model A" || models[1].ID != "model-b" || models[1].DisplayName != "Model B" {
		t.Fatalf("models = %#v", models)
	}
}
