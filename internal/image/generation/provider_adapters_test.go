package generation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"denova/config"
)

var adapterTestPNG = []byte("\x89PNG\r\n\x1a\n")

func TestXAIAdapterMapsAspectRatioResolutionAndQuality(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/images/generations" || request.Header.Get("Authorization") != "Bearer xai-key" {
			t.Errorf("unexpected request: %s auth=%q", request.URL.Path, request.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["aspect_ratio"] != "16:9" || body["resolution"] != "2k" || body["quality"] != "low" || body["response_format"] != "b64_json" {
			t.Errorf("unexpected xAI request: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(adapterTestPNG)}}})
	}))
	defer server.Close()

	result, err := NewXAIAdapter(server.Client()).Generate(context.Background(), config.ResolvedImageAPIProfile{
		ProfileID: "grok", Provider: config.ImageProviderXAI, BaseURL: server.URL, APIKey: "xai-key", Model: "grok-imagine-image-2.0",
	}, GenerateRequest{Prompt: "prompt", N: 1, Size: "1920x1080", Resolution: "2K", Quality: "low"})
	if err != nil || len(result.Images) != 1 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}

func TestArkAdapterMakesIndependentRequestsForMultipleImages(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["size"] != "2K" || body["response_format"] != "url" || body["watermark"] != false {
			t.Errorf("unexpected Ark request: %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"data": []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(adapterTestPNG)}}})
	}))
	defer server.Close()

	result, err := NewArkAdapter(server.Client()).Generate(context.Background(), config.ResolvedImageAPIProfile{
		ProfileID: "seedream", Provider: config.ImageProviderVolcengine, BaseURL: server.URL, APIKey: "ark-key", Model: "seedream",
	}, GenerateRequest{Prompt: "prompt", N: 2, Resolution: "2K", OutputFormat: "png"})
	if err != nil || len(result.Images) != 2 || calls.Load() != 2 {
		t.Fatalf("calls=%d result=%#v error=%v", calls.Load(), result, err)
	}
}

func TestGeminiAdapterUsesNativeGenerateContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/models/gemini-test:generateContent" || request.Header.Get("x-goog-api-key") != "gemini-key" {
			t.Errorf("unexpected Gemini request: %s key=%q", request.URL.Path, request.Header.Get("x-goog-api-key"))
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		generationConfig := body["generationConfig"].(map[string]any)
		imageConfig := generationConfig["responseFormat"].(map[string]any)["image"].(map[string]any)
		if imageConfig["aspectRatio"] != "9:16" || imageConfig["imageSize"] != "2K" {
			t.Errorf("unexpected Gemini image config: %#v", imageConfig)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"candidates": []map[string]any{{"content": map[string]any{"parts": []map[string]any{{
				"inlineData": map[string]any{"mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(adapterTestPNG)},
			}}}}},
		})
	}))
	defer server.Close()

	result, err := NewGeminiAdapter(server.Client()).Generate(context.Background(), config.ResolvedImageAPIProfile{
		ProfileID: "gemini", Provider: config.ImageProviderGoogle, BaseURL: server.URL, APIKey: "gemini-key", Model: "gemini-test",
	}, GenerateRequest{Prompt: "prompt", N: 1, AspectRatio: "9:16", Resolution: "2k"})
	if err != nil || len(result.Images) != 1 || result.Images[0].Extension != "png" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
}
