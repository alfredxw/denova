package automation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	workspacelayout "denova/internal/workspace"
)

type Store struct {
	userDir            string
	projectID          string
	workspace          string
	workspaceStateRoot string
	knownWorkspaces    []string
	projectLocations   map[string]ProjectLocation
}

// ProjectLocation separates an automation's execution target from its
// user-owned persistence root. Workspace files remain the target of work;
// definitions, inbox items, and run obligations live under Denova's data dir.
type ProjectLocation struct {
	ProjectID string
	Workspace string
	StateRoot string
}

// ErrRevisionConflict identifies a stale automation definition update.
var ErrRevisionConflict = errors.New("automation revision conflict")

// RevisionConflictError describes the task definition observed by a rejected
// compare-and-swap update without exposing persistence implementation details.
type RevisionConflictError struct {
	TaskID   string
	Expected string
	Actual   string
}

func (e *RevisionConflictError) Error() string {
	if e == nil {
		return ErrRevisionConflict.Error()
	}
	return fmt.Sprintf("%s: task_id=%s expected=%s actual=%s", ErrRevisionConflict, e.TaskID, e.Expected, e.Actual)
}

func (e *RevisionConflictError) Unwrap() error {
	return ErrRevisionConflict
}

// WithWorkspaces returns the same user-level automation store configured to
// discover tasks from every registered workspacelayout. Paths are canonicalized and
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

