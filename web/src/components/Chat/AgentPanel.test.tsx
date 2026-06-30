import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentPanel } from './AgentPanel'

const useWritingSkillOptionsMock = vi.hoisted(() => vi.fn())

vi.mock('@/features/settings/api', () => ({
  fetchSettings: vi.fn().mockResolvedValue({
    effective: { ide_story_teller_id: 'classic', writing_skill_default: 'novel-lite' },
    workspace: {},
  }),
  updateWorkspaceSettings: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/hooks/useSkillCommands', () => ({
  useSkillCommands: () => [],
}))

vi.mock('@/hooks/useWritingSkillOptions', () => ({
  DEFAULT_WRITING_SKILL: 'novel-lite',
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
    renderAgentPanel()

    expect(useWritingSkillOptionsMock).toHaveBeenCalledWith('/workspace')
    expect(screen.getByRole('button', { name: '对话' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '会话' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '运行追踪' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '输入动作' }))
    expect(screen.getByText('叙事')).toBeInTheDocument()
    expect(screen.getByText('默认叙事')).toBeInTheDocument()
    expect(screen.getByText('写作 Skill')).toBeInTheDocument()
    expect(screen.getByText(/Lite/)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Review' })).not.toBeInTheDocument()
  })

  it('将新建会话按钮放在标题切换器旁边并隐藏会话摘要和空闲状态文字', async () => {
    const user = userEvent.setup()
    const handleCreateSession = vi.fn()
    renderAgentPanel({ onCreateSession: handleCreateSession })

    expect(screen.queryByText('等待')).not.toBeInTheDocument()
    expect(screen.queryByText('当前：')).not.toBeInTheDocument()
    expect(screen.queryByText('当前会话')).not.toBeInTheDocument()
    const createButton = screen.getByRole('button', { name: '新建会话' })
    expect(createButton).toHaveClass('w-7')
    expect(createButton).not.toHaveTextContent('新建')

    await user.click(createButton)
    expect(handleCreateSession).toHaveBeenCalledTimes(1)
  })

  it('打开 SubAgent 详情时通知外层扩展右栏', async () => {
    const user = userEvent.setup()
    const handleDetailsChange = vi.fn()

    renderAgentPanel({
      messages: [{
        id: 'subagent-output-1',
        role: 'assistant',
        content: '调研摘要',
        agent_name: 'researcher',
        subagent: true,
        subagent_session_id: 'run-1-subagent-01-researcher',
      } as any],
      onSubAgentDetailsChange: handleDetailsChange,
    })

    expect(handleDetailsChange).toHaveBeenLastCalledWith(false)
    await user.click(screen.getByRole('button', { name: /researcher 输出/ }))
    expect(handleDetailsChange).toHaveBeenLastCalledWith(true)
    expect(screen.getAllByText('researcher 子会话').length).toBeGreaterThan(0)
    expect(screen.getByRole('separator', { name: '调整 SubAgent 详情宽度' })).toBeInTheDocument()

    await user.click(screen.getAllByRole('button', { name: '关闭 SubAgent 详情' })[0])
    expect(handleDetailsChange).toHaveBeenLastCalledWith(false)
  })

  it('根据浮动输入区高度为消息列表预留底部空间', async () => {
    const rectSpy = vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
      if (this.classList.contains('nova-chat-input-area-floating')) {
        return { width: 520, height: 220, top: 500, left: 0, right: 520, bottom: 720, x: 0, y: 500, toJSON: () => ({}) } as DOMRect
      }
      return { width: 0, height: 0, top: 0, left: 0, right: 0, bottom: 0, x: 0, y: 0, toJSON: () => ({}) } as DOMRect
    })

    try {
      const { container } = renderAgentPanel({
        messages: [{ id: 'assistant-1', role: 'assistant', content: '最后一行内容' } as any],
      })

      await waitFor(() => {
        expect(container.querySelector('[data-nova-chat-bottom-spacer]')).toHaveStyle({ height: '240px' })
      })
    } finally {
      rectSpy.mockRestore()
    }
  })

  it('收到章节插画 autoSend 事件时直接发送到创作 Agent', async () => {
    const handleSend = vi.fn()
    renderAgentPanel({
      selectedFile: 'chapters/ch01.md',
      currentChapter: {
        path: 'chapters/ch01.md',
        file_name: 'ch01.md',
        display_title: '第一章',
        index: 1,
        words: 100,
        status: 'draft',
        confirmed: false,
        updated_at: '',
        volume: '',
        volume_path: '',
      },
      onSend: handleSend,
    })

    window.dispatchEvent(new CustomEvent('nova:writing-agent-init', {
      detail: { autoSend: true, prompt: '/<chapter-illustration>\n目标章节 / Target chapter: chapters/ch01.md' },
    }))

    await waitFor(() => {
      expect(handleSend).toHaveBeenCalledWith(
        expect.stringContaining('/<chapter-illustration>'),
        expect.objectContaining({ writingSkill: 'novel-lite' }),
      )
    })
  })
})

function renderAgentPanel(overrides: Partial<ComponentProps<typeof AgentPanel>> = {}) {
  return render(
    <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 52 }}>
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
        planMode={false}
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
        onPlanModeChange={vi.fn()}
        onPlanModeToggle={vi.fn()}
        onSubmitPlanQuestion={vi.fn()}
        onApproveProposedPlan={vi.fn()}
        onExitPlanMode={vi.fn()}
        onClose={vi.fn()}
        {...overrides}
      />
    </VirtuosoMockContext.Provider>,
  )
}
