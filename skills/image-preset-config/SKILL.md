---
name: image-preset-config
description: Use when config_manager creates or updates Denova image preset configurations.
agent: config_manager
---

# Image Preset Config

Use this skill before calling `write_image_presets`.

## Workflow

1. Call `list_image_presets` first. For updates, call `read_image_presets` for the exact preset IDs.
2. Use `write_image_presets` for create/update/delete. Do not edit image preset JSON files directly.
3. Built-in image presets can be read and copied as examples. Deleting built-ins is rejected.
4. For update, preserve fields the user did not ask to change.
5. For delete, require an explicit user request.

## Image Preset Shape

Important fields:

- `id`: stable ID. Required for update/delete; create may generate one if omitted.
- `name`: user visible name.
- `description`: short explanation of the visual style.
- `prompt`: visual constraints for image generation only. Keep it under 4000 characters.
- `tags`: short searchable labels.

Do not change `version`, `path`, `custom`, `invalid`, `error`, `created_at`, or `updated_at` unless preserving an existing complete object from `read_image_presets`.

## Prompt Guidance

The preset prompt is only for image generation. It should describe:

- medium or rendering style
- subject framing and composition
- lighting, color, texture, and mood
- negative constraints such as no text, no watermark, no logo, and no future-story spoilers

Do not put story facts, chapter prose, tool permissions, model settings, API keys, or temporary scene state into an image preset.
