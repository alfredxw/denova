// Package speech synthesizes plain text through a user-configured endpoint.
// Playback, text selection, and story lifecycle remain application concerns.
package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"denova/config"
)

// Error contains only a safe localization key. Upstream bodies and URLs may
// contain credentials and must never be returned or logged.
type Error string

func (e Error) Error() string { return string(e) }

const (
	Unconfigured Error = "speech.error.unconfigured"
	InvalidURL   Error = "speech.error.url"
	InvalidInput Error = "speech.error.input"
	Network      Error = "speech.error.network"
	Auth         Error = "speech.error.auth"
	ModelVoice   Error = "speech.error.modelVoice"
	RateLimit    Error = "speech.error.rateLimit"
	Service      Error = "speech.error.service"
	Audio        Error = "speech.error.audio"
)

// Synthesize returns MP3 bytes. The caller owns cancellation; this HTTP timeout
// bounds an infrastructure request, independently of any story or Agent run.
func Synthesize(ctx context.Context, settings config.SpeechSettings, input string) ([]byte, error) {
	endpoint := strings.TrimSpace(settings.Endpoint)
	if endpoint == "" || strings.TrimSpace(settings.Model) == "" || strings.TrimSpace(settings.Voice) == "" {
		return nil, Unconfigured
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
		return nil, InvalidURL
	}
	if strings.TrimSpace(input) == "" || utf8.RuneCountInString(input) > 4096 {
		return nil, InvalidInput
	}
	body, err := json.Marshal(struct {
		Model  string `json:"model"`
		Voice  string `json:"voice"`
		Input  string `json:"input"`
		Format string `json:"response_format"`
	}{strings.TrimSpace(settings.Model), strings.TrimSpace(settings.Voice), input, "mp3"})
	if err != nil {
		return nil, InvalidInput
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, InvalidURL
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")
	if key := strings.TrimSpace(settings.APIKey); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := &http.Client{Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, Network
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == 401 || res.StatusCode == 403:
		return nil, Auth
	case res.StatusCode == 400 || res.StatusCode == 404 || res.StatusCode == 422:
		return nil, ModelVoice
	case res.StatusCode == 429:
		return nil, RateLimit
	case res.StatusCode != http.StatusOK:
		return nil, Service
	}
	const maxAudioBytes = 32 << 20
	data, err := io.ReadAll(io.LimitReader(res.Body, maxAudioBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, Network
	}
	if len(data) < 3 || len(data) > maxAudioBytes || !(bytes.HasPrefix(data, []byte("ID3")) || (data[0] == 0xff && data[1]&0xe0 == 0xe0)) {
		return nil, Audio
	}
	return data, nil
}

func ErrorKey(err error) string {
	var speechErr Error
	if errors.As(err, &speechErr) {
		return string(speechErr)
	}
	return string(Network)
}
