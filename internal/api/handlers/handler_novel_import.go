package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	"denova/internal/api/sse"
	novaApp "denova/internal/app"
	"denova/internal/book"
)

// MaxNovelImportUploadBytes limits txt/md novel imports.
const MaxNovelImportUploadBytes int64 = 64 * 1024 * 1024

type novelImportProgressEvent struct {
	Step string `json:"step"`
}

type novelImportErrorEvent struct {
	Error string `json:"error"`
}

// HandlePreviewNovelImport POST /api/books/import-novel/preview — 预览 txt/md 小说章节，不写入 workspace。
func (h *Handlers) HandlePreviewNovelImport(ctx context.Context, c *app.RequestContext) {
	filename, data, ok := readNovelImportUpload(c)
	if !ok {
		return
	}
	opts := h.novelImportOptions(ctx, c)
	log.Printf("[api] 小说导入预览 begin filename=%q size=%d sample_chars=%d split_strategy=%q has_split_regex=%t", filename, len(data), opts.SampleChars, opts.SplitStrategy, opts.SplitRegex != "")
	preview, err := book.PreviewNovelImport(filename, data, opts)
	if err != nil {
		log.Printf("[api] 小说导入预览 failed filename=%q err=%v", filename, err)
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.parseFailed", "detail", err.Error())
		return
	}
	localizeNovelImportWarnings(c, &preview)
	log.Printf("[api] 小说导入预览 done filename=%q strategy=%s regex=%q chapters=%d warnings=%v", filename, preview.SplitStrategy, preview.SplitRegex, preview.ChapterCount, preview.Warnings)
	writeJSON(c, consts.StatusOK, preview)
}

// HandlePreviewNovelImportStream POST /api/books/import-novel/preview/stream — 流式预览 txt/md 小说章节。
func (h *Handlers) HandlePreviewNovelImportStream(ctx context.Context, c *app.RequestContext) {
	filename, data, ok := readNovelImportUpload(c)
	if !ok {
		return
	}
	opts := h.novelImportOptions(ctx, c)
	localizer := requestLocalizer(c)
	log.Printf("[api] 小说导入流式预览 begin filename=%q size=%d sample_chars=%d split_strategy=%q has_split_regex=%t", filename, len(data), opts.SampleChars, opts.SplitStrategy, opts.SplitRegex != "")

	c.Response.Header.Set("Content-Type", "text/event-stream")
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.ImmediateHeaderFlush = true

	pr, pw := io.Pipe()
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("[api] 小说导入流式预览 panic recovered filename=%q err=%v", filename, recovered)
				_ = writeNovelImportPreviewEvent(pw, "error", novelImportErrorEvent{Error: fmt.Sprint(recovered)})
			}
			_ = pw.Close()
		}()

		if err := writeNovelImportPreviewProgress(pw, "uploaded"); err != nil {
			return
		}
		streamOpts := opts
		if streamOpts.SplitRegex == "" && streamOpts.SplitStrategy != book.NovelImportSplitStrategyBuiltin {
			streamOpts.InferSplitRegex = func(sample string) (string, error) {
				if err := writeNovelImportPreviewProgress(pw, "agent_start"); err != nil {
					return "", err
				}
				inferCtx, cancel := context.WithTimeout(ctx, novaApp.NovelImportToolAgentTimeout)
				defer cancel()
				regex, err := h.app.InferNovelSplitRegex(inferCtx, sample)
				if err != nil {
					_ = writeNovelImportPreviewProgress(pw, "agent_error")
					return "", err
				}
				_ = writeNovelImportPreviewProgress(pw, "agent_done")
				return regex, nil
			}
		}
		if err := writeNovelImportPreviewProgress(pw, "split_start"); err != nil {
			return
		}
		preview, err := book.PreviewNovelImport(filename, data, streamOpts)
		if err != nil {
			log.Printf("[api] 小说导入流式预览 failed filename=%q err=%v", filename, err)
			_ = writeNovelImportPreviewEvent(pw, "error", novelImportErrorEvent{Error: err.Error()})
			return
		}
		localizeNovelImportWarningsWith(localizer.T, &preview)
		log.Printf("[api] 小说导入流式预览 done filename=%q strategy=%s regex=%q chapters=%d warnings=%v", filename, preview.SplitStrategy, preview.SplitRegex, preview.ChapterCount, preview.Warnings)
		if err := writeNovelImportPreviewEvent(pw, "preview", preview); err != nil {
			return
		}
		_ = writeNovelImportPreviewEvent(pw, "done", map[string]string{"status": "ok"})
	}()
	c.Response.SetBodyStream(pr, -1)
}

