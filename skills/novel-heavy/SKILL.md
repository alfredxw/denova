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

如果这些角色 subagent 可用，请按顺序使用：

1. `context-planner` 整理 Context Plan。
2. `writer` 根据计划生成正文。
3. `reviewer` 做一次综合审稿。
4. `fixer` 只修真正需要修的问题。
5. `final-gate` 检查修订稿是否满足用户要求、计划、canon 和风格约束。
6. `memory-patcher` 生成 progress 和 character-state 等状态更新。
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

长期稳定资料库不同于 progress 和 character-state：

- 不要因为普通进度自动改写长期资料库。
- 只有身份、长期关系、能力体系、世界规则或其他稳定 canon 发生重大变化时，才提出资料库更新建议。
- 如果需要更新长期稳定资料库，先请求用户确认，再执行。

## 最终输出

- 返回最终正文或用户要求的写作产物。
- 只有任务产生了可持久化进展，或用户要求说明时，才附带简短状态更新摘要。
- 除非用户明确要求检查流程，否则隐藏内部角色对话。
