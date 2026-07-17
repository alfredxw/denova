package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"denova/config"
	"denova/internal/automation"
	"denova/internal/imagepreset"
	"denova/internal/interactive"
	novaskills "denova/internal/skills"
	"denova/internal/styleref"
)

type idListInput struct {
	IDs []string `json:"ids" jsonschema:"description=要读取的资源 ID 列表"`
}

type tellerWriteInput struct {
	Message    string                 `json:"message" jsonschema:"description=本次叙事风格变更说明"`
	Operations []tellerWriteOperation `json:"operations" jsonschema:"description=批量叙事风格操作"`
}

type tellerWriteOperation struct {
	Op     string             `json:"op" jsonschema:"description=操作类型：create/update/delete"`
	ID     string             `json:"id" jsonschema:"description=目标叙事风格 ID；update/delete 必填"`
	Teller interactive.Teller `json:"teller" jsonschema:"description=create/update 使用的完整叙事风格配置；这里只维护文风、提示词槽位、文风参考和上下文策略，故事编排使用 story_directors"`
}

type styleReferenceWriteInput struct {
	Message    string                         `json:"message" jsonschema:"description=本次文风参考变更说明"`
	Operations []styleReferenceWriteOperation `json:"operations" jsonschema:"description=批量文风参考操作"`
}

type styleReferenceWriteOperation struct {
	Op        string                `json:"op" jsonschema:"description=操作类型：create/update/delete"`
	Path      string                `json:"path" jsonschema:"description=delete 使用的文风参考路径，例如 .denova/styles/name.md"`
	Reference styleref.WriteRequest `json:"reference" jsonschema:"description=create/update 使用的 Markdown 文风参考；content 必须是最终提炼后的 md，不要写原始长文"`
}

type storyDirectorWriteInput struct {
	Message    string                        `json:"message" jsonschema:"description=本次故事导演变更说明"`
	Operations []storyDirectorWriteOperation `json:"operations" jsonschema:"description=批量故事导演操作"`
}

type storyDirectorWriteOperation struct {
	Op       string                    `json:"op" jsonschema:"description=操作类型：create/update/delete"`
	ID       string                    `json:"id" jsonschema:"description=目标故事导演 ID；update/delete 必填"`
	Director interactive.StoryDirector `json:"director" jsonschema:"description=create/update 使用的完整故事导演配置；module_refs 保存叙事风格、多个事件包、TRPG 检定、状态系统（actor_state）和图像方案引用，并用 *_disabled 显式关闭某个模块；事件包使用 event_package_ids；TRPG 检定使用 rule_system_id 选择一个 DM 检定风格资源；状态系统使用 actor_state_id，状态模板可表示故事上下文、主角、重要角色、敌人、怪物、世界、故事倒计时、势力、基地或副本等 Actor；词条库属于状态系统 actor_state.trait_pools，模板通过 trait_rules 声明可用池和 draw_count；TRPG 资源只配置一项 rule_templates[0]，检定固定 d20，可配置 trigger、must_check_examples、skip_check_examples、difficulty_guidance、state_effect_guidance 和 state_bindings；带 state_bindings 的 TRPG 资源需要 actor_state_id；strategy 建议使用枚举 mainline_strength=soft_guidance/balanced/strong_arc，failure_policy=reversible/consequence/fail_forward，pacing_curve=progressive/wave/goal-pressure-payoff，event_frequency=off/sparse/balanced/frequent，state_schema_adaptation_mode=after_opening/off 默认 after_opening，rule_state_consumption_mode=hybrid_auto/director_only 默认 hybrid_auto，rule_visibility_mode=audit_only/public_roll 默认 audit_only，branch_planning_turns 默认 5；strategy.planning_templates.plan 可配置单份 director.md Markdown 模板且必须保留固定标题；strategy.prompt_markdown 可写纯 Markdown 高级策略提示，最多 64KB，不能覆盖结构化策略和输出协议"`
}

