import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentPanel } from './AgentPanel'

const useWritingSkillOptionsMock = vi.hoisted(() => vi.fn())

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({
    effective: { ide_story_teller_id: 'classic', writing_skill_default: 'novel-standard' },
    workspace: {},
  }),
  updateWorkspaceSettings: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/hooks/useSkillCommands', () => ({
  useSkillCommands: () => [],
}))

vi.mock('@/hooks/useWritingSkillOptions', () => ({
  DEFAULT_WRITING_SKILL: 'novel-standard',
  BUILTIN_WRITING_SKILLS: ['novel-lite', 'novel-standard', 'novel-heavy'],
  useWritingSkillOptions: useWritingSkillOptionsMock,
}))

describe('AgentPanel', () => {
  beforeEach(() => {
    useWritingSkillOptionsMock.mockReset()
    useWritingSkillOptionsMock.mockReturnValue([
      { name: 'novel-lite', description: 'Lite', scope: 'builtin', path: '/skills/novel-lite/SKILL.md', active: true, agent: 'ide' },
      { name: 'novel-standard', description: 'Standard', scope: 'builtin', path: '/skills/novel-standard/SKILL.md', active: true, agent: 'ide' },
      { name: 'novel-heavy', description: 'Heavy', scope: 'builtin', path: '/skills/novel-heavy/SKILL.md', active: true, agent: 'ide' },
      { name: 'slow-burn', description: '慢热写作', scope: 'workspace', path: '/book/.nova/skills/slow-burn/SKILL.md', active: true, agent: 'ide' },
    ])
  })

  it('创作 Agent 顶部切换器不再展示 Review tab，并在输入选项中切换写作 Skill', async () => {
    const user = userEvent.setup()
    render(
      <AgentPanel
        workspace="/workspace"
        selectedFile={null}
        tellers={[{ id: 'classic', name: '默认叙事', style_rules: [] } as any]}
        messages={[]}
        sessions={[{ id: 'session-1', title: '当前会话', active: true, message_count: 0, created_at: '', updated_at: '' }]}
        activeSessionId="session-1"
        isStreaming={false}
        activityContent=""
        references={[]}
        loreReferences={[]}
        loreReferenceLabels={{}}
        loreSuggestions={[]}
        styleScenes={[]}
        textSelections={[]}
        fileSuggestions={[]}
        onCreateSession={vi.fn()}
        onSwitchSession={vi.fn()}
        onRenameSession={vi.fn()}
        onDeleteSession={vi.fn()}
        onSend={vi.fn()}
        onAnalyzeContext={vi.fn().mockResolvedValue({} as any)}
        onStop={vi.fn()}
        onReferenceRemove={vi.fn()}
        onLoreReferenceAdd={vi.fn()}
        onLoreReferenceRemove={vi.fn()}
        onStyleSceneAdd={vi.fn()}
        onStyleSceneRemove={vi.fn()}
        onTextSelectionRemove={vi.fn()}
        onClose={vi.fn()}
      />,
    )

    expect(useWritingSkillOptionsMock).toHaveBeenCalledWith('/workspace')
    expect(screen.getByRole('button', { name: '对话' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '会话' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '运行追踪' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '输入动作' }))
    expect(screen.getByText('写作 Skill')).toBeInTheDocument()
    expect(screen.getByText(/Standard/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Review' })).not.toBeInTheDocument()
  })
})
