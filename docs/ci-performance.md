# CI 与发布耗时

## 基线与验收口径

使用成功运行比较，不把失败、取消或重试累计时间作为优化前基线。

| 流程 | 成功基线 | 原耗时 | 减半目标 |
| --- | --- | --- | --- |
| CI | [35746679946，v0.5.0 提交](https://github.com/alfredxw/denova/actions/runs/35746679946) | 16 分 47 秒 | ≤ 8 分 23 秒 |
| Release | [34347053449，v0.4.5](https://github.com/alfredxw/denova/actions/runs/34347053449) | 作业执行 26 分 14 秒 | ≤ 13 分 7 秒 |

Release 基线另有 4 分 21 秒排队，总历时 30 分 36 秒。v0.5.0 的 Release 运行被取消，不作为成功基线。比较时分别记录排队与执行时间，不能用增加重试或放宽超时冒充提速。

原 CI 的浏览器分片按文件分配，两个 browser 分片分别执行约 891 秒和 403 秒；Go、单测、构建与生产烟测又在同一个作业串行执行。发布脚本重新跑全部测试，再串行编译五个平台。

## 当前流程与覆盖边界

- CI 并行执行两个 Go module、前端单测和完整发行目录构建。浏览器作业复用该目录，按用例分成六片，每片仍只有一个 worker，避免共享后端状态串扰。
- browser、e2e 与 production 用例在同一生产包上各执行一次；删除各分片中的重复 Go 构建、Vite 开发服务和独立烟测启动。失败仍保留 trace、截图和报告，不再全程录视频。
- 纯逻辑单测使用 Node，DOM 测试保留 jsdom、setup 与文件隔离。CI 固定两个 worker；本地按 CPU 数使用 50%。新增依赖 DOM 的 `.test.ts` 时需加入 `web/vite.config.ts` 的 `domTests`，`.test.tsx` 自动归入 DOM。
- browser 从 105 项收敛到 88 项：模型持久化、压缩与恢复不再重复所有主题/语言/宽度的笛卡尔积；仍覆盖各产品、运行时、两种主题、中英文和宽窄屏。移动导航完整遍历保留在 320/1023px，其他尺寸保留布局检查。
- 移动工作台测试保留草稿、布局和焦点断言；后台执行与并发会话由既有 e2e 覆盖。功能测试默认减少动画，导航反馈测试显式保留正常动画。
- 历史恢复测试将 1,000 次同构执行缩为 100 次、保留十批写入；首尾历史、未完成输入、两个产品 journal、冷启动与索引重建、索引大小比例及原始 journal 不变性断言全部保留。
- 仅说明文档变更跳过昂贵测试，但仍检查 whitespace 并生成 `Test and build` 必需状态；Skills Markdown 仍触发完整 CI。Go 依赖清单改变时检查两个 module 的 `go mod tidy -diff`。

Release 必须等待**标签对应 SHA** 的 push/手动 CI 成功，并下载该次 CI 的发行目录。缺少 CI 或产物过期时，只补跑一次正常 CI。已有 CI 失败时阻止发布。五个平台在独立作业中并行打包，全部成功后才合并校验和并发布。

## 本地验证（2026-09-23，Windows 原生）

工具链：PowerShell 7.6、Go 1.26.6、Node 24，打包脚本使用 Git Bash。GitHub 使用 Ubuntu 与 Node 22；本机耗时不能代替托管 runner 的最终验收。

| 项目 | 结果 |
| --- | --- |
| 前端单测基线 | 134 文件、778 项，原配置固定一个 worker：313.86 秒 |
| 前端单测优化后 | CI 的两个 worker 配置：118.88 秒（减少 62.1%）；134 文件、779 项全部通过。相比基线多一项现有工作新增的测试；默认本机并发约 60 秒 |
| 历史恢复测试 | 33.88 → 2.97 秒；两个产品子用例分别为 1.69 / 1.28 秒 |
| 浏览器与端到端 | 137 项；Windows 下 10 项按既有条件跳过，127 项通过（包含首次失败后的定向复核） |
| Go | agent module 全量 test/vet；根 module 全部受版本管理 package 的 test/vet 通过 |
| 工作流与打包 | actionlint、Bash 语法、五平台归档/覆盖/校验和测试、CI 门禁成功/等待/失败/缺失/过期场景通过；实际 Windows x64 完整打包及原生生产包浏览器验证通过 |

根目录存在已有的 ignored Go 草稿，直接 `go test ./...` 会收集它们；本地根 module 使用 `go test ./cmd/... ./config/... ./internal/... ./scripts/...` 与同范围 vet，覆盖全部受版本管理根 module 包。干净 CI checkout 继续使用 `./...`。

六组浏览器在同一 Windows 主机同时运行时发生资源竞争，首次有六项失败。修正故事加载、菜单关闭、收件箱 mock 与开场按钮统计时序后，定向复核六项在 36.6 秒内全部通过。除上文说明的重复模型执行段外，保留流程的行为断言不变，没有添加自动重试。

按成功 CI 中每项测试的历史时长重新分配，六片分别约 307 / 259 / 246 / 246 / 142 / 243 秒（最后一片另有两个生产包用例）。这是排期估算，尚不是改动后 GitHub 整体耗时；“至少减半”须以推送后的实际成功 CI 和后续 Release 运行完成验收。

## 本地入口

```bash
# 完整打包；不再隐式重复全部测试。
bash scripts/build-github-release.sh vX.Y.Z
# 单个平台；保留其他平台归档，重新生成当前版本校验和。
bash scripts/build-github-release.sh vX.Y.Z windows-x64
# 复用同一提交的已验证前端，跳过前端依赖安装与构建。
DENOVA_RELEASE_FRONTEND_DIR=/absolute/path/to/tested/web bash scripts/build-github-release.sh vX.Y.Z linux-x64
```

浏览器验证生产包使用 `DENOVA_E2E_PACKAGE_DIR`。同机并行验证需同时分配独立的 `DENOVA_E2E_RUNTIME_DIR`、后端端口和模型端口；runtime 目录只能位于 `web/test-results` 内。
