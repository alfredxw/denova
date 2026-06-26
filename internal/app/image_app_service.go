package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"nova/config"
	"nova/internal/book"
	"nova/internal/imagegen"
)

type ImageAppService struct {
	app *App
}

type ImageGenerateResult struct {
	ProfileID    string                `json:"profile_id"`
	Provider     string                `json:"provider"`
	Model        string                `json:"model"`
	Created      int64                 `json:"created,omitempty"`
	Size         string                `json:"size,omitempty"`
	Quality      string                `json:"quality,omitempty"`
	OutputFormat string                `json:"output_format,omitempty"`
	Images       []SavedGeneratedImage `json:"images"`
}

type SavedGeneratedImage struct {
	Path          string `json:"path"`
	MIMEType      string `json:"mime_type"`
	SizeBytes     int    `json:"size_bytes"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

func (a *App) GenerateImage(ctx context.Context, request imagegen.GenerateRequest) (ImageGenerateResult, error) {
	return a.images().Generate(ctx, request)
}

func (s *ImageAppService) Generate(ctx context.Context, request imagegen.GenerateRequest) (ImageGenerateResult, error) {
	cfg, bookService, err := s.runtimeSnapshot()
	if err != nil {
		return ImageGenerateResult{}, err
	}
	result, err := imagegen.NewService().Generate(ctx, &cfg, request)
	if err != nil {
		return ImageGenerateResult{}, err
	}
	saved := ImageGenerateResult{
		ProfileID:    result.ProfileID,
		Provider:     result.Provider,
		Model:        result.Model,
		Created:      result.Created,
		Size:         result.Size,
		Quality:      result.Quality,
		OutputFormat: result.OutputFormat,
		Images:       make([]SavedGeneratedImage, 0, len(result.Images)),
	}
	for index, image := range result.Images {
		relPath, err := generatedImagePath(index, image.Extension)
		if err != nil {
			return ImageGenerateResult{}, err
		}
		if err := bookService.WriteBinaryFile(relPath, image.Data); err != nil {
			return ImageGenerateResult{}, fmt.Errorf("保存生成图片失败: %w", err)
		}
		log.Printf("[imagegen] saved image path=%s bytes=%d mime=%s", relPath, len(image.Data), image.MIMEType)
		saved.Images = append(saved.Images, SavedGeneratedImage{
			Path:          relPath,
			MIMEType:      image.MIMEType,
			SizeBytes:     len(image.Data),
			RevisedPrompt: image.RevisedPrompt,
		})
	}
	return saved, nil
}

func (s *ImageAppService) runtimeSnapshot() (config.Config, *book.Service, error) {
	app := s.app
	app.mu.RLock()
	defer app.mu.RUnlock()
	if app.workspace == "" || app.bookService == nil {
		return config.Config{}, nil, ErrNoWorkspace
	}
	if app.cfg == nil {
		return config.Config{}, nil, fmt.Errorf("运行配置未初始化")
	}
	cfg := *app.cfg
	return cfg, app.bookService, nil
}

func generatedImagePath(index int, extension string) (string, error) {
	if extension == "" {
		return "", fmt.Errorf("无法保存未知格式图片")
	}
	return filepath.ToSlash(filepath.Join("assets", "image", "generated", fmt.Sprintf("%s-%s-%02d.%s", time.Now().Format("20060102-150405"), randomImageSuffix(), index+1, extension))), nil
}

func randomImageSuffix() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
