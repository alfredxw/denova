package generation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
)

const maxImageAPIJSONBytes = 64 << 20

func doJSON(ctx context.Context, client *http.Client, method, endpoint string, headers map[string]string, body, target any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode image API request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return fmt.Errorf("create image API request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	request.Header.Set("Accept", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("send image API request: %w", err)
	}
	defer response.Body.Close()
	payload, err := readLimited(response.Body, maxImageAPIJSONBytes)
	if err != nil {
		return fmt.Errorf("read image API response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return imageAPIStatusError(response.StatusCode, payload)
	}
	if target == nil || len(payload) == 0 {
		return nil
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("decode image API response: %w", err)
	}
	return nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return data, nil
}

func imageAPIStatusError(status int, payload []byte) error {
	var envelope struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
	}
	message := strings.TrimSpace(string(payload))
	if json.Unmarshal(payload, &envelope) == nil {
		switch value := envelope.Error.(type) {
		case string:
			message = value
		case map[string]any:
			if text, ok := value["message"].(string); ok {
				message = text
			}
		}
		if message == "" {
			message = envelope.Message
		}
	}
	if len(message) > 1000 {
		message = message[:1000] + "..."
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return fmt.Errorf("image API returned HTTP %d: %s", status, message)
}

func bearerHeaders(apiKey string, extra map[string]string) map[string]string {
	headers := make(map[string]string, len(extra)+1)
	if strings.TrimSpace(apiKey) != "" {
		headers["Authorization"] = "Bearer " + strings.TrimSpace(apiKey)
	}
	for key, value := range extra {
		headers[key] = value
	}
	return headers
}

func endpointURL(baseURL, suffix string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid image API base URL %q", baseURL)
	}
	parsed.Path = path.Join(strings.TrimSuffix(parsed.Path, "/"), suffix)
	return parsed.String(), nil
}
