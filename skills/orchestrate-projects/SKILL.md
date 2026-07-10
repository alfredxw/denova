---
name: orchestrate-projects
description: Use when the user wants Codex to coordinate a long-running or multi-milestone project, especially when work may span threads, subagents, reviews, local browser/desktop checks, or persistent Goal-mode objectives.
agent: ide
---

# Orchestrate Projects

Coordinate long-running work around current evidence, not the first plan.

## Workflow

1. Start conversationally. Interview the user for goal, constraints, risks, success evidence, and any local-only requirements before turning rough ideas into a plan.
2. For projects with multiple milestones, create or update `GOALS.md` as the shared roadmap. Organize each milestone by outcome, scope, decisions, blockers, and evidence required.
3. Keep one active objective at a time. If Goal mode is available, set the current objective from the active milestone and define what evidence is required before marking it complete.
4. Keep the main thread focused on coordination: objective, constraints, decisions, current state, delegation, and result evaluation.
5. Delegate bounded implementation, research, testing, or review to subagents or separate threads. Use separate threads when the user may want visible history or later continuation; use subagents for disposable bounded work.
6. Require workers to return conclusions, changed files, evidence, blockers, and recommended next action. Do not pull full transcripts, large logs, or unrelated details back into the coordinator context.
7. After each milestone, audit `GOALS.md` against the actual repository state and run a code review when implementation changed. Update the roadmap before activating the next objective.
8. Use local threads for checks that need the user's machine: signed-in browser state, credentials, desktop apps, permissions, simulators, or device-specific behavior. Feed screenshots, logs, and findings back into the main thread.
9. When project state changes, report only:
   - What's done
   - What's next
   - Any blockers
10. For projects with several milestones or parallel workstreams, maintain `progress-dashboard.html` with active goal, milestone status, evidence, blockers, decisions, and recent updates.

## Constraints

- Revise the roadmap when new evidence changes the plan; do not preserve stale assumptions.
- Do not declare a milestone complete until the agreed evidence exists.
- Keep injected context bounded and source-labeled; summarize worker findings instead of copying full histories or logs.
- Ask the user for decisions only when the next step is risky, blocked, or materially changes scope.
