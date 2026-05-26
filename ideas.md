# Ideas
## WIP
- 基础体验
  - 完善的 Skills 系统
  - /brainstorm /spec /plan /execute /review
  - 章节细纲，从大纲到->章节细纲->章节草稿->章节定稿
- Agent能力
  - 支持完善的上下文管理，memory
  - 考虑：自定义Agent
- ide 日志
- Tantivy / MeiliSearch 全局搜索
- 重构：考虑 Code Mirror6 编辑器，实验效果
- 调试模式，开启后可以看到context组成

## 互动模式
- story teller 可以按自己风格规划暗线剧情和事件，根据用户行为动态调整，保证用户有一个连续的互动体验
- 初始化世界观
- 导入酒馆v2角色卡，优化内容和世界观

## NEED FIX
jsonl 格式不对
- 一行应该只有一个narrative，alt里的和第一层的冲突了，alt只在编辑修改的情况下才有
- 每一行都应该代表一轮对话（user+agent），state delta 的内容是对话内状态的变化，应该和对话合并到一行，而不是起新的一行

我发现输出的 jsonl 里还是没有最新的 state 和 state_delta，原因是agent输出的内容里面没有带上state内容
- 最好在agent架构上优化，先由agent直接生成内容，再由一个异步的agent根据上下文输出state，此时不阻塞用户和主agent的对话，合并叙事和state再落到jsonl

# 规划
- 多语言支持
- 互动创作模式
- 剧情分支系统，允许从特定节点开始，分出不同的剧情线延续，允许对比不同的分支然后选择一个合并
- 版本管理：不用git，自己实现
- 支持导入小说

- prompt 高级自定义
- 支持在diff view中点击accept/reject按钮，确认或拒绝当前diff