// WithProjects configures every registered Project visible to a user-level
// catalog. A stable Project ID keeps automation identity intact after relink.
func (s *Store) WithProjects(projects ...ProjectLocation) *Store {
	if s == nil {
		return s
	}
	s.knownWorkspaces = s.knownWorkspaces[:0]
	s.projectLocations = make(map[string]ProjectLocation, len(projects))
	for _, project := range projects {
		workspace := canonicalStoreRoot(project.Workspace)
		if workspace == "" {
			continue
		}
		project.Workspace = workspace
		project.ProjectID = strings.TrimSpace(project.ProjectID)
		project.StateRoot = strings.TrimSpace(project.StateRoot)
		if _, exists := s.projectLocations[workspace]; !exists {
			s.knownWorkspaces = append(s.knownWorkspaces, workspace)
		}
		s.projectLocations[workspace] = project
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

// NewProjectStore binds one content directory to its central Project state.
// NewStore remains the legacy path-based constructor for imports and callers
// that do not yet have a registered Project identity.
func NewProjectStore(userNovaDir, projectID, workspace, stateRoot string) *Store {
	return &Store{
		userDir:            strings.TrimSpace(userNovaDir),
		projectID:          strings.TrimSpace(projectID),
		workspace:          canonicalStoreRoot(workspace),
		workspaceStateRoot: strings.TrimSpace(stateRoot),
	}
}

func (s *Store) storeForWorkspace(workspace string) *Store {
	canonical := canonicalStoreRoot(workspace)
	if location, ok := s.projectLocations[canonical]; ok {
		return NewProjectStore(s.userDir, location.ProjectID, location.Workspace, location.StateRoot)
	}
	if canonical == canonicalStoreRoot(s.workspace) && strings.TrimSpace(s.workspaceStateRoot) != "" {
		return NewProjectStore(s.userDir, s.projectID, s.workspace, s.workspaceStateRoot)
	}
	return NewStore(s.userDir, canonical)
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
		tasks, readErr := s.storeForWorkspace(workspace).readScopeLocked(ScopeWorkspace)
		if readErr != nil {
			return nil, readErr
		}
		workspaceTasks = append(workspaceTasks, tasks...)
	}
	tasks := append(userTasks, workspaceTasks...)
	visible := tasks[:0]
	for _, task := range tasks {
		if task.ArchivedAt == nil {
			visible = append(visible, task)
		}
	}
	tasks = visible
	sort.SliceStable(tasks, func(i, j int) bool {
		if tasks[i].Scope != tasks[j].Scope {
			return tasks[i].Scope < tasks[j].Scope
		}
		return tasks[i].UpdatedAt.After(tasks[j].UpdatedAt)
	})
	return tasks, nil
}

// ListForTarget returns the tasks that execute in one explicit context. It
// never falls back to the currently open workspace.
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
	projectID := strings.TrimSpace(target.ProjectID)
	workspace := canonicalStoreRoot(target.Workspace)
	filtered := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if task.Target.Kind != kind {
			continue
		}
		if kind == TargetKindWorkspace {
			taskProjectID := strings.TrimSpace(task.Target.ProjectID)
			if projectID != "" && taskProjectID != "" {
				if taskProjectID != projectID {
					continue
				}
			} else if canonicalStoreRoot(task.Target.Workspace) != workspace {
				continue
			}
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
		// their content triggers fire against that workspacelayout.
		if kind == TargetKindWorkspace && task.Target.Kind == TargetKindUser {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

func (s *Store) Create(definition TaskDefinition) (Task, error) {
	now := time.Now().UTC()
	task := Task{
		ID: newID("auto"), Scope: definition.Scope, Target: definition.Target,
		Enabled: definition.Enabled, Name: definition.Name, Template: definition.Template,
		Prompt: definition.Prompt, ModelProfileID: definition.ModelProfileID,
		Schedule: definition.Schedule, Triggers: append([]TriggerDefinition(nil), definition.Triggers...),
		DefaultActionPolicy: definition.DefaultActionPolicy, SessionStrategy: definition.SessionStrategy,
		TriggerState: map[string]TriggerState{}, RecentRuns: []RunRecord{},
		CreatedAt: now, UpdatedAt: now,
	}
	normalized, err := s.normalizeTaskTarget(task)
	if err != nil {
		return Task{}, err
	}
	destination := s
	if normalized.Target.Kind == TargetKindWorkspace {
		destination = s.storeForWorkspace(normalized.Target.Workspace)
	}
	// The catalog store resolves the target first; the destination owns the
	// stable Project identity used by every durable task locator.
	normalized, err = destination.normalizeTaskTarget(normalized)
	if err != nil {
		return Task{}, err
	}
	path, err := destination.pathForScope(normalized.Scope)
	if err != nil {
		return Task{}, err
	}
	return withTaskStoreWriteLease(context.Background(), path, func() (Task, error) {
		tasks, readErr := destination.readScope(normalized.Scope)
		if readErr != nil {
			return Task{}, readErr
		}
		tasks = append(tasks, normalized)
		if writeErr := destination.writeScope(normalized.Scope, tasks); writeErr != nil {
			return Task{}, writeErr
		}
		return normalized, nil
	})
}

func (s *Store) Update(id string, patch Task) (Task, error) {
	return s.update(id, patch, "")
}

// UpdateIfRevision applies a task-definition patch only when the caller still
// owns the current definition revision. Trusted runtime callers that do not
// mutate definitions may continue to use Update or the runtime-specific APIs.
func (s *Store) UpdateIfRevision(id string, patch Task, expectedRevision string) (Task, error) {
	return s.update(id, definitionOnlyPatch(patch), strings.TrimSpace(expectedRevision))
}

// definitionOnlyPatch prevents user/API or Agent definition updates from
// replaying stale scheduler-owned dedupe state and run history.
func definitionOnlyPatch(patch Task) Task {
	patch.TriggerState = nil
	patch.LastRun = nil
	patch.RecentRuns = nil
	return patch
}

func (s *Store) update(id string, patch Task, expectedRevision string) (Task, error) {
	if strings.TrimSpace(id) == "" {
		return Task{}, fmt.Errorf("task id is required")
	}
	for _, location := range s.taskLocations() {
		path, err := location.store.pathForScope(location.scope)
		if err != nil {
			return Task{}, err
		}
		updated, err := withTaskStoreWriteLease(context.Background(), path, func() (Task, error) {
			tasks, readErr := location.store.readScope(location.scope)
			if readErr != nil {
				return Task{}, readErr
			}
			for i := range tasks {
				if !taskMatchesID(tasks[i], id) {
					continue
				}
				if tasks[i].ArchivedAt != nil {
					return Task{}, fmt.Errorf("%w: task_id=%s", ErrTaskArchived, id)
				}
				if expectedRevision != "" && tasks[i].Revision != expectedRevision {
					return Task{}, &RevisionConflictError{TaskID: id, Expected: expectedRevision, Actual: tasks[i].Revision}
				}
				next := mergeTaskPatch(tasks[i], patch)
				next.Scope = tasks[i].Scope
				next.Target = tasks[i].Target
				next.UpdatedAt = time.Now().UTC()
				normalized, normalizeErr := location.store.normalizeTaskTarget(next)
				if normalizeErr != nil {
					return Task{}, normalizeErr
				}
				tasks[i] = normalized
				if writeErr := location.store.writeScope(location.scope, tasks); writeErr != nil {
					return Task{}, writeErr
				}
				return normalized, nil
			}
			return Task{}, nil
		})
		if err != nil {
			return Task{}, err
		}
		if updated.ID != "" {
			return updated, nil
		}
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
		found, err := withTaskStoreWriteLease(context.Background(), path, func() (bool, error) {
			tasks, readErr := location.store.readScope(location.scope)
			if readErr != nil {
				return false, readErr
			}
			for index := range tasks {
				if !taskMatchesID(tasks[index], id) {
					continue
				}
				if tasks[index].ArchivedAt != nil {
					return true, nil
				}
				entries, listErr := location.store.readDurableRunObligations(location.scope)
				if listErr != nil {
					return false, listErr
				}
				for _, entry := range entries {
					if durableRunMatchesTask(entry, tasks[index]) && RunHasRuntimeObligation(entry.Run) {
						return false, fmt.Errorf("%w: task_id=%s run_id=%s", ErrTaskHasActiveRun, tasks[index].CatalogID, entry.Run.ID)
					}
				}
				for _, run := range tasks[index].RecentRuns {
					if RunHasRuntimeObligation(run) {
						return false, fmt.Errorf("%w: task_id=%s run_id=%s", ErrTaskHasActiveRun, tasks[index].CatalogID, run.ID)
					}
				}
				now := time.Now().UTC()
				tasks[index].Enabled = false
				tasks[index].ArchivedAt = &now
				tasks[index].UpdatedAt = now
				normalized, normalizeErr := location.store.normalizeTaskTarget(tasks[index])
				if normalizeErr != nil {
					return false, normalizeErr
				}
				tasks[index] = normalized
				return true, location.store.writeScope(location.scope, tasks)
			}
			return false, nil
		})
		if err != nil {
			return err
		}
		if found {
			return nil
		}
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
		locations = append(locations, taskStoreLocation{store: s.storeForWorkspace(canonical), scope: ScopeWorkspace})
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
	if id == "" {
		return false
	}
	if task.ID == id || task.CatalogID == id {
		return true
	}
	// Project migration replaces the workspace-hash catalog prefix with a
	// stable Project ID. Keep the old public locator as a read alias so retries
	// cannot allocate duplicate tasks or runs after the one-way migration.
	return task.Scope == ScopeWorkspace && CatalogTaskID(task.Scope, task.Target.Workspace, task.ID) == id
}

// CatalogTaskID returns the unambiguous persistence locator for a local task
// identity in its durable scope. API records intentionally keep the compact
// local ID; callers crossing back into Store must reconstruct this locator so
// equal imported IDs in user/workspace catalogs cannot alias.
func CatalogTaskID(scope, workspace, taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" || strings.TrimSpace(scope) != ScopeWorkspace {
		return taskID
	}
	return catalogTaskID(Task{
		ID: taskID,
		Target: ExecutionTarget{
			Kind: TargetKindWorkspace, Workspace: canonicalStoreRoot(workspace),
		},
	})
}

func (s *Store) availableScopes() []string {
	if strings.TrimSpace(s.workspace) == "" {
		return []string{ScopeUser}
	}
	return []string{ScopeUser, ScopeWorkspace}
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
		migrated, _ := removePristineLegacyWorkspaceSeeds(file.Tasks)
		file.Tasks = migrated
		// Reads remain side-effect free. The next task-file mutation runs under
		// the shared cross-process lease and persists this one-way migration
		// together with that mutation, avoiding an unlocked read/modify/write.
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
		out = append(out, s.bindProjectTaskRuntime(normalized))
	}
	return out, nil
}

func (s *Store) normalizeTaskTarget(task Task) (Task, error) {
	normalized, err := NormalizeTask(task)
	if err != nil {
		return Task{}, err
	}
	if normalized.Target.Kind == TargetKindWorkspace {
		if strings.TrimSpace(s.workspaceStateRoot) != "" {
			// The state root owns exactly one registered Project. Its current path
			// is authoritative even when a persisted task predates a relink.
			normalized.Target.Workspace = s.workspace
		} else if strings.TrimSpace(normalized.Target.Workspace) == "" {
			normalized.Target.Workspace = s.workspace
		}
		normalized.Target.Workspace = canonicalStoreRoot(normalized.Target.Workspace)
		if normalized.Target.Workspace == "" {
			return Task{}, fmt.Errorf("workspace target is required")
		}
		if strings.TrimSpace(s.projectID) != "" {
			normalized.Target.ProjectID = s.projectID
		} else {
			normalized.Target.ProjectID = workspaceTargetID(normalized.Target.Workspace)
		}
		normalized.Scope = ScopeWorkspace
	} else {
		normalized.Target = ExecutionTarget{Kind: TargetKindUser}
		normalized.Scope = ScopeUser
	}
	normalized.CatalogID = catalogTaskID(normalized)
	revision, err := taskDefinitionRevision(normalized)
	if err != nil {
		return Task{}, err
	}
	normalized.Revision = revision
	return normalized, nil
}

func taskDefinitionRevision(task Task) (string, error) {
	definition := struct {
		Enabled             bool                `json:"enabled"`
		Name                string              `json:"name"`
		Template            string              `json:"template"`
		Prompt              string              `json:"prompt"`
		ModelProfileID      string              `json:"model_profile_id,omitempty"`
		Schedule            Schedule            `json:"schedule"`
		Triggers            []TriggerDefinition `json:"triggers"`
		DefaultActionPolicy string              `json:"default_action_policy"`
		SessionStrategy     string              `json:"session_strategy"`
	}{
		Enabled:             task.Enabled,
		Name:                task.Name,
		Template:            task.Template,
		Prompt:              task.Prompt,
		ModelProfileID:      task.ModelProfileID,
		Schedule:            task.Schedule,
		Triggers:            task.Triggers,
		DefaultActionPolicy: task.DefaultActionPolicy,
		SessionStrategy:     task.SessionStrategy,
	}
	data, err := json.Marshal(definition)
	if err != nil {
		return "", fmt.Errorf("marshal normalized automation definition: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func catalogTaskID(task Task) string {
	if task.Target.Kind == TargetKindWorkspace {
		projectID := strings.TrimSpace(task.Target.ProjectID)
		if projectID == "" {
			projectID = workspaceTargetID(task.Target.Workspace)
		}
		if projectID != "" {
			return projectID + ":" + strings.TrimSpace(task.ID)
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
	persisted := storeFile{SeedVersion: file.SeedVersion, Tasks: make([]Task, len(file.Tasks))}
	copy(persisted.Tasks, file.Tasks)
	for i := range persisted.Tasks {
		persisted.Tasks[i] = portableTask(persisted.Tasks[i])
		persisted.Tasks[i].Revision = ""
	}
	data, err := json.MarshalIndent(persisted, "", "  ")
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
		if strings.TrimSpace(s.workspaceStateRoot) != "" {
			return filepath.Join(s.workspaceStateRoot, "automations", "tasks.json"), nil
		}
		return workspacelayout.Path(s.workspace, "automations", "tasks.json"), nil
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
		task.Target.ProjectID = ""
	} else {
		task.Scope = ScopeWorkspace
		task.Target.Workspace = strings.TrimSpace(task.Target.Workspace)
		task.Target.ProjectID = strings.TrimSpace(task.Target.ProjectID)
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
	task.SessionStrategy = normalizeSessionStrategy(task.SessionStrategy)
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
	task.DefaultActionPolicy = ActionPolicyAutoRun
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

func normalizeSessionStrategy(strategy string) string {
	switch strings.TrimSpace(strategy) {
	case SessionStrategyPerTask:
		return SessionStrategyPerTask
	default:
		return SessionStrategyPerRun
	}
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
	if patch.SessionStrategy != "" {
		next.SessionStrategy = patch.SessionStrategy
	}
	if patch.LastRun != nil {
		next.LastRun = patch.LastRun
	}
	if patch.RecentRuns != nil {
		next.RecentRuns = patch.RecentRuns
	}
	return next
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
