# Agent Runtime Simplification Audit

> This is an explicit design audit. Competitor/product names in this document are research references only and never enter Denova model-visible prompts, tool descriptions, or runtime feedback.

## Scope

The audit compared Denova's unreleased Agent package with the local `../codex`, `../claude-code`, and `../oh-my-pi` implementations. The useful shared pattern is not identical APIs; it is a narrower recovery boundary:

- the live process owns execution coordination;
- conversation state is the durable asset;
- a process restart ends unfinished execution;
- normal retry starts a new turn;
- tool and product writes rely on their own idempotent boundaries instead of a second generic runtime ledger.

Denova previously combined a Session actor, command envelopes, journal reducer, checkpoints, command index, host-effect outbox, recovery actions, replay projections, retention accounting, and a large failure-window test matrix. That design offered stronger theoretical guarantees than the product's real operating environment required and made every lifecycle change cross many layers.

## Decision

Adopt an in-process `Agent → Session → Run` coordinator and persist only the provider-neutral transcript plus Agent capability snapshots.

| Concern | Previous design | Current design |
|---|---|---|
| Run scheduling | Actor mailbox + reducer | Session mutex + pending Run slice |
| Events | Durable journal authority | Typed process-local projection |
| Restart | Exact replay/recovery actions | Mark unfinished turn interrupted |
| Canonical writes | Intent/receipt/outbox/reconcile | Direct idempotent Adapter call |
| Tool recovery | Persisted open-tool/retry state | Live open-tool snapshot only |
| Interactions | Durable waiter reconstruction | In-process waiter |
| File storage | Journal + checkpoints + indexes | Manifest + checksummed JSONL + lease |
| Limits | Runtime-wide safety budget graph | Existing context/tool-specific limits |

## User-visible comparison

Normal use is unchanged:

- streaming text and thinking;
- tool cards and progress;
- Ask/Permission interactions;
- Stop, Steer, Follow-up, Next turn;
- Writing and Game conversation history;
- Goal, Todo, Cleanup, Compaction and context continuity;
- browser refresh attachment while the backend remains alive.

The observable difference occurs only when the backend process exits during an active Run:

- before: the UI attempted to expose ordered recovery actions and reconstruct accepted work;
- now: the unfinished turn is shown as interrupted, its partial live stream is not authoritative, and the user retries as a new Run.

Completed conversation content remains. Product files, Writing sessions, Game turns, and committed tool effects remain in their own stores. The new behavior may repeat external work if a non-idempotent tool completed immediately before a crash but its final UI event was lost; Denova accepts this lower guarantee because the event is rare and the previous generic reconciliation system carried disproportionate complexity.

## Why the subtraction is reasonable

1. It preserves the high-frequency path and makes its ownership obvious.
2. It removes a second persistence authority competing with product conversations.
3. It dramatically reduces schema evolution and fault-injection surface.
4. It improves cache stability because completed transcript remains the only restored model history.
5. It lets individual high-value tools add idempotency where actually needed, without forcing every tool through a universal recovery protocol.

## Guardrails retained

- exclusive per-Session file lease;
- append revision checks and checksummed transaction lines;
- torn-tail handling and explicit corruption errors;
- canonical commit/effect identities and per-item effect results;
- Session serialization and goroutine panic recovery;
- bounded model context, tool results, snapshots, and UI projections;
- exact same-process Run attachment and controls;
- integration coverage for Writing and Game.

## Rejected alternatives

- Keeping the old runtime behind a feature flag would preserve two architectures and their tests.
- Adding a compatibility reader for unreleased journals would make the obsolete schema permanent.
- Persisting only selected actor events would retain reducer/order coupling without delivering exact recovery.
- Rebuilding tool stacks from conversation text would be ambiguous and unsafe.

No compatibility layer or migration is provided for the unreleased Agent runtime store. The clean boundary is the product's existing canonical conversation data plus new `agent-transcripts` state.
