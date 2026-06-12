# Ideas
## WIP
- Agent能力：支持更完善的上下文管理，memory compact，dreaming，etc.
- 从0，脑暴开始初始化，在agent引导下，生成资料库（角色，世界观）、大纲。
  - 然后开始 由 细纲 到 章节 的创作
- html渲染
- skills:
  - more built-in skills for all different type agents
- self evolve: bugfix, feature build
- writing workflow: 
  - 故事设定
  - 卷规划
  - 章节组-章
  - 初稿-定稿
- 版本管理 go-git，agent变更文案优化，页面优化，diff查看
- 自动检查更新 release，用户可选择是否更新
- log -> trace

## 互动模式
- 资料库应该可以支持自动更新，随着剧情推移会有变化，但不是每一轮都需要更新，需要探讨下更新的时机
- 互动模式需要专门的记忆压缩层，memory compaction：互动故事不适合无限注入历史回合。建议拆成：最近 N 回合原文、较早剧情摘要、稳定世界状态、未解决线索、角色短期状态、分支差异。当前 Snapshot.State 全量 JSON 和所有 turns 一起进上下文，早期好用，长线会不稳。

AI互动小说通用问题，通过 目标+节奏/压力+结果/代价+状态 来管理互动流程
- 叙事编排：负责管理目标、节奏、压力、结果、代价、状态。

# 规划
- TTS
- 图片/动图生成
- 视频生成（短剧）
