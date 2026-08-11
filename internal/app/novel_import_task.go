package app

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"denova/internal/agent"
	"denova/internal/book"
)

// NovelImportToolAgentTimeout bounds the model-only split-regex inference
// inside a background import. The task context owns the timeout so inference
// survives the originating HTTP request being canceled.
const NovelImportToolAgentTimeout = 90 * time.Second

// NovelImportTaskRequest carries everything the background import task needs.
// The handler owns upload parsing and split-regex inference options.
type NovelImportTaskRequest struct {
	Filename    string
	Data        []byte
	Title       string
	Author      string
	Description string
	Options     book.NovelImportOptions
}

type novelImportTaskResult struct {
	workspace string
	title     string
	error     string
}

type novelImportTaskState struct {
	task            *Task
	title           string
	sourceWorkspace string
	result          *novelImportTaskResult
}

type novelImportTaskSnapshot struct {
	task            TaskSnapshot
	title           string
	sourceWorkspace string
	result          *novelImportTaskResult
}

// StartNovelImportTask runs a confirmed novel import in the recoverable task
// pipeline and keeps every execution visible in the task center.
func (a *App) StartNovelImportTask(ctx context.Context, req NovelImportTaskRequest) (*Task, error) {
	layered, err := a.Settings()
	if err != nil {
		return nil, err
	}
	if layered.Paths.DenovaDir == "" {
		return nil, fmt.Errorf("缺少数据目录")
	}
	novaDir := layered.Paths.DenovaDir
	title := strings.TrimSpace(req.Title)
	start := make(chan struct{})
	task := NewTask(func(ctx context.Context, task *Task, emit func(agent.Event)) {
		select {
		case <-start:
		case <-ctx.Done():
			return
		}
		options := req.Options
		if options.SplitRegex == "" && options.SplitStrategy != book.NovelImportSplitStrategyBuiltin {
			options.InferSplitRegex = func(sample string) (string, error) {
				inferCtx, cancel := context.WithTimeout(ctx, NovelImportToolAgentTimeout)
				defer cancel()
				return a.InferNovelSplitRegex(inferCtx, sample)
			}
		}
		emit(agent.Event{Type: "progress", Data: novelImportTaskProgress{Step: "uploaded"}})
		preview, err := book.PreviewNovelImport(req.Filename, req.Data, options)
		if err != nil {
			a.recordNovelImportResult(task, "", req.Filename, err)
			emit(agent.Event{Type: "error", Data: novelImportTaskError{Error: err.Error()}})
			return
		}
		if title == "" {
			title = preview.Title
		}
		emit(agent.Event{Type: "progress", Data: novelImportTaskProgress{Step: "create_book"}})
		workspace, meta, err := a.CreateBook(ctx, novaDir, title, req.Author, req.Description)
		if err != nil {
			a.recordNovelImportResult(task, "", title, err)
			emit(agent.Event{Type: "error", Data: novelImportTaskError{Error: err.Error()}})
			return
		}
		emit(agent.Event{Type: "progress", Data: novelImportTaskProgress{Step: "importing"}})
		importPreview, paths, err := book.ImportNovelToWorkspace(workspace, req.Filename, req.Data, options)
		if err != nil {
			a.recordNovelImportResult(task, workspace, title, err)
			emit(agent.Event{Type: "error", Data: novelImportTaskError{Error: err.Error()}})
			return
		}
		a.recordNovelImportResult(task, workspace, title, nil)
		emit(agent.Event{Type: "done", Data: book.NovelImportResult{
			Workspace:    workspace,
			BookMeta:     &meta,
			Title:        importPreview.Title,
			ChapterCount: importPreview.ChapterCount,
			TotalChars:   importPreview.TotalChars,
			ChapterPaths: paths,
		}})
	})
	a.mu.Lock()
	if a.novelImportTasks == nil {
		a.novelImportTasks = make(map[string]*novelImportTaskState)
	}
	a.novelImportTasks[task.ID()] = &novelImportTaskState{
		task:            task,
		title:           req.Filename,
		sourceWorkspace: a.workspace,
	}
	a.mu.Unlock()
	close(start)
	return task, nil
}

func (a *App) recordNovelImportResult(task *Task, workspace, title string, err error) {
	result := &novelImportTaskResult{
		workspace: canonicalTaskWorkspace(workspace),
		title:     title,
	}
	status := "completed"
	if err != nil {
		result.error = err.Error()
		status = "failed"
	}
	a.mu.Lock()
	if a.novelImportTasks == nil {
		a.novelImportTasks = make(map[string]*novelImportTaskState)
	}
	state := a.novelImportTasks[task.ID()]
	if state == nil {
		state = &novelImportTaskState{task: task}
		a.novelImportTasks[task.ID()] = state
	}
	state.result = result
	a.mu.Unlock()
	log.Printf("[novel-import] task result recorded task_id=%s status=%s workspace=%q title=%q error=%q", task.ID(), status, result.workspace, result.title, result.error)
}

func (a *App) novelImportTasksSnapshot() []novelImportTaskSnapshot {
	a.mu.RLock()
	tasks := make([]*Task, 0, len(a.novelImportTasks))
	for _, state := range a.novelImportTasks {
		if state == nil || state.task == nil {
			continue
		}
		tasks = append(tasks, state.task)
	}
	a.mu.RUnlock()

	runs := make([]novelImportTaskSnapshot, 0, len(tasks))
	for _, task := range tasks {
		// Result metadata is recorded before a normal terminal Task state. Reading
		// the Task first prevents one catalog response from combining that terminal
		// state with metadata copied before the result was recorded.
		taskSnapshot := task.Snapshot()
		a.mu.RLock()
		state := a.novelImportTasks[taskSnapshot.ID]
		if state == nil || state.task != task {
			a.mu.RUnlock()
			continue
		}
		copied := novelImportTaskSnapshot{
			task:            taskSnapshot,
			title:           state.title,
			sourceWorkspace: state.sourceWorkspace,
		}
		if state.result != nil {
			result := *state.result
			copied.result = &result
		}
		a.mu.RUnlock()
		runs = append(runs, copied)
	}
	return runs
}

type novelImportTaskProgress struct {
	Step string `json:"step"`
}

type novelImportTaskError struct {
	Error string `json:"error"`
}
