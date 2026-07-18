package automation

import (
	"reflect"
	"time"
)

type legacySeedDefinition struct {
	Name                string
	Template            string
	Prompt              string
	ModelProfileID      string
	Schedule            Schedule
	Triggers            []TriggerDefinition
	DefaultActionPolicy string
	WriteMode           string
	WriteScope          string
	OutputPolicy        string
	OutputPath          string
}

func removePristineLegacyWorkspaceSeeds(tasks []Task) ([]Task, bool) {
	legacy := legacyDefaultWorkspaceAutomations(time.Unix(0, 0).UTC())
	defaults := make(map[string]Task, len(legacy))
	for _, task := range legacy {
		defaults[task.ID] = task
	}

	filtered := make([]Task, 0, len(tasks))
	changed := false
	for _, task := range tasks {
		seed, known := defaults[task.ID]
		if known && isPristineLegacyWorkspaceSeed(task, seed) {
			changed = true
			continue
		}
		filtered = append(filtered, task)
	}
	return filtered, changed
}

func isPristineLegacyWorkspaceSeed(task, seed Task) bool {
	if task.Scope != ScopeWorkspace || task.Enabled || task.CreatedAt.IsZero() || task.UpdatedAt.IsZero() || !task.CreatedAt.Equal(task.UpdatedAt) {
		return false
	}
	if task.LastRun != nil || len(task.RecentRuns) > 0 || len(task.TriggerState) > 0 {
		return false
	}
	if task.Prompt == "" {
		// The first seeded format predated editable task prompts.
		task.Prompt = seed.Prompt
	}
	normalizedTask, err := NormalizeTask(task)
	if err != nil {
		return false
	}
	normalizedSeed, err := NormalizeTask(seed)
	if err != nil {
		return false
	}
	return reflect.DeepEqual(legacySeedTaskDefinition(normalizedTask), legacySeedTaskDefinition(normalizedSeed))
}

func legacySeedTaskDefinition(task Task) legacySeedDefinition {
	return legacySeedDefinition{
		Name:                task.Name,
		Template:            task.Template,
		Prompt:              task.Prompt,
		ModelProfileID:      task.ModelProfileID,
		Schedule:            task.Schedule,
		Triggers:            task.Triggers,
		DefaultActionPolicy: task.DefaultActionPolicy,
		WriteMode:           task.WriteMode,
		WriteScope:          task.WriteScope,
		OutputPolicy:        task.OutputPolicy,
		OutputPath:          task.OutputPath,
	}
}
