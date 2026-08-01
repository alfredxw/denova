package prompts

import (
	"strings"

	agentcontext "denova/internal/agents/context"
)

func ChapterSplitRegexSystemInstruction() string {
	return strings.Join([]string{
		"你负责为 Denova 小说导入识别章节和分卷标题行。",
		"只输出 JSON object，schema 为 {\"split_regex\":\"...\",\"reason\":\"...\"}。",
		"split_regex 必须是 Go regexp，可用于逐行匹配章节标题行和分卷标题行；不要使用跨行匹配。",
		"如果标题里有编号前缀和正文标题，优先用第 1 个捕获组捕获完整章节标题；否则不使用捕获组也可以。",
		"正则应尽量保守，只匹配章节/分卷标题行，不匹配普通正文句子。",
		"不要返回 Markdown、解释文本或代码块。",
	}, "\n")
}

func ContextCompactionSystemInstruction() string {
	base := strings.TrimSpace(`
你是 Denova 的独立“上下文 checkpoint 编译器”。输入会明确给出 source_agent_kind；你必须按来源模式生成可继续工作的增量 checkpoint，而不是写一段泛化摘要。

输入边界：
1. existing_checkpoint：同一来源链的旧 checkpoint，可能为空。
2. reference_context：调用点显式提供的有界参考，可能为空。
3. new_context：旧 checkpoint 之后新增的有效消息与稳定 tool receipt。
不得假定存在未提供的记忆；checkpoint 不是新的事实真源，原始 journal、工作区文件、Turn、Actor State、Lore、DirectorPlan 与 artifact 仍各自拥有其事实边界。

共同规则：
- 增量合并三类输入；不要重复旧 checkpoint 已覆盖且未变化的事实。
- 用户目标、明确约束、已确认决定、未完成事项、失败原因、矛盾、不确定性、不可逆副作用、验证结果和恢复引用必须保留。
- 新输入只有明确证明旧信息失效、解决或被推翻时，才能更新旧 checkpoint；保留原因与新证据。
- tool receipt 只提炼其中的状态、结论、证据 ID、artifact 可读路径、文件/版本/Turn 引用和恢复提示；不得把已省略正文重新猜出来。
- 排除 thinking/reasoning、UI 日志、流式片段、重复工具卡片、无结论探索和传输噪音。
- 禁止编造；矛盾不得擅自裁决，不确定时明确标记。
- 目标长度由用户消息给出的范围控制，按三类输入总字符数计算；信息密度高时使用上半区，不得为达成比例丢失关键状态。
- checkpoint 必须覆盖 new_context 全部 durable facts，包括会作为 verbatim convenience tail 暂时保留的最近回合；简洁提炼这些事实，不要逐字复制 tail。后续压缩会让旧 tail 退出，因此不得依赖 tail 代替 checkpoint 记忆。

当 source_agent_kind 是 interactive_story 或 interactive_director 时，使用“叙事/游戏 checkpoint”：
- 保留事件顺序、用户行动与对白、因果后果、关系变化、任务、秘密、危险、倒计时和长期创作约束。
- 有 source turn_id 时必须保留；缺失时标记来源缺失，不得自造。
- 当前 Actor 数值/位置/资源以 Actor State 为准；未来安排以 DirectorPlan 为准；稳定设定以 Lore 为准。checkpoint 只保留历史原因和已发生变更，不复制当前真源，不把计划写成事实。
- 可以合并纯氛围、重复心理描写、无后果闲聊和修辞。

其他 source_agent_kind 使用“工作区任务 checkpoint”，同时适用于写作、配置、图像、自动化和工程任务：
- 保留用户目标与边界、创作/产品/技术决定及理由、当前实现或作品状态、文件与 artifact 引用、已确认发现、变更与验证、失败和被否决方案、未解决问题及下一步。
- 文件正文、日志和搜索结果只保留后续决策所需结论及可恢复引用；不要复制大段源内容。
- 已完成步骤可以合并，但必须保留结果、行为变化、兼容性影响和验证证据。
`)
	return base + "\n\n所有来源模式都使用以下唯一、稳定的 Markdown checkpoint schema；不适用的 section 可以为空，但不要改名或另造一套格式：\n" +
		agentcontext.CompactionCheckpointSchema()
}
