package automation

const DefaultContinueWritingPrompt = "续写下一章。请先读取 CREATOR.md、长期大纲、章节组细纲、progress.md、角色状态、资料库和最近章节，判断下一章所属分卷、章节标题和目标路径；再按现有故事节奏创作正文。写入前确认章节路径符合当前章节命名规则和大纲安排；完成后按需同步 progress.md 和 setting/character-states.md。"

const DefaultReviewPrompt = "对本次触发范围中的新增章节做自动 Review。若触发范围包含章节路径，只评审这些新增章节，不要把全书当作被评审正文；可读取必要前文、CREATOR.md、大纲、进度、角色状态和资料库作为对照依据。重点检查新增章节是否符合任务要求/用户 Prompt、CREATOR.md、长期大纲、角色设定与状态、世界观和已有连续性；评估剧情推进、人物行为动机、设定一致性、节奏、语言质量和可读性。按严重程度输出问题、证据位置、影响和可执行改进建议；执行模式不允许写入时只输出 Review 和修订方案。"

const GenericTaskPrompt = "根据任务配置完成这次自动化。请先自行读取必要信息，再执行；如果任务目标不明确，只输出你需要用户补充的配置建议。"
