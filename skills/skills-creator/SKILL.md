---
name: skills-creator
description: Use this skill when the user wants to create, revise, review, or organize Nova custom Skills in user or workspace scope.
agent: ide,config_manager,automation
---

# Skills Creator

Help the user create or revise a Nova Skill compatible with Nova's Skills layout:

```text
<skills-root>/<skill-name>/SKILL.md
```

Use this workflow:

1. Confirm the target scope when it is not explicit:
   - user scope: reusable across books, stored under the Nova user skills directory
   - workspace scope: specific to the current book, stored under `<workspace>/.nova/skills`
   - built-in or preset Skills are read-only; to customize one, create or update a same-name user scope Skill by default, falling back to workspace scope only when the user scope is unavailable, instead of editing `NOVA_SKILLS_DIR` / `skills_dir`
2. Choose a slash-command-friendly skill name: lowercase letters, digits, `_`, or `-`; start with a letter or digit.
3. Write one concise `SKILL.md` with YAML frontmatter:
   - `name`: exact skill name and directory name
   - `description`: when the agent should use this skill
4. Keep the body actionable:
   - when to use the skill
   - what context to gather
   - concrete steps the agent should follow
   - output or safety constraints
5. Do not add README, guide, docs, or sample files unless the user explicitly needs supporting assets.
6. If editing files is available and the user has already confirmed the goal, write the skill to the selected scope. For built-in Skill customization, write the override to user scope with the same name by default, and use workspace scope only when user scope is unavailable. Otherwise, show the proposed `SKILL.md` and ask for confirmation.

After creating or updating the skill, tell the user they can trigger it in the writing agent or other Skills-enabled agents with:

```text
/<skill-name>
```
