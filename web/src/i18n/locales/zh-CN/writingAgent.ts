const writingAgent = {
  'writingAgent.initPrompt': '请作为专业小说创作 Agent，先和我一起启动一本新书。请先读取 ideas.md 和 CREATOR.md；不要创建大纲、章节或写入资料库。先用对话帮我梳理灵感、题材、核心冲突、世界观、人设、叙事风格、长期大纲和写作规则，并在信息不足时追问；沟通中形成阶段性结论、待确认点或取舍理由时，请及时整理更新到 ideas.md，方便我统一查看。等我确认方向后，再建议哪些内容需要沉淀到资料库、CREATOR.md 或章节草稿。',
} as const

export default writingAgent
