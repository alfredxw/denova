---
name: continue
description: 续写小说章节。当用户要求写下一章、继续写、开始写某一章时使用。Continue writing the next novel chapter based on outline, character states, and prior text.
agent: ide,interactive_story
---

# 章节续写

根据大纲、角色状态、资料库设定和前文内容续写小说章节。先加载 `writing-common` 获取公共规范（工具使用、错误检查、读回验证、章节正文格式、状态文件边界）。

## 工作流程

1. 使用 `read_file` 读取 `setting/outline.md` 确认长期方向。
2. 使用 `read_file` 读取 `setting/progress.md` 获取最近章节摘要，并读取 `setting/character-states.md` 确认角色当前状态。
3. 如 `setting/chapter-groups/` 下存在当前章节组细纲，优先读取对应细纲，确认本章在组内的作用、冲突、信息揭示和钩子。
4. 必须读取前面至少 2 章定稿正文，确保本章与前文自然衔接。
5. 根据大纲方向、章节组细纲、角色当前状态、资料库长期设定和前文正文创作本章内容；如本轮涉及资料库索引中的相关自动加载条目（基于简介判断），先用 `read_lore_items` 读取完整资料。
6. 使用 `write_file` 将章节写入 `chapters/` 下符合系统提示章节文件名模板的位置；非空新章节默认是初稿，由章节状态控制是否成章。
7. 不更改 `setting/outline.md`。
8. 只有作者明确确认成章或明确要求同步状态后，才按 `writing-common` 的状态文件边界规则更新 `setting/progress.md` 和 `setting/character-states.md`；只有长期设定发生明确变化时，才使用 `write_lore_items` 同步资料库。

## 写作要求

- 严格遵循大纲的章节走向和摘要。
- 如有章节组细纲，优先保证本章完成细纲中分配的短期情节功能。
- 保持角色性格和说话方式一致。
- 与前面至少两章自然衔接（注意情节、时间、地点、人物状态的连贯）。
- 注意伏笔的埋设和呼应。
- 遵循指定的字数要求。

## 进度文件格式

```markdown
# 创作进度

- 当前进度：第X章已完成
- 总章节数：Y 章
- 已完成：Z 章

## 最近章节摘要

### 第X章：章节标题
摘要内容（200字以内，概括主要情节和角色变化）
```