package agents

import (
	"context"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/internal/automation"
)

type automationWriteInput struct {
	Message    string                     `json:"message,omitempty" jsonschema:"description=本次自动化任务变更说明；省略时使用默认说明"`
	Operations []automationWriteOperation `json:"operations" jsonschema:"minItems=1,description=批量自动化任务操作"`
}

type automationWriteOperation struct {
	Op   string                    `json:"op" jsonschema:"enum=create,enum=update,enum=delete,description=操作类型：create/update/delete"`
	ID   string                    `json:"id,omitempty" jsonschema:"description=目标自动化任务 catalog_id；update/delete 必填"`
	Task *automationTaskWriteInput `json:"task,omitempty" jsonschema:"description=create/update 使用的自动化任务定义；运行记录、时间戳和派生字段不可写"`
}

// automationTaskWriteInput is the user-editable automation definition. Keeping
// it separate from automation.Task prevents runtime state and derived storage
// fields from leaking into the Agent tool protocol.
type automationTaskWriteInput struct {
	Revision       string                        `json:"revision,omitempty" jsonschema:"description=update 必填的定义 revision，来自 read_automations"`
	Target         *automationTargetWriteInput   `json:"target,omitempty" jsonschema:"description=create 必填的执行目标；任务创建后不可修改"`
	Enabled        *bool                         `json:"enabled,omitempty" jsonschema:"description=是否启用自动化任务；默认 false"`
	Name           *string                       `json:"name,omitempty" jsonschema:"description=任务名称；默认 Automation"`
	Template       *string                       `json:"template,omitempty" jsonschema:"enum=memory_consolidation,enum=review,enum=continue_writing,enum=custom_prompt,description=任务模板；默认 custom_prompt"`
	Prompt         *string                       `json:"prompt,omitempty" jsonschema:"description=任务提示词"`
	ModelProfileID *string                       `json:"model_profile_id,omitempty" jsonschema:"description=可选模型配置 ID"`
	Schedule       *automationScheduleWriteInput `json:"schedule,omitempty" jsonschema:"description=兼容的主调度；省略时为 manual，cron 由后端派生"`
	Triggers       []automationTriggerWriteInput `json:"triggers,omitempty" jsonschema:"description=触发器定义；update 传空数组可清空"`
	WriteMode      *string                       `json:"write_mode,omitempty" jsonschema:"enum=read_only,enum=confirm_write,enum=auto_write,description=写入模式；默认 read_only"`
	WriteScope     *string                       `json:"write_scope,omitempty" jsonschema:"enum=none,enum=lore,enum=file,enum=lore_and_file,description=写入范围；read_only 时固定为 none"`
	OutputPolicy   *string                       `json:"output_policy,omitempty" jsonschema:"enum=run_record_only,enum=optional_file,description=输出策略；默认 run_record_only"`
	OutputPath     *string                       `json:"output_path,omitempty" jsonschema:"description=optional_file 输出路径"`
}

type automationTargetWriteInput struct {
	Kind      string `json:"kind" jsonschema:"enum=user,enum=workspace,description=执行目标类型"`
	Workspace string `json:"workspace,omitempty" jsonschema:"description=workspace 目标路径；省略时使用当前工作区"`
}

type automationScheduleWriteInput struct {
	Kind       string `json:"kind,omitempty" jsonschema:"enum=manual,enum=daily,enum=weekly,enum=monthly,enum=every_hours,description=调度类型；默认 manual"`
	EveryHours int    `json:"every_hours,omitempty" jsonschema:"minimum=1,maximum=168,description=every_hours 的间隔小时数"`
	Weekday    int    `json:"weekday,omitempty" jsonschema:"minimum=0,maximum=6,description=weekly 的星期，0 到 6"`
	DayOfMonth int    `json:"day_of_month,omitempty" jsonschema:"minimum=1,maximum=31,description=monthly 的日期"`
	Hour       int    `json:"hour,omitempty" jsonschema:"minimum=0,maximum=23,description=执行小时"`
	Minute     int    `json:"minute,omitempty" jsonschema:"minimum=0,maximum=59,description=执行分钟"`
}

type automationTriggerWriteInput struct {
	ID                string                        `json:"id,omitempty" jsonschema:"description=稳定触发器 ID；省略时由后端生成"`
	Type              string                        `json:"type,omitempty" jsonschema:"enum=manual,enum=schedule,enum=semantic,enum=chapter_batch,description=触发器类型；默认 manual"`
	Enabled           bool                          `json:"enabled,omitempty" jsonschema:"description=是否启用触发器；默认 false"`
	Name              string                        `json:"name,omitempty" jsonschema:"description=触发器名称"`
	NotifyPolicy      string                        `json:"notify_policy,omitempty" jsonschema:"enum=inbox,enum=silent,description=通知策略"`
	Schedule          *automationScheduleWriteInput `json:"schedule,omitempty" jsonschema:"description=schedule 触发器的调度配置"`
	SemanticCondition string                        `json:"semantic_condition,omitempty" jsonschema:"description=semantic 触发条件"`
	ChapterBatchSize  int                           `json:"chapter_batch_size,omitempty" jsonschema:"minimum=1,description=chapter_batch 或 semantic 的章节批大小"`
}

