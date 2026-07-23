---
name: rewrite
description: 重写或修改已有章节。当用户要求修改某章、改写某段、换视角重写、调整对话时使用。Rewrite or revise existing chapters, paragraphs, dialogue, or perspective.
agent: ide
depends: writing-common
---

# 章节重写与修改

根据作者要求修改已有的章节内容。先加载 `writing-common` 获取公共规范（工具使用、错误检查、读回验证、状态文件边界）。

## 工作流程

1. 使用 `read_file` 读取目标章节全文，以及 `setting/outline.md`、`setting/progress.md`、`setting/character-states.md`；如修改内容属于当前章节组，也读取 `setting/chapter-groups/` 下对应细纲。
2. 根据修改类型选择工具：
   - **局部修改**（改对话、风格调整、扩写/缩写片段）：使用 `edit_file`，`old_string` 必须来自最近一次 `read_file` 的实际内容。
   - **大幅重写**（换视角、调情节、整章重写）：使用 `write_file` 写回完整章节。
3. 修改后必须与前后章节保持连贯。重大情节变化需提醒作者检查后续章节是否需要调整。
4. 仅当情节或角色状态发生实质变化时，才更新 `setting/progress.md` 和 `setting/character-states.md`。小幅修改（改对话、风格调整）不触发状态更新。
5. 只有角色身份、人设、长期关系、能力体系、世界规则等稳定设定有变化时，才使用 `write_lore_items` 同步资料库。
6. 除非作者明确要求调整大纲，不更新 `setting/outline.md`。

## 修改类型

- **改对话**：调整角色对白，保持性格一致。用 `edit_file` 局部替换。
- **调情节**：修改事件走向，注意前后文连贯。视范围选择 `edit_file` 或 `write_file`。
- **换视角**：用不同角色的视角重写，保持信息一致。通常需要 `write_file` 整章重写。
- **扩写**：在保持主线不变的前提下丰富细节。用 `edit_file` 插入或替换段落。
- **缩写**：精简冗余描写，保留核心情节。用 `edit_file` 替换。
- **风格调整**：改变叙事语调或文风。视范围选择工具。

## 注意事项

- 保持角色性格和说话方式的一致性。
- 修改章节定稿时同步检查当前章节组细纲是否还成立；如明显偏离，只提示作者确认是否调整细纲，不要擅自重写长期大纲。
- 不要把已完成章节复盘写进 `setting/outline.md`，也不要把未来章节规划写进资料库。