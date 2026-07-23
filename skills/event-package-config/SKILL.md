---
name: event-package-config
description: 配置管理 Agent 创建或更新事件包和事件卡时使用。Use when config_manager creates or updates event packages and event cards for Story Directors.
agent: config_manager
---

# Event Package Config

Use this skill before calling `write_event_packages`. For director-level module references and attachment, see `story-director-config`.

## Workflow

1. Call `list_event_packages` first. For updates, call `read_event_packages` for the exact package IDs.
2. Use `write_event_packages` for create/update/delete. Do not edit JSON files directly.
3. Built-in event packages can be read and copied as examples. Deleting built-ins restores their built-in version.
4. For update, preserve fields the user did not ask to change.
5. For delete, require an explicit user request.
6. When grounding event cards in the current work, call `list_lore_items` first, then `read_lore_items` for only the small relevant set. Do not claim concrete world, faction, character, or relationship facts unless they came from read lore, read package data, or explicit user input.

## Event Card Schema

Event packages are standalone resources made of rich event cards. Do not generate keyword-only category packages.

Each `events[]` item in an event package should use this schema:

- `id`: stable ASCII ID, unique inside the package.
- `type_name`: user-visible event type name, for example `外门考核打脸`.
- `description_markdown`: Markdown event card, up to 8000 characters.
- `enabled`: boolean.
- `category`: broad category such as `打脸`, `奇遇`, `学院`, `恋爱`.
- `tags`: short searchable labels.
- `weight`: positive number, usually `1`.
- `cooldown_turns`: non-negative integer, usually `2`.
- `intensity`: short value such as `low`, `medium`, `high`.

`description_markdown` should contain these sections:

```markdown
## 触发场景

## 背景融合方式

## 大致事件逻辑（起承转合）

## 事件回收 / 后果

## 奖励 / 代价

## 避免生硬的约束
```

## Card Content Rules

- Every card must bind to at least one concrete source from the work: a world rule, faction, place, item, character relationship, current conflict source, or user-provided premise.
- Do not generate generic "any protagonist anywhere" cards unless the user explicitly asks for a generic template package.
- Each card should describe a flexible reusable situation, not a fixed future chapter outline.
- The event must integrate with user action and current background; do not force the protagonist into a single choice.
- Include payoff/recovery hooks so the Director Agent can close the event later without leaving dangling pressure.
- If lore was not read, write cards using only user-provided facts and clearly keep them generic.

## Default Generation Strategy

- Generate 12-24 event cards in one package when the user asks for an event pack.
- Write the package with `write_event_packages`, then add its ID to `story_director.module_refs.event_package_ids` only if the user asked to attach it to a director (use `write_story_directors` for that step).
- Cover a mix of 打脸, 扮猪吃虎, 奇遇, 秘境, 天降, 意外, 世界事件, 冲突, 学院, 比拼, 排行, 恋爱, 英雄救美, 误会与消解 where suitable for the actual work.