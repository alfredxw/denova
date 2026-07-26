---
name: skills-creator
description: Use this skill when the user wants to create, revise, review, or organize Denova custom Skills in user or workspace scope.
agent: ide,config_manager,automation
---

# Skills Creator

Help the user create or revise a Denova Skill compatible with Denova's progressive-disclosure layout:

```text
<skills-root>/<skill-name>/SKILL.md
<skills-root>/<skill-name>/references/<topic>.md   # optional
```

Use this workflow:

1. Confirm the target scope when it is not explicit:
   - user scope: reusable across books, stored under the Denova user skills directory
   - workspace scope: specific to the current book, stored under `<workspace>/.denova/skills`
   - built-in or preset Skills are read-only; to customize one, create or update a same-name user scope Skill by default, falling back to workspace scope only when the user scope is unavailable, instead of editing `DENOVA_SKILLS_DIR` / `skills_dir`
2. Choose a slash-command-friendly skill name: lowercase letters, digits, `_`, or `-`; start with a letter or digit.
3. Write one concise root `SKILL.md` with YAML frontmatter:
   - `name`: exact skill name and directory name
   - `description`: when the agent should use this skill
4. Keep the root body actionable and small:
   - when to use the skill
   - what context to gather
   - concrete steps the agent should follow
   - output or safety constraints
5. Move detailed schemas, variants, examples, or domain-specific procedures into focused Markdown files under `references/` when keeping them in the root would make every invocation load irrelevant context. The root must route by task and name the exact reference URI to read, for example:

   ```text
   read({"path":"skill://<skill-name>/references/<topic>.md"})
   ```

   Do not invent a `sub_skill` argument and do not create nested Skill names for these references. A reference is inert documentation loaded by `read`, not another Skill invocation.
6. Keep each reference cohesive and independently useful. Do not duplicate the same contract across the root and references. Put reusable non-instruction assets under `assets/`, not `references/`.
7. Do not add README, guide, or sample files outside this layout unless the user explicitly needs them.
8. If editing files is available and the user has already confirmed the goal, write the skill to the selected scope. For built-in Skill customization, write the override to user scope with the same name by default, and use workspace scope only when user scope is unavailable. Otherwise, show the proposed files and ask for confirmation.

After creating or updating the skill, tell the user they can trigger it in the writing agent or other Skills-enabled agents with:

```text
/<skill-name>
```
