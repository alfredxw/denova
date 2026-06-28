package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"nova/config"
	"nova/internal/automation"
	"nova/internal/imagepreset"
	"nova/internal/interactive"
	novaskills "nova/internal/skills"
)

type idListInput struct {
	IDs []string `json:"ids" jsonschema:"description=要读取的资源 ID 列表"`
}

type tellerWriteInput struct {
	Message    string                 `json:"message" jsonschema:"description=本次叙事方案变更说明"`
	Operations []tellerWriteOperation `json:"operations" jsonschema:"description=批量叙事方案操作"`
}

type tellerWriteOperation struct {
	Op     string             `json:"op" jsonschema:"description=操作类型：create/update/delete"`
	ID     string             `json:"id" jsonschema:"description=目标导演 ID；update/delete 必填"`
	Teller interactive.Teller `json:"teller" jsonschema:"description=create/update 使用的完整导演配置"`
}

type imagePresetWriteInput struct {
	Message    string                      `json:"message" jsonschema:"description=本次图像方案变更说明"`
	Operations []imagePresetWriteOperation `json:"operations" jsonschema:"description=批量图像方案操作"`
}

type imagePresetWriteOperation struct {
	Op     string             `json:"op" jsonschema:"description=操作类型：create/update/delete"`
	ID     string             `json:"id" jsonschema:"description=目标图像方案 ID；update/delete 必填"`
	Preset imagepreset.Preset `json:"preset" jsonschema:"description=create/update 使用的完整图像方案配置；slots 只支持 target=agent_system 或 tool_request"`
}

type automationWriteInput struct {
	Message    string                     `json:"message" jsonschema:"description=本次自动化任务变更说明"`
	Operations []automationWriteOperation `json:"operations" jsonschema:"description=批量自动化任务操作"`
}

type automationWriteOperation struct {
	Op   string          `json:"op" jsonschema:"description=操作类型：create/update/delete"`
	ID   string          `json:"id" jsonschema:"description=目标自动化任务 ID；update/delete 必填"`
	Task automation.Task `json:"task" jsonschema:"description=create/update 使用的自动化任务配置"`
}

type skillRef struct {
	Scope string `json:"scope" jsonschema:"description=Skill 作用域：user 或 workspace"`
	Name  string `json:"name" jsonschema:"description=Skill 名称"`
}

type readSkillsInput struct {
	Items []skillRef `json:"items" jsonschema:"description=要读取的 Skill 列表，每项包含 scope 和 name"`
}

type skillsWriteInput struct {
	Message    string                `json:"message" jsonschema:"description=本次 Skills 变更说明"`
	Operations []skillWriteOperation `json:"operations" jsonschema:"description=批量 Skill 操作"`
}

type skillWriteOperation struct {
	Op          string   `json:"op" jsonschema:"description=操作类型：create/update/delete"`
	Scope       string   `json:"scope" jsonschema:"description=Skill 作用域：user 或 workspace"`
	Name        string   `json:"name" jsonschema:"description=Skill 名称"`
	Description string   `json:"description" jsonschema:"description=create 且 content 为空时使用的描述"`
	Agents      []string `json:"agents" jsonschema:"description=create 且 content 为空时写入 front matter 的 Agent 列表"`
	Content     string   `json:"content" jsonschema:"description=create/update 使用的完整 SKILL.md 内容"`
}

type storyMemoryInput struct {
	StoryID         string   `json:"story_id" jsonschema:"description=互动故事 ID"`
	BranchID        string   `json:"branch_id,omitempty" jsonschema:"description=分支 ID；为空时使用当前分支"`
	IncludeArchived bool     `json:"include_archived,omitempty" jsonschema:"description=是否包含归档记录"`
	IDs             []string `json:"ids,omitempty" jsonschema:"description=要读取的故事记忆记录 ID 列表"`
}

