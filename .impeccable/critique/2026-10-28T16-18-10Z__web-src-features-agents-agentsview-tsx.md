---
target: Agents 页设计
total_score: 24
max_score: 40
na_heuristics: 
p0_count: 0
p1_count: 2
target_identity: "file:D:\\Code\\denova\\web\\src\\features\\agents\\AgentsView.tsx"
target_fingerprint: "sha256:043a38b22772737409232b17e2f32953c9879c2aca5b6a8897db0db55c515121"
target_path: "D:\\Code\\denova\\web\\src\\features\\agents\\AgentsView.tsx"
timestamp: 2026-10-28T16-18-10Z
slug: web-src-features-agents-agentsview-tsx
closed: true
---
Method: dual-agent (A: /root/agents_design_review · B: /root/agents_detector_review)

**Agents 页的视觉基底成立，主要短板在配置层级和操作含义。设计健康度为 24/40。** 页面符合 Denova 克制的工作台方向，写作、游戏的职责也清楚；最大的改进机会是让创作者先看懂自己的改动及影响，再进入运行时和提示词细节。它属于 Operate 界面，熟悉、可靠的控件比视觉表现力更重要。

| 评估项 | 分数 / 4 | 主要依据 |
|---|---:|---|
| 系统状态 | 3 | 保存状态常驻，作用范围缺少选中语义 |
| 贴近用户语言 | 2 | 用途清楚，运行契约等术语偏工程 |
| 用户控制 | 3 | 可取消和折叠，恢复继承入口不清楚 |
| 一致性 | 3 | 控件统一，状态标签兼任重置动作 |
| 错误预防 | 2 | 创建校验已有，重置容易误解 |
| 识别与理解 | 2 | Agent 用途明确，继承后果需推理 |
| 操作效率 | 2 | 高级参数仍需逐项查找 |
| 美观与简约 | 3 | 视觉克制，默认提示词占据主要空间 |
| 错误恢复 | 2 | 源码有重试；实际失败路径未触发 |
| 帮助说明 | 2 | 有说明，但更常解释机制而非选择后果 |
| **总分** | **24/40** | **可用，仍需明显改善** |

**值得保留的部分**

- Agent 按通用、创作、游戏、工具、版本分组，并附用途说明，选择对象清楚。
- 折叠区整行可点击，标题和状态摘要有效；自动保存反馈常驻。
- 黑白主题方向一致；390×844 实测没有横向溢出，主要范围按钮及引擎选择器达到 44px 高度。

**优先问题**

1. **[P1] “覆盖”看起来是状态，点击却执行恢复继承。** Native 旁的“覆盖”按钮直接调用 `onReset`，引擎处理会删除当前层的选择。用户可能为了理解状态而点击，实际却改了配置。建议将“已自定义”作为状态，另设明确的“恢复继承”动作，并说明将采用的来源和值。单字段恢复可提供撤销反馈。此后果由源码确认，本次未点击重置。[agent-form-controls.tsx](D:/Code/denova/web/src/features/agents/agent-form-controls.tsx:77)、[AgentEngineSection.tsx](D:/Code/denova/web/src/features/agents/AgentEngineSection.tsx:72)。建议：`$impeccable clarify`。

2. **[P1] 默认展开的内置提示词压过了用户自己的规则。** 写作和游戏的首屏都先呈现运行时与长篇流程原文；“行为”默认展开，内部所有可编辑来源又默认展开，能力和上下文被推到下方。建议先展示用户规则、继承状态和生效摘要，将内置流程全文收进明确的编辑入口。模型提示词继续保持英文；调整的是展示顺序。高级能力也应按任务分组，避免展开后一次面对 16 项工具或 8 项上下文控制。[agent-configuration-sections.tsx](D:/Code/denova/web/src/features/agents/agent-configuration-sections.tsx:137)。建议：`$impeccable distill`。

3. **[P2] 打开配置助手后，桌面表单被挤到无法完整显示简单选项。** 1280×720 实测中，运行时区域只剩约 291px，Native 选择器约 71px，文字显示不全。表单按浏览器宽度保持两列，但实际所在面板已经很窄。建议按容器宽度转为单列，并让辅助面板收纳策略保护主要表单的可用宽度。[AgentEngineSection.tsx](D:/Code/denova/web/src/features/agents/AgentEngineSection.tsx:70)、[AgentsView.tsx](D:/Code/denova/web/src/features/agents/AgentsView.tsx:442)。建议：`$impeccable adapt`。

