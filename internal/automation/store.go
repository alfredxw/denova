package automation

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"denova/internal/workspacepath"
)

type Store struct {
	userDir         string
	workspace       string
	knownWorkspaces []string
}

// WithWorkspaces returns the same user-level automation store configured to
// discover tasks from every registered workspace. Paths are canonicalized and
// deduplicated so aliases never create duplicate catalog entries.
func (s *Store) WithWorkspaces(workspaces ...string) *Store {
	if s == nil {
		return s
	}
	seen := map[string]bool{}
	s.knownWorkspaces = s.knownWorkspaces[:0]
	for _, workspace := range workspaces {
		canonical := canonicalStoreRoot(workspace)
		if canonical == "" || seen[canonical] {
			continue
		}
		seen[canonical] = true
		s.knownWorkspaces = append(s.knownWorkspaces, canonical)
	}
	return s
}

type storeFile struct {
	SeedVersion int    `json:"seed_version,omitempty"`
	Tasks       []Task `json:"tasks"`
}

func NewStore(userNovaDir, workspace string) *Store {
	return &Store{
		userDir:   strings.TrimSpace(userNovaDir),
		workspace: strings.TrimSpace(workspace),
	}
}

func (s *Store) List() ([]Task, error) {
	userTasks, err := s.readScopeLocked(ScopeUser)
	if err != nil {
		return nil, err
	}
	workspaceTasks := []Task{}
	workspaces := s.knownWorkspaces
	if len(workspaces) == 0 && strings.TrimSpace(s.workspace) != "" {
		workspaces = []string{s.workspace}
	}
	for _, workspace := range workspaces {
		tasks, readErr := NewStore(s.userDir, workspace).readScopeLocked(ScopeWorkspace)
		if readErr != nil {
			return nil, readErr
		}
		workspaceTasks = append(workspaceTasks, tasks...)
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

// ListForTarget returns the tasks that execute in one explicit context. It is
// the scheduler-facing view of the user catalog and never falls back to the
// currently open workspace.
//
// ListForTarget keeps each execution target's list exclusive: a workspace
// target returns only workspace-scoped tasks for that workspace, and the user
// target returns only user-scoped tasks. Callers that need per-workspace trigger
// evaluation (where user-scoped content triggers also fire against a specific
// workspace) should use ListForTriggerEvaluation.
func (s *Store) ListForTarget(target ExecutionTarget) ([]Task, error) {
	tasks, err := s.List()
	if err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(target.Kind)
	if kind == "" {
		kind = TargetKindUser
	}
	workspace := canonicalStoreRoot(target.Workspace)
	filtered := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Target.Kind != kind {
			continue
		}
		if kind == TargetKindWorkspace && canonicalStoreRoot(task.Target.Workspace) != workspace {
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered, nil
}

// ListForTriggerEvaluation returns the tasks that should be evaluated for one
// execution target. Unlike ListForTarget it also includes user-scoped tasks
// when evaluating a workspace target: user-scoped automations carry no fixed
// workspace, so their content triggers (chapter_batch / semantic) are evaluated
// individually against each workspace and produce per-workspace inbox items and
// trigger state.
func (s *Store) ListForTriggerEvaluation(target ExecutionTarget) ([]Task, error) {
	tasks, err := s.List()
	if err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(target.Kind)
	if kind == "" {
		kind = TargetKindUser
	}
	workspace := canonicalStoreRoot(target.Workspace)
	filtered := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Target.Kind == kind {
			if kind == TargetKindWorkspace && canonicalStoreRoot(task.Target.Workspace) != workspace {
				continue
			}
			filtered = append(filtered, task)
			continue
		}
		// When evaluating a workspace target, also include user-scoped tasks so
		// their content triggers fire against that workspace.
		if kind == TargetKindWorkspace && task.Target.Kind == TargetKindUser {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

func (s *Store) Create(task Task) (Task, error) {
	now := time.Now().UTC()
	task.ID = newID("auto")
	task.CreatedAt = now
	task.UpdatedAt = now
	normalized, err := s.normalizeTaskTarget(task)
	if err != nil {
		return Task{}, err
	}
	destination := s
	if normalized.Target.Kind == TargetKindWorkspace {
		destination = NewStore(s.userDir, normalized.Target.Workspace)
	}
	path, err := destination.pathForScope(normalized.Scope)
	if err != nil {
		return Task{}, err
	}
	unlock := storePathLocks.Lock(path)
	defer unlock()
	tasks, err := destination.readScope(normalized.Scope)
	if err != nil {
		return Task{}, err
	}
	tasks = append(tasks, normalized)
	if err := destination.writeScope(normalized.Scope, tasks); err != nil {
		return Task{}, err
	}
	return normalized, nil
}

func (s *Store) Update(id string, patch Task) (Task, error) {
	if strings.TrimSpace(id) == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	for _, location := range s.taskLocations() {
		path, err := location.store.pathForScope(location.scope)
		if err != nil {
			return Task{}, err
		}
		unlock := storePathLocks.Lock(path)
		tasks, err := location.store.readScope(location.scope)
		if err != nil {
			unlock()
			return Task{}, err
		}
		for i := range tasks {
			if !taskMatchesID(tasks[i], id) {
				continue
			}
			next := mergeTaskPatch(tasks[i], patch)
			next.Scope = tasks[i].Scope
			next.Target = tasks[i].Target
			next.UpdatedAt = time.Now().UTC()
			normalized, err := location.store.normalizeTaskTarget(next)
			if err != nil {
				unlock()
				return Task{}, err
			}
			tasks[i] = normalized
			if err := location.store.writeScope(location.scope, tasks); err != nil {
				unlock()
				return Task{}, err
			}
			unlock()
			return normalized, nil
		}
		unlock()
	}
	return Task{}, fmt.Errorf("automation task %s not found", id)
}

func (s *Store) Delete(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("task id is required")
	}
	for _, location := range s.taskLocations() {
		path, err := location.store.pathForScope(location.scope)
		if err != nil {
			return err
		}
		unlock := storePathLocks.Lock(path)
		tasks, err := location.store.readScope(location.scope)
		if err != nil {
			unlock()
			return err
		}
		next := tasks[:0]
		found := false
		for _, task := range tasks {
			if taskMatchesID(task, id) {
				found = true
				continue
			}
			next = append(next, task)
		}
		if found {
			err := location.store.writeScope(location.scope, next)
			unlock()
			return err
		}
		unlock()
	}
	return fmt.Errorf("automation task %s not found", id)
}

func (s *Store) Get(id string) (Task, error) {
	for _, location := range s.taskLocations() {
		path, err := location.store.pathForScope(location.scope)
		if err != nil {
			return Task{}, err
		}
		unlock := storePathLocks.Lock(path)
		tasks, err := location.store.readScope(location.scope)
		if err != nil {
			unlock()
			return Task{}, err
		}
		for _, task := range tasks {
			if taskMatchesID(task, id) {
				unlock()
				return task, nil
			}
		}
		unlock()
	}
	return Task{}, fmt.Errorf("automation task %s not found", id)
}

// GetRunByID resolves a single run across the user and workspace scopes this
// store can see. The app layer must not load every task and scan RecentRuns
// itself; that lookup belongs next to the persisted run data so callers get a
// single, lock-aware entry point.
func (s *Store) GetRunByID(runID string) (Task, RunRecord, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return Task{}, RunRecord{}, fmt.Errorf("run_id is required")
	}
	for _, location := range s.taskLocations() {
		path, err := location.store.pathForScope(location.scope)
		if err != nil {
			return Task{}, RunRecord{}, err
		}
		unlock := storePathLocks.Lock(path)
		tasks, err := location.store.readScope(location.scope)
		if err != nil {
			unlock()
			return Task{}, RunRecord{}, err
		}
		for _, task := range tasks {
			for _, run := range task.RecentRuns {
				if strings.TrimSpace(run.ID) == runID {
					unlock()
					return task, run, nil
				}
			}
		}
		unlock()
	}
	return Task{}, RunRecord{}, fmt.Errorf("automation run %s not found", runID)
}

type taskStoreLocation struct {
	store *Store
	scope string
}

func (s *Store) taskLocations() []taskStoreLocation {
	locations := []taskStoreLocation{{store: NewStore(s.userDir, ""), scope: ScopeUser}}
	seen := map[string]bool{}
	appendWorkspace := func(workspace string) {
		canonical := canonicalStoreRoot(workspace)
		if canonical == "" || seen[canonical] {
			return
		}
		seen[canonical] = true
		locations = append(locations, taskStoreLocation{store: NewStore(s.userDir, canonical), scope: ScopeWorkspace})
	}
	appendWorkspace(s.workspace)
	for _, workspace := range s.knownWorkspaces {
		appendWorkspace(workspace)
	}
	return locations
}

func taskMatchesID(task Task, id string) bool {
	return TaskMatchesID(task, id)
}

// TaskMatchesID reports whether id identifies task. It matches against both the
// stable per-workspace task ID and the cross-workspace catalog ID so callers can
// resolve a task regardless of which form they hold.
func TaskMatchesID(task Task, id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && (task.ID == id || task.CatalogID == id)
}

func (s *Store) availableScopes() []string {
	if strings.TrimSpace(s.workspace) == "" {
		return []string{ScopeUser}
	}
	return []string{ScopeUser, ScopeWorkspace}
}

func (s *Store) AppendRun(id string, run RunRecord) (Task, error) {
	if strings.TrimSpace(id) == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	for _, location := range s.taskLocations() {
		path, err := location.store.pathForScope(location.scope)
		if err != nil {
			return Task{}, err
		}
		unlock := storePathLocks.Lock(path)
		tasks, err := location.store.readScope(location.scope)
		if err != nil {
			unlock()
			return Task{}, err
		}
		for i := range tasks {
			if !taskMatchesID(tasks[i], id) {
				continue
			}
			task := tasks[i]
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
			task.UpdatedAt = time.Now().UTC()
			normalized, normalizeErr := location.store.normalizeTaskTarget(task)
			if normalizeErr != nil {
				unlock()
				return Task{}, normalizeErr
			}
			tasks[i] = normalized
			if writeErr := location.store.writeScope(location.scope, tasks); writeErr != nil {
				unlock()
				return Task{}, writeErr
			}
			unlock()
			return normalized, nil
		}
		unlock()
	}
	return Task{}, fmt.Errorf("automation task %s not found", id)
}

func (s *Store) readScopeLocked(scope string) ([]Task, error) {
	path, err := s.pathForScope(scope)
	if err != nil {
		return nil, err
	}
	unlock := storePathLocks.Lock(path)
	defer unlock()
	return s.readScope(scope)
}

func (s *Store) readScope(scope string) ([]Task, error) {
	path, err := s.pathForScope(scope)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s.normalizeTaskList(path, scope, []Task{})
	}
	if err != nil {
		return nil, err
	}
	var file storeFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("read automations %s failed: %w", path, err)
	}
	if scope == ScopeWorkspace && file.SeedVersion > 0 {
		migrated, changed := removePristineLegacyWorkspaceSeeds(file.Tasks)
		file.Tasks = migrated
		if changed {
			file.SeedVersion = 0
			if writeErr := s.writeScopeFile(scope, file); writeErr != nil {
				return nil, writeErr
			}
		}
	}
	return s.normalizeTaskList(path, scope, file.Tasks)
}

func (s *Store) normalizeTaskList(path, scope string, tasks []Task) ([]Task, error) {
	out := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Scope == "" {
			task.Scope = scope
		}
		normalized, err := s.normalizeTaskTarget(task)
		if err != nil {
			return nil, fmt.Errorf("invalid automation task %s: %w", task.ID, err)
		}
		out = append(out, normalized)
	}
	return out, nil
}

