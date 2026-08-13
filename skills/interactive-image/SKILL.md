---
name: interactive-image
description: Generate one image for an already established interactive-story turn.
category: image
agent: image
---

# Interactive Image

Use this Skill when the caller's `purpose` is `interactive_image`.

## Workflow

1. Build the image prompt only from interactive-story events already established in `source_context`.
2. Choose the most visually compelling moment that best represents the current turn. Do not reveal future plot, hidden identities, or undisclosed canon.
3. If the caller provides an image preset, incorporate it as the visual direction for the image prompt.
4. Write a complete Chinese image prompt containing the subject, scene, composition, lighting, mood, and visual style, with explicit negative constraints against text, watermarks, and logos.
5. Call `generate_image` with:
   - `purpose`: `interactive_image`.
   - `story_id`, `branch_id`, and `turn_id`: copy the caller-provided values exactly.
   - `prompt`: the complete Chinese image prompt.
   - `alt_text`: use the caller-provided value, or write a short Chinese description when it is absent.
   - `size`: optionally choose a supported 2K, 3K, or 4K size based on the composition; otherwise omit it.
   - `output_format`: `png` or `jpeg`; omit it when uncertain.
6. After the tool returns, report the generation result briefly and do not advance the story.

## Constraints

- Generate exactly one interactive image per request.
- Do not generate batches of candidates.
- Do not modify story prose, chapters, lore, or configuration.
- Do not insert image Markdown into narrative text.
