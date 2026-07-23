---
name: novel-lite
description: 快速续写、灵感初稿和低延迟正文生成；由主 Agent 直接输出结果，不启动审稿或修稿子流程。Quick drafting and low-latency prose generation without review subagents.
agent: ide
---

# novel-lite

快速正文生成流程。先加载 `writing-common` 获取公共规范（写作范围判断、上下文读取、工具使用、错误检查、状态文件边界），再按本 Skill 的轻量流程执行。

## 流程

main agent -> final output

## 规则

- 只由主 Agent 直接写出最终结果。
- 不启动 reviewer、fixer、task、General SubAgent 或任何已配置 subagent 流程。
- 可以做轻量内部自检（连续性、用户要求、明显文句问题），但不要输出审稿过程。
- 保留用户的控制感，不要过度规划、过度解释，也不要把用户要的初稿改写成另一个故事。
- 如果用户要求一次写多章，只做轻量内部拆分：先判断整体方向和分章边界，再按用户要求的输出规模创作。
- 如果用户只要求在对话里生成片段、灵感稿或示例，直接输出正文，不要写入工作区文件。
- 只在与本轮写作范围相关时使用工作区上下文、选中文本、资料库引用和风格规则。

## 输出

- 直接输出用户要求的创作结果。
- 只有用户要求说明，或存在无法满足的重要约束时，才补充简短说明。