type storyMemoryStructureWriteInput struct {
	StoryID    string                               `json:"story_id" jsonschema:"description=互动故事 ID"`
	Message    string                               `json:"message" jsonschema:"description=本次故事记忆结构变更说明"`
	Operations []storyMemoryStructureWriteOperation `json:"operations" jsonschema:"description=批量故事记忆结构操作"`
}

type storyMemoryStructureWriteOperation struct {
	Op        string                                  `json:"op" jsonschema:"description=操作类型：create/update/delete"`
	ID        string                                  `json:"id" jsonschema:"description=目标结构 ID；update/delete 必填"`
	Structure interactive.StoryMemoryStructureRequest `json:"structure" jsonschema:"description=create/update 使用的完整结构定义"`
}

type storyMemoryRecordWriteInput struct {
	StoryID    string                            `json:"story_id" jsonschema:"description=互动故事 ID"`
	BranchID   string                            `json:"branch_id,omitempty" jsonschema:"description=分支 ID；为空时使用当前分支"`
	Message    string                            `json:"message" jsonschema:"description=本次故事记忆记录变更说明"`
	Operations []storyMemoryRecordWriteOperation `json:"operations" jsonschema:"description=批量故事记忆记录操作"`
}

type storyMemoryRecordWriteOperation struct {
	Op     string                               `json:"op" jsonschema:"description=操作类型：create/update/archive/restore/delete"`
	ID     string                               `json:"id" jsonschema:"description=目标记录 ID；update/archive/restore/delete 必填"`
	Record interactive.StoryMemoryRecordRequest `json:"record" jsonschema:"description=create/update 使用的故事记忆记录"`
}

type configManagerToolBuilder struct {
	enabled bool
	build   func() (tool.BaseTool, error)
}

func newConfigManagerTools(cfg *config.Config, settings config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	novaDir := strings.TrimSpace(cfg.NovaDir)
	workspace := strings.TrimSpace(cfg.Workspace)
	builders := []configManagerToolBuilder{
		{enabled: settings.LoreRead, build: func() (tool.BaseTool, error) { return newListTellersTool(novaDir) }},
		{enabled: settings.LoreRead, build: func() (tool.BaseTool, error) { return newReadTellersTool(novaDir) }},
		{enabled: settings.LoreWrite, build: func() (tool.BaseTool, error) { return newWriteTellersTool(novaDir) }},
		{enabled: settings.LoreRead, build: func() (tool.BaseTool, error) { return newListImagePresetsTool(novaDir) }},
		{enabled: settings.LoreRead, build: func() (tool.BaseTool, error) { return newReadImagePresetsTool(novaDir) }},
		{enabled: settings.LoreWrite, build: func() (tool.BaseTool, error) { return newWriteImagePresetsTool(novaDir) }},
		{enabled: settings.Todo, build: func() (tool.BaseTool, error) { return newListAutomationsTool(novaDir, workspace) }},
		{enabled: settings.Todo, build: func() (tool.BaseTool, error) { return newReadAutomationsTool(novaDir, workspace) }},
		{enabled: settings.Todo, build: func() (tool.BaseTool, error) { return newWriteAutomationsTool(novaDir, workspace) }},
		{enabled: settings.Skills, build: func() (tool.BaseTool, error) { return newListSkillsTool(cfg) }},
		{enabled: settings.Skills, build: func() (tool.BaseTool, error) { return newReadSkillsTool(cfg) }},
		{enabled: settings.Skills, build: func() (tool.BaseTool, error) { return newWriteSkillsTool(cfg) }},
		{enabled: settings.AgentConfigRead, build: func() (tool.BaseTool, error) { return newListAgentConfigsTool(cfg) }},
		{enabled: settings.AgentConfigWrite, build: func() (tool.BaseTool, error) { return newWriteAgentConfigsTool(cfg) }},
		{enabled: settings.LoreRead, build: func() (tool.BaseTool, error) { return newListStoryMemoryStructuresTool(workspace) }},
		{enabled: settings.LoreWrite, build: func() (tool.BaseTool, error) { return newWriteStoryMemoryStructuresTool(workspace) }},
		{enabled: settings.LoreRead, build: func() (tool.BaseTool, error) { return newListStoryMemoryRecordsTool(workspace) }},
		{enabled: settings.LoreRead, build: func() (tool.BaseTool, error) { return newReadStoryMemoryRecordsTool(workspace) }},
		{enabled: settings.LoreWrite, build: func() (tool.BaseTool, error) { return newWriteStoryMemoryRecordsTool(workspace) }},
	}
	tools := make([]tool.BaseTool, 0, len(builders))
	for _, builder := range builders {
		if !builder.enabled {
			continue
		}
		t, err := builder.build()
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	return tools, nil
}

func newListImagePresetsTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("list_image_presets", "列出图像方案索引，返回 ID、名称、简介、标签、类型和注入规则概览；需要完整 slots 内容时再调用 read_image_presets。", func(ctx context.Context, input struct{}) (string, error) {
		_ = ctx
		_ = input
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取图像方案")
		}
		presets, err := imagepreset.NewLibrary(novaDir).List()
		if err != nil {
			return "", err
		}
		if len(presets) == 0 {
			return "暂无图像方案。", nil
		}
		var sb strings.Builder
		sb.WriteString("# 图像方案索引\n\n")
		for _, preset := range presets {
			fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  类型: %s\n", preset.ID, preset.Name, boolLabel(preset.Custom, "custom", "built-in"))
			if preset.Description != "" {
				fmt.Fprintf(&sb, "  简介: %s\n", preset.Description)
			}
			if len(preset.Tags) > 0 {
				fmt.Fprintf(&sb, "  标签: %s\n", strings.Join(preset.Tags, "、"))
			}
			if len(preset.Slots) > 0 {
				enabled := 0
				for _, slot := range preset.Slots {
					if slot.Enabled {
						enabled++
					}
				}
				fmt.Fprintf(&sb, "  注入规则: %d/%d 启用\n", enabled, len(preset.Slots))
			}
			sb.WriteString("\n")
		}
		return strings.TrimSpace(sb.String()), nil
	})
}

func newReadImagePresetsTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("read_image_presets", "按图像方案 ID 批量读取完整图像方案配置。图像方案使用 slots：agent_system 注入图像提示构造 Agent 的 system prompt，tool_request 原样前置注入最终图像请求 prompt。", func(ctx context.Context, input idListInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取图像方案")
		}
		lib := imagepreset.NewLibrary(novaDir)
		result := []imagepreset.Preset{}
		for _, id := range input.IDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			preset, err := lib.Get(id)
			if err != nil {
				return "", err
			}
			result = append(result, preset)
		}
		return marshalToolJSON(result)
	})
}

func newWriteImagePresetsTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("write_image_presets", "批量创建、更新或删除图像方案配置。create/update 必须写完整 slots；target 仅支持 agent_system 和 tool_request。旧 prompt 字段只作为兼容输入，会被后端转换为 tool_request slot。删除内置图像方案会被后端拒绝；删除必须来自用户明确指令。", func(ctx context.Context, input imagePresetWriteInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法写入图像方案")
		}
		lib := imagepreset.NewLibrary(novaDir)
		result := map[string][]string{"created": []string{}, "updated": []string{}, "deleted": []string{}}
		for _, op := range input.Operations {
			switch strings.TrimSpace(op.Op) {
			case "create":
				preset, err := lib.Create(op.Preset)
				if err != nil {
					return "", err
				}
				result["created"] = append(result["created"], preset.ID)
			case "update":
				id := firstConfigNonEmpty(op.ID, op.Preset.ID)
				preset, err := lib.Update(id, op.Preset)
				if err != nil {
					return "", err
				}
				result["updated"] = append(result["updated"], preset.ID)
			case "delete":
				id := strings.TrimSpace(op.ID)
				if err := lib.Delete(id); err != nil {
					return "", err
				}
				result["deleted"] = append(result["deleted"], id)
			default:
				return "", fmt.Errorf("未知图像方案操作: %s", op.Op)
			}
		}
		return marshalToolJSON(result)
	})
}

func newListTellersTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("list_tellers", "列出叙事方案索引，返回 ID、名称、简介、标签和槽位概览；需要完整配置时再调用 read_tellers。", func(ctx context.Context, input struct{}) (string, error) {
		_ = ctx
		_ = input
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取叙事方案")
		}
		tellers, err := interactive.NewTellerLibrary(novaDir).List()
		if err != nil {
			return "", err
		}
		if len(tellers) == 0 {
			return "暂无叙事方案。", nil
		}
		var sb strings.Builder
		sb.WriteString("# 叙事方案索引\n\n")
		for _, teller := range tellers {
			fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  类型: %s\n  槽位: %d\n", teller.ID, teller.Name, boolLabel(teller.Custom, "custom", "built-in"), len(teller.Slots))
			if teller.Description != "" {
				fmt.Fprintf(&sb, "  简介: %s\n", teller.Description)
			}
			if len(teller.Tags) > 0 {
				fmt.Fprintf(&sb, "  标签: %s\n", strings.Join(teller.Tags, "、"))
			}
			sb.WriteString("\n")
		}
		return strings.TrimSpace(sb.String()), nil
	})
}

func newReadTellersTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("read_tellers", "按叙事方案 ID 批量读取完整配置。", func(ctx context.Context, input idListInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取叙事方案")
		}
		lib := interactive.NewTellerLibrary(novaDir)
		result := []interactive.Teller{}
		for _, id := range input.IDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			teller, err := lib.Get(id)
			if err != nil {
				return "", err
			}
			result = append(result, teller)
		}
		return marshalToolJSON(result)
	})
}

func newWriteTellersTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("write_tellers", "批量创建、更新或删除叙事方案配置。删除内置方案会被后端拒绝；删除必须来自用户明确指令。", func(ctx context.Context, input tellerWriteInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法写入叙事方案")
		}
		lib := interactive.NewTellerLibrary(novaDir)
		result := map[string][]string{"created": []string{}, "updated": []string{}, "deleted": []string{}}
		for _, op := range input.Operations {
			switch strings.TrimSpace(op.Op) {
			case "create":
				teller, err := lib.Create(op.Teller)
				if err != nil {
					return "", err
				}
				result["created"] = append(result["created"], teller.ID)
			case "update":
				id := firstConfigNonEmpty(op.ID, op.Teller.ID)
				teller, err := lib.Update(id, op.Teller)
				if err != nil {
					return "", err
				}
				result["updated"] = append(result["updated"], teller.ID)
			case "delete":
				id := strings.TrimSpace(op.ID)
				if err := lib.Delete(id); err != nil {
					return "", err
				}
				result["deleted"] = append(result["deleted"], id)
			default:
				return "", fmt.Errorf("不支持的叙事方案操作: %s", op.Op)
			}
		}
		return formatBatchResult(firstConfigNonEmpty(input.Message, "叙事方案已更新"), result), nil
	})
}

func newListAutomationsTool(novaDir, workspace string) (tool.BaseTool, error) {
	return utils.InferTool("list_automations", "列出自动化任务索引，返回 ID、名称、启用状态、模板、触发器和写入策略；需要完整配置时再调用 read_automations。", func(ctx context.Context, input struct{}) (string, error) {
		_ = ctx
		_ = input
		tasks, err := automation.NewStore(novaDir, workspace).List()
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		sb.WriteString("# 自动化任务索引\n\n")
		for _, task := range tasks {
			fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  scope: %s\n  启用: %t\n  模板: %s\n  触发器: %d\n  写入: %s/%s\n\n", task.ID, task.Name, task.Scope, task.Enabled, task.Template, len(task.Triggers), task.WriteMode, task.WriteScope)
		}
		if len(tasks) == 0 {
			return "暂无自动化任务。", nil
		}
		return strings.TrimSpace(sb.String()), nil
	})
}

