package generation

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"denova/config"
)

var (
	ErrPromptRequired       = errors.New("image prompt is required")
	ErrUnsupportedProtocol  = errors.New("unsupported image protocol")
	ErrImageCountOutOfRange = errors.New("image count must be between 1 and 10")
)

type Service struct {
	adapters map[string]Adapter
}

func NewService() *Service {
	return &Service{adapters: map[string]Adapter{
		config.ImageProtocolOpenAI:  NewOpenAIAdapter(nil),
		config.ImageProtocolXAI:     NewXAIAdapter(nil),
		config.ImageProtocolArk:     NewArkAdapter(nil),
		config.ImageProtocolGemini:  NewGeminiAdapter(nil),
		config.ImageProtocolComfyUI: NewComfyUIAdapter(nil),
	}}
}

func (s *Service) Generate(ctx context.Context, cfg *config.Config, request GenerateRequest) (Result, error) {
	if strings.TrimSpace(request.Prompt) == "" {
		return Result{}, ErrPromptRequired
	}
	profile, err := config.ResolveImageAPIProfile(cfg, request.ProfileID)
	if err != nil {
		return Result{}, err
	}
	request.Prompt = strings.TrimSpace(request.Prompt)
	if request.N == 0 {
		request.N = 1
	}
	if request.N < 1 || request.N > 10 {
		return Result{}, ErrImageCountOutOfRange
	}
	if request.Size == "" {
		request.Size = profile.Size
	}
	if request.AspectRatio == "" {
		request.AspectRatio = profile.AspectRatio
	}
	if request.Resolution == "" {
		request.Resolution = profile.Resolution
	}
	if request.Quality == "" {
		request.Quality = profile.Quality
	}
	if request.OutputFormat == "" {
		request.OutputFormat = profile.OutputFormat
	}
	request, err = normalizeRequestOptions(request)
	if err != nil {
		return Result{}, err
	}

	adapter := s.adapters[profile.Protocol]
	if adapter == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedProtocol, profile.Protocol)
	}
	slog.InfoContext(ctx, fmt.Sprintf("[imagegen] generate begin provider=%s protocol=%s profile_id=%s model=%q size=%q ratio=%q resolution=%q quality=%q format=%q n=%d prompt=%s", profile.Provider, profile.Protocol, profile.ProfileID, profile.Model, request.Size, request.AspectRatio, request.Resolution, request.Quality, request.OutputFormat, request.N, promptSummary(request.Prompt)))
	result, err := adapter.Generate(ctx, profile, request)
	if err != nil {
		slog.ErrorContext(ctx, fmt.Sprintf("[imagegen] generate failed provider=%s protocol=%s profile_id=%s model=%q err=%v", profile.Provider, profile.Protocol, profile.ProfileID, profile.Model, err))
		return Result{}, err
	}
	slog.InfoContext(ctx, fmt.Sprintf("[imagegen] generate done provider=%s protocol=%s profile_id=%s model=%q images=%d failures=%d", profile.Provider, profile.Protocol, profile.ProfileID, profile.Model, len(result.Images), len(result.Failures)))
	return result, nil
}

func normalizeRequestOptions(request GenerateRequest) (GenerateRequest, error) {
	request.Size = strings.TrimSpace(request.Size)
	if strings.EqualFold(request.Size, "auto") {
		request.Size = ""
	}
	request.AspectRatio = strings.ToLower(strings.TrimSpace(request.AspectRatio))
	if request.AspectRatio == "auto" {
		request.AspectRatio = ""
	}
	request.Resolution = strings.TrimSpace(request.Resolution)
	if request.Quality != "" {
		quality := normalizeQuality(request.Quality)
		if quality == "" {
			return GenerateRequest{}, fmt.Errorf("unsupported image quality: %s", request.Quality)
		}
		request.Quality = quality
	}
	if request.OutputFormat != "" {
		format := normalizeOutputFormat(request.OutputFormat)
		if format == "" {
			return GenerateRequest{}, fmt.Errorf("unsupported image format: %s", request.OutputFormat)
		}
		request.OutputFormat = format
	}
	return request, nil
}

func promptSummary(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	sum := sha256.Sum256([]byte(prompt))
	preview := prompt
	const limit = 160
	if len(preview) > limit {
		preview, _ = truncateUTF8Bytes(preview, limit)
	}
	return fmt.Sprintf("chars=%d bytes=%d hash=sha256:%x preview=%q", utf8.RuneCountInString(prompt), len(prompt), sum[:8], preview)
}

func truncateUTF8Bytes(value string, limit int) (string, bool) {
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	out := value[:limit]
	for !utf8.ValidString(out) && len(out) > 0 {
		out = out[:len(out)-1]
	}
	return out + "...", true
}

func normalizeQuality(quality string) string {
	switch strings.ToLower(strings.TrimSpace(quality)) {
	case "auto", "standard", "hd", "low", "medium", "high":
		return strings.ToLower(strings.TrimSpace(quality))
	default:
		return ""
	}
}

func normalizeOutputFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "png", "jpeg", "webp":
		return strings.ToLower(strings.TrimSpace(format))
	case "jpg":
		return "jpeg"
	default:
		return ""
	}
}
