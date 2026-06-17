package automation

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Store struct {
	userDir   string
	workspace string
}

const workspaceDefaultAutomationSeedVersion = 2

type storeFile struct {
	SeedVersion int    `json:"seed_version,omitempty"`
	Tasks       []Task `json:"tasks"`
}

func NewStore(userNovaDir, workspace string) *Store {
	return &Store{userDir: userNovaDir, workspace: workspace}
}

func (s *Store) List() ([]Task, error) {
	userTasks, err := s.readScope(ScopeUser)
	if err != nil {
		return nil, err
	}
	workspaceTasks := []Task{}
	if strings.TrimSpace(s.workspace) != "" {
		var err error
		workspaceTasks, err = s.readScope(ScopeWorkspace)
		if err != nil {
			return nil, err
		}
	}
	tasks := append(userTasks, workspaceTasks...)
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Scope != tasks[j].Scope {
			return tasks[i].Scope < tasks[j].Scope
		}
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})
	return tasks, nil
}

func (s *Store) Create(task Task) (Task, error) {
	now := time.Now().UTC()
	task.ID = newID("auto")
	task.CreatedAt = now
	task.UpdatedAt = now
	normalized, err := NormalizeTask(task)
	if err != nil {
		return Task{}, err
	}
	tasks, err := s.readScope(normalized.Scope)
	if err != nil {
		return Task{}, err
	}
	tasks = append(tasks, normalized)
	if err := s.writeScope(normalized.Scope, tasks); err != nil {
		return Task{}, err
	}
	return normalized, nil
}

func (s *Store) Update(id string, patch Task) (Task, error) {
	if strings.TrimSpace(id) == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	for _, scope := range s.availableScopes() {
		tasks, err := s.readScope(scope)
		if err != nil {
			return Task{}, err
		}
		for i := range tasks {
			if tasks[i].ID != id {
				continue
			}
			next := mergeTaskPatch(tasks[i], patch)
			next.Scope = tasks[i].Scope
			next.UpdatedAt = time.Now().UTC()
			normalized, err := NormalizeTask(next)
			if err != nil {
				return Task{}, err
			}
			tasks[i] = normalized
			if err := s.writeScope(scope, tasks); err != nil {
				return Task{}, err
			}
			return normalized, nil
		}
	}
	return Task{}, fmt.Errorf("automation task %s not found", id)
}

func (s *Store) Delete(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("task id is required")
	}
	for _, scope := range s.availableScopes() {
		tasks, err := s.readScope(scope)
		if err != nil {
			return err
		}
		next := tasks[:0]
		found := false
		for _, task := range tasks {
			if task.ID == id {
				found = true
				continue
			}
			next = append(next, task)
		}
		if found {
			return s.writeScope(scope, next)
		}
	}
	return fmt.Errorf("automation task %s not found", id)
}

func (s *Store) Get(id string) (Task, error) {
	for _, scope := range s.availableScopes() {
		tasks, err := s.readScope(scope)
		if err != nil {
			return Task{}, err
		}
		for _, task := range tasks {
			if task.ID == id {
				return task, nil
			}
		}
	}
	return Task{}, fmt.Errorf("automation task %s not found", id)
}

func (s *Store) availableScopes() []string {
	if strings.TrimSpace(s.workspace) == "" {
		return []string{ScopeUser}
	}
	return []string{ScopeUser, ScopeWorkspace}
}

func (s *Store) AppendRun(id string, run RunRecord) (Task, error) {
	task, err := s.Get(id)
	if err != nil {
		return Task{}, err
	}
	task.LastRun = &run
	nextRuns := []RunRecord{run}
	for _, existing := range task.RecentRuns {
		if existing.ID == run.ID {
			continue
		}
		nextRuns = append(nextRuns, existing)
	}
	task.RecentRuns = nextRuns
	if len(task.RecentRuns) > MaxRecentRuns {
		task.RecentRuns = task.RecentRuns[:MaxRecentRuns]
	}
	return s.Update(id, task)
}

func (s *Store) readScope(scope string) ([]Task, error) {
	path, err := s.pathForScope(scope)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		file := storeFile{}
		if scope == ScopeWorkspace {
			file = seedWorkspaceDefaultAutomations(file)
			if writeErr := s.writeScopeFile(scope, file); writeErr != nil {
				return nil, writeErr
			}
		}
		return normalizeTaskList(path, file.Tasks)
	}
	if err != nil {
		return nil, err
	}
	var file storeFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("read automations %s failed: %w", path, err)
	}
	if scope == ScopeWorkspace && file.SeedVersion < workspaceDefaultAutomationSeedVersion {
		file = seedWorkspaceDefaultAutomations(file)
		if writeErr := s.writeScopeFile(scope, file); writeErr != nil {
			return nil, writeErr
		}
	}
	return normalizeTaskList(path, file.Tasks)
}

func normalizeTaskList(path string, tasks []Task) ([]Task, error) {
	out := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		normalized, err := NormalizeTask(task)
		if err != nil {
			return nil, fmt.Errorf("invalid automation task %s: %w", task.ID, err)
		}
		out = append(out, normalized)
	}
	return out, nil
}

