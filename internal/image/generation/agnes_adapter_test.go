package generation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"denova/config"
)

const testPixelPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

type agnesCapturedRequest struct {
	body map[string]any
}

func agnesTestProfile(baseURL string) config.ResolvedImageAPIProfile {
	return config.ResolvedImageAPIProfile{
		ProfileID: "agnes",
		Provider:  config.ImageProviderAgnes,
		Protocol:  config.ImageProtocolAgnes,
		APIKey:    "test-key",
		BaseURL:   baseURL,
		Model:     "agnes-image-2.5-flash",
		Size:      "2K",
	}
}

func agnesTestServer(t *testing.T, failFirst bool, captured *[]agnesCapturedRequest) *httptest.Server {
	t.Helper()
	calls := 0
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/images/generations" {
			t.Errorf("unexpected path %q", request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		*captured = append(*captured, agnesCapturedRequest{body: body})
		calls++
		if failFirst && calls == 1 {
			writer.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(writer, `{"error":{"message":"boom"}}`)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(writer, `{"created":1780000000,"data":[{"url":null,"b64_json":%q,"revised_prompt":null}]}`, testPixelPNGBase64)
	}))
}

func TestAgnesAdapterSendsAgnesShape(t *testing.T) {
	var captured []agnesCapturedRequest
	server := agnesTestServer(t, false, &captured)
	defer server.Close()

	adapter := NewAgnesAdapter(nil)
	result, err := adapter.Generate(context.Background(), agnesTestProfile(server.URL+"/v1"), GenerateRequest{
		Prompt:       "a glass cube",
		N:            2,
		Size:         "2K",
		AspectRatio:  "16:9",
		Quality:      "high",
		OutputFormat: "png",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(captured) != 2 {
		t.Fatalf("requests = %d, want 2", len(captured))
	}
	for index, call := range captured {
		body := call.body
		if body["model"] != "agnes-image-2.5-flash" || body["prompt"] != "a glass cube" {
			t.Fatalf("request %d body = %v", index, body)
		}
		if body["size"] != "2K" || body["ratio"] != "16:9" {
			t.Fatalf("request %d size/ratio = %v/%v", index, body["size"], body["ratio"])
		}
		if body["return_base64"] != true {
			t.Fatalf("request %d return_base64 = %v", index, body["return_base64"])
		}
		for _, rejected := range []string{"output_format", "quality", "response_format", "n"} {
			if _, present := body[rejected]; present {
				t.Fatalf("request %d must not send %q: %v", index, rejected, body)
			}
		}
	}
	if len(result.Images) != 2 || len(result.Failures) != 0 {
		t.Fatalf("images=%d failures=%d", len(result.Images), len(result.Failures))
	}
	pixel, _ := base64.StdEncoding.DecodeString(testPixelPNGBase64)
	if string(result.Images[0].Data) != string(pixel) || result.Images[0].Extension != "png" {
		t.Fatalf("image = %d bytes ext=%q", len(result.Images[0].Data), result.Images[0].Extension)
	}
}

func TestAgnesAdapterSkipsRatioForExactOrUnsupported(t *testing.T) {
	var captured []agnesCapturedRequest
	server := agnesTestServer(t, false, &captured)
	defer server.Close()

	adapter := NewAgnesAdapter(nil)
	profile := agnesTestProfile(server.URL + "/v1")
	if _, err := adapter.Generate(context.Background(), profile, GenerateRequest{Prompt: "x", N: 1, Size: "1024x768", AspectRatio: "4:3"}); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Generate(context.Background(), profile, GenerateRequest{Prompt: "x", N: 1, Size: "1K", AspectRatio: "4:5"}); err != nil {
		t.Fatal(err)
	}
	if len(captured) != 2 {
		t.Fatalf("requests = %d, want 2", len(captured))
	}
	if _, present := captured[0].body["ratio"]; present {
		t.Fatalf("exact size must not send ratio: %v", captured[0].body)
	}
	if _, present := captured[1].body["ratio"]; present {
		t.Fatalf("unsupported ratio must be dropped: %v", captured[1].body)
	}
}

func TestAgnesAdapterKeepsPartialFailures(t *testing.T) {
	var captured []agnesCapturedRequest
	server := agnesTestServer(t, true, &captured)
	defer server.Close()

	adapter := NewAgnesAdapter(nil)
	result, err := adapter.Generate(context.Background(), agnesTestProfile(server.URL+"/v1"), GenerateRequest{Prompt: "x", N: 2, Size: "2K"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Images) != 1 || len(result.Failures) != 1 {
		t.Fatalf("images=%d failures=%d, want 1/1", len(result.Images), len(result.Failures))
	}
	if result.Failures[0].Index != 0 || !strings.Contains(result.Failures[0].Message, "HTTP 400") {
		t.Fatalf("failure = %#v", result.Failures[0])
	}
}

func TestAgnesAdapterFailsWhenAllRequestsFail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(writer, `{"error":{"message":"boom"}}`)
	}))
	defer server.Close()

	adapter := NewAgnesAdapter(nil)
	_, err := adapter.Generate(context.Background(), agnesTestProfile(server.URL+"/v1"), GenerateRequest{Prompt: "x", N: 1, Size: "2K"})
	if err == nil || !strings.Contains(err.Error(), "Agnes image generation failed") {
		t.Fatalf("err = %v", err)
	}
}