type eventPackageWriteInput struct {
	Message    string                       `json:"message" jsonschema:"description=本次事件包变更说明"`
	Operations []eventPackageWriteOperation `json:"operations" jsonschema:"description=批量事件包操作"`
}

type eventPackageWriteOperation struct {
	Op      string                         `json:"op" jsonschema:"description=操作类型：create/update/delete"`
	ID      string                         `json:"id" jsonschema:"description=目标事件包 ID；update/delete 必填"`
	Package interactive.EventPackageModule `json:"package" jsonschema:"description=create/update 使用的完整事件包配置；events 是事件卡列表"`
}

type actorStateWriteInput struct {
	Message    string                     `json:"message" jsonschema:"description=本次状态系统变更说明"`
	Operations []actorStateWriteOperation `json:"operations" jsonschema:"description=批量状态系统操作"`
}

type actorStateWriteOperation struct {
	Op         string                       `json:"op" jsonschema:"description=操作类型：create/update/delete"`
	ID         string                       `json:"id" jsonschema:"description=目标状态系统 ID；update/delete 必填"`
	ActorState interactive.ActorStateModule `json:"actor_state" jsonschema:"description=create/update 使用的完整状态系统模块配置；actor_state.templates 定义状态模板和字段 schema，templates[].trait_rules 通过 pool_id 与 draw_count 绑定词条池；actor_state.trait_pools 是可复用词条库，词条只含 id、name、summary、weight、visibility，不得含 ops；initial_actors 是初始 Actor 实例。Actor 创建时后端会写入默认值、实例覆盖值并自动抽取词条快照"`
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

type configManagerToolBuilder struct {
	build func() (tool.BaseTool, error)
}

func newConfigManagerTools(cfg *config.Config, settings config.ResolvedAgentToolSettings) ([]tool.BaseTool, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	_ = settings
	novaDir := strings.TrimSpace(cfg.NovaDir)
	workspace := strings.TrimSpace(cfg.Workspace)
	builders := []configManagerToolBuilder{
		{build: func() (tool.BaseTool, error) { return newListStyleReferencesTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newWriteStyleReferencesTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newListTellersTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newReadTellersTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newWriteTellersTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newListStoryDirectorsTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newReadStoryDirectorsTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newWriteStoryDirectorsTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newListEventPackagesTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newReadEventPackagesTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newWriteEventPackagesTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newListActorStatesTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newReadActorStatesTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newWriteActorStatesTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newListImagePresetsTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newReadImagePresetsTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newWriteImagePresetsTool(novaDir) }},
		{build: func() (tool.BaseTool, error) { return newListAutomationsTool(novaDir, workspace) }},
		{build: func() (tool.BaseTool, error) { return newReadAutomationsTool(novaDir, workspace) }},
		{build: func() (tool.BaseTool, error) { return newWriteAutomationsTool(novaDir, workspace) }},
		{build: func() (tool.BaseTool, error) { return newListSkillsTool(cfg) }},
		{build: func() (tool.BaseTool, error) { return newReadSkillsTool(cfg) }},
		{build: func() (tool.BaseTool, error) { return newWriteSkillsTool(cfg) }},
		{build: func() (tool.BaseTool, error) { return newListAgentConfigsTool(cfg) }},
		{build: func() (tool.BaseTool, error) { return newWriteAgentConfigsTool(cfg) }},
	}
	tools := make([]tool.BaseTool, 0, len(builders)+2)
	for _, builder := range builders {
		t, err := builder.build()
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	loreTools, err := newLoreTools(workspace, true)
	if err != nil {
		return nil, err
	}
	tools = append(tools, loreTools...)
	return tools, nil
}

func newListImagePresetsTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("list_image_presets", "列出图像方案索引，返回 ID、名称、简介、类型和注入规则概览；图像方案是共享模块，可用于写作模式和游戏模式；需要完整 slots 内容时再调用 read_image_presets。", func(ctx context.Context, input struct{}) (string, error) {
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
			fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  类型: %s\n  适用: 共享模块（写作模式 / 游戏模式）\n", preset.ID, preset.Name, boolLabel(preset.Custom, "custom", "built-in"))
			if preset.Description != "" {
				fmt.Fprintf(&sb, "  简介: %s\n", preset.Description)
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
	return utils.InferTool("read_image_presets", "按图像方案 ID 批量读取完整图像方案配置。图像方案是共享模块，使用 slots：agent_system 注入图像提示构造 Agent 的 system prompt，tool_request 原样前置注入最终图像请求 prompt。", func(ctx context.Context, input idListInput) (string, error) {
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
	return utils.InferTool("write_image_presets", "批量创建、更新或删除图像方案配置。图像方案是共享模块，不存在每个方案可配置的模式字段。create/update 必须写完整 slots；target 仅支持 agent_system 和 tool_request。旧 prompt 字段只作为兼容输入，会被后端转换为 tool_request slot。删除内置图像方案会被后端拒绝；删除必须来自用户明确指令。", func(ctx context.Context, input imagePresetWriteInput) (string, error) {
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

func newListStyleReferencesTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("list_style_references", "列出共享文风参考索引。文风参考统一位于 .denova/styles/，返回 name、description、path；叙事风格的 style_rules 只能引用这些 path，不应内联长文风内容。", func(ctx context.Context, input struct{}) (string, error) {
		_ = ctx
		_ = input
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取文风参考")
		}
		refs, err := styleref.NewLibrary(novaDir).List()
		if err != nil {
			return "", err
		}
		if len(refs) == 0 {
			return "暂无共享文风参考。", nil
		}
		var sb strings.Builder
		sb.WriteString("# 共享文风参考索引\n\n")
		for _, ref := range refs {
			fmt.Fprintf(&sb, "- name: %s\n  description: %s\n  path: %s\n", ref.Name, ref.Description, ref.DisplayPath)
			if ref.Missing {
				fmt.Fprintf(&sb, "  status: missing %s\n", ref.Error)
			}
			sb.WriteString("\n")
		}
		return strings.TrimSpace(sb.String()), nil
	})
}

func newWriteStyleReferencesTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("write_style_references", "批量创建、更新或删除共享文风参考 Markdown。用于把用户源文件提炼为 .denova/styles/*.md；content 必须是最终可复用的 md 文风参考，以提炼出的典型参考段落为主，辅以风格总结，不要写现实作者名、作品名、来源说明或大段原文。", func(ctx context.Context, input styleReferenceWriteInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法写入文风参考")
		}
		lib := styleref.NewLibrary(novaDir)
		result := map[string][]string{"created": []string{}, "updated": []string{}, "deleted": []string{}}
		for _, op := range input.Operations {
			switch strings.TrimSpace(op.Op) {
			case "create", "update":
				req := op.Reference
				if strings.TrimSpace(op.Op) == "update" && strings.TrimSpace(req.Filename) == "" {
					if stored := styleref.NormalizeStoragePath(op.Path); stored != "" {
						req.Filename = path.Base(stored)
					}
				}
				ref, err := lib.Write(req)
				if err != nil {
					return "", err
				}
				if strings.TrimSpace(op.Op) == "update" {
					result["updated"] = append(result["updated"], ref.DisplayPath)
				} else {
					result["created"] = append(result["created"], ref.DisplayPath)
				}
			case "delete":
				path := strings.TrimSpace(op.Path)
				if path == "" {
					path = strings.TrimSpace(op.Reference.Filename)
				}
				if err := lib.Delete(path); err != nil {
					return "", err
				}
				result["deleted"] = append(result["deleted"], styleref.NormalizeStoragePath(path))
			default:
				return "", fmt.Errorf("不支持的文风参考操作: %s", op.Op)
			}
		}
		return formatBatchResult(firstConfigNonEmpty(input.Message, "共享文风参考已更新"), result), nil
	})
}

func newListTellersTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("list_tellers", "列出叙事风格索引，返回 ID、名称、简介和槽位概览；叙事风格是共享模块，可用于写作模式和游戏模式；需要完整配置时再调用 read_tellers。叙事风格只负责文风、提示词槽位、场景风格和上下文策略；场景风格应引用 list_style_references 返回的共享 path。", func(ctx context.Context, input struct{}) (string, error) {
		_ = ctx
		_ = input
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取叙事风格")
		}
		tellers, err := interactive.NewTellerLibrary(novaDir).List()
		if err != nil {
			return "", err
		}
		if len(tellers) == 0 {
			return "暂无叙事风格。", nil
		}
		var sb strings.Builder
		sb.WriteString("# 叙事风格索引\n\n")
		for _, teller := range tellers {
			fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  类型: %s\n  适用: 共享模块（写作模式 / 游戏模式）\n  槽位: %d\n", teller.ID, teller.Name, boolLabel(teller.Custom, "custom", "built-in"), len(teller.Slots))
			if teller.Description != "" {
				fmt.Fprintf(&sb, "  简介: %s\n", teller.Description)
			}
			sb.WriteString("\n")
		}
		return strings.TrimSpace(sb.String()), nil
	})
}

func newReadTellersTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("read_tellers", "按叙事风格 ID 批量读取完整配置。顶层 style_refs 是所有场景默认生效的文风参考；style_rules 使用 scene + style_refs 引用 .denova/styles/*.md 表示分场景文风参考。旧 style_contents 只为兼容已发布配置保留，新配置不要继续内联长文风内容。事件、TRPG 检定、状态系统和图像方案属于故事导演；Actor 词条属于状态系统 trait_pools，并通过模板 trait_rules 绑定。", func(ctx context.Context, input idListInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取叙事风格")
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
	return utils.InferTool("write_tellers", "批量创建、更新或删除叙事风格配置。叙事风格是共享模块，不存在每个风格可配置的模式字段；只维护文风、提示词槽位、文风参考和上下文策略。顶层 style_refs 表示所有场景默认生效的文风参考；style_rules 表示分场景文风参考，必须优先使用 style_refs 引用 .denova/styles/*.md。如需新增文风参考，先用 write_style_references 创建 md，再把 path 写入顶层 style_refs 或对应 style_rules[].style_refs。故事编排请使用 write_story_directors。更新内置 ID 会在用户空间覆盖同一个叙事风格；删除内置 ID 只用于恢复内置默认内容，必须来自用户明确指令。", func(ctx context.Context, input tellerWriteInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法写入叙事风格")
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
				return "", fmt.Errorf("不支持的叙事风格操作: %s", op.Op)
			}
		}
		return formatBatchResult(firstConfigNonEmpty(input.Message, "叙事风格已更新"), result), nil
	})
}

func newListEventPackagesTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("list_event_packages", "列出事件包索引，返回 ID、名称、简介、类型和事件卡数量；事件包是游戏模式独占模块，一个事件包就是一组事件卡。需要完整事件卡内容时再调用 read_event_packages。", func(ctx context.Context, input struct{}) (string, error) {
		_ = ctx
		_ = input
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取事件包")
		}
		items, err := interactive.NewEventPackageLibrary(novaDir).List()
		if err != nil {
			return "", err
		}
		if len(items) == 0 {
			return "暂无事件包。", nil
		}
		var sb strings.Builder
		sb.WriteString("# 事件包索引\n\n")
		for _, item := range items {
			fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  类型: %s\n  事件卡: %d\n", item.ID, item.Name, boolLabel(item.Custom, "custom", "built-in"), len(item.Events))
			if item.Description != "" {
				fmt.Fprintf(&sb, "  简介: %s\n", item.Description)
			}
			sb.WriteString("\n")
		}
		return strings.TrimSpace(sb.String()), nil
	})
}

func newReadEventPackagesTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("read_event_packages", "按事件包 ID 批量读取完整配置。事件包直接包含 events 事件卡列表。", func(ctx context.Context, input idListInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取事件包")
		}
		lib := interactive.NewEventPackageLibrary(novaDir)
		result := []interactive.EventPackageModule{}
		for _, id := range input.IDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			item, err := lib.Get(id)
			if err != nil {
				return "", err
			}
			result = append(result, item)
		}
		return marshalToolJSON(result)
	})
}

func newWriteEventPackagesTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("write_event_packages", "批量创建、更新或删除事件包。事件包是游戏模式独占模块，一个事件包就是一组事件卡；create/update 必须写完整 events。删除内置事件包会恢复内置版本；删除自定义事件包必须来自用户明确指令。", func(ctx context.Context, input eventPackageWriteInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法写入事件包")
		}
		lib := interactive.NewEventPackageLibrary(novaDir)
		result := map[string][]string{"created": []string{}, "updated": []string{}, "deleted": []string{}}
		for _, op := range input.Operations {
			switch strings.TrimSpace(op.Op) {
			case "create":
				item, err := lib.Create(op.Package)
				if err != nil {
					return "", err
				}
				result["created"] = append(result["created"], item.ID)
			case "update":
				id := firstConfigNonEmpty(op.ID, op.Package.ID)
				item, err := lib.Update(id, op.Package, "")
				if err != nil {
					return "", err
				}
				result["updated"] = append(result["updated"], item.ID)
			case "delete":
				id := strings.TrimSpace(op.ID)
				if err := lib.Delete(id); err != nil {
					return "", err
				}
				result["deleted"] = append(result["deleted"], id)
			default:
				return "", fmt.Errorf("不支持的事件包操作: %s", op.Op)
			}
		}
		return formatBatchResult(firstConfigNonEmpty(input.Message, "事件包已更新"), result), nil
	})
}

func newListActorStatesTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("list_actor_states", "列出状态系统索引，返回 ID、名称、简介、类型、模板数量、初始 Actor 数量和词条池数量；状态系统是游戏模式结构化 Actor 状态和词条库的唯一真源。模板可表示主角、重要角色、敌人、怪物、世界、故事倒计时、势力、基地或副本等 Actor。需要完整字段 schema、trait_rules 或词条定义时再调用 read_actor_states。", func(ctx context.Context, input struct{}) (string, error) {
		_ = ctx
		_ = input
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取状态系统")
		}
		items, err := interactive.NewActorStateLibrary(novaDir).List()
		if err != nil {
			return "", err
		}
		if len(items) == 0 {
			return "暂无状态系统。", nil
		}
		var sb strings.Builder
		sb.WriteString("# 状态系统索引\n\n")
		for _, item := range items {
			fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  类型: %s\n  适用: 游戏模式\n  模板: %d\n  初始 Actor: %d\n  词条池: %d\n", item.ID, item.Name, boolLabel(item.Custom, "custom", "built-in"), len(item.ActorState.Templates), len(item.ActorState.InitialActors), len(item.ActorState.TraitPools))
			if item.Description != "" {
				fmt.Fprintf(&sb, "  简介: %s\n", item.Description)
			}
			sb.WriteString("\n")
		}
		return strings.TrimSpace(sb.String()), nil
	})
}

func newReadActorStatesTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("read_actor_states", "按状态系统 ID 批量读取完整配置。字段 schema 支持 name/type/default/min/max/options/visibility/description/update_instruction/order；规范化后的 name 同时是状态 ID，同一模板内不可重名。trait_pools 定义可复用词条，模板 trait_rules 声明创建 Actor 时的自动抽取规则；initial_actors 定义初始 Actor。故事先冻结原始预设，首轮正文落盘后默认由初始化 Director 生成一次故事专属差异并迁移；state_schema_adaptation_mode=off 时始终使用原始预设。后续所有字段引用使用 actor_id + field_id。", func(ctx context.Context, input idListInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取状态系统")
		}
		lib := interactive.NewActorStateLibrary(novaDir)
		result := []interactive.ActorStateModule{}
		for _, id := range input.IDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			item, err := lib.Get(id)
			if err != nil {
				return "", err
			}
			result = append(result, item)
		}
		return marshalToolJSON(result)
	})
}

func newWriteActorStatesTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("write_actor_states", "批量创建、更新或删除状态系统。create/update 必须写完整 actor_state.templates、actor_state.trait_pools 和 initial_actors。模板是 Actor 状态 schema；templates[].trait_rules 只能引用存在的 pool_id，draw_count 必须为正且不超过池内词条数。词条只写 id、name、summary、weight、visibility，禁止写 ops、路径或 StateOp。主角、重要角色、敌人和怪物均在 Actor 创建时由后端应用模板默认值、实例覆盖值并自动抽取词条快照；initial_actors 仅声明开局就存在的 Actor。当前时间、地点、事件、资源、关系数值、持续状态、规则标记，以及会影响后续承接或规则检定的结构化状态都放进状态系统；普通叙事记录和场景流水只保留在 Turn 历史。字段 path 不要带 actors.<actor_id>.state 前缀，只写模板内字段路径，例如 crisis.countdown。删除内置状态系统会恢复内置版本；删除自定义状态系统必须来自用户明确指令。", func(ctx context.Context, input actorStateWriteInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法写入状态系统")
		}
		lib := interactive.NewActorStateLibrary(novaDir)
		result := map[string][]string{"created": []string{}, "updated": []string{}, "deleted": []string{}}
		for _, op := range input.Operations {
			switch strings.TrimSpace(op.Op) {
			case "create":
				item, err := lib.Create(op.ActorState)
				if err != nil {
					return "", err
				}
				result["created"] = append(result["created"], item.ID)
			case "update":
				id := firstConfigNonEmpty(op.ID, op.ActorState.ID)
				item, err := lib.Update(id, op.ActorState, "")
				if err != nil {
					return "", err
				}
				result["updated"] = append(result["updated"], item.ID)
			case "delete":
				id := strings.TrimSpace(op.ID)
				if err := lib.Delete(id); err != nil {
					return "", err
				}
				result["deleted"] = append(result["deleted"], id)
			default:
				return "", fmt.Errorf("不支持的状态系统操作: %s", op.Op)
			}
		}
		return formatBatchResult(firstConfigNonEmpty(input.Message, "状态系统已更新"), result), nil
	})
}

func newListStoryDirectorsTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("list_story_directors", "列出故事导演索引，返回 ID、名称、简介、策略、模块引用开关和系统配置概览；策略会用中文名称展示，完整枚举 ID 见 read/write 工具说明。故事导演是游戏模式独占模块；需要完整配置时再调用 read_story_directors。故事导演可插拔组合叙事风格、多个事件包、TRPG 检定、状态系统和图像方案；Actor 词条库属于状态系统。", func(ctx context.Context, input struct{}) (string, error) {
		_ = ctx
		_ = input
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取故事导演")
		}
		directors, err := interactive.NewStoryDirectorLibrary(novaDir).List()
		if err != nil {
			return "", err
		}
		if len(directors) == 0 {
			return "暂无故事导演。", nil
		}
		var sb strings.Builder
		sb.WriteString("# 故事导演索引\n\n")
		for _, director := range directors {
			eventPackages := len(director.EventPackages)
			eventCards := 0
			for _, pkg := range director.EventPackages {
				eventCards += len(pkg.Events)
			}
			fmt.Fprintf(&sb, "- id: %s\n  名称: %s\n  类型: %s\n  适用: 游戏模式\n  策略: enabled=%t 主线=%s 失败=%s 节奏=%s 扰动=%s\n  模块: narrative=%s events=%s trpg=%s state=%s image=%s\n  事件: %d 包 / %d 卡\n  状态系统: %d 模板 / %d 初始 Actor / %d 词条池\n  TRPG 检定: %d 条\n",
				director.ID,
				director.Name,
				boolLabel(director.Custom, "custom", "built-in"),
				director.Strategy.Enabled,
				storyDirectorStrategyLabel("mainline", director.Strategy.MainlineStrength),
				storyDirectorStrategyLabel("failure", director.Strategy.FailurePolicy),
				storyDirectorStrategyLabel("pacing", director.Strategy.PacingCurve),
				storyDirectorEventFrequencyLabel(director.Strategy.EventFrequency),
				boolLabel(!director.ModuleRefs.NarrativeStyleDisabled, "on:"+director.ModuleRefs.NarrativeStyleID, "off:"+director.ModuleRefs.NarrativeStyleID),
				boolLabel(!director.ModuleRefs.EventPackagesDisabled, "on:"+strings.Join(director.ModuleRefs.EventPackageIDs, ","), "off:"+strings.Join(director.ModuleRefs.EventPackageIDs, ",")),
				boolLabel(!director.ModuleRefs.RuleSystemDisabled, "on:"+director.ModuleRefs.RuleSystemID, "off:"+director.ModuleRefs.RuleSystemID),
				boolLabel(!director.ModuleRefs.ActorStateDisabled, "on:"+director.ModuleRefs.ActorStateID, "off:"+director.ModuleRefs.ActorStateID),
				boolLabel(!director.ModuleRefs.ImagePresetDisabled, "on:"+director.ModuleRefs.ImagePresetID, "off:"+director.ModuleRefs.ImagePresetID),
				eventPackages,
				eventCards,
				len(director.ActorState.Templates),
				len(director.ActorState.InitialActors),
				len(director.ActorState.TraitPools),
				len(director.TRPGSystem.RuleTemplates),
			)
			if director.Description != "" {
				fmt.Fprintf(&sb, "  简介: %s\n", director.Description)
			}
			sb.WriteString("\n")
		}
		return strings.TrimSpace(sb.String()), nil
	})
}

func newReadStoryDirectorsTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("read_story_directors", "按故事导演 ID 批量读取完整配置。故事导演是游戏模式独占模块；module_refs 决定引用哪些模块，event_package_ids 可引用多个事件包，rule_system_id 引用一个 TRPG 检定资源，actor_state_id 引用状态系统，*_disabled=true 表示对应模块关闭且保留原 ID 以便重新启用。状态系统的 trait_pools 是通用词条库，模板 trait_rules 决定各类 Actor 创建时从哪些池抽取多少词条。TRPG 资源只配置一项 rule_templates[0]，检定固定 d20，支持 trigger、must_check_examples、skip_check_examples、difficulty_guidance、state_effect_guidance 和 state_bindings；带 state_bindings 的 TRPG 资源需要 actor_state_id。strategy 使用枚举：mainline_strength=soft_guidance/balanced/strong_arc，failure_policy=reversible/consequence/fail_forward，pacing_curve=progressive/wave/goal-pressure-payoff，event_frequency=off/sparse/balanced/frequent，state_schema_adaptation_mode=after_opening/off，rule_state_consumption_mode=hybrid_auto/director_only，rule_visibility_mode=audit_only/public_roll；branch_planning_turns 控制最近分支规划回合数；planning_templates.plan 是单份 director.md Markdown 模板；strategy.prompt_markdown 是纯 Markdown 高级策略提示，最多 64KB。", func(ctx context.Context, input idListInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法读取故事导演")
		}
		lib := interactive.NewStoryDirectorLibrary(novaDir)
		result := []interactive.StoryDirector{}
		for _, id := range input.IDs {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			director, err := lib.Get(id)
			if err != nil {
				return "", err
			}
			result = append(result, director)
		}
		return marshalToolJSON(result)
	})
}

