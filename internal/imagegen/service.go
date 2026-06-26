package imagegen

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"nova/config"
)

var (
	ErrPromptRequired       = errors.New("图片提示词不能为空")
	ErrUnsupportedProvider  = errors.New("不支持的图片 API provider")
	ErrImageCountOutOfRange = errors.New("图片数量必须在 1 到 10 之间")
)

type Service struct {
	adapters map[string]Adapter
}

func NewService() *Service {
	return &Service{adapters: map[string]Adapter{
		config.DefaultImageAPIProvider: NewOpenAIAdapter(nil),
	}}
}

func NewServiceWithAdapters(adapters map[string]Adapter) *Service {
	out := make(map[string]Adapter, len(adapters))
	for key, adapter := range adapters {
		out[strings.TrimSpace(key)] = adapter
	}
	return &Service{adapters: out}
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

	adapter := s.adapters[profile.Provider]
	if adapter == nil {
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedProvider, profile.Provider)
	}
	log.Printf("[imagegen] generate begin provider=%s profile_id=%s model=%q size=%q quality=%q format=%q n=%d prompt_chars=%d", profile.Provider, profile.ProfileID, profile.OpenAIModel, request.Size, request.Quality, request.OutputFormat, request.N, len([]rune(request.Prompt)))
	result, err := adapter.Generate(ctx, profile, request)
	if err != nil {
		log.Printf("[imagegen] generate failed provider=%s profile_id=%s model=%q err=%v", profile.Provider, profile.ProfileID, profile.OpenAIModel, err)
		return Result{}, err
	}
	log.Printf("[imagegen] generate done provider=%s profile_id=%s model=%q images=%d", profile.Provider, profile.ProfileID, profile.OpenAIModel, len(result.Images))
	return result, nil
}

func normalizeRequestOptions(request GenerateRequest) (GenerateRequest, error) {
	if request.Size != "" {
		size := normalizeSize(request.Size)
		if size == "" {
			return GenerateRequest{}, fmt.Errorf("不支持的图片尺寸: %s", request.Size)
		}
		request.Size = size
	}
	if request.Quality != "" {
		quality := normalizeQuality(request.Quality)
		if quality == "" {
			return GenerateRequest{}, fmt.Errorf("不支持的图片质量: %s", request.Quality)
		}
		request.Quality = quality
	}
	if request.OutputFormat != "" {
		format := normalizeOutputFormat(request.OutputFormat)
		if format == "" {
			return GenerateRequest{}, fmt.Errorf("不支持的图片格式: %s", request.OutputFormat)
		}
		request.OutputFormat = format
	}
	return request, nil
}

func normalizeSize(size string) string {
	switch strings.TrimSpace(size) {
	case "auto", "256x256", "512x512", "1024x1024", "1536x1024", "1024x1536", "1792x1024", "1024x1792":
		return strings.TrimSpace(size)
	default:
		return ""
	}
}

func normalizeQuality(quality string) string {
	switch strings.TrimSpace(quality) {
	case "auto", "standard", "hd", "low", "medium", "high":
		return strings.TrimSpace(quality)
	default:
		return ""
	}
}

func normalizeOutputFormat(format string) string {
	switch strings.TrimSpace(format) {
	case "png", "jpeg", "webp":
		return strings.TrimSpace(format)
	default:
		return ""
	}
}
