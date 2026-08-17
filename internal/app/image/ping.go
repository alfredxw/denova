package imageapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"denova/config"
	imagegen "denova/internal/image/generation"
)

type PingResult struct {
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	ProfileID string `json:"profile_id"`
	Provider  string `json:"provider"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
}

// ProviderRequestError distinguishes a valid image configuration whose real
// upstream request failed from a malformed settings draft.
type ProviderRequestError struct{ cause error }

func (err *ProviderRequestError) Error() string {
	if err == nil || err.cause == nil {
		return "image provider request failed"
	}
	return err.cause.Error()
}

func (err *ProviderRequestError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.cause
}

func IsProviderRequestError(err error) bool {
	var target *ProviderRequestError
	return errors.As(err, &target)
}

// Ping performs one minimal real image generation without writing the returned
// bytes into a workspace. This validates the same adapter used by real runs.
func (service *Service) Ping(ctx context.Context, profile config.ImageAPIProfileSettings) (PingResult, error) {
	if service == nil || service.host == nil {
		return PingResult{}, fmt.Errorf("image ping: service host is unavailable")
	}
	host, ok := service.host.(ConfigHost)
	if !ok {
		return PingResult{}, fmt.Errorf("image ping: configuration snapshot is unavailable")
	}
	snapshot := host.ImageConfigSnapshot()
	resolved, err := config.ResolveImageAPIProfileDraft(&snapshot, profile)
	if err != nil {
		return PingResult{}, fmt.Errorf("image ping: %w", err)
	}
	const validationProfileID = "__image_ping__"
	validationConfig := config.Config{
		DefaultImageAPIProfileID: validationProfileID,
		ImageAPIProfiles: []config.ImageAPIProfileSettings{{
			ID: validationProfileID, Name: resolved.Name, Provider: resolved.Provider,
			OpenAIAPIKey: resolved.OpenAIAPIKey, OpenAIBaseURL: resolved.OpenAIBaseURL,
			OpenAIModel: resolved.OpenAIModel, DefaultSize: resolved.Size,
			DefaultQuality: resolved.Quality, DefaultOutputFormat: resolved.OutputFormat,
		}},
	}
	started := time.Now()
	_, err = imagegen.NewService().Generate(ctx, &validationConfig, imagegen.GenerateRequest{
		ProfileID: validationProfileID,
		Prompt:    "A plain white square on a plain white background.",
		N:         1,
	})
	if err != nil {
		return PingResult{}, &ProviderRequestError{cause: fmt.Errorf("image ping: %w", err)}
	}
	return PingResult{
		OK: true, LatencyMS: time.Since(started).Milliseconds(), ProfileID: resolved.ProfileID,
		Provider: resolved.Provider, BaseURL: resolved.OpenAIBaseURL, Model: resolved.OpenAIModel,
	}, nil
}