func newWriteStoryDirectorsTool(novaDir string) (tool.BaseTool, error) {
	return utils.InferTool("write_story_directors", "批量创建、更新或删除故事导演配置。故事导演通过 module_refs 可插拔组合叙事风格、多个事件包、TRPG 检定、状态系统和图像方案；用 narrative_style_disabled、event_packages_disabled、rule_system_disabled、actor_state_disabled、image_preset_disabled 关闭模块，关闭时保留对应 ID。Actor 词条库和模板抽取规则通过 write_actor_states 更新。事件包引用写 event_package_ids；TRPG 检定引用写 rule_system_id；带 state_bindings 的 TRPG 检定资源必须配置 actor_state_id；状态系统引用写 actor_state_id。strategy 使用枚举：mainline_strength=soft_guidance/balanced/strong_arc，failure_policy=reversible/consequence/fail_forward，pacing_curve=progressive/wave/goal-pressure-payoff，event_frequency=off/sparse/balanced/frequent，state_schema_adaptation_mode=after_opening/off 默认 after_opening，rule_state_consumption_mode=hybrid_auto/director_only 默认 hybrid_auto，rule_visibility_mode=audit_only/public_roll 默认 audit_only；branch_planning_turns 默认 5；planning_templates.plan 可写单份 director.md Markdown 模板并必须保留固定标题；strategy.prompt_markdown 可写纯 Markdown 高级策略提示，最多 64KB，不能覆盖结构化策略、工具权限和输出协议。删除内置故事导演会被后端拒绝；删除必须来自用户明确指令。", func(ctx context.Context, input storyDirectorWriteInput) (string, error) {
		_ = ctx
		if novaDir == "" {
			return "", fmt.Errorf("nova_dir 不可用，无法写入故事导演")
		}
		lib := interactive.NewStoryDirectorLibrary(novaDir)
		result := map[string][]string{"created": []string{}, "updated": []string{}, "deleted": []string{}}
		for _, op := range input.Operations {
			switch strings.TrimSpace(op.Op) {
			case "create":
				director, err := lib.Create(op.Director)
				if err != nil {
					return "", err
				}
				result["created"] = append(result["created"], director.ID)
			case "update":
				id := firstConfigNonEmpty(op.ID, op.Director.ID)
				director, err := lib.Update(id, op.Director, "")
				if err != nil {
					return "", err
				}
				result["updated"] = append(result["updated"], director.ID)
			case "delete":
				id := strings.TrimSpace(op.ID)
				if err := lib.Delete(id); err != nil {
					return "", err
				}
				result["deleted"] = append(result["deleted"], id)
			default:
				return "", fmt.Errorf("不支持的故事导演操作: %s", op.Op)
			}
		}
		return formatBatchResult(firstConfigNonEmpty(input.Message, "故事导演已更新"), result), nil
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

func skillDirs(cfg *config.Config) []novaskills.Directory {
	if cfg == nil {
		return nil
	}
	return novaskills.NewDirectories(cfg.SkillsDir, cfg.NovaDir, cfg.Workspace)
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

func storyDirectorStrategyLabel(kind, value string) string {
	switch kind + ":" + strings.TrimSpace(value) {
	case "mainline:soft_guidance":
		return "柔性主线"
	case "mainline:balanced":
		return "平衡牵引"
	case "mainline:strong_arc":
		return "强主线"
	case "failure:reversible":
		return "可逆失败"
	case "failure:consequence":
		return "带后果推进"
	case "failure:fail_forward":
		return "失败前进"
	case "pacing:progressive":
		return "递进节奏"
	case "pacing:wave":
		return "波峰波谷"
	case "pacing:goal-pressure-payoff":
		return "目标-压力-回报"
	default:
		return firstConfigNonEmpty(strings.TrimSpace(value), "默认")
	}
}

func storyDirectorEventFrequencyLabel(frequency string) string {
	switch frequency {
	case interactive.EventFrequencyOff:
		return "关闭扰动"
	case interactive.EventFrequencySparse:
		return "低扰动"
	case interactive.EventFrequencyBalanced:
		return "中等扰动"
	default:
		return "高扰动"
	}
}

func boolLabel(value bool, trueLabel, falseLabel string) string {
	if value {
		return trueLabel
	}
	return falseLabel
}
