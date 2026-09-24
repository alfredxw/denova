package builtin

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
)

type imageRoundTrip func(*http.Request) (*http.Response, error)

func (f imageRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestProtocolsSendPreparedImagesAndClassifyTransportRejection(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewGray(image.Rect(0, 0, 2000, 1500))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "original.png")
	if err := os.WriteFile(path, encoded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	attachment := agent.Attachment{Name: "reference.png", Path: path, MediaType: "image/png", SHA256: fmt.Sprintf("%x", sha256.Sum256(encoded.Bytes()))}
	for _, protocol := range []providers.ProtocolID{providers.ProtocolAnthropicMessages, providers.ProtocolOpenAIResponses, providers.ProtocolOpenAIChatCompletions} {
		t.Run(string(protocol), func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: imageRoundTrip(func(request *http.Request) (*http.Response, error) {
				calls++
				raw, err := io.ReadAll(request.Body)
				_ = request.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
				var body any
				if err := json.Unmarshal(raw, &body); err != nil {
					t.Fatal(err)
				}
				images := 0
				var visit func(any)
				visit = func(value any) {
					switch v := value.(type) {
					case map[string]any:
						for key, part := range v {
							if text, ok := part.(string); ok && (strings.HasPrefix(text, "data:image/") || key == "data" && v["type"] == "base64") {
								if strings.HasPrefix(text, "data:") {
									_, text, _ = strings.Cut(text, ",")
								}
								data, err := base64.StdEncoding.DecodeString(text)
								if err != nil {
									t.Fatal(err)
								}
								dimensions, _, err := image.DecodeConfig(bytes.NewReader(data))
								if err != nil || dimensions.Width >= 2000 || dimensions.Height >= 1500 {
									t.Fatalf("unprepared wire image: %+v %v", dimensions, err)
								}
								images++
							} else {
								visit(part)
							}
						}
					case []any:
						for _, part := range v {
							visit(part)
						}
					}
				}
				visit(body)
				if images != 2 {
					t.Fatalf("wire request has %d images, want 2", images)
				}
				return &http.Response{StatusCode: 413, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"error":{"type":"request_too_large","message":"request too large"}}`)), Request: request}, nil
			})}
			registry, err := NewRegistry()
			if err != nil {
				t.Fatal(err)
			}
			model, err := registry.NewChatModel(t.Context(), providers.ModelConfig{Provider: providers.ProviderOpenAICompatible, Protocol: protocol, Model: "claude-sonnet-4-6", BaseURL: "https://gateway.example/v1", APIKey: "test", HTTPClient: client})
			if err != nil {
				t.Fatal(err)
			}
			messages := []*agent.Message{agent.UserMessageWithAttachments("inspect", []agent.Attachment{attachment}), agent.UserMessageWithAttachments("compare", []agent.Attachment{attachment})}
			for _, stream := range []bool{false, true} {
				if stream {
					reader, streamErr := model.Stream(t.Context(), messages)
					err = streamErr
					if err == nil {
						_, err = reader.Recv()
						reader.Close()
					}
				} else {
					_, err = model.Generate(t.Context(), messages)
				}
				var apiError *providers.APIError
				if !errors.As(err, &apiError) || apiError.ModelErrorReason() != agent.ModelRequestTooLargeReason || apiError.Retryable() {
					t.Fatalf("wrong transport classification: %v", err)
				}
			}
			if calls != 2 {
				t.Fatalf("unexpected request retries: %d", calls)
			}
			if _, err := agent.ReadAttachmentImage(attachment); err != nil {
				t.Fatal(err)
			}
		})
	}
}
