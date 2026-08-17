package imageapp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"denova/config"
)

type pingTestHost struct{ config config.Config }

func (host pingTestHost) AcquireImageRuntime(context.Context, string) (*Runtime, error) {
	return nil, ErrNoWorkspace
}

func (host pingTestHost) AcquireProjectImageRuntime(context.Context, string) (*Runtime, error) {
	return nil, ErrNoWorkspace
}

func (host pingTestHost) ImageConfigSnapshot() config.Config { return host.config }

func TestImagePingUsesStoredSecretAndRealGenerationAdapter(t *testing.T) {
	requestSeen := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/images/generations" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer stored-secret" {
			t.Errorf("authorization = %q", request.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(request.Body)
		if len(body) == 0 {
			t.Error("request body is empty")
		}
		requestSeen <- struct{}{}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"created":1,"data":[{"b64_json":"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9Z8WQAAAAASUVORK5CYII="}],"output_format":"png"}`)
	}))
	defer server.Close()

	service := NewService(pingTestHost{config: config.Config{ImageAPIProfiles: []config.ImageAPIProfileSettings{{
		ID: "private", Provider: config.DefaultImageAPIProvider, OpenAIAPIKey: "stored-secret",
		OpenAIBaseURL: server.URL, OpenAIModel: "image-model",
	}}}})
	result, err := service.Ping(context.Background(), config.ImageAPIProfileSettings{ID: "private"})
	if err != nil {
		t.Fatal(err)
	}
	<-requestSeen
	if !result.OK || result.ProfileID != "private" || result.Provider != config.DefaultImageAPIProvider || result.Model != "image-model" {
		t.Fatalf("result = %#v", result)
	}
}

func TestImagePingDoesNotForwardStoredSecretToChangedOrigin(t *testing.T) {
	requests := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- request
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	service := NewService(pingTestHost{config: config.Config{ImageAPIProfiles: []config.ImageAPIProfileSettings{{
		ID: "private", Provider: config.DefaultImageAPIProvider, OpenAIAPIKey: "stored-secret",
		OpenAIBaseURL: "https://original.example.test/v1", OpenAIModel: "image-model",
	}}}})
	_, err := service.Ping(context.Background(), config.ImageAPIProfileSettings{ID: "private", OpenAIBaseURL: server.URL})
	if !errors.Is(err, config.ErrImageAPIKeyMissing) {
		t.Fatalf("error = %T %v, want missing API key", err, err)
	}
	select {
	case request := <-requests:
		t.Fatalf("changed origin received a request with authorization %q", request.Header.Get("Authorization"))
	default:
	}
}

func TestImagePingClassifiesProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(writer, `{"error":{"message":"invalid key","type":"authentication_error","code":"invalid_api_key"}}`)
	}))
	defer server.Close()

	service := NewService(pingTestHost{})
	_, err := service.Ping(context.Background(), config.ImageAPIProfileSettings{
		ID: "invalid", Provider: config.DefaultImageAPIProvider, OpenAIAPIKey: "bad",
		OpenAIBaseURL: server.URL, OpenAIModel: "image-model",
	})
	if !IsProviderRequestError(err) {
		t.Fatalf("error classification = %T %v", err, err)
	}
}
