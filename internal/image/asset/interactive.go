package asset

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"denova/config"
	"denova/internal/book"
	imagegen "denova/internal/image/generation"
)

const (
	InteractiveResultSchema = "interactive_image.v1"
	interactiveSourceTool   = "generate_image"
)

type InteractiveGenerateRequest struct {
	StoryID      string
	BranchID     string
	TurnID       string
	Prompt       string
	AltText      string
	ProfileID    string
	Size         string
	Quality      string
	OutputFormat string
}

type InteractiveResult struct {
	Schema       string `json:"schema"`
	StoryID      string `json:"story_id"`
	BranchID     string `json:"branch_id"`
	TurnID       string `json:"turn_id"`
	ImagePath    string `json:"image_path"`
	MetaPath     string `json:"meta_path"`
	AltText      string `json:"alt_text,omitempty"`
	ProfileID    string `json:"profile_id"`
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Size         string `json:"size,omitempty"`
	Quality      string `json:"quality,omitempty"`
	OutputFormat string `json:"output_format,omitempty"`
	CreatedAt    string `json:"created_at,omitempty"`

	RevisedPrompt string `json:"revised_prompt,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
	SizeBytes     int    `json:"size_bytes,omitempty"`
}

type interactiveMeta struct {
	Schema        string `json:"schema"`
	Source        string `json:"source"`
	StoryID       string `json:"story_id"`
	BranchID      string `json:"branch_id"`
	TurnID        string `json:"turn_id"`
	Prompt        string `json:"prompt"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	ImagePath     string `json:"image_path"`
	MetaPath      string `json:"meta_path"`
	AltText       string `json:"alt_text,omitempty"`
	ProfileID     string `json:"profile_id"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	Size          string `json:"size,omitempty"`
	Quality       string `json:"quality,omitempty"`
	OutputFormat  string `json:"output_format,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
	SizeBytes     int    `json:"size_bytes,omitempty"`
	CreatedAt     string `json:"created_at"`
}

func (s *Service) GenerateInteractive(ctx context.Context, cfg *config.Config, bookService *book.Service, request InteractiveGenerateRequest) (InteractiveResult, error) {
	if s == nil {
		s = NewService()
	}
	if s.generator == nil {
		s.generator = imagegen.NewService()
	}
	if cfg == nil {
		return InteractiveResult{}, fmt.Errorf("运行配置不可用")
	}
	if bookService == nil || strings.TrimSpace(bookService.Workspace()) == "" {
		return InteractiveResult{}, fmt.Errorf("workspace 不可用")
	}
	storyID := interactiveSafePathSegment(request.StoryID)
	branchID := interactiveSafePathSegment(request.BranchID)
	turnID := interactiveSafePathSegment(request.TurnID)
	if storyID == "" || branchID == "" || turnID == "" {
		return InteractiveResult{}, fmt.Errorf("互动图像缺少 story_id、branch_id 或 turn_id")
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return InteractiveResult{}, imagegen.ErrPromptRequired
	}
	generated, err := s.generator.Generate(ctx, cfg, imagegen.GenerateRequest{
		ProfileID:    strings.TrimSpace(request.ProfileID),
		Prompt:       prompt,
		N:            1,
		Size:         strings.TrimSpace(request.Size),
		Quality:      strings.TrimSpace(request.Quality),
		OutputFormat: strings.TrimSpace(request.OutputFormat),
	})
	if err != nil {
		return InteractiveResult{}, err
	}
	if len(generated.Images) == 0 {
		return InteractiveResult{}, fmt.Errorf("图像模型未返回图像")
	}
	image := generated.Images[0]
	if len(image.Data) == 0 {
		return InteractiveResult{}, fmt.Errorf("图像模型返回了空图像")
	}
	ext := normalizeImageExtension(image.Extension, generated.OutputFormat, request.OutputFormat)
	if ext == "" {
		return InteractiveResult{}, fmt.Errorf("无法识别图像格式")
	}

	createdAt := s.now().UTC()
	dir := filepath.ToSlash(filepath.Join(
		"assets",
		"interactive",
		"images",
		storyID,
		branchID,
		turnID,
		fmt.Sprintf("%s-%s", createdAt.Format("20060102-150405"), s.suffix()),
	))
	imagePath := filepath.ToSlash(filepath.Join(dir, "image."+ext))
	metaPath := filepath.ToSlash(filepath.Join(dir, "meta.json"))
	if err := bookService.WriteBinaryFile(imagePath, image.Data); err != nil {
		return InteractiveResult{}, fmt.Errorf("保存互动图像失败: %w", err)
	}

	result := InteractiveResult{
		Schema:        InteractiveResultSchema,
		StoryID:       storyID,
		BranchID:      branchID,
		TurnID:        turnID,
		ImagePath:     imagePath,
		MetaPath:      metaPath,
		AltText:       strings.TrimSpace(request.AltText),
		ProfileID:     generated.ProfileID,
		Provider:      generated.Provider,
		Model:         generated.Model,
		Size:          generated.Size,
		Quality:       generated.Quality,
		OutputFormat:  firstNonEmpty(generated.OutputFormat, ext),
		CreatedAt:     createdAt.Format(time.RFC3339),
		RevisedPrompt: image.RevisedPrompt,
		MIMEType:      image.MIMEType,
		SizeBytes:     len(image.Data),
	}
	meta := interactiveMeta{
		Schema:        InteractiveResultSchema,
		Source:        interactiveSourceTool,
		StoryID:       result.StoryID,
		BranchID:      result.BranchID,
		TurnID:        result.TurnID,
		Prompt:        prompt,
		RevisedPrompt: result.RevisedPrompt,
		ImagePath:     result.ImagePath,
		MetaPath:      result.MetaPath,
		AltText:       result.AltText,
		ProfileID:     result.ProfileID,
		Provider:      result.Provider,
		Model:         result.Model,
		Size:          result.Size,
		Quality:       result.Quality,
		OutputFormat:  result.OutputFormat,
		MIMEType:      result.MIMEType,
		SizeBytes:     result.SizeBytes,
		CreatedAt:     result.CreatedAt,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return InteractiveResult{}, err
	}
	if err := bookService.WriteFile(metaPath, string(data)+"\n"); err != nil {
		return InteractiveResult{}, fmt.Errorf("保存互动图像元数据失败: %w", err)
	}
	return result, nil
}

func interactiveSafePathSegment(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			if !lastDash && b.Len() > 0 {
				b.WriteRune(r)
				lastDash = true
			}
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-_")
}
