package interactiveimage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"nova/config"
	"nova/internal/book"
	"nova/internal/imagegen"
)

const (
	ResultSchema = "interactive_image.v1"
	sourceTool   = "generate_image"
)

type ImageGenerator interface {
	Generate(ctx context.Context, cfg *config.Config, request imagegen.GenerateRequest) (imagegen.Result, error)
}

type Service struct {
	generator ImageGenerator
	now       func() time.Time
	suffix    func() string
}

type GenerateRequest struct {
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

type Result struct {
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

type Meta struct {
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

func NewService() *Service {
	return NewServiceWithGenerator(imagegen.NewService())
}

func NewServiceWithGenerator(generator ImageGenerator) *Service {
	return &Service{
		generator: generator,
		now:       time.Now,
		suffix:    randomSuffix,
	}
}

func (s *Service) Generate(ctx context.Context, cfg *config.Config, bookService *book.Service, request GenerateRequest) (Result, error) {
	if s == nil {
		s = NewService()
	}
	if s.generator == nil {
		s.generator = imagegen.NewService()
	}
	if cfg == nil {
		return Result{}, fmt.Errorf("运行配置不可用")
	}
	if bookService == nil || strings.TrimSpace(bookService.Workspace()) == "" {
		return Result{}, fmt.Errorf("workspace 不可用")
	}
	storyID := safePathSegment(request.StoryID)
	branchID := safePathSegment(request.BranchID)
	turnID := safePathSegment(request.TurnID)
	if storyID == "" || branchID == "" || turnID == "" {
		return Result{}, fmt.Errorf("互动图像缺少 story_id、branch_id 或 turn_id")
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return Result{}, imagegen.ErrPromptRequired
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
		return Result{}, err
	}
	if len(generated.Images) == 0 {
		return Result{}, fmt.Errorf("图像模型未返回图像")
	}
	image := generated.Images[0]
	if len(image.Data) == 0 {
		return Result{}, fmt.Errorf("图像模型返回了空图像")
	}
	ext := normalizeImageExtension(image.Extension, generated.OutputFormat, request.OutputFormat)
	if ext == "" {
		return Result{}, fmt.Errorf("无法识别图像格式")
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
		return Result{}, fmt.Errorf("保存互动图像失败: %w", err)
	}

	result := Result{
		Schema:        ResultSchema,
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
	meta := Meta{
		Schema:        ResultSchema,
		Source:        sourceTool,
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
		return Result{}, err
	}
	if err := bookService.WriteFile(metaPath, string(data)+"\n"); err != nil {
		return Result{}, fmt.Errorf("保存互动图像元数据失败: %w", err)
	}
	return result, nil
}

func normalizeImageExtension(values ...string) string {
	for _, value := range values {
		value = strings.ToLower(strings.Trim(strings.TrimSpace(value), "."))
		switch value {
		case "jpg":
			return "jpeg"
		case "jpeg", "png":
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func safePathSegment(value string) string {
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

func randomSuffix() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
