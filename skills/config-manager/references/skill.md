# Skill (`skill`)

Skills are editable in `user` or `workspace` scope. Built-in Skills are read-only; customize one by creating a same-name override, normally in user scope unless the behavior is book-specific.

## Value

- Create without full content: `name`, `description`, and optional `agents`; the backend creates valid `SKILL.md` content.
- Create with full content: `name` plus complete `content`.
- Update: complete `content` for the exact Skill name.
- Every get/update/delete must specify `scope`; update/delete require the document `revision`.

The stable ID for the root document is `<skill-name>`. A supporting reference is independently addressed as `<skill-name>/references/<file>.md`:

- Create a reference with that ID and `value.content`; the parent Skill must already exist.
- Get the same ID to obtain the file revision, then update/delete it with that exact revision.
- One `config_apply` changes one root document or one reference file. Do not send a hidden multi-file batch.

The content must have YAML frontmatter with matching `name`, a clear trigger-oriented `description`, and optional comma-separated `agent` availability. Keep the root actionable and concise. Move detailed schemas or variants into `<skill>/references/*.md`, route to them from the root, and read them at runtime through `skill://<name>/references/<file>.md`. References are not sub-Skills and never use a `sub_skill` parameter.

Do not delete or disable a Skill unless explicitly requested. After mutation, read the exact scope again; an effective same-name Skill may still come from a higher-priority scope.
