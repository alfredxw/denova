# 项目目标
一个面向长篇小说创作的 AI 写作工作台，支持从灵感、设定、大纲、分卷、分章、细纲、正文到一致性检查的完整创作流程。

小说 IDE

# 项目约定
1. 每次commit变更前都将变更的具体内容写在 CHANGELOG.md 中，遵循通用的 CHANGELOG 规范
2. 模块依赖需要划分清晰，避免循环依赖
3. 能用依赖解决方案就不要自己实现，避免重复造轮子，譬如 TipTap 编辑器，目录树，对话区域，各类常见组件等功能，多用组件
4. 前后端分离架构
   1. 前端负责UI交互
   2. 后端负责 AI Agent 运行 + 文件管理，Agent 类似 claude code 这种后端
5. 发布版本时需要同步更新前端版本号、CHANGELOG.md 和 README.md，并创建对应 Git tag
6. 当前beta版本不需要考虑兼容性问题，以优化功能为主要目标
7. 每次支持新的功能/功能较大变更时，考虑是否需要增加配置项供用户配置
8. 所有面向用户的交互，都要尽量用中文展示
9.  不需要kill用户的前端进程，也不需要自己启动前端，因为前端是热加载的，只需要打开浏览器访问对应页面就能看到效果变化

# 核心概念
- 编辑区：用户在 WebUI 中编辑小说的区域，用户可以在其中输入、查看、修改小说内容。
- AI 对话区：用户在 WebUI 中与 AI 进行对话的区域，用户可以在其中输入指令、查看 AI 回复、与 AI 进行工具调用。
- 左侧目录树：用户在 WebUI 中查看当前工作区的文件树，用户可以在其中导航到不同的文件。
- 顶部任务面板：用户在 WebUI 中查看当前任务、切换任务、查看任务状态等。
- 风格参考：用户在 `<nova_dir>/styles/` 中维护的 Markdown 或 TXT 文风样本，仅在 AI 对话区本轮通过 `#` 指定或命中当前讲述者场景化规则时注入 Agent。
- 书籍：一个 workspace 代表一本书，后端需要记录最近打开的书籍目录并在重启后恢复上次书籍。
- 章节文件：正文放在 `chapters/` 下，命名遵循配置的章节文件名模板；默认 `chNNNN-章节名.md`，例如 `ch0001-废材开局.md`，便于目录整体浏览并支持千章排序。
- 版本管理：每个书籍 workspace 使用 Nova 原生快照系统，通过右侧版本管理面板执行手动保存版本、查看历史、查看差异和恢复；系统支持定时自动保存与 Agent 大量输出自动保存，不依赖 Git。
- 会话管理：每个书籍 workspace 支持多个独立会话；`/clear` 只追加上下文清理标记，不物理删除历史，Agent 只读取当前会话有效上下文。

# 后端模块边界
1. `internal/agent`：只负责 AI Agent 构建、Prompt 注入、Runner 和流式事件编排
2. `internal/book`：只负责作品工作区、书籍状态、文件树、文件读写和 CREATOR.md 初始化
3. `internal/api`：只负责 Hertz 路由、请求响应 DTO、SSE 输出等 HTTP 适配
4. `internal/app`：负责组装 workspace、session、book service、agent runner，并处理 workspace 热切换
5. `internal/session`：独立管理会话持久化，不归入 agent 或 book

# 代码注意事项
- goroutine 都需要 recover，避免 panic 导致整个服务崩溃
- 写代码时注意前后端都打印必要日志，帮助调试问题，避免隐藏错误，日志信息要充分说明具体在干什么，具体文件位置和行号，方便定位问题
- go package 解耦一点，遵循高内聚低耦合原则，不要一大堆不同功能的文件塞一起
- Split files and packages/directories by responsibility and reason-to-change.
- Do not optimize for fewer lines. Optimize for fewer concepts per file.

# 构建逻辑
1. 使用 go mod tidy 确保依赖拉下来了
2. 使用 ./build.sh 构建项目
3. 使用 ./bootstrap.sh fe/be 启动项目
