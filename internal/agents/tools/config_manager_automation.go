package tools

import "denova/internal/automation"

// automationTaskWriteInput is the user-editable definition surface. Runtime
// identity, timestamps, trigger state, and run history never enter config_apply.
type automationTaskWriteInput struct {
	Target         *automationTargetWriteInput   `json:"target,omitempty"`
	Enabled        *bool                         `json:"enabled,omitempty"`
	Name           *string                       `json:"name,omitempty"`
	Template       *string                       `json:"template,omitempty"`
	Prompt         *string                       `json:"prompt,omitempty"`
	ModelProfileID *string                       `json:"model_profile_id,omitempty"`
	Schedule       *automationScheduleWriteInput `json:"schedule,omitempty"`
	Triggers       []automationTriggerWriteInput `json:"triggers,omitempty"`
	WriteMode      *string                       `json:"write_mode,omitempty"`
	WriteScope     *string                       `json:"write_scope,omitempty"`
	OutputPolicy   *string                       `json:"output_policy,omitempty"`
	OutputPath     *string                       `json:"output_path,omitempty"`
}

type automationTargetWriteInput struct {
	Kind      string `json:"kind"`
	Workspace string `json:"workspace,omitempty"`
}

type automationScheduleWriteInput struct {
	Kind       string `json:"kind,omitempty"`
	EveryHours int    `json:"every_hours,omitempty"`
	Weekday    int    `json:"weekday,omitempty"`
	DayOfMonth int    `json:"day_of_month,omitempty"`
	Hour       int    `json:"hour,omitempty"`
	Minute     int    `json:"minute,omitempty"`
}

type automationTriggerWriteInput struct {
	ID                string                        `json:"id,omitempty"`
	Type              string                        `json:"type,omitempty"`
	Enabled           bool                          `json:"enabled,omitempty"`
	Name              string                        `json:"name,omitempty"`
	NotifyPolicy      string                        `json:"notify_policy,omitempty"`
	Schedule          *automationScheduleWriteInput `json:"schedule,omitempty"`
	SemanticCondition string                        `json:"semantic_condition,omitempty"`
	ChapterBatchSize  int                           `json:"chapter_batch_size,omitempty"`
}

func (input *automationTaskWriteInput) newTask() automation.Task {
	var task automation.Task
	if input == nil {
		return task
	}
	if input.Target != nil {
		task.Target = automation.ExecutionTarget{Kind: input.Target.Kind, Workspace: input.Target.Workspace}
	}
	input.applyDefinition(&task)
	return task
}

func (input *automationTaskWriteInput) applyDefinition(task *automation.Task) {
	if input == nil || task == nil {
		return
	}
	if input.Enabled != nil {
		task.Enabled = *input.Enabled
	}
	if input.Name != nil {
		task.Name = *input.Name
	}
	if input.Template != nil {
		task.Template = *input.Template
	}
	if input.Prompt != nil {
		task.Prompt = *input.Prompt
	}
	if input.ModelProfileID != nil {
		task.ModelProfileID = *input.ModelProfileID
	}
	if input.Schedule != nil {
		task.Schedule = input.Schedule.toSchedule()
	}
	if input.Triggers != nil {
		task.Triggers = make([]automation.TriggerDefinition, len(input.Triggers))
		for index := range input.Triggers {
			task.Triggers[index] = input.Triggers[index].toTrigger()
		}
	}
	if input.WriteMode != nil {
		task.WriteMode = *input.WriteMode
	}
	if input.WriteScope != nil {
		task.WriteScope = *input.WriteScope
	}
	if input.OutputPolicy != nil {
		task.OutputPolicy = *input.OutputPolicy
	}
	if input.OutputPath != nil {
		task.OutputPath = *input.OutputPath
	}
}

func (input automationScheduleWriteInput) toSchedule() automation.Schedule {
	return automation.Schedule{
		Kind: input.Kind, EveryHours: input.EveryHours, Weekday: input.Weekday,
		DayOfMonth: input.DayOfMonth, Hour: input.Hour, Minute: input.Minute,
	}
}

func (input automationTriggerWriteInput) toTrigger() automation.TriggerDefinition {
	trigger := automation.TriggerDefinition{
		ID: input.ID, Type: input.Type, Enabled: input.Enabled, Name: input.Name,
		NotifyPolicy: input.NotifyPolicy, SemanticCondition: input.SemanticCondition,
		ChapterBatchSize: input.ChapterBatchSize,
	}
	if input.Schedule != nil {
		trigger.Schedule = input.Schedule.toSchedule()
	}
	return trigger
}

func configManagerAutomationStore(novaDir, workspace string, workspaces []string) *automation.Store {
	all := append([]string{workspace}, workspaces...)
	return automation.NewStore(novaDir, workspace).WithWorkspaces(all...)
}