4. **[P2] 从游戏 Agent 点击总“新建”入口，仍默认创建写作 Agent。** 实际弹窗和源码一致：总入口固定传入写作类型，分组加号则使用对应类型。两个同类入口的默认行为不一致，容易建立错误类型。建议总入口沿用当前可创建类型；将“运行契约”改为“基于哪个 Agent”等面向用户的字段名，保留复制独立定义的说明。[custom-agent-management.tsx](D:/Code/denova/web/src/features/agents/custom-agent-management.tsx:82)。建议：`$impeccable clarify`。

5. **[P2] 配置范围与标题层级缺少正确语义。** “用户配置／当前工作区”在可访问树中都是普通按钮，未暴露哪个被选中；页面又出现外层 h2、对象 h1、内部 h3，主要折叠标题没有对应标题层级。用户需要可靠判断正在影响哪个范围，读屏导航也应反映页面结构。建议复用 Tabs 或 ToggleGroup 表达选中范围，并建立一致的页面、对象、章节标题层级，保持现有紧凑字号。[AgentsView.tsx](D:/Code/denova/web/src/features/agents/AgentsView.tsx:359)、[agent-configuration-disclosure.tsx](D:/Code/denova/web/src/features/agents/agent-configuration-disclosure.tsx:36)。建议：`$impeccable harden`。

**认知与使用体验**

高级区展开时，认知检查 8 项中有 4 项未通过：分块、视觉层级、一次处理一个决定、选项组织。问题不是功能数量本身，而是工具权限、继承状态、预算和协议说明缺少进一步的任务分组。

进入时，安静的界面和明确分组带来稳定感；随后长提示词与工程术语提高理解成本，“覆盖”的隐藏后果增加犹豫。保存反馈很好，但还需回答“这些修改影响什么”。

- **新手创作者**：难以判断应改内置流程、用户规则，还是另建 Agent。
- **熟练用户**：查找特定权限或上下文限制需要逐项浏览，打开助手后表单更难用。
- **读屏用户**：范围选中状态与标题结构无法可靠表达当前配置位置。此项依据实际 AX/DOM，未运行真实读屏软件。

**检测结果及次要观察**

源码检测为 **0 条**；浏览器检测在写作、游戏、图像视图分别报告 **10、9、6 次命中**，包括 `nested cards`、`flat type hierarchy`，另有 `skipped heading level` 诊断。这些跨视图重复，不能相加称为 25 个问题。

嵌套区域主要位于 [agent-configuration-disclosure.tsx](D:/Code/denova/web/src/features/agents/agent-configuration-disclosure.tsx:26) 和提示词来源块；顶层折叠是有用的结构，不应全部去掉。字体比例告警也不构成把配置页做成大标题页面的理由。标题语义跳级是确认的问题，已纳入第 5 项；提示词层层边框和默认展开则支持第 2 项。

`Denova built-in`、`user/workspace config` 等界面来源标签应本地化；模型提示词正文仍保持英文。部分 10–11px 信息承担真实决策含义，应减少冗余并保留可读性，本次未作完整对比度认证。

**视觉证据**

- [写作默认页](D:/Code/denova/.impeccable/critique/evidence/agents-2026-10-29/a-writing-default.png)
- [游戏默认页](D:/Code/denova/.impeccable/critique/evidence/agents-2026-10-29/a-game-default.png)
- [能力展开态](D:/Code/denova/.impeccable/critique/evidence/agents-2026-10-29/a-capabilities.png)
- [创建弹窗](D:/Code/denova/.impeccable/critique/evidence/agents-2026-10-29/a-create-dialog.png)
- [配置助手展开态](D:/Code/denova/.impeccable/critique/evidence/agents-2026-10-29/parent-config-helper.png)
- [390px 窄屏](D:/Code/denova/.impeccable/critique/evidence/agents-2026-10-29/parent-mobile.png)
- [浅色样式预览](D:/Code/denova/.impeccable/critique/evidence/agents-2026-10-29/parent-light.png)
- [浏览器检测覆盖层](D:/Code/denova/.impeccable/critique/evidence/agents-2026-10-29/b-overlay-writing.png)

后续设计讨论的核心问题：首屏应先展示用户自己的规则与生效摘要，还是继续让运行时和原始提示词成为主体？前者更符合本次确认的创作者定位。
