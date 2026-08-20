# fix(interactive-turn): cap guard retries and reasoning in completion protocol

## Problem

In interactive-story mode, when the model lost the `submit_interactive_turn` submission path, `newInteractiveCompletionGuard` retried with **generic feedback**. Every retry regenerated a **full reasoning pass** on the heavy-reasoning Minimax-M3 model, so thinking content reached **7.6×–23.9× of narrative prose** on retry turns (sampled from `.denova/projects/Lyozes RPG/interactive/story/story-st_dkn2ti1p61m0d3704b6f.jsonl`).

- Per-turn `llm_call` up to **6×** in a single turn.
- `tool_calls` count strongly correlates with thinking volume (15 tool calls → 47kB thinking).
- Front-end streaming experience was dominated by reasoning dump after the visible narrative.

## Fix — three layers in `internal/agent/interactive_turn_protocol.go`

| Layer | What | Where |
|---|---|---|
| **A — Targeted retry feedback** | Extract the last `submit_interactive_turn` receipt from conversation history and embed field-level diagnostics (Module / Path / Expected / Actual) in the retry feedback so the model self-corrects on the first retry. | new `lastInteractiveSubmitReceipt` + `interactiveRetryFeedbackFromReceipt` |
| **D0 — Guard circuit breaker** | `guardRetries` atomic counter fails fast after **2 guard retries** with a typed error (`ErrInteractiveCompletionRetriesExceeded`), instead of silently burning the shared `MaxRetries` budget. | `interactiveTurnProtocolRunState` + `newInteractiveCompletionGuard` |
| **B — Retry-phase completion budget** | Inject `openai.WithMaxCompletionTokens(8192)` + `openai.WithReasoningEffort(low)` on retry iterations. On Minimax-M3 `reasoning_effort` is ignored, but `max_completion_tokens` is the only effective provider-side lever (thinking + visible output share this limit). | `interactiveNarrativeBudgetOptions` |

The retry feedback remains ephemeral (`PersistModifiedInputMessages=false`) — no historical leakage.

## Provider constraint note

`reasoning_effort` is **silently ignored** on the Minimax-M3 OpenAI-compatible endpoint. The `thinking` parameter only supports `enabled/disabled/adaptive` (no budget sub-field). Only `max_completion_tokens` constrains reasoning on Minimax-M3. See `internal/providercompat/polyfill.go` for the existing model polyfill layer.

## Tests

- **WR-03** — `TestInteractiveRetryFeedbackFromReceiptTargetsExactErrors` now covers real history-pairing path (≥2 submissions, wrong-ID decoy, malformed receipt).
- **WR-04** — `TestInteractiveTurnProtocolRetryCapsOpenAICompletionAndDropsReasoning` verifies `MaxCompletionTokens=8192` and `ReasoningEffort=low` fire **only on retry phase**, not on narrative phase.
- **Existing** — `TestInteractiveTurnProtocolAppliesStoryCompletionBudgetOnlyToNarrativeCandidate` continues to enforce the narrative-phase common-options path.

## Follow-up tooling

- `scripts/turn_metrics.go` — stdlib Go CLI for per-turn story metrics (`narrative_bytes` / `thinking_bytes` / ratio), `--compare` mode for before/after, `--runs` mode aggregates `llm_call` via `run_id → turn_id` attribution (WR-01 fix).
- `scripts/turn_metrics_test.go` — covers parser, ratio edge cases, and oversized-line exit codes.

## Validation

- `go build ./...` — clean
- `go test ./internal/agent -run 'Interactive.*Guard|Interactive.*Feedback|Interactive.*Protocol' -count=1` — all pass
- `go test ./scripts -count=1` — passes (new tests included)
- One real post-fix turn (story-st_dkn2ti1p61m0d3704b6f turn 11, 2026-08-14 01:51):
  - narrative=7,545 B / thinking=8,228 B / ratio=**1.09×** / tool_calls=1
  - vs. pre-fix worst turn 7: ratio **7.64×**, tool_calls 15
  - Clean submission path (`accepted all result modules completion_requested=true`); reasoning budget not exercised on this turn because the model did not hit retry.

## Code review

Standard-depth review produced 0 critical / 5 warning / 0 info. All 5 warnings applied atomically as separate commits (`fix(REVIEW): WR-01..WR-05`). Full report at `.planning/phases/current-task/current-task-REVIEW.md`.

## Risk

- `interactiveRetryCompletionBudget=8192` is sized for a complex opening's full `state_changes` submission. If a future story schema pushes submissions past 8k completion tokens, retry will truncate and the next round will be rejected again — at which point `D0` cleanly caps damage at 2 retries.
- `MaxCompletionTokens` and `ReasoningEffort` are OpenAI-flavored provider-specific options. Other providers in the registry (`go-eino-ext/components/model/openai`) accept the same options via the same wrapper; behavior on non-OpenAI providers is a no-op (harmless).