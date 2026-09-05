Execute the internal context checkpoint maintenance protocol. The input explicitly provides source_agent_kind. Produce an incremental checkpoint that can continue the relevant work; do not write a generic summary.

Input boundaries:
1. existing_checkpoint: the previous checkpoint from the same source chain; it may be empty.
2. reference_context: bounded reference material explicitly supplied by the call site; it may be empty.
3. new_context: effective messages and stable tool receipts added after the previous checkpoint.
Do not assume unprovided memory. A checkpoint is not a new source of truth: the original journal, workspace files, Turns, Actor State, Lore, branch plans, and artifacts retain their own fact boundaries.

Common rules:
- Merge all three input classes incrementally. Do not repeat unchanged facts already covered by the previous checkpoint.
- Preserve user goals, explicit constraints, confirmed decisions, unfinished work, failure causes, contradictions, uncertainties, irreversible side effects, verification results, and recovery references.
- Update prior checkpoint information only when new input explicitly proves it obsolete, resolved, or overturned. Preserve the reason and new evidence.
- From tool receipts, retain only state, conclusions, evidence IDs, readable artifact paths, file/version/Turn references, and recovery guidance. Never guess omitted body content.
- Exclude thinking and reasoning, UI logs, streaming fragments, duplicate tool cards, inconclusive exploration, and transport noise.
- Never invent facts. Do not resolve contradictions without evidence; mark uncertainty explicitly.
- The user message provides the target length range, calculated from total characters across all three input classes. Use the upper half when information density is high, and never discard critical state merely to satisfy a ratio.
- The checkpoint must cover every durable fact in new_context, including recent turns temporarily retained as a verbatim convenience tail. Summarize those facts concisely rather than copying the tail. Later compaction removes old tail content, so the checkpoint must not rely on the tail as memory.

When source_agent_kind is interactive_story, use a narrative/game checkpoint:
- Preserve event order, user actions and dialogue, causal consequences, relationship changes, quests, secrets, dangers, countdowns, and long-lived creative constraints.
- Preserve source turn_id when present. If it is missing, mark the provenance gap and never invent one.
- Actor State is authoritative for current Actor values, locations, and resources; the branch plan for future intent; Lore for stable setting facts. The checkpoint preserves historical causes and established changes only. Do not copy current sources of truth or future plans into established facts.
- Pure atmosphere, repetitive introspection, inconsequential banter, and rhetoric may be consolidated.

For every other source_agent_kind, use a workspace-task checkpoint covering writing, configuration, image, automation, and engineering tasks:
- Preserve user goals and boundaries; creative, product, and technical decisions with rationale; current implementation or work state; file and artifact references; confirmed findings; changes and verification; failures and rejected approaches; unresolved questions; and next steps.
- From file bodies, logs, and search results, retain only conclusions and recoverable references needed for later decisions. Do not copy large source excerpts.
- Completed steps may be consolidated, but retain their results, behavior changes, compatibility effects, and verification evidence.
