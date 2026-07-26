# Skill (`skill`)

Skills are editable in `user` or `workspace` scope. Built-in Skills are read-only; customize one by creating a same-name override. Active precedence is `workspace` over `user` over `builtin`.

Each `config_apply` changes exactly one root `SKILL.md` or one supporting reference file. Root and supporting-file updates are complete content replacements.

## Identity, scope, and revision

- Skill names must match `[A-Za-z0-9][A-Za-z0-9_-]{0,63}`.
- Root document ID is `<skill-name>`.
- Supporting reference ID is `<skill-name>/references/<file>.md`; nested paths below `references/` are allowed, traversal and backslashes are not.
- Every `get`, update, and delete must specify exact `user` or `workspace` scope.
- Updating/deleting a root requires its latest directory `revision`. Any supporting-file change invalidates an older root revision.
- A supporting file has its own returned revision for its update/delete.
- Supporting files must be UTF-8 and no larger than 512 KiB.

Use `workspace` only for book-specific behavior. Use `user` for reusable personal behavior and normally for same-name built-in overrides.

## Root create values

### Generated root

Supply `name`, trigger-oriented `description`, optional `agents`, optional catalog `category`, and optional `capabilities`. The backend creates valid frontmatter and a concise starter body. Omitted categories resolve to `general`.

```text
config_apply({
  "operation": "create",
  "resource": "skill",
  "scope": "workspace",
  "value": {
    "name": "scene-continuity-check",
    "description": "Use when reviewing a drafted scene for continuity against nearby chapters and supplied Lore.",
    "category": "writing",
    "agents": ["ide"]
  }
})
```

### Complete root

Supply `name` plus complete `content`. The frontmatter `name` must match the directory name. The optional `agent` field is a comma-separated availability list; `category` organizes the management catalog independently from Agent visibility.

```text
config_apply({
  "operation": "create",
  "resource": "skill",
  "scope": "workspace",
  "value": {
    "name": "scene-continuity-check",
    "content": "---\nname: scene-continuity-check\ndescription: Use when reviewing a drafted scene for continuity against nearby chapters and supplied Lore.\ncategory: writing\nagent: ide\n---\n\n# Scene continuity check\n\n1. Read the target scene, the immediately adjacent scenes, and only the relevant Lore entries.\n2. Report contradictions in timeline, location, names, knowledge, inventory, injuries, and unresolved setup.\n3. Separate confirmed contradictions from questions that require author intent.\n\nFinish only when every reported issue cites its source. For the issue format, read `skill://scene-continuity-check/references/report-format.md`.\n"
  }
})
```

Keep the root actionable and concise. Put resource schemas, variants, and long examples in supporting files, then add a context pointer that says exactly when to read each one. References are not sub-Skills and never use a `sub_skill` argument.

Use `capabilities: [writing-workflow]` only for a complete IDE writing workflow that can safely serve as the configured Writing Skill. Writing-related helpers such as polishing, continuity review, or research should use `category: writing` without that capability; they remain directly invocable but do not appear in the Writing Skill selector.

## Supporting reference lifecycle

The parent root must exist before a supporting file is created:

```text
config_apply({
  "operation": "create",
  "resource": "skill",
  "scope": "workspace",
  "id": "scene-continuity-check/references/report-format.md",
  "value": {
    "content": "# Report format\n\nFor each issue provide: severity, claim, source file/section, conflicting source, and the smallest safe correction.\n"
  }
})
```

Read and update the exact supporting-file ID:

```text
config_read({
  "operation": "get",
  "resource": "skill",
  "scope": "workspace",
  "ids": ["scene-continuity-check/references/report-format.md"]
})

config_apply({
  "operation": "update",
  "resource": "skill",
  "scope": "workspace",
  "id": "scene-continuity-check/references/report-format.md",
  "revision": "REVISION_FROM_GET",
  "value": {
    "content": "# Report format\n\nFor each issue provide: severity, claim, both exact sources, confidence, and the smallest safe correction. Put unresolved author choices in a separate Questions section.\n"
  }
})
```

## Root update example

Read the root in the exact scope and replace its complete content. Preserve frontmatter availability and every context pointer the user did not ask to change:

```text
config_apply({
  "operation": "update",
  "resource": "skill",
  "scope": "workspace",
  "id": "scene-continuity-check",
  "revision": "REVISION_FROM_GET",
  "value": {
    "content": "COMPLETE_UPDATED_SKILL_MD_WITH_MATCHING_FRONTMATTER"
  }
})
```

The marker means the actual complete document composed from `get`; never save the marker text.

## Overrides, deletion, and verification

Creating a same-name `user` or `workspace` Skill shadows lower-priority copies but does not mutate them. After every mutation, read the exact scope and list the catalog to verify both saved content and which scope is active.

Do not delete or disable a Skill unless explicitly requested. Deleting a root removes that editable Skill directory and its references; an effective same-name Skill may then reappear from a lower-priority scope. Deleting one reference leaves the root intact, so also verify that no remaining context pointer requires the deleted file.