func newReadAutomationsTool(novaDir, workspace string) (tool.BaseTool, error) {
	return utils.InferTool("read_automations", "按自动化任务 ID 批量读取完整任务配置。", func(ctx context.Context, input idListInput) (string, error) {
		_ = ctx
		store := automation.NewStore(novaDir, workspace)
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

func newWriteAutomationsTool(novaDir, workspace string) (tool.BaseTool, error) {
	return utils.InferTool("write_automations", "批量创建、更新或删除自动化任务。删除必须来自用户明确指令。", func(ctx context.Context, input automationWriteInput) (string, error) {
		_ = ctx
		store := automation.NewStore(novaDir, workspace)
		result := map[string][]string{"created": []string{}, "updated": []string{}, "deleted": []string{}}
		for i, op := range input.Operations {
			switch strings.TrimSpace(op.Op) {
			case "create":
				task, err := store.Create(op.Task)
				if err != nil {
					return "", fmt.Errorf("自动化操作 #%d create %q 配置无效: %w", i+1, op.Task.Name, err)
				}
				result["created"] = append(result["created"], task.ID)
			case "update":
				id := firstConfigNonEmpty(op.ID, op.Task.ID)
				task, err := store.Update(id, op.Task)
				if err != nil {
					return "", fmt.Errorf("自动化操作 #%d update %q 配置无效: %w", i+1, id, err)
				}
				result["updated"] = append(result["updated"], task.ID)
			case "delete":
				id := strings.TrimSpace(op.ID)
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

func newListSkillsTool(cfg *config.Config) (tool.BaseTool, error) {
	return utils.InferTool("list_skills", "列出 Skills 索引，返回名称、scope、agent、描述、是否可编辑和是否生效；需要完整 SKILL.md 时再调用 read_skills。", func(ctx context.Context, input struct{}) (string, error) {
		_ = input
		snapshot, err := novaskills.SnapshotFor(ctx, skillDirs(cfg))
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		sb.WriteString("# Skills 索引\n\n")
		for _, skill := range snapshot.Skills {
			fmt.Fprintf(&sb, "- name: %s\n  scope: %s\n  active: %t\n  editable: %t\n  agent: %s\n  description: %s\n\n", skill.Name, skill.Scope, skill.Active, skill.Editable, skill.Agent, skill.Description)
		}
		if len(snapshot.Skills) == 0 {
			return "暂无 Skills。", nil
		}
		return strings.TrimSpace(sb.String()), nil
	})
}

func newReadSkillsTool(cfg *config.Config) (tool.BaseTool, error) {
	return utils.InferTool("read_skills", "按 scope/name 批量读取完整 SKILL.md。", func(ctx context.Context, input readSkillsInput) (string, error) {
		docs := []novaskills.Document{}
		for _, item := range input.Items {
			doc, err := novaskills.ReadDocument(ctx, skillDirs(cfg), novaskills.Scope(strings.TrimSpace(item.Scope)), strings.TrimSpace(item.Name))
			if err != nil {
				return "", err
			}
			docs = append(docs, doc)
		}
		return marshalToolJSON(docs)
	})
}

func newWriteSkillsTool(cfg *config.Config) (tool.BaseTool, error) {
	return utils.InferTool("write_skills", "批量创建、更新或删除 Skills。scope 必须是 user 或 workspace；修改内置/预制 Skill 时使用 workspace 同名覆盖，禁止写 builtin；删除必须来自用户明确指令。", func(ctx context.Context, input skillsWriteInput) (string, error) {
		result := map[string][]string{"created": []string{}, "updated": []string{}, "deleted": []string{}}
		for _, op := range input.Operations {
			scope := novaskills.Scope(strings.TrimSpace(op.Scope))
			name := strings.TrimSpace(op.Name)
			switch strings.TrimSpace(op.Op) {
			case "create":
				var doc novaskills.Document
				var err error
				if strings.TrimSpace(op.Content) == "" {
					doc, err = novaskills.CreateDocument(ctx, skillDirs(cfg), scope, name, op.Description, op.Agents...)
				} else {
					doc, err = novaskills.SaveDocument(ctx, skillDirs(cfg), scope, name, op.Content)
				}
				if err != nil {
					return "", err
				}
				result["created"] = append(result["created"], string(doc.Scope)+"/"+doc.Name)
			case "update":
				doc, err := novaskills.SaveDocument(ctx, skillDirs(cfg), scope, name, op.Content)
				if err != nil {
					return "", err
				}
				result["updated"] = append(result["updated"], string(doc.Scope)+"/"+doc.Name)
			case "delete":
				if err := novaskills.DeleteDocument(ctx, skillDirs(cfg), scope, name); err != nil {
					return "", err
				}
				result["deleted"] = append(result["deleted"], string(scope)+"/"+name)
			default:
				return "", fmt.Errorf("不支持的 Skill 操作: %s", op.Op)
			}
		}
		return formatBatchResult(firstConfigNonEmpty(input.Message, "Skills 已更新"), result), nil
	})
}

func newListStoryMemoryStructuresTool(workspace string) (tool.BaseTool, error) {
	return utils.InferTool("list_story_memory_structures", "读取某个互动故事的完整故事记忆结构定义；结构数量较少，本工具直接返回完整结构，无需 read 工具。", func(ctx context.Context, input storyMemoryInput) (string, error) {
		_ = ctx
		state, err := interactive.NewStore(workspace).StoryMemory(input.StoryID, input.BranchID, input.IncludeArchived)
		if err != nil {
			return "", err
		}
		return marshalToolJSON(state.Structures)
	})
}

func newWriteStoryMemoryStructuresTool(workspace string) (tool.BaseTool, error) {
	return utils.InferTool("write_story_memory_structures", "批量创建、更新或删除故事记忆结构。只改结构定义，不改故事记忆记录内容。", func(ctx context.Context, input storyMemoryStructureWriteInput) (string, error) {
		_ = ctx
		store := interactive.NewStore(workspace)
		result := map[string][]string{"created": []string{}, "updated": []string{}, "deleted": []string{}}
		for _, op := range input.Operations {
			switch strings.TrimSpace(op.Op) {
			case "create":
				structure, err := store.SaveStoryMemoryStructure(input.StoryID, op.Structure)
				if err != nil {
					return "", err
				}
				result["created"] = append(result["created"], structure.ID)
			case "update":
				req := op.Structure
				if req.ID == "" {
					req.ID = op.ID
				}
				structure, err := store.SaveStoryMemoryStructure(input.StoryID, req)
				if err != nil {
					return "", err
				}
				result["updated"] = append(result["updated"], structure.ID)
			case "delete":
				id := strings.TrimSpace(op.ID)
				if err := store.DeleteStoryMemoryStructure(input.StoryID, id); err != nil {
					return "", err
				}
				result["deleted"] = append(result["deleted"], id)
			default:
				return "", fmt.Errorf("不支持的故事记忆结构操作: %s", op.Op)
			}
		}
		return formatBatchResult(firstConfigNonEmpty(input.Message, "故事记忆结构已更新"), result), nil
	})
}

func newListStoryMemoryRecordsTool(workspace string) (tool.BaseTool, error) {
	return utils.InferTool("list_story_memory_records", "列出某个互动故事当前分支的故事记忆记录索引；需要完整 values 时再调用 read_story_memory_records。", func(ctx context.Context, input storyMemoryInput) (string, error) {
		_ = ctx
		state, err := interactive.NewStore(workspace).StoryMemory(input.StoryID, input.BranchID, input.IncludeArchived)
		if err != nil {
			return "", err
		}
		var sb strings.Builder
		sb.WriteString("# 故事记忆记录索引\n\n")
		for _, record := range state.Records {
			fmt.Fprintf(&sb, "- id: %s\n  structure_id: %s\n  key: %s\n  archived: %t\n  branch: %s\n  updated_at: %s\n\n", record.ID, record.StructureID, record.Key, record.Archived, record.BranchID, record.UpdatedAt)
		}
		if len(state.Records) == 0 {
			return "暂无故事记忆记录。", nil
		}
		return strings.TrimSpace(sb.String()), nil
	})
}

func newReadStoryMemoryRecordsTool(workspace string) (tool.BaseTool, error) {
	return utils.InferTool("read_story_memory_records", "按记录 ID 批量读取故事记忆记录详情。", func(ctx context.Context, input storyMemoryInput) (string, error) {
		_ = ctx
		state, err := interactive.NewStore(workspace).StoryMemory(input.StoryID, input.BranchID, true)
		if err != nil {
			return "", err
		}
		want := map[string]bool{}
		for _, id := range input.IDs {
			if id = strings.TrimSpace(id); id != "" {
				want[id] = true
			}
		}
		records := []interactive.StoryMemoryRecord{}
		for _, record := range state.Records {
			if want[record.ID] {
				records = append(records, record)
			}
		}
		return marshalToolJSON(records)
	})
}

func newWriteStoryMemoryRecordsTool(workspace string) (tool.BaseTool, error) {
	return utils.InferTool("write_story_memory_records", "批量创建、更新、归档或恢复故事记忆记录。只改记录内容，不改故事记忆结构定义；delete 等同归档。", func(ctx context.Context, input storyMemoryRecordWriteInput) (string, error) {
		_ = ctx
		store := interactive.NewStore(workspace)
		result := map[string][]string{"created": []string{}, "updated": []string{}, "archived": []string{}, "restored": []string{}}
		for _, op := range input.Operations {
			switch strings.TrimSpace(op.Op) {
			case "create":
				record, err := store.SaveStoryMemoryRecord(input.StoryID, withRecordBranch(op.Record, input.BranchID))
				if err != nil {
					return "", err
				}
				result["created"] = append(result["created"], record.ID)
			case "update":
				req := withRecordBranch(op.Record, input.BranchID)
				if req.ID == "" {
					req.ID = op.ID
				}
				record, err := store.SaveStoryMemoryRecord(input.StoryID, req)
				if err != nil {
					return "", err
				}
				result["updated"] = append(result["updated"], record.ID)
			case "archive", "delete":
				record, err := store.SetStoryMemoryRecordArchived(input.StoryID, op.ID, input.BranchID, true)
				if err != nil {
					return "", err
				}
				result["archived"] = append(result["archived"], record.ID)
			case "restore":
				record, err := store.SetStoryMemoryRecordArchived(input.StoryID, op.ID, input.BranchID, false)
				if err != nil {
					return "", err
				}
				result["restored"] = append(result["restored"], record.ID)
			default:
				return "", fmt.Errorf("不支持的故事记忆记录操作: %s", op.Op)
			}
		}
		return formatBatchResult(firstConfigNonEmpty(input.Message, "故事记忆记录已更新"), result), nil
	})
}

func skillDirs(cfg *config.Config) []novaskills.Directory {
	if cfg == nil {
		return nil
	}
	return novaskills.NewDirectories(cfg.SkillsDir, cfg.NovaDir, cfg.Workspace)
}

func withRecordBranch(req interactive.StoryMemoryRecordRequest, branchID string) interactive.StoryMemoryRecordRequest {
	if strings.TrimSpace(req.BranchID) == "" {
		req.BranchID = strings.TrimSpace(branchID)
	}
	return req
}

func marshalToolJSON(v any) (string, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func formatBatchResult(message string, result map[string][]string) string {
	data, _ := json.Marshal(result)
	return strings.TrimSpace(message) + "\n" + string(data)
}

func firstConfigNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func boolLabel(value bool, trueLabel, falseLabel string) string {
	if value {
		return trueLabel
	}
	return falseLabel
}
