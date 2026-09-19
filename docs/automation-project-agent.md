# Automation and Project Agent ownership

Automation is a trigger and delivery service. Every accepted trigger runs in the
target Project's workbench conversation through `agentchat.Service.AcceptTurn`
and the same `AcceptedTurn.Start` used for interactive requests.

## Ownership

| Responsibility | Owner |
| --- | --- |
| Trigger definitions, schedules, evidence, inbox confirmation | Automation |
| Frozen delivery input and immutable acceptance receipt | Automation |
| Conversation, execution, tools, pause, continuation, abort, recovery | Project Agent |
| Execution result and output | Canonical Agent Session journal |
| Committed tool mutations and downstream trigger delivery | Common Agent host-effect outbox and trigger coordinator |

Automation has no execution worker, active-run registry, terminal-state writer,
or Agent recovery loop. Closing Automation or deleting an accepted trigger does
not cancel Project Agent work. A pending delivery must settle before its trigger
can be deleted.

## Delivery and recovery

1. Resolve the target by stable ProjectID and persist the trigger's input.
2. Accept it through the common Project Agent service with a deterministic
   command ID. Persist the returned command, operation, and cursor receipt.
3. Start the common worker. A receipt-write failure still transfers the accepted
   worker to AgentChat; reconciliation reads its exact canonical command receipt.
4. Retry pending deliveries with their original input. Replaying an accepted
   trigger returns the original receipt and does not resume or restart its Run.

A busy conversation leaves the trigger pending for a later delivery attempt;
it never holds the common AgentChat admission lock while waiting for a Run.

The Automation API derives execution status and output from the canonical Run.
Exact command lookup includes runs outside the bounded recent-history display.
A suspended Run remains suspended across reloads and resumes through the normal
Project Agent controls. Missing or unreadable canonical state does not prove
completion or authorize a replacement execution.

## Released data and effects

Released legacy records remain readable. Before adopting an old durable Run,
the store writes its original file to a `.v1.bak` sibling. Adoption removes
Automation's execution ownership while retaining pending effect paths and IDs.
Old grouped effects drain under their original idempotency keys. A global effect
already transferred into an old Run outbox is only acknowledged globally.

New tool mutations use the common outbox regardless of trigger origin. They wait
for their exact Agent operation to settle, including operations in delegated
child journals. Unknown, queued, running, and suspended operations retain the
obligation. Trigger evaluation supplies durable deduplication after handoff.

## Regression coverage

- Agent tests cover exact lookup after 40 runs, cold reopen, and repeated pause
  and continuation; execution tests cover delegated mutation settlement.
- Automation tests cover frozen-input retries, accepted worker ownership,
  immutable receipts, stale pending copies, backup, and old effect ownership.
- `web/tests/e2e/automation-agent.spec.ts` exercises Book and General Projects:
  Project routing, pause, reload, duplicate trigger, repeated continuation,
  terminal replay, pending delivery while busy, and deleting a trigger during
  execution.
- `web/tests/e2e/task-pause.spec.ts` covers Writing and Game continuation.

Browser tests use an isolated backend and a deterministic model. They verify
application behavior, not live-provider quality or native Windows behavior.
