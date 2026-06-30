---
name: novel-heavy
description: 关键内容、复杂剧情和长篇连续性要求高的写作流程；先规划、综合审稿、再生成状态更新。
agent: ide
---

# novel-heavy

这个 Skill 用于关键场景、复杂剧情、长链路连续性和需要同步更新作品状态的写作任务。

## 写作范围判断

- 从用户的实际指令判断写作范围，例如“续写一段”“写一个场景”“写一章”“写三章”“写一个剧情 arc”或用户自定义目标。
- 除非用户明确说“写下一章”，否则不要假设任务一定是下一章。
- 用户消息是判断范围、目标、约束和输出形态的唯一来源。
- 没有 `writing_scope` 字段；不要等待或编造额外字段。
- 当用户要求一次写 N 章或多段 arc 时，Context Plan 必须包含整体计划和分章计划。

## 流程

context-planner -> writer -> reviewer -> fixer -> final-gate -> memory-patcher -> final output

## 工具使用要求

- 写作前使用 `read_file` 读取必要上下文：`CREATOR.md`、`setting/outline.md`、`setting/progress.md`、`setting/character-states.md`、相关章节组细纲和最近章节；涉及资料库条目时先用 `list_lore_items` 判断，再用 `read_lore_items` 读取相关完整资料。
- 所有角色 subagent 都必须通过 `task` 工具委派。每次调用 `task` 时，在 description 中写清角色名、用户目标、必要上下文来源、文件路径、允许/禁止写入、期望输出格式和交付物。
- `context-planner`、`reviewer`、`final-gate`、`memory-patcher` 默认只返回计划、审稿、检查或 patch，不直接改文件；`writer` 和 `fixer` 是否写文件由主 Agent 的委派说明决定。主 Agent 对最终落盘结果负责。
- 创建新章节、写入整章初稿、覆盖整章修订稿时使用 `write_file`；局部修改已有章节或状态文件时使用 `edit_file`，并确保 `old_string` 来自最近一次 `read_file` 的实际内容且不包含行号前缀。
- 写入 `setting/progress.md` 和 `setting/character-states.md` 时，优先用 `edit_file` 更新对应条目；只有文件不存在、结构严重不匹配或需要全量重排时才用 `write_file`。
- 每次调用 `write_file` 或 `edit_file` 后都要检查工具结果。若结果包含 `[tool error]`、参数 JSON 错误、`string not found`、路径错误或截断提示，不得宣称已完成；应重新读取目标文件、修正参数后重试，或明确告诉用户未写入成功。
- Final Gate 通过后，使用 `read_file` 读回最终章节关键片段；如果写入了状态文件，也读回对应关键片段，确认内容已经落盘。

如果这些角色 subagent 可用，请按顺序使用：

1. 使用 `task` 工具委派 `context-planner` 整理 Context Plan。
2. 使用 `task` 工具委派 `writer` 根据计划生成正文。
3. 使用 `task` 工具委派 `reviewer` 做一次综合审稿。
4. 使用 `task` 工具委派 `fixer` 只修真正需要修的问题。
5. 使用 `task` 工具委派 `final-gate` 检查修订稿是否满足用户要求、计划、canon 和风格约束。
6. 使用 `task` 工具委派 `memory-patcher` 生成 progress 和 character-state 等状态更新。
7. 主 Agent 输出最终结果，以及必要的用户可见状态更新摘要。

## Context Plan

写作前先生成轻量计划，格式如下：

```md
# Context Plan

## Writing Scope
本次要写什么范围，例如一段、一个场景、一章、N 章、一个剧情 arc。

## Goal
本次写作要完成的剧情目标。

## Required Beats
必须发生的关键事件。

## Character State
主要角色当前状态、动机、关系、已知信息。

## Canon Constraints
世界观、时间线、地点、道具、能力、伏笔等不能违背的约束。

## Style Constraints
叙事人称、文风、节奏、禁用表达。

## Risks
本次最容易写崩的地方。
```

如果用户要求一次写 N 章，补充：

- `整体计划`: 共享剧情弧线、升级节奏、转折点和结束状态。
- `分章计划`: 每章一段简洁计划，包含章节目标、关键事件、POV 或焦点、结尾钩子或状态。

## 审稿协议

reviewer 必须返回结构化问题，每项包含：

- `severity`: `blocker` / `major` / `minor`
- `dimension`: `continuity` / `character_voice` / `pacing` / `prose` / `dialogue` / `plot_logic` / `style` / `user_requirement`
- `problem`
- `fix_instruction`
- `keep`

## Final Gate

- 只有修订稿满足用户要求、Context Plan、canon 约束、风格约束和明显连续性检查时才通过。
- 如果存在 blocker，把稿件带着明确指令交回 fixer 一次。
- 不要增加额外 reviewer agent。

## Memory Patch

最终稿完成后，`memory-patcher` 必须生成这些更新：

- `progress`: 剧情、时间线、地点、风险、未解决线索的变化。
- `character_state`: 当前状态、动机、关系变化、伤病、已知信息、资源、承诺和秘密。
- `world_state`: 只记录本轮即时故事状态中已经变化的事实。
- `foreshadowing`: 新埋、推进、兑现或退场的伏笔。

主 Agent 应在工具权限允许时把 `progress` 和 `character_state` 更新写入工作区对应状态文件；如果当前上下文无法确认文件路径，或用户明确要求只输出正文，则输出可应用的 patch 并说明未写入原因。

写入状态更新时必须使用文件工具：局部更新用 `edit_file`，全量重写用 `write_file`；写入后用 `read_file` 验证关键条目已经存在。

长期稳定资料库不同于 progress 和 character-state：

- 不要因为普通进度自动改写长期资料库。
- 只有身份、长期关系、能力体系、世界规则或其他稳定 canon 发生重大变化时，才提出资料库更新建议。
- 如果需要更新长期稳定资料库，先请求用户确认，再执行。

## 最终输出

- 返回最终正文或用户要求的写作产物。
- 只有任务产生了可持久化进展，或用户要求说明时，才附带简短状态更新摘要。
- 除非用户明确要求检查流程，否则隐藏内部角色对话。
