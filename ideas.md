# Ideas
## WIP
- 基础体验: 支持自定义Skills配置，Skills 放一级菜单，允许用户自己定义skills，通过和agent对话创建，内置 skills-creator
    skill，可以在用户目录也可以在工作区。格式参考通用的 .agent/skills 即可，提示用户在创作agent和其他支持skill的
    agent对话时可使用 /{skills} 命令触发 skill
    复用 .agent/skills，比
- Agent能力：支持更完善的上下文管理，memory compact，dreaming，etc.
- tools: websearch
- 从0，脑暴开始初始化，在agent引导下，生成资料库（角色，世界观）、大纲。
  - 然后开始 由 细纲 到 章节 的创作
- html渲染
- self evolve: bugfix, feature build
- workflow: 
  - 故事设定
  - 卷规划
  - 章节组-章
  - 初稿-定稿
- 版本管理 go-git，agent变更文案优化，页面优化，diff查看
- 自动检查更新 release，用户可选择是否更新
- 架构统一：
  - 所有agent 对话 复用一套实现

## 互动模式
- 资料库应该可以支持自动更新，随着剧情推移会有变化，但不是每一轮都需要更新，需要探讨下更新的时机

AI互动小说通用问题，通过 目标+节奏/压力+结果/代价+状态 来管理互动流程
- 叙事编排：负责管理目标、节奏、压力、结果、代价、状态。

# 规划
- 考虑agent框架自己实现，不依赖eino
- 图片/动图生成
- 视频生成（短剧）