func (s *Store) writeScope(scope string, tasks []Task) error {
	file := storeFile{Tasks: tasks}
	if scope == ScopeWorkspace {
		file.SeedVersion = workspaceDefaultAutomationSeedVersion
	}
	return s.writeScopeFile(scope, file)
}

func (s *Store) writeScopeFile(scope string, file storeFile) error {
	path, err := s.pathForScope(scope)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func seedWorkspaceDefaultAutomations(file storeFile) storeFile {
	file.SeedVersion = workspaceDefaultAutomationSeedVersion
	for _, seeded := range defaultWorkspaceAutomations() {
		file.Tasks = fillExistingWorkspaceDefaultAutomationPrompt(file.Tasks, seeded)
		if workspaceAutomationSeedExists(file.Tasks, seeded) {
			continue
		}
		file.Tasks = append(file.Tasks, seeded)
	}
	return file
}

func fillExistingWorkspaceDefaultAutomationPrompt(tasks []Task, seeded Task) []Task {
	for i := range tasks {
		if tasks[i].ID != seeded.ID {
			continue
		}
		if strings.TrimSpace(tasks[i].Prompt) == "" {
			tasks[i].Prompt = seeded.Prompt
		}
	}
	return tasks
}

func workspaceAutomationSeedExists(tasks []Task, seeded Task) bool {
	for _, task := range tasks {
		if task.ID == seeded.ID {
			return true
		}
		if task.Scope == ScopeWorkspace && task.Template == seeded.Template {
			return true
		}
	}
	return false
}

func defaultWorkspaceAutomations() []Task {
	now := time.Now().UTC()
	schedule := Schedule{Kind: ScheduleManual, Hour: 9, Minute: 0, Weekday: 1, DayOfMonth: 1, EveryHours: 6}
	return []Task{
		{
			ID:                  "workspace-auto-continue-writing",
			Scope:               ScopeWorkspace,
			Enabled:             false,
			Name:                "续写章节",
			Template:            TemplateContinueWriting,
			Prompt:              DefaultContinueWritingPrompt,
			Schedule:            schedule,
			Triggers:            []TriggerDefinition{legacyScheduleTrigger(schedule)},
			DefaultActionPolicy: ActionPolicyAutoRun,
			WriteMode:           WriteModeConfirmWrite,
			WriteScope:          WriteScopeFile,
			OutputPolicy:        OutputPolicyRunRecordOnly,
			RecentRuns:          []RunRecord{},
			CreatedAt:           now,
			UpdatedAt:           now,
		},
		{
			ID:       "workspace-auto-review",
			Scope:    ScopeWorkspace,
			Enabled:  false,
			Name:     "自动 Review",
			Template: TemplateReview,
			Prompt:   DefaultReviewPrompt,
			Schedule: schedule,
			Triggers: []TriggerDefinition{{
				ID:               "chapter_batch_review",
				Type:             TriggerTypeChapterBatch,
				Enabled:          true,
				NotifyPolicy:     NotifyPolicyInbox,
				ChapterBatchSize: 5,
			}},
			DefaultActionPolicy: ActionPolicyAutoRun,
			WriteMode:           WriteModeReadOnly,
			WriteScope:          WriteScopeNone,
			OutputPolicy:        OutputPolicyRunRecordOnly,
			RecentRuns:          []RunRecord{},
			CreatedAt:           now,
			UpdatedAt:           now,
		},
	}
}

func (s *Store) pathForScope(scope string) (string, error) {
	switch scope {
	case ScopeUser:
		if strings.TrimSpace(s.userDir) == "" {
			return "", fmt.Errorf("user nova dir is required")
		}
		return filepath.Join(s.userDir, "automations", "tasks.json"), nil
	case ScopeWorkspace:
		if strings.TrimSpace(s.workspace) == "" {
			return "", fmt.Errorf("workspace is required")
		}
		return filepath.Join(s.workspace, ".nova", "automations", "tasks.json"), nil
	default:
		return "", fmt.Errorf("unknown automation scope %q", scope)
	}
}

func NormalizeTask(task Task) (Task, error) {
	task.Scope = strings.TrimSpace(task.Scope)
	if task.Scope == "" {
		task.Scope = ScopeWorkspace
	}
	if task.Scope != ScopeUser && task.Scope != ScopeWorkspace {
		return Task{}, fmt.Errorf("invalid scope %q", task.Scope)
	}
	task.Name = strings.TrimSpace(task.Name)
	if task.Name == "" {
		task.Name = "Automation"
	}
	task.Template = strings.TrimSpace(task.Template)
	if task.Template == "" {
		task.Template = TemplateCustomPrompt
	}
	if !validTemplate(task.Template) {
		return Task{}, fmt.Errorf("invalid template %q", task.Template)
	}
	task.ModelProfileID = strings.TrimSpace(task.ModelProfileID)
	schedule, err := NormalizeSchedule(task.Schedule)
	if err != nil {
		return Task{}, err
	}
	task.Schedule = schedule
	task.Triggers = normalizeTriggers(task.Triggers, task.Schedule)
	if len(task.Triggers) == 0 {
		task.Triggers = []TriggerDefinition{legacyScheduleTrigger(task.Schedule)}
	}
	if firstSchedule, ok := firstScheduleTrigger(task.Triggers); ok {
		task.Schedule = firstSchedule.Schedule
	}
	if task.TriggerState == nil {
		task.TriggerState = map[string]TriggerState{}
	}
	task.WriteMode, task.WriteScope = normalizeWriteModeScope(task.WriteMode, task.WriteScope, task.WritePolicy)
	task.WritePolicy = legacyWritePolicyForModeScope(task.WriteMode, task.WriteScope)
	task.DefaultActionPolicy = actionPolicyForWriteMode(task.WriteMode)
	task.OutputPolicy = normalizeOutputPolicy(task.OutputPolicy)
	task.OutputPath = filepath.ToSlash(strings.TrimSpace(task.OutputPath))
	task.Prompt = strings.TrimSpace(task.Prompt)
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	if task.UpdatedAt.IsZero() {
		task.UpdatedAt = task.CreatedAt
	}
	if task.RecentRuns == nil {
		task.RecentRuns = []RunRecord{}
	}
	return task, nil
}

func mergeTaskPatch(current, patch Task) Task {
	next := current
	if patch.Scope != "" {
		next.Scope = patch.Scope
	}
	next.Enabled = patch.Enabled
	if patch.Name != "" {
		next.Name = patch.Name
	}
	if patch.Template != "" {
		next.Template = patch.Template
	}
	next.Prompt = patch.Prompt
	next.ModelProfileID = patch.ModelProfileID
	if patch.Schedule.Kind != "" {
		next.Schedule = patch.Schedule
	}
	if patch.Triggers != nil {
		next.Triggers = patch.Triggers
	}
	if patch.DefaultActionPolicy != "" {
		next.DefaultActionPolicy = patch.DefaultActionPolicy
	}
	if patch.TriggerState != nil {
		next.TriggerState = patch.TriggerState
	}
	if patch.WritePolicy != "" {
		next.WritePolicy = patch.WritePolicy
	}
	if patch.WriteMode != "" {
		next.WriteMode = patch.WriteMode
	}
	if patch.WriteScope != "" {
		next.WriteScope = patch.WriteScope
	}
	if patch.OutputPolicy != "" {
		next.OutputPolicy = patch.OutputPolicy
	}
	next.OutputPath = patch.OutputPath
	if patch.LastRun != nil {
		next.LastRun = patch.LastRun
	}
	if patch.RecentRuns != nil {
		next.RecentRuns = patch.RecentRuns
	}
	return next
}

func normalizeWritePolicy(policy string) string {
	switch policy {
	case WritePolicyAllowLoreWrite, WritePolicyAllowFileWrite, WritePolicyAllowLoreAndFileWrite:
		return policy
	default:
		return WritePolicyReadOnly
	}
}

func normalizeWriteModeScope(mode, scope, legacyPolicy string) (string, string) {
	mode = strings.TrimSpace(mode)
	scope = strings.TrimSpace(scope)
	if mode == "" {
		mode, scope = writeModeScopeFromLegacyPolicy(legacyPolicy)
	}
	switch mode {
	case WriteModeConfirmWrite, WriteModeAutoWrite:
	default:
		mode = WriteModeReadOnly
	}
	if mode == WriteModeReadOnly {
		return WriteModeReadOnly, WriteScopeNone
	}
	switch scope {
	case WriteScopeLore, WriteScopeFile, WriteScopeLoreAndFile:
		return mode, scope
	default:
		return mode, WriteScopeFile
	}
}

func writeModeScopeFromLegacyPolicy(policy string) (string, string) {
	switch normalizeWritePolicy(policy) {
	case WritePolicyAllowLoreWrite:
		return WriteModeAutoWrite, WriteScopeLore
	case WritePolicyAllowFileWrite:
		return WriteModeAutoWrite, WriteScopeFile
	case WritePolicyAllowLoreAndFileWrite:
		return WriteModeAutoWrite, WriteScopeLoreAndFile
	default:
		return WriteModeReadOnly, WriteScopeNone
	}
}

func legacyWritePolicyForModeScope(mode, scope string) string {
	if mode == WriteModeReadOnly {
		return WritePolicyReadOnly
	}
	switch scope {
	case WriteScopeLore:
		return WritePolicyAllowLoreWrite
	case WriteScopeFile:
		return WritePolicyAllowFileWrite
	case WriteScopeLoreAndFile:
		return WritePolicyAllowLoreAndFileWrite
	default:
		return WritePolicyReadOnly
	}
}

func normalizeOutputPolicy(policy string) string {
	if policy == OutputPolicyOptionalFile {
		return policy
	}
	return OutputPolicyRunRecordOnly
}

func validTemplate(template string) bool {
	switch template {
	case TemplateMemoryConsolidation, TemplateReview, TemplateContinueWriting, TemplateCustomPrompt:
		return true
	default:
		return false
	}
}

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

func NewRunID() string {
	return newID("run")
}
