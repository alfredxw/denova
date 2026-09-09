# CI 失败归因与稳定性验证

调查日期：2026-09-09。基线：`b959abef`（v0.4.5）。

## 结论

GitHub 失败记录包含测试时序、测试数据、真实产品回归和依赖配置问题，不能全部算作 flaky test。
9 月以来的 20 次 CI workflow 运行中，10 次成功、8 次失败、2 次取消；排除取消后的失败比例为 8/18。
这是不同提交的运行结果，不是同一提交重复执行的随机失败率。最近四次未取消的 CI 已连续通过。
最新成功运行包含 658 个前端单元测试、51 个浏览器/E2E 测试和 6 个生产前端冒烟测试。

## 已发生的失败

| 日期 / 运行 | 失败测试或阶段 | 证据与归因 | 基线已有处理 |
| --- | --- | --- | --- |
| [09-07 / 34126711531](https://github.com/alfredxw/denova/actions/runs/34126711531) | `agent-chat.spec.ts` 并发会话；`permissions.spec.ts` | 多次切换、reload 和三个权限流程共用 30 秒总预算，测试总超时 | `93ecf372` 对长场景使用 `test.slow()` |
| [09-05 / 33952550694](https://github.com/alfredxw/denova/actions/runs/33952550694)、[33951319639](https://github.com/alfredxw/denova/actions/runs/33951319639) | `keeps concurrent sessions independent and delivers Follow Up to its exact session` | Session A 初始回复缺失；预期一条、实际零条，不应以重试或删除断言处理 | `103fc2d1` 修复跨 display cycle 的回复显示；保留回归断言 |
| [09-05 / 33950644654](https://github.com/alfredxw/denova/actions/runs/33950644654) | `keeps three interleaved SubAgent streams responsive, isolated, and restorable` | 三轮子会话检查和两次 reload 超过 30 秒 | `e2a13886` 增加该长场景总预算 |
| [09-04 / 33830026848](https://github.com/alfredxw/denova/actions/runs/33830026848) | `TestManagerCreateRunsCommandAndStreamsOutput` | 测试得到空输出；Attach 前输出已进入 scrollback，测试只读 live channel | `96080bc7` 同时读取 retained history 和 live output |
| [09-03 / 33763399349](https://github.com/alfredxw/denova/actions/runs/33763399349) | SubAgent E2E | 预期三个子 Agent 卡片，实际两个；异步创建与流式展示缺少明确同步点 | `e7eccddc` 为模型流增加显式 gate |
| [09-03 / 33758658553](https://github.com/alfredxw/denova/actions/runs/33758658553)、[33707201612](https://github.com/alfredxw/denova/actions/runs/33707201612) | `agent-prompt-context.spec.ts`、`attachments.spec.ts`、`game.spec.ts` 的提交/分支/计划/重生成/Follow Up；后一运行另有 SubAgent 失败 | 多处等待 composer、上传或响应超时；游戏初始化与测试模型 fixture/操作前提不一致，不能单凭 Timeout 判为机器慢 | `ac79bde5` 修正游戏开场模型响应、创建与等待方式；SubAgent gate 随后修复 |
| [09-03 / 33707201613](https://github.com/alfredxw/denova/actions/runs/33707201613) | Windows `TestReadSupportsExternalPathsAndRejectsBinaryContent` | 路径断言跨平台不一致 | `ac79bde5` 修正测试的路径处理 |
| [08-23 / 32652607737](https://github.com/alfredxw/denova/actions/runs/32652607737) | 安装前端依赖 | `ERR_PNPM_LOCKFILE_CONFIG_MISMATCH`，测试尚未开始 | 已统一 pnpm 版本；保留 frozen lockfile 校验 |
| [08-22 / 32554195643](https://github.com/alfredxw/denova/actions/runs/32554195643) | `useWorkspace.test.tsx` 文件操作 | Project identity 尚未就绪即触发操作 | `e7eccddc` 等待明确就绪状态 |
| [08-21 / 32501211056](https://github.com/alfredxw/denova/actions/runs/32501211056) | `BranchTimeline.test.tsx` 运行期间的未捕获异常 | Radix 延迟 focus cleanup 的 Event 类型不匹配，疑似 jsdom realm 生命周期问题；断言通过也会使 Vitest 失败 | 原文件已在后续重构删除；本次全量现有单测复核，不全局屏蔽未捕获异常 |

## 本次改进

基线在 Windows 原生用 `--repeat-each=2` 运行 `agent-chat.spec.ts` 和 `permissions.spec.ts`：
第一轮 5/5 通过，第二轮 5/5 失败。失败直接落在同名会话的 strict locator 冲突，证明这些测试不具备重复运行的数据隔离。

- General Agent 与权限用例每次创建独立目录，避免继承前一轮会话、文件与权限状态。
- 公共会话 helper 按 Project 容器查找会话，允许不同 Project 中存在同名会话。
- queued Follow Up 的模型计数比较本次增量，保持 exactly-once 断言，同时允许同一测试服务中重复运行。
- SubAgent 详情打开后等待实际子流内容加载，避免把可见空壳当作已完成 journal 恢复。
- Ask、Write、Full access 分为三个独立测试，各自正常预算；保留 Ask 连续三次独立审批，以及拒绝/免审批行为验证。
- CI 单测和 Playwright 分步显示，单测失败仍执行浏览器覆盖。失败仍使整个 CI 失败，重试保持零次。
- CI 保存 JUnit、Playwright JSON、HTML 与失败 trace；成功运行也上传报告，便于比较耗时和定位慢测试。保留 7 天，避免积累大量视频。

仍使用一个 Playwright worker：测试后端和模型控制标记是套件共享状态。
本次只保证上述高风险场景可以在同一服务中顺序重复执行，没有声称整套测试已支持任意并发或任意顺序。

## 验证方法与边界

在空闲测试端口执行，测试入口管理自己的模型、后端、前端及隔离数据，不连接真实模型或开发后端。

```powershell
pnpm --dir web exec playwright test --project=e2e agent-chat.spec.ts permissions.spec.ts --repeat-each=3
pnpm --dir web test:unit --reporter=default --reporter=junit --outputFile=test-results/unit.xml
pnpm --dir web test:playwright
pnpm --dir web exec tsc --noEmit
```

PTY 在 WSL Ubuntu/Linux Go 工具链执行目标测试 30 次及 `go vet ./internal/terminal`。
Windows 路径测试在原生 Windows 执行 20 次及 `go vet ./tools`。
重复通过只证明本轮覆盖未复现问题，不保证永久没有竞态。GitHub Linux runner 上的新 workflow 仍需提交后验证；本次不自动提交、推送或创建 PR。

本次已完成验证：

- Windows 原生：高风险 7 个 E2E 连续三轮，21/21 通过，零重试（5 分钟）。
- Windows 原生：118 个单测文件、658 个测试全部通过（89.84 秒），JUnit 文件成功生成。
- Windows 原生：全量 Chromium browser/E2E 53/53 通过，零重试（8.6 分钟），覆盖写作、游戏、移动端、权限、版本恢复；Playwright JSON 和 HTML 报告成功生成。
- 应用 TypeScript 检查，以及修改的 E2E/support/config 的独立 TypeScript 检查通过。
- `actionlint v1.7.7` 校验修改的 CI workflow 通过。
- WSL PTY 目标测试 30 轮、Windows 外部路径读取目标测试 20 轮及对应 vet 均通过。
