---
name: novel-lite
description: Fast continuation, exploratory drafts, and low-latency prose generation performed directly by the main Agent without review or revision subprocesses.
category: writing
capabilities:
  - writing-workflow
agent: ide
---

# novel-lite

Use this Skill for fast prose generation by the IDE Writing Agent.

## Determine the writing scope

- Infer the scope from the user's actual request, such as continuing a passage, writing a scene, one chapter, three chapters, a story arc, or a custom target.
- Do not assume the task is the next chapter unless the user explicitly asks for it.
- There is no `writing_scope` field. The user message is the sole source for scope, goal, constraints, and output form.
- When the user requests multiple chapters, do only a lightweight internal breakdown: determine the overall direction and chapter boundaries, then write at the requested scale.

## Workflow

main Agent -> final output

## Tool requirements

- When continuity matters, first use `read` for the relevant workspace files, such as `CREATOR.md`, `setting/outline.md`, `setting/progress.md`, `setting/character-states.md`, the current chapter-group plan, and recent chapters. For lore, use `list_lore_items` to identify relevant entries and `read_lore_items` to load their complete bodies.
- If the user asks only for a passage, exploratory draft, or example in chat, output the prose directly and do not write workspace files.
- When the user asks to create or update workspace prose, use `write` for a new file or complete rewrite and `edit` for localized replacement. `old_string` must match the current file exactly and uniquely and must not contain line-number prefixes returned by `read`.
- Inspect every `write` or `edit` result. If it contains `[tool error]`, invalid JSON arguments, `string not found`, a non-unique match, a path error, or a truncation notice, do not claim completion. Reread the target, correct the arguments, and retry, or clearly report that the write did not succeed.
- After user-visible file changes, use `read` to verify key excerpts from the final file. If verification is impossible, say so in the final response.
- After writing a complete chapter or materially changing its plot, finish the prose self-review and final revision, then update `setting/progress.md` and `setting/character-states.md` in the same turn. Chapter status in the UI does not affect synchronization. Pure typo, punctuation, or wording edits that do not change narrative facts need no state update.
- Treat `setting/progress.md` only as a summary. Determine the next chapter from actual chapter paths and non-empty prose, and correct progress in the same turn when they conflict.

## Rules

- The main Agent produces the final result directly.
- Do not start reviewer, fixer, task, General SubAgent, or configured subagent workflows.
- A lightweight internal check for continuity, user requirements, and obvious prose issues is allowed, but do not expose the review process.
- Preserve user control. Do not over-plan, over-explain, or turn the requested draft into a different story.
- Use workspace context, selected text, lore references, and style rules only when relevant to this turn's writing scope.

## Output

- Return the creative result the user requested directly.
- Add a brief explanation only when the user asks for one or an important constraint cannot be satisfied.
