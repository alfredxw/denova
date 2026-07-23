---
name: novel-heavy
description: 关键内容、复杂剧情和长篇连续性要求高的写作流程；先规划、综合审稿、再生成状态更新。Heavy writing flow for critical scenes with planning, multi-role review, and state patching.
agent: ide
---

# novel-heavy

关键场景写作流程。先加载 `writing-common` 获取公共规范（写作范围判断、上下文读取、工具使用、错误检查、状态文件边界），再按本 Skill 的重型流程执行。

## 流程

context-planner -> writer -> reviewer -> fixer -> final-gate -> memory-patcher -> final output

所有角色 subagent 都必须通过 `task` 工具委派。每次调用 `task` 时，在 description 中写清角色名、用户目标、必要上下文来源、文件路径、允许/禁止写入、期望输出格式和交付物。

如果这些角色 subagent 可用，按顺序使用：

1. `context-planner`：整理 Context Plan（只返回计划，不改文件）。
2. `writer`：根据计划生成正文（是否写文件由主 Agent 委派说明决定）。
3. `reviewer`：做一次综合审稿（只返回审稿结论，不改文件）。
4. `fixer`：只修真正需要修的问题（是否写文件由主 Agent 委派说明决定）。
5. `final-gate`：检查修订稿是否满足用户要求、计划、canon 和风格约束（只返回检查结论）。
6. `memory-patcher`：生成 progress 和 character-state 等状态更新（只返回 patch）。
7. 主 Agent 输出最终结果，以及必要的用户可见状态更新摘要。

主 Agent 对最终落盘结果负责。`context-planner`、`reviewer`、`final-gate`、`memory-patcher` 默认只返回计划、审稿、检查或 patch，不直接改文件。

## Context Plan

写作前先生成轻量计划：

```md
# Context Plan

## Writing Scope
本次要写什么范围。

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

`memory-patcher` 必须生成这些更新：

- `progress`: 剧情、时间线、地点、风险、未解决线索的变化。
- `character_state`: 当前状态、动机、关系变化、伤病、已知信息、资源、承诺和秘密。
- `world_state`: 只记录本轮即时故事状态中已经变化的事实。
- `foreshadowing`: 新埋、推进、兑现或退场的伏笔。

主 Agent 应在工具权限允许时把 `progress` 和 `character_state` 更新写入工作区对应状态文件；如果当前上下文无法确认文件路径，或用户明确要求只输出正文，则输出可应用的 patch 并说明未写入原因。

长期稳定资料库不同于 progress 和 character-state：

- 不要因为普通进度自动改写长期资料库。
- 只有身份、长期关系、能力体系、世界规则或其他稳定 canon 发生重大变化时，才提出资料库更新建议。
- 如果需要更新长期稳定资料库，先请求用户确认，再执行。