func (s *Store) normalizeTaskTarget(task Task) (Task, error) {
	normalized, err := NormalizeTask(task)
	if err != nil {
		return Task{}, err
	}
	if normalized.Target.Kind == TargetKindWorkspace {
		if strings.TrimSpace(normalized.Target.Workspace) == "" {
			normalized.Target.Workspace = s.workspace
		}
		normalized.Target.Workspace = canonicalStoreRoot(normalized.Target.Workspace)
		if normalized.Target.Workspace == "" {
			return Task{}, fmt.Errorf("workspace target is required")
		}
		normalized.Target.WorkspaceID = workspaceTargetID(normalized.Target.Workspace)
		normalized.Scope = ScopeWorkspace
	} else {
		normalized.Target = ExecutionTarget{Kind: TargetKindUser}
		normalized.Scope = ScopeUser
		normalized.WriteMode = WriteModeReadOnly
		normalized.WriteScope = WriteScopeNone
		normalized.OutputPolicy = OutputPolicyRunRecordOnly
		normalized.OutputPath = ""
	}
	normalized.CatalogID = catalogTaskID(normalized)
	return normalized, nil
}

func catalogTaskID(task Task) string {
	if task.Target.Kind == TargetKindWorkspace {
		workspaceID := strings.TrimSpace(task.Target.WorkspaceID)
		if workspaceID == "" {
			workspaceID = workspaceTargetID(task.Target.Workspace)
		}
		if workspaceID != "" {
			return workspaceID + ":" + strings.TrimSpace(task.ID)
		}
	}
	return strings.TrimSpace(task.ID)
}

