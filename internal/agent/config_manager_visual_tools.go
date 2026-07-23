package agent

import (
	"context"
	"fmt"
	"path"
	"strings"

	adk "github.com/alfredxw/denova/adk"

	"denova/internal/imagepreset"
	"denova/internal/styleref"
)

type styleReferenceWriteInput struct {
	Message    string                         `json:"message,omitempty" jsonschema:"description=本次文风参考变更说明；省略时使用默认说明"`
	Operations []styleReferenceWriteOperation `json:"operations" jsonschema:"minItems=1,description=批量文风参考操作"`
}

type styleReferenceWriteOperation struct {
	Op        string                `json:"op" jsonschema:"enum=create,enum=update,enum=delete,description=操作类型：create/update/delete"`
	Path      string                `json:"path,omitempty" jsonschema:"description=delete 使用的文风参考路径，例如 .denova/styles/name.md"`
	Reference styleref.WriteRequest `json:"reference,omitempty" jsonschema:"description=create/update 使用的 Markdown 文风参考；content 必须是最终提炼后的 md，不要写原始长文"`
}

type imagePresetWriteInput struct {
	Message    string                      `json:"message,omitempty" jsonschema:"description=本次图像方案变更说明；省略时使用默认说明"`
	Operations []imagePresetWriteOperation `json:"operations" jsonschema:"minItems=1,description=批量图像方案操作"`
}

type imagePresetWriteOperation struct {
	Op     string             `json:"op" jsonschema:"enum=create,enum=update,enum=delete,description=操作类型：create/update/delete"`
	ID     string             `json:"id,omitempty" jsonschema:"description=目标图像方案 ID；update/delete 必填"`
	Preset imagepreset.Preset `json:"preset,omitempty" jsonschema:"description=create/update 使用的完整图像方案配置；slots 只支持 target=agent_system 或 tool_request"`
}

func newListImagePresetsTool(novaDir string) (adk.BaseTool, error) {
	return adk.InferTool("list_image_presets", "列出图像方案索引，返回 ID、名称、简介、类型和注入规则概览；图像方案是共享模块，可用于写作模式和游戏模式；需要完整 slots 内容时再调用 read_image_presets。", func(ctx context.Context, input struct{}) (string, error) {
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

func newReadImagePresetsTool(novaDir string) (adk.BaseTool, error) {
	return adk.InferTool("read_image_presets", "按图像方案 ID 批量读取完整图像方案配置。图像方案是共享模块，使用 slots：agent_system 注入图像提示构造 Agent 的 system prompt，tool_request 原样前置注入最终图像请求 prompt。", func(ctx context.Context, input idListInput) (string, error) {
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

func newWriteImagePresetsTool(novaDir string) (adk.BaseTool, error) {
	return adk.InferTool("write_image_presets", "批量创建、更新或删除图像方案配置。图像方案是共享模块，不存在每个方案可配置的模式字段。create/update 必须写完整 slots；target 仅支持 agent_system 和 tool_request。旧 prompt 字段只作为兼容输入，会被后端转换为 tool_request slot。删除内置图像方案会被后端拒绝；删除必须来自用户明确指令。", func(ctx context.Context, input imagePresetWriteInput) (string, error) {
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

func newListStyleReferencesTool(novaDir string) (adk.BaseTool, error) {
	return adk.InferTool("list_style_references", "列出共享文风参考索引。文风参考统一位于 .denova/styles/，返回 name、description、path；叙事风格的 style_rules 只能引用这些 path，不应内联长文风内容。", func(ctx context.Context, input struct{}) (string, error) {
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

func newWriteStyleReferencesTool(novaDir string) (adk.BaseTool, error) {
	return adk.InferTool("write_style_references", "批量创建、更新或删除共享文风参考 Markdown。用于把用户源文件提炼为 .denova/styles/*.md；content 必须是最终可复用的 md 文风参考，以提炼出的典型参考段落为主，辅以风格总结，不要写现实作者名、作品名、来源说明或大段原文。", func(ctx context.Context, input styleReferenceWriteInput) (string, error) {
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
