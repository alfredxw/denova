# Style reference (`style_reference`)

Shared Markdown prose references are available to both Writing and Game modes and use user scope.

## Values

- Create value: `name`, `description`, optional `filename`, and complete Markdown `content`.
- Update value: complete replacement `content`. Use the exact display path returned by `config_read` as `id` and its `revision` as the mutation revision.
- Delete: exact display path plus latest revision; only after an explicit request.

Prefer distilled, reusable sample paragraphs and concise style guidance. Do not preserve long copyrighted passages, source/work names, temporary story facts, tool rules, model settings, or secrets. Keep the content useful in both Writing and Game modes.

When attaching a reference to a narrative style, first create/read this resource and then use its returned `display_path` in `style_refs` or a scene rule.