func workspaceTargetID(workspace string) string {
	canonical := canonicalStoreRoot(workspace)
	if canonical == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(canonical))
	return "workspace-" + hex.EncodeToString(sum[:8])
}

func (s *Store) writeScope(scope string, tasks []Task) error {
	file := storeFile{Tasks: tasks}
	return s.writeScopeFile(scope, file)
}

func (s *Store) writeScopeFile(scope string, file storeFile) error {
	path, err := s.pathForScope(scope)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return durableWriteJSON(path, append(data, '\n'), 0o644)
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
		return workspacepath.Path(s.workspace, "automations", "tasks.json"), nil
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
	task.Target.Kind = strings.TrimSpace(task.Target.Kind)
	if task.Target.Kind == "" {
		if task.Scope == ScopeUser {
			task.Target.Kind = TargetKindUser
		} else {
			task.Target.Kind = TargetKindWorkspace
		}
	}
	if task.Target.Kind != TargetKindUser && task.Target.Kind != TargetKindWorkspace {
		return Task{}, fmt.Errorf("invalid target kind %q", task.Target.Kind)
	}
	if task.Target.Kind == TargetKindUser {
		task.Scope = ScopeUser
		task.Target.Workspace = ""
		task.Target.WorkspaceID = ""
	} else {
		task.Scope = ScopeWorkspace
		task.Target.Workspace = strings.TrimSpace(task.Target.Workspace)
		task.Target.WorkspaceID = strings.TrimSpace(task.Target.WorkspaceID)
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
	task.WriteMode, task.WriteScope = normalizeWriteModeScope(task.WriteMode, task.WriteScope)
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
	if patch.Target.Kind != "" {
		next.Target = patch.Target
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

func normalizeWriteModeScope(mode, scope string) (string, string) {
	mode = strings.TrimSpace(mode)
	scope = strings.TrimSpace(scope)
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