// HandleNovelImport POST /api/books/import-novel — 导入 txt/md 小说为新书并写入章节。
func (h *Handlers) HandleNovelImport(ctx context.Context, c *app.RequestContext) {
	filename, data, ok := readNovelImportUpload(c)
	if !ok {
		return
	}
	opts := h.novelImportOptions(ctx, c)
	title := strings.TrimSpace(string(c.FormValue("book_title")))
	author := strings.TrimSpace(string(c.FormValue("author")))
	description := strings.TrimSpace(string(c.FormValue("description")))

	log.Printf("[api] 小说导入任务开始 filename=%q size=%d title=%q split_strategy=%q has_split_regex=%t", filename, len(data), title, opts.SplitStrategy, opts.SplitRegex != "")
	task, err := h.app.StartNovelImportTask(ctx, novaApp.NovelImportTaskRequest{
		Filename:    filename,
		Data:        data,
		Title:       title,
		Author:      author,
		Description: description,
		Options:     opts,
	})
	if err != nil {
		writeError(c, consts.StatusInternalServerError, err.Error())
		return
	}
	sse.StreamTask(c, task)
}

func (h *Handlers) novelImportOptions(ctx context.Context, c *app.RequestContext) book.NovelImportOptions {
	opts := book.NovelImportOptions{
		SplitRegex:    strings.TrimSpace(string(c.FormValue("split_regex"))),
		SplitStrategy: strings.TrimSpace(string(c.FormValue("split_strategy"))),
	}
	if raw := strings.TrimSpace(string(c.FormValue("sample_chars"))); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil {
			opts.SampleChars = value
		}
	}
	if opts.SplitRegex == "" && opts.SplitStrategy != book.NovelImportSplitStrategyBuiltin {
		opts.InferSplitRegex = func(sample string) (string, error) {
			inferCtx, cancel := context.WithTimeout(ctx, novaApp.NovelImportToolAgentTimeout)
			defer cancel()
			return h.app.InferNovelSplitRegex(inferCtx, sample)
		}
	}
	return opts
}

func writeNovelImportPreviewProgress(w io.Writer, step string) error {
	return writeNovelImportPreviewEvent(w, "progress", novelImportProgressEvent{Step: step})
}

func writeNovelImportPreviewEvent(w io.Writer, eventType string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, payload)
	return err
}

func readNovelImportUpload(c *app.RequestContext) (string, []byte, bool) {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.uploadRequired")
		return "", nil, false
	}
	if fileHeader.Size > MaxNovelImportUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.tooLarge")
		return "", nil, false
	}

	file, err := fileHeader.Open()
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.readFailed", "detail", err.Error())
		return "", nil, false
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaxNovelImportUploadBytes+1))
	if err != nil {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.readFailed", "detail", err.Error())
		return "", nil, false
	}
	if int64(len(data)) > MaxNovelImportUploadBytes {
		writeErrorKey(c, consts.StatusBadRequest, "api.novelImport.tooLarge")
		return "", nil, false
	}
	return fileHeader.Filename, data, true
}

func localizeNovelImportWarnings(c *app.RequestContext, preview *book.NovelImportPreview) {
	localizeNovelImportWarningsWith(func(key string, args ...any) string {
		return messageKey(c, key, args...)
	}, preview)
}

func localizeNovelImportWarningsWith(message func(key string, args ...any) string, preview *book.NovelImportPreview) {
	for i, warning := range preview.Warnings {
		switch warning {
		case book.NovelImportSingleChapterWarning:
			preview.Warnings[i] = message("api.novelImport.singleChapterWarning")
		case book.NovelImportAgentFallbackWarning:
			preview.Warnings[i] = message("api.novelImport.agentFallbackWarning")
		case book.NovelImportRegexFewChaptersWarning:
			preview.Warnings[i] = message("api.novelImport.regexFewChaptersWarning")
		default:
			if strings.HasPrefix(warning, book.NovelImportRegexFallbackWarningPrefix) {
				preview.Warnings[i] = message("api.novelImport.regexFallbackWarning", "detail", strings.TrimPrefix(warning, book.NovelImportRegexFallbackWarningPrefix))
			}
		}
	}
}
