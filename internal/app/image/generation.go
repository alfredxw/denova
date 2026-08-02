package imageapp

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

type GenerateResult struct {
	ProfileID    string       `json:"profile_id"`
	Provider     string       `json:"provider"`
	Model        string       `json:"model"`
	Created      int64        `json:"created,omitempty"`
	Size         string       `json:"size,omitempty"`
	Quality      string       `json:"quality,omitempty"`
	OutputFormat string       `json:"output_format,omitempty"`
	Images       []SavedImage `json:"images"`
}

type SavedImage struct {
	Path          string `json:"path"`
	MIMEType      string `json:"mime_type"`
	SizeBytes     int    `json:"size_bytes"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
}

func (service *Service) Generate(ctx context.Context, request imagegen.GenerateRequest) (GenerateResult, error) {
	runtime, err := service.AcquireRuntime(ctx, "")
	if err != nil {
		return GenerateResult{}, err
	}
	defer runtime.Release()
	result, err := imagegen.NewService().Generate(runtime.Context(), &runtime.Config, request)
	if err != nil {
		return GenerateResult{}, err
	}
	if err := runtime.Context().Err(); err != nil {
		return GenerateResult{}, err
	}
	saved := GenerateResult{
		ProfileID:    result.ProfileID,
		Provider:     result.Provider,
		Model:        result.Model,
		Created:      result.Created,
		Size:         result.Size,
		Quality:      result.Quality,
		OutputFormat: result.OutputFormat,
		Images:       make([]SavedImage, 0, len(result.Images)),
	}
	for index, image := range result.Images {
		relPath, err := generatedImagePath(index, image.Extension)
		if err != nil {
			return GenerateResult{}, err
		}
		if err := runtime.Context().Err(); err != nil {
			return GenerateResult{}, err
		}
		if err := runtime.BookService.WriteBinaryFile(relPath, image.Data); err != nil {
			return GenerateResult{}, fmt.Errorf("save generated image: %w", err)
		}
		slog.InfoContext(ctx, fmt.Sprintf("[imagegen] saved image path=%s bytes=%d mime=%s", relPath, len(image.Data), image.MIMEType))
		saved.Images = append(saved.Images, SavedImage{
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
		return "", fmt.Errorf("cannot save an image with an unknown format")
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
