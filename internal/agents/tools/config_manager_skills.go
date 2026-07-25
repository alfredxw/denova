package tools

import (
	"context"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	novaskills "denova/internal/agents/skills"
)

type skillRef struct {
	Scope string `json:"scope" jsonschema:"description=Skill 作用域：user 或 workspace"`
	Name  string `json:"name" jsonschema:"description=Skill 名称"`
}

type readSkillsInput struct {
	Items []skillRef `json:"items" jsonschema:"description=要读取的 Skill 列表，每项包含 scope 和 name"`
}

type skillsWriteInput struct {
	Message    string                `json:"message,omitempty" jsonschema:"description=本次 Skills 变更说明；省略时使用默认说明"`
	Operations []skillWriteOperation `json:"operations" jsonschema:"minItems=1,description=批量 Skill 操作"`
}

type skillWriteOperation struct {
	Op          string   `json:"op" jsonschema:"enum=create,enum=update,enum=delete,description=操作类型：create/update/delete"`
	Scope       string   `json:"scope" jsonschema:"enum=user,enum=workspace,description=Skill 作用域：user 或 workspace"`
	Name        string   `json:"name" jsonschema:"description=Skill 名称"`
	Description string   `json:"description,omitempty" jsonschema:"description=create 且 content 为空时使用的描述"`
	Agents      []string `json:"agents,omitempty" jsonschema:"description=create 且 content 为空时写入 front matter 的 Agent 列表"`
	Content     string   `json:"content,omitempty" jsonschema:"description=create/update 使用的完整 SKILL.md 内容"`
}

func newListSkillsTool(cfg *config.Config) (agent.Tool, error) {
	return agent.InferTool("list_skills", "列出 Skills 索引，返回名称、scope、agent、描述、是否可编辑和是否生效；需要完整 SKILL.md 时再调用 read_skills。", func(ctx context.Context, input struct{}) (string, error) {
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

func newReadSkillsTool(cfg *config.Config) (agent.Tool, error) {
	return agent.InferTool("read_skills", "按 scope/name 批量读取完整 SKILL.md。", func(ctx context.Context, input readSkillsInput) (string, error) {
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

func newWriteSkillsTool(cfg *config.Config) (agent.Tool, error) {
	return agent.InferTool("write_skills", "批量创建、更新或删除 Skills。scope 必须是 user 或 workspace；修改内置/预制 Skill 时使用 workspace 同名覆盖，禁止写 builtin；删除必须来自用户明确指令。", func(ctx context.Context, input skillsWriteInput) (string, error) {
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
	return novaskills.NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace)
}
