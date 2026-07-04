---
name: lore
description: 使用资料库工具读取、整理、更新或删除长期设定时使用；覆盖 list_lore_items、read_lore_items 和 write_lore_items 的基本方法。
agent: ide,config_manager,interactive_story
---

# Lore

资料库只保存长期稳定设定：角色身份、人设、长期关系、世界观、地点、势力、规则、能力体系和关键物品。章节后的当前位置、伤势、心理、目标、持有物等短期状态写入 `setting/character-states.md`，不要写进资料库。

## 工具顺序

1. `list_lore_items`：先看索引。空参数返回全量极简索引；需要筛选时使用 `query`、`type`、`limit`。索引只用于判断相关性，不当作完整设定。
2. `read_lore_items`：只读取本轮需要的少量 ID。不要无界读取全部正文。
3. `write_lore_items`：只在当前 Agent 拥有该工具，且作者要求保存或长期设定确实变化时使用。互动叙事 Agent 默认只读，不要承诺已写入。

## 常见参数示例

`list_lore_items` 全量索引：

```json
{}
```

`list_lore_items` 按关键词或类型检索：

```json
{"query":"林川 火光","type":"character","limit":10}
```

`read_lore_items` 批量读取正文：

```json
{"ids":["hero-linchuan","ember-city"]}
```

`write_lore_items` 创建或更新条目：

```json
{
  "message": "补充主角与城市设定",
  "items": [
    {
      "id": "hero-linchuan",
      "type": "character",
      "name": "林川",
      "importance": "major",
      "tags": ["主角", "火光"],
      "brief_description": "角色 林川。主视角幸存者，常被称为小林。关键事实包括他掌握火光能力，并与余烬城长期绑定。上下文出现林川、小林、火光或余烬城相关内容时，一定要参考本项详情。",
      "keywords": ["林川", "小林", "火光"],
      "load_mode": "auto",
      "content": "## 核心设定\n\n林川是主视角幸存者，长期目标是在余烬城建立稳定据点。"
    }
  ],
  "delete_ids": []
}
```

`write_lore_items` 删除条目：

```json
{"message":"删除废弃重复资料","items":[],"delete_ids":["old-hero-draft"]}
```

## 写入规则

- `items` 必须是数组；创建或更新条目都填写完整字段，避免覆盖时丢失已有设定。
- `delete_ids` 必须是数组，例如 `[]` 或 `["lore-id"]`；不要传字符串 `"[]"`。只有作者明确要求删除时才填写非空数组。
- 更新已有条目必须填写准确 `id`；新建条目可留空自动生成。
- `brief_description` 以“类型 名称。”开头，写清身份、别名、关键事实、适用场景和触发词，并以“上下文出现相关内容时，一定要参考本项详情。”收束。
- `content` 使用中文 Markdown，只写稳定事实，不写章节规划、未来剧情或临时状态。
- `load_mode` 默认使用 `auto`；只有短小且必须常驻的核心设定才用 `resident`。

完成后简要说明读取或写入了哪些资料条目；工具失败时先修正参数或说明未完成，不要假装成功。
