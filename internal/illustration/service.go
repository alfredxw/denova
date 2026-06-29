package illustration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"nova/config"
	"nova/internal/book"
	"nova/internal/imagegen"
)

const (
	ResultSchema = "chapter_illustration.v1"
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
	ChapterPath  string
	Prompt       string
	AltText      string
	ProfileID    string
	Size         string
	Quality      string
	OutputFormat string
}

type Result struct {
	Schema       string `json:"schema"`
	ChapterPath  string `json:"chapter_path"`
	ImagePath    string `json:"image_path"`
	MetaPath     string `json:"meta_path"`
	Markdown     string `json:"markdown"`
	AltText      string `json:"alt_text"`
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
	ChapterPath   string `json:"chapter_path"`
	Prompt        string `json:"prompt"`
	RevisedPrompt string `json:"revised_prompt,omitempty"`
	ImagePath     string `json:"image_path"`
	MetaPath      string `json:"meta_path"`
	Markdown      string `json:"markdown"`
	AltText       string `json:"alt_text"`
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
	chapterPath := filepath.ToSlash(strings.TrimSpace(request.ChapterPath))
	if chapterPath == "" {
		return Result{}, fmt.Errorf("chapter_path 不能为空")
	}
	if _, err := bookService.FileRevision(chapterPath); err != nil {
		return Result{}, fmt.Errorf("读取章节路径失败: %w", err)
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
		"illustrations",
		chapterSlug(chapterPath),
		fmt.Sprintf("%s-%s", createdAt.Format("20060102-150405"), s.suffix()),
	))
	imagePath := filepath.ToSlash(filepath.Join(dir, "image."+ext))
	metaPath := filepath.ToSlash(filepath.Join(dir, "meta.json"))
	altText := strings.TrimSpace(request.AltText)
	if altText == "" {
		altText = defaultAltText(chapterPath)
	}
	markdown := fmt.Sprintf("![%s](%s)", escapeMarkdownAlt(altText), imagePath)

	if err := bookService.WriteBinaryFile(imagePath, image.Data); err != nil {
		return Result{}, fmt.Errorf("保存章节插画失败: %w", err)
	}
	result := Result{
		Schema:        ResultSchema,
		ChapterPath:   chapterPath,
		ImagePath:     imagePath,
		MetaPath:      metaPath,
		Markdown:      markdown,
		AltText:       altText,
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
		ChapterPath:   result.ChapterPath,
		Prompt:        prompt,
		RevisedPrompt: result.RevisedPrompt,
		ImagePath:     result.ImagePath,
		MetaPath:      result.MetaPath,
		Markdown:      result.Markdown,
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
		return Result{}, fmt.Errorf("保存章节插画元数据失败: %w", err)
	}
	return result, nil
}

func chapterSlug(path string) string {
	base := filepath.Base(filepath.FromSlash(path))
	ext := filepath.Ext(base)
	name := strings.TrimSpace(strings.TrimSuffix(base, ext))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			lastDash = false
		case r == '-' || r == '_':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	slug := strings.Trim(b.String(), "-_")
	if slug == "" {
		return "chapter"
	}
	return slug
}

func defaultAltText(chapterPath string) string {
	base := strings.TrimSuffix(filepath.Base(filepath.FromSlash(chapterPath)), filepath.Ext(chapterPath))
	if strings.TrimSpace(base) == "" {
		return "章节插画"
	}
	return "章节插画：" + base
}

func escapeMarkdownAlt(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\\", "\\\\"), "]", "\\]")
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

func randomSuffix() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