func (input *automationTaskWriteInput) newTask() automation.Task {
	var task automation.Task
	if input == nil {
		return task
	}
	if input.Target != nil {
		task.Target = automation.ExecutionTarget{
			Kind:      input.Target.Kind,
			Workspace: input.Target.Workspace,
		}
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
		for i := range input.Triggers {
			task.Triggers[i] = input.Triggers[i].toTrigger()
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
		Kind:       input.Kind,
		EveryHours: input.EveryHours,
		Weekday:    input.Weekday,
		DayOfMonth: input.DayOfMonth,
		Hour:       input.Hour,
		Minute:     input.Minute,
	}
}

func (input automationTriggerWriteInput) toTrigger() automation.TriggerDefinition {
	trigger := automation.TriggerDefinition{
		ID:                input.ID,
		Type:              input.Type,
		Enabled:           input.Enabled,
		Name:              input.Name,
		NotifyPolicy:      input.NotifyPolicy,
		SemanticCondition: input.SemanticCondition,
		ChapterBatchSize:  input.ChapterBatchSize,
	}
	if input.Schedule != nil {
		trigger.Schedule = input.Schedule.toSchedule()
	}
	return trigger
}

func newListAutomationsTool(novaDir, workspace string, workspaces []string) (agent.BaseTool, error) {
	return agent.InferTool("list_automations", "列出用户的全局自动化任务索引，按显式执行目标返回 catalog_id、名称、启用状态、模板、触发器和写入策略；需要完整配置时再调用 read_automations。", func(ctx context.Context, input struct{}) (string, error) {
		_ = ctx
		_ = input
		tasks, err := configManagerAutomationStore(novaDir, workspace, workspaces).List()
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		sb.WriteString("# 自动化任务索引\n\n")
		for _, task := range tasks {
			fmt.Fprintf(&sb, "- catalog_id: %s\n  id: %s\n  名称: %s\n  target: %s\n  workspace: %s\n  启用: %t\n  模板: %s\n  触发器: %d\n  写入: %s/%s\n\n", task.CatalogID, task.ID, task.Name, task.Target.Kind, task.Target.Workspace, task.Enabled, task.Template, len(task.Triggers), task.WriteMode, task.WriteScope)
		}
		if len(tasks) == 0 {
			return "暂无自动化任务。", nil
		}
		return strings.TrimSpace(sb.String()), nil
	})
}

func newReadAutomationsTool(novaDir, workspace string, workspaces []string) (agent.BaseTool, error) {
	return agent.InferTool("read_automations", "按自动化任务 catalog_id 批量读取完整任务配置。", func(ctx context.Context, input idListInput) (string, error) {
		_ = ctx
		store := configManagerAutomationStore(novaDir, workspace, workspaces)
		tasks := []automation.Task{}
		for _, id := range input.IDs {
			task, err := store.Get(strings.TrimSpace(id))
			if err != nil {
				return "", err
			}
			tasks = append(tasks, task)
		}
		return marshalToolJSON(tasks)
	})
}

func newWriteAutomationsTool(novaDir, workspace string, workspaces []string) (agent.BaseTool, error) {
	return agent.InferTool("write_automations", "批量创建、更新或删除用户自动化任务；create 必须显式指定 target，update 必须携带 read_automations 返回的 revision，update/delete 使用 catalog_id。删除必须来自用户明确指令。", func(ctx context.Context, input automationWriteInput) (string, error) {
		_ = ctx
		store := configManagerAutomationStore(novaDir, workspace, workspaces)
		result := map[string][]string{"created": []string{}, "updated": []string{}, "deleted": []string{}}
		for i, op := range input.Operations {
			switch strings.TrimSpace(op.Op) {
			case "create":
				if op.Task == nil || op.Task.Target == nil || strings.TrimSpace(op.Task.Target.Kind) == "" {
					return "", fmt.Errorf("自动化操作 #%d create 必须显式指定 task.target.kind", i+1)
				}
				definition := op.Task.newTask()
				task, err := store.Create(definition)
				if err != nil {
					return "", fmt.Errorf("自动化操作 #%d create %q 配置无效: %w", i+1, definition.Name, err)
				}
				result["created"] = append(result["created"], task.CatalogID)
			case "update":
				id := strings.TrimSpace(op.ID)
				if id == "" {
					return "", fmt.Errorf("自动化操作 #%d update 必须显式指定 catalog_id", i+1)
				}
				if op.Task == nil {
					return "", fmt.Errorf("自动化操作 #%d update %q 缺少 task 定义", i+1, id)
				}
				baseRevision := strings.TrimSpace(op.Task.Revision)
				if baseRevision == "" {
					return "", fmt.Errorf("自动化操作 #%d update %q 缺少 revision，请先用 read_automations 读取最新配置", i+1, id)
				}
				current, err := store.Get(id)
				if err != nil {
					return "", fmt.Errorf("自动化操作 #%d update %q 读取失败: %w", i+1, id, err)
				}
				op.Task.applyDefinition(&current)
				task, err := store.UpdateIfRevision(id, current, baseRevision)
				if err != nil {
					return "", fmt.Errorf("自动化操作 #%d update %q 配置无效: %w", i+1, id, err)
				}
				result["updated"] = append(result["updated"], task.CatalogID)
			case "delete":
				id := strings.TrimSpace(op.ID)
				if id == "" {
					return "", fmt.Errorf("自动化操作 #%d delete 必须显式指定 catalog_id", i+1)
				}
				if err := store.Delete(id); err != nil {
					return "", fmt.Errorf("自动化操作 #%d delete %q 失败: %w", i+1, id, err)
				}
				result["deleted"] = append(result["deleted"], id)
			default:
				return "", fmt.Errorf("自动化操作 #%d 不支持的 op: %s", i+1, op.Op)
			}
		}
		return formatBatchResult(firstConfigNonEmpty(input.Message, "自动化任务已更新"), result), nil
	})
}

func configManagerAutomationStore(novaDir, workspace string, workspaces []string) *automation.Store {
	all := append([]string{workspace}, workspaces...)
	return automation.NewStore(novaDir, workspace).WithWorkspaces(all...)
}
