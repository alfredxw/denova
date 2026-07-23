---
name: novel-standard
description: 默认写作流程，由主 Agent 写作和修订，审稿子 Agent 严格审稿，在质量和速度之间取得平衡。Default writing flow with main agent drafting and a reviewer subagent for quality balance.
agent: ide
depends: writing-common
---

# novel-standard

默认写作流程。先加载 `writing-common` 获取公共规范（写作范围判断、上下文读取、工具使用、错误检查、状态文件边界），再按本 Skill 的标准流程执行。

## 流程

主 Agent 写初稿 -> 审稿子 Agent（`reviewer`）审稿 -> 主 Agent 修订和更新状态 -> 最终输出

标准流程只使用两个 Agent：主 Agent 和审稿子 Agent（`reviewer`）。不要启动 `writer`、`fixer` 或其他额外写作子流程。

## 步骤

1. 主 Agent 按用户要求的范围和约束生成初稿，通过 `write_file` 写入 `chapters/` 下符合命名规则的章节文件，暂不更新进度和角色状态。
2. 使用 `task` 工具委派 `reviewer` 审稿。`task` 的 description 里写清用户目标、章节路径、必要上下文来源、审稿重点、输出格式，以及 `reviewer` 只审稿不改文件。
3. `reviewer` 只审稿并返回结构化问题，不直接改正文。重点检查连续性、资料库匹配、节奏、文风、人物动机、剧情逻辑和创作规则遵守情况；不要输出赞扬。
4. 主 Agent 接收审稿结论后直接修订章节，只修真正需要修的问题，保留原故事内容、强段落、有效情节节点、人物声线和连续性。
5. 主 Agent 确认最终修订稿后，按 `writing-common` 的状态文件边界规则更新 `setting/progress.md` 和 `setting/character-states.md`。只有长期稳定设定发生明确变化时，才提出资料库更新建议。

## 审稿要求

`reviewer` 审稿时可读取必要前文、CREATOR.md、大纲、进度、角色状态和资料库作为对照依据。按严重程度输出问题、证据位置、影响和可执行改进建议。如果执行模式不允许写入，只输出审稿结论和修订方案。