---
name: novel-standard
description: 默认写作流程，使用 writer、reviewer、fixer 在质量和速度之间取得平衡。
agent: ide
---

# novel-standard

这是 IDE 创作 Agent 的默认写作流程，在质量和速度之间取得平衡。

## 写作范围判断

- 从用户的实际指令判断写作范围，例如“续写一段”“写一个场景”“写一章”“写三章”“写一个小剧情段落 / arc”或用户自定义目标。
- 除非用户明确说“写下一章”，否则不要假设任务一定是下一章。
- 没有 `writing_scope` 字段。用户消息是判断范围、目标、约束和输出形态的唯一来源。
- 当用户要求一次写 N 章或其他多段写作时，先制定整体计划和分章计划。计划要简洁，并用于指导初稿。

## 流程

writer -> reviewer -> fixer -> final output

如果这些角色 subagent 可用，请按顺序使用：

1. `writer` 按用户要求的范围和约束生成初稿，通过 write_file 工具写入文件，返回给主agent文件名和决策信息，不更新进度和角色状态相关文件
2. `reviewer` 只审稿并返回结构化问题，不直接改正文，多个维度去check，确认文章连续性，资料库匹配，节奏，文风，以及是否每条创作规则都有遵守
3. `fixer` 根据 reviewer 意见修稿，同时保留原故事。
4. 主 Agent 确认最终修订稿，如果有问题再自己进行一些修复，然后主agent自己最终更新 progress.md 和 character-state.md 等状态文件。

## Reviewer 输出协议

reviewer 输出必须结构化

- `severity`: `blocker` / `major` / `minor`
- `dimension`: `continuity` / `character_voice` / `pacing` / `prose` / `dialogue` / `plot_logic` / `style` / `user_requirement`
- `problem`: 具体问题
- `fix_instruction`: 给 fixer 的明确修复指令

reviewer 只提出可执行问题，不制造无意义工作。

## Fixer 规则

- 只修真正需要修的问题。
- 不要把初稿重写成另一个故事。
- 保留用户要求的内容、强段落、有效情节节点、人物声线和连续性。
- 如果 review 没有 blocker 或 major 问题，只做轻度修订。

## 最终输出

- 返回最终正文或用户要求的写作产物。
- 除非用户要求，不输出 reviewer 报告或 fixer 说明。
- 如果关键约束无法满足，先简短说明 blocker 或请求用户确认。
