---
phase: current-task
reviewed: 2026-08-14T12:57:43Z
depth: standard
files_reviewed: 3
files_reviewed_list:
  - internal/agent/interactive_turn_protocol.go
  - internal/agent/interactive_turn_protocol_test.go
  - scripts/turn_metrics.go
findings:
  critical: 0
  warning: 5
  info: 0
  total: 5
status: issues_found
---

# Phase current-task: Code Review Report

**Reviewed:** 2026-08-14T12:57:43Z  
**Depth:** standard  
**Files Reviewed:** 3  
**Status:** issues_found

## Summary

已完成三份 Go 文件的标准深度审查，覆盖互动回合 A/D0/B 修复、原子状态与消息历史配对、测试覆盖，以及 `turn_metrics.go` 的 JSONL 解析和输出关联逻辑。

验证结果：

- `go test ./internal/agent -run 'TestInteractive(TurnProtocol|CompletionGuard|RetryFeedback)' -count=1 -v`：通过。
- `go test ./scripts`：通过（无测试文件）。
- `go vet ./internal/agent ./scripts`：通过。
- 完整 `go test ./internal/agent ./scripts` 还受到仓库中与本次改动无关的 Windows 路径/权限/反斜杠测试失败影响。
- `gofmt` 检查发现 `scripts/turn_metrics.go` 需要格式化。

A/D0 的核心实现路径看起来一致：原子递增返回新值、重试预算按 guard 调用计数、`narrativeCandidateReady` 在锁内写入并通过原子 Load 读取，临时反馈设置了 `PersistModifiedInputMessages: false`。但下面列出的测试和指标 CLI 问题会让这些修复在回归时缺乏可靠的验证，或者让工具输出错误。

## Critical Issues

未发现经过证实的安全漏洞、数据丢失风险或必须立即修复的生产逻辑错误。

## Warnings

### WR-01: `turn_metrics` 的 runs 关联字段与项目实际 trace 格式不匹配

**File:** `scripts/turn_metrics.go:148-174`  
**Related file:** `internal/agent/run_ledger.go:83-100,287-303`

**Issue:** `parseRuns` 只从每个 `llm_call` 记录的 `data.turn_id` 取回合 ID。但项目自身的 run ledger 只在 `run_created`（以及后续的 `run_context`）记录 `turn_id`；普通 `llm_call` 记录只有 `data.attrs`，没有 `data.turn_id`。因此用项目生成的实际 runs JSONL 执行 `--runs` 时，所有 LLM 调用都会被聚合到 `"other"`，不会匹配 story 行的 `turn_id`，输出的 `llm_calls` 和 `reasoning_tok` 会系统性缺失或串槽。

**Fix:** 让解析器维护当前 run 的 `run_id` 到 `turn_id` 映射：先读取 `run_created`/`run_context`，再按 `llm_call.run_id` 归属到对应 turn；或者在生成 `llm_call` trace span 时把 `turn_id` 明确写入其 `data.attrs`/专用字段。补充一个包含 `run_created`、无 `data.turn_id` 的 `llm_call` 的回归测试。

### WR-02: `parseRuns` 静默吞掉读取错误

**File:** `scripts/turn_metrics.go:141-175`

**Issue:** `parseRuns` 检查了 `scanner.Scan()` 的返回值，却完全没有返回 `scanner.Err()`。磁盘读取错误或超过 Scanner 16 MiB 上限的行会以成功返回的部分聚合结果继续打印，用户可能把不完整统计误认为有效结果。文件关闭错误也未传播。

**Fix:** 将 `scanner.Err()` 纳入 `parseRuns` 错误返回，并在 `main` 中以非零状态退出；使用 `defer f.Close()`，如需要则传播关闭错误。增加一个超过 16 MiB 的输入测试，确保退出码为 1。

### WR-03: 新增的定向回执测试没有覆盖实际历史配对路径

**File:** `internal/agent/interactive_turn_protocol_test.go:166-193`

**Issue:** `TestInteractiveRetryFeedbackFromReceiptTargetsExactErrors` 直接把构造好的 receipt 传给格式化函数，没有调用 `lastInteractiveSubmitReceipt`。因此即使 `submit_interactive_turn` 配错 `ToolCallID`、错误地忽略 `schema.Tool` 角色、在多个历史调用中选错 receipt，或根本没有扫描消息历史，这个测试仍然会通过；当前 A 修复的核心行为没有被测试锁定。

**Fix:** 增加一个消息历史测试，构造至少两次提交：第一次成功、第二次带不同 ID 的拒绝回执，并加入一个错误 ID 的干扰回执；断言函数只返回最后一次正确配对的 receipt。再分别覆盖没有回执和回执 JSON 无效的情况。

### WR-04: completion budget 测试只检查通用 `max_tokens`，无法验证 B 的实际 OpenAI 选项

**File:** `internal/agent/interactive_turn_protocol_test.go:111-133`

**Issue:** `interactiveProtocolOptionModel` 只读取 `model.GetCommonOptions(...).MaxTokens`，而本次 B 修复新增的是 `openai.WithMaxCompletionTokens(8192)` 和 `openai.WithReasoningEffort(low)` 这类实现特定选项。现有测试在候选出现后检查 `base.maxTokens == 9999`，即使 retry 请求完全没有发送 `max_completion_tokens` 或仍保留高 reasoning effort，测试也会通过。上下文中的 Minimax-M3 provider quirk 因此没有在测试中得到验证。

**Fix:** 使用 mock HTTP server 检查最终序列化请求体中的 `max_completion_tokens == 8192` 和 `reasoning_effort == "low"`，或在可访问实现特定选项的测试位置断言两个值；分别覆盖 candidate 尚未 ready 和已经 ready 两条路径。

### WR-05: `scripts/turn_metrics.go` 未通过 gofmt 格式检查

**File:** `scripts/turn_metrics.go:2-6,357`

**Issue:** 当前 gofmt 仍会重排文件顶部的 doc comment 示例，并为文件末尾补换行。该脚本是标准 Go 工具入口，格式漂移会让项目统一的 gofmt 校验持续失败。

**Fix:** 对该文件执行 `gofmt -w scripts/turn_metrics.go`，并把 gofmt 校验加入脚本/仓库的 Go hygiene 检查。

---

_Reviewed: 2026-08-14T12:57:43Z_  
_Reviewer: Claude (gsd-code-reviewer)_  
_Depth: standard_
