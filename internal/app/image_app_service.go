package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	imagegen "denova/internal/image/generation"
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
	runtime, err := s.acquireWorkspaceRuntime(ctx)
	if err != nil {
		return ImageGenerateResult{}, err
	}
	defer runtime.Release()
	result, err := imagegen.NewService().Generate(runtime.Context(), &runtime.cfg, request)
	if err != nil {
		return ImageGenerateResult{}, err
	}
	if err := runtime.Context().Err(); err != nil {
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
		if err := runtime.Context().Err(); err != nil {
			return ImageGenerateResult{}, err
		}
		if err := runtime.bookService.WriteBinaryFile(relPath, image.Data); err != nil {
			return ImageGenerateResult{}, fmt.Errorf("保存生成图像失败: %w", err)
		}
		slog.InfoContext(ctx, fmt.Sprintf("[imagegen] saved image path=%s bytes=%d mime=%s", relPath, len(image.Data), image.MIMEType))
		saved.Images = append(saved.Images, SavedGeneratedImage{
			Path:          relPath,
			MIMEType:      image.MIMEType,
			SizeBytes:     len(image.Data),
			RevisedPrompt: image.RevisedPrompt,
		})
	}
	return saved, nil
}

func generatedImagePath(index int, extension string) (string, error) {
	if extension == "" {
		return "", fmt.Errorf("无法保存未知格式图像")
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
