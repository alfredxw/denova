---
name: novel-standard
description: 默认写作流程，由主 Agent 写作和修订，审稿子 Agent 严格审稿，在质量和速度之间取得平衡。
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

主 Agent 写初稿 -> 审稿子 Agent 审稿 -> 主 Agent 修订和更新状态 -> 最终输出

标准流程只使用两个 Agent：主 Agent 和审稿子 Agent（`reviewer`）。不要启动 `writer`、`fixer` 或其他额外写作子流程。

## 工具使用要求

- 写作前使用 `read_file` 读取必要上下文：`CREATOR.md`、`setting/outline.md`、`setting/progress.md`、`setting/character-states.md`、相关章节组细纲和最近章节；涉及资料库条目时先用 `list_lore_items` 判断，再用 `read_lore_items` 读取相关完整资料。
- 主 Agent 生成初稿后，使用 `write_file` 写入 `chapters/` 下符合命名规则的章节文件；如果是在已有章节上做局部修订，使用 `edit_file`，并确保 `old_string` 来自最近一次 `read_file` 的实际内容且不包含行号前缀。
- 审稿必须通过 `task` 工具委派给 `reviewer`。`task` 的 description 里要写清用户目标、章节路径、必要上下文来源、审稿重点、输出格式，以及 `reviewer` 只审稿不改文件。
- 修订后如果需要覆盖整章，使用 `write_file`；如果只修少量段落，使用 `edit_file`。更新 `setting/progress.md` 和 `setting/character-states.md` 时同样按“局部修改用 `edit_file`、全量重写用 `write_file`”选择。
- 每次调用 `write_file` 或 `edit_file` 后都要检查工具结果。若结果包含 `[tool error]`、参数 JSON 错误、`string not found`、路径错误或截断提示，不得宣称已完成；应重新读取目标文件、修正参数后重试，或明确告诉用户未写入成功。
- 最终输出前，使用 `read_file` 读回新增或修订后的章节关键片段；如更新了状态文件，也读回对应关键片段，确认内容已经落盘。

1. 主 Agent 按用户要求的范围和约束生成初稿，通过 `write_file` 工具写入 `chapters/` 下的章节文件，暂不更新进度和角色状态相关文件。
2. 主 Agent 使用 `task` 工具启动审稿子 Agent（`reviewer`）审稿，并把新增章节路径、用户要求、必要上下文和需要重点检查的规则交给 `reviewer`。
3. `reviewer` 只审稿并返回结构化问题，不直接改正文；需要严格检查连续性、资料库匹配、节奏、文风、人物动机、剧情逻辑，以及每条创作规则是否遵守；不要输出赞扬。
4. 主 Agent 接收审稿结论后直接修订章节，只修真正需要修的问题，保留原故事内容、强段落、有效情节节点、人物声线和连续性。
5. 主 Agent 确认最终修订稿后，更新 `setting/progress.md` 和 `setting/character-states.md`。只有长期稳定设定发生明确变化时，才提出资料库更新建议或按用户要求更新资料库。

## 审稿要求

`reviewer` 审稿时可读取必要前文、CREATOR.md、大纲、进度、角色状态和资料库作为对照依据。重点检查新增章节是否符合任务要求、用户提示词、CREATOR.md、长期大纲、角色设定与当前状态、世界观和已有连续性；评估剧情推进、人物行为动机、设定一致性、节奏、语言质量和可读性。按严重程度输出问题、证据位置、影响和可执行改进建议；如果执行模式不允许写入，只输出审稿结论和修订方案。

## 最终输出

- 返回最终正文或用户要求的写作产物。
- 除非用户要求，不输出 `reviewer` 报告或内部修订说明。
- 如果关键约束无法满足，先简短说明阻断问题或请求用户确认。
