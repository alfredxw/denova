package bookcover

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nova/config"
	"nova/internal/book"
	"nova/internal/imagegen"
)

const (
	ResultSchema       = "book_cover.v1"
	CoverPath          = "assets/image/cover.png"
	defaultCoverSize   = "1728x2304"
	defaultCoverFormat = "png"
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
	Title             string
	Description       string
	Instruction       string
	ImagePresetID     string
	ImagePresetPrompt string
	ProfileID         string
}

type Result struct {
	Schema         string `json:"schema"`
	CoverPath      string `json:"cover_path"`
	SourcePath     string `json:"source_path"`
	MetaPath       string `json:"meta_path"`
	BackupPath     string `json:"backup_path,omitempty"`
	CoverUpdatedAt string `json:"cover_updated_at"`
	ImagePresetID  string `json:"image_preset_id,omitempty"`
	ProfileID      string `json:"profile_id"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	OutputFormat   string `json:"output_format,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`

	RevisedPrompt string `json:"revised_prompt,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
	SizeBytes     int    `json:"size_bytes,omitempty"`
}

type Meta struct {
	Schema         string `json:"schema"`
	Source         string `json:"source"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	Instruction    string `json:"instruction,omitempty"`
	ImagePresetID  string `json:"image_preset_id,omitempty"`
	Prompt         string `json:"prompt"`
	RevisedPrompt  string `json:"revised_prompt,omitempty"`
	CoverPath      string `json:"cover_path"`
	SourcePath     string `json:"source_path"`
	MetaPath       string `json:"meta_path"`
	BackupPath     string `json:"backup_path,omitempty"`
	ProfileID      string `json:"profile_id"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Size           string `json:"size,omitempty"`
	Quality        string `json:"quality,omitempty"`
	OutputFormat   string `json:"output_format,omitempty"`
	MIMEType       string `json:"mime_type,omitempty"`
	SizeBytes      int    `json:"size_bytes,omitempty"`
	CoverUpdatedAt string `json:"cover_updated_at"`
	CreatedAt      string `json:"created_at"`
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
	prompt := BuildPrompt(request)
	if prompt == "" {
		return Result{}, imagegen.ErrPromptRequired
	}

	generated, err := s.generator.Generate(ctx, cfg, imagegen.GenerateRequest{
		ProfileID:    strings.TrimSpace(request.ProfileID),
		Prompt:       prompt,
		N:            1,
		Size:         defaultCoverSize,
		OutputFormat: defaultCoverFormat,
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
	ext := normalizeImageExtension(image.Extension, generated.OutputFormat, defaultCoverFormat)
	if ext == "" {
		return Result{}, fmt.Errorf("无法识别图像格式")
	}

	createdAt := s.now().UTC()
	dir := filepath.ToSlash(filepath.Join(
		"assets",
		"image",
		"covers",
		fmt.Sprintf("%s-%s", createdAt.Format("20060102-150405"), s.suffix()),
	))
	sourcePath := filepath.ToSlash(filepath.Join(dir, "cover."+ext))
	metaPath := filepath.ToSlash(filepath.Join(dir, "meta.json"))

	if err := bookService.WriteBinaryFile(sourcePath, image.Data); err != nil {
		return Result{}, fmt.Errorf("保存封面原图失败: %w", err)
	}

	backupPath, err := backupExistingCover(bookService, createdAt, ext)
	if err != nil {
		return Result{}, err
	}
	if err := bookService.WriteBinaryFile(CoverPath, image.Data); err != nil {
		return Result{}, fmt.Errorf("写入展示封面失败: %w", err)
	}
	coverUpdatedAt := createdAt.Format(time.RFC3339Nano)
	if info, statErr := os.Stat(filepath.Join(bookService.Workspace(), filepath.FromSlash(CoverPath))); statErr == nil {
		coverUpdatedAt = info.ModTime().UTC().Format(time.RFC3339Nano)
	}

	result := Result{
		Schema:         ResultSchema,
		CoverPath:      CoverPath,
		SourcePath:     sourcePath,
		MetaPath:       metaPath,
		BackupPath:     backupPath,
		CoverUpdatedAt: coverUpdatedAt,
		ImagePresetID:  strings.TrimSpace(request.ImagePresetID),
		ProfileID:      generated.ProfileID,
		Provider:       generated.Provider,
		Model:          generated.Model,
		Size:           generated.Size,
		Quality:        generated.Quality,
		OutputFormat:   firstNonEmpty(generated.OutputFormat, ext),
		CreatedAt:      createdAt.Format(time.RFC3339),
		RevisedPrompt:  image.RevisedPrompt,
		MIMEType:       image.MIMEType,
		SizeBytes:      len(image.Data),
	}
	meta := Meta{
		Schema:         ResultSchema,
		Source:         "book_cover_generate",
		Title:          trimRunes(request.Title, 200),
		Description:    trimRunes(request.Description, 2000),
		Instruction:    trimRunes(request.Instruction, 1000),
		ImagePresetID:  result.ImagePresetID,
		Prompt:         prompt,
		RevisedPrompt:  result.RevisedPrompt,
		CoverPath:      result.CoverPath,
		SourcePath:     result.SourcePath,
		MetaPath:       result.MetaPath,
		BackupPath:     result.BackupPath,
		ProfileID:      result.ProfileID,
		Provider:       result.Provider,
		Model:          result.Model,
		Size:           result.Size,
		Quality:        result.Quality,
		OutputFormat:   result.OutputFormat,
		MIMEType:       result.MIMEType,
		SizeBytes:      result.SizeBytes,
		CoverUpdatedAt: result.CoverUpdatedAt,
		CreatedAt:      result.CreatedAt,
	}
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return Result{}, err
	}
	if err := bookService.WriteFile(metaPath, string(data)+"\n"); err != nil {
		return Result{}, fmt.Errorf("保存封面元数据失败: %w", err)
	}
	return result, nil
}

func BuildPrompt(request GenerateRequest) string {
	title := trimRunes(request.Title, 200)
	description := trimRunes(request.Description, 2000)
	instruction := trimRunes(request.Instruction, 1000)
	preset := strings.TrimSpace(request.ImagePresetPrompt)
	var sb strings.Builder
	if preset != "" {
		sb.WriteString("# 图像风格要求\n\n")
		sb.WriteString(preset)
		sb.WriteString("\n\n")
	}
	sb.WriteString("# 本次封面请求\n\n")
	sb.WriteString("为一本小说生成竖版书籍封面视觉图，画面可作为书架封面。构图必须清晰、有强主体和可识别的题材氛围；不要生成任何文字、书名、作者名、水印、logo、UI 面板或二维码。\n\n")
	if title != "" {
		sb.WriteString("书名：")
		sb.WriteString(title)
		sb.WriteString("\n")
	}
	if description != "" {
		sb.WriteString("简介：")
		sb.WriteString(description)
		sb.WriteString("\n")
	}
	if instruction != "" {
		sb.WriteString("用户生成要求：")
		sb.WriteString(instruction)
		sb.WriteString("\n")
	}
	return strings.TrimSpace(sb.String())
}

func backupExistingCover(bookService *book.Service, createdAt time.Time, ext string) (string, error) {
	absCover, err := book.SafePath(bookService.Workspace(), CoverPath)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(absCover)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("读取旧封面失败: %w", err)
	}
	if len(data) == 0 {
		return "", nil
	}
	backupPath := filepath.ToSlash(filepath.Join(
		"assets",
		"image",
		"covers",
		"backups",
		fmt.Sprintf("%s-previous.%s", createdAt.Format("20060102-150405"), ext),
	))
	if err := bookService.WriteBinaryFile(backupPath, data); err != nil {
		return "", fmt.Errorf("备份旧封面失败: %w", err)
	}
	return backupPath, nil
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

func trimRunes(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func randomSuffix() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf[:])
}
