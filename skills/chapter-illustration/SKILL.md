---
name: chapter-illustration
description: Generate one spoiler-free illustration for the current or specified chapter and save it under workspace assets/illustrations.
category: image
agent: ide
---

# Chapter Illustration

Use this Skill when the author requests an illustration for a chapter or the editor sends `/chapter-illustration`.

## Workflow

1. Determine the target chapter path:
   - Use the chapter path explicitly provided by the user.
   - Otherwise, use the chapter currently focused in the IDE context.
   - If no chapter is focused, identify the most recent chapter from the workspace summary or `chapters/` directory.
2. Read the target chapter before composing the image prompt. If the selected path is not a chapter, ask the user to select one.
3. Choose a visually compelling, spoiler-free moment from the chapter. Do not reveal future reversals, hidden identities, or undisclosed canon.
4. Write a clear Chinese image prompt containing the subject, scene, composition, lighting, style, and mood, with explicit negative constraints against text, watermarks, and logos.
5. Call the general `generate_image` tool with:
   - `purpose`: `chapter_illustration`.
   - `target_path`: the target chapter's relative path.
   - `prompt`: the complete Chinese image prompt.
   - `alt_text`: a Chinese description suitable for a Markdown image.
   - `size`: optionally choose a supported 2K, 3K, or 4K size based on the composition; otherwise omit it and use the default policy.
   - `output_format`: `png` or `jpeg`; omit it when uncertain.
6. After the tool returns, briefly report what was generated and tell the user to use the illustration card's insertion action. Do not repeat the Markdown image reference in a fenced code block. Do not edit the chapter prose automatically.

## Constraints

- Generate exactly one chapter illustration per request.
- Do not generate batches of candidates.
- Do not use file-write tools to insert Markdown image syntax into the chapter.
- If the user asks to insert the result, prefer the UI insertion action.
