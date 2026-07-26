# Automation (`automation`)

Automations are user- or workspace-scoped definitions. Runtime IDs, timestamps, trigger state, and run history are read-only and must not be copied into `config_apply.value`.

## Editable value

- `target`: required on create; `kind` plus optional `workspace`
- `enabled`, `name`, `template`, `prompt`, optional `model_profile_id`
- `schedule`, `triggers`
- `write_mode`: `read_only`, `confirm_write`, or `auto_write`
- `write_scope`: `none`, `lore`, `file`, or `lore_and_file`
- `output_policy`: `run_record_only` or `optional_file`; use `output_path` only with the latter

Trigger types are `manual`, `schedule`, `semantic`, or `chapter_batch`. Use stable trigger IDs. Schedule kinds are `manual`, `daily`, `weekly`, `monthly`, or `every_hours`; hours are 0–23, minutes 0–59, weekday 0–6 (Sunday is 0), day-of-month 1–31, and every-hours 1–168. Do not set derived cron or trigger-level action policy.

Safe defaults: reviews are `read_only` + `none` + `run_record_only`; prefer `confirm_write` for file-writing tasks; unattended writes require an explicit request, a narrow scope, and a prompt naming exact boundaries.

Use `workspace` for book-specific work and `user` for reusable personal tasks. Read the exact task before update, pass only editable fields, and use its `revision` for update/delete.
