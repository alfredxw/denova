import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { VirtuosoMockContext } from 'react-virtuoso'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import '@/test/msw/server'
import { fetchSettings, refreshSettings } from '@/features/settings/api'
import { usePersistedUserSettings } from '@/hooks/usePersistedUserSettings'
import { setConfiguredLocale } from '@/i18n'
import { createAgentToolMessage } from '@/lib/agent-ui-message'
import { AgentPanel, WRITING_COMPOSER_SETTING_DEFAULTS, type WritingComposerSettingsController } from './AgentPanel'

const useWritingSkillOptionsMock = vi.hoisted(() => vi.fn())
const useProjectChangeGroupsMock = vi.hoisted(() => vi.fn())
const updateUserSettings = vi.hoisted(() => vi.fn().mockResolvedValue(undefined))

vi.mock('@/features/settings/api', () => {
  return {
    fetchSettings: vi.fn().mockResolvedValue({
      effective: {
        ide_story_teller_id: 'classic',
        writing_skill_default: 'novel-lite',
      },
      user: {},
    }),
    fetchProjectSettings: vi.fn().mockResolvedValue({
      effective: {
        ide_story_teller_id: 'classic',
        writing_skill_default: 'novel-lite',
      },
      user: {},
      workspace: {},
    }),
    refreshSettings: vi.fn(),
    refreshProjectSettings: vi.fn(),
    createSettingsMergePatch: (_baseline: unknown, draft: unknown) => draft,
    patchSettings: (_layer: string, changes: unknown, revision?: string) => revision === undefined
      ? updateUserSettings(changes)
      : updateUserSettings(changes, revision),
  }
})

vi.mock('@/features/agent-approval/AgentApprovalProvider', () => ({
  useAgentApprovalMode: () => ({
    mode: 'write',
    initialized: true,
    saving: false,
    setMode: vi.fn().mockResolvedValue(true),
  }),
}))

vi.mock('@/features/agent-goal/use-conversation-goal', () => ({
  useConversationGoal: () => ({
    goal: null,
    loading: false,
    saving: false,
    set: vi.fn().mockResolvedValue(null),
    pause: vi.fn().mockResolvedValue(null),
    resume: vi.fn().mockResolvedValue(null),
    clear: vi.fn().mockResolvedValue(null),
    reload: vi.fn().mockResolvedValue(null),
  }),
}))

vi.mock('@/hooks/useSkillCommands', () => ({
  useSkillCommands: () => [],
}))

vi.mock('@/hooks/useWritingSkillOptions', () => ({
  DEFAULT_WRITING_SKILL: 'novel-lite',
  BUILTIN_WRITING_SKILLS: ['novel-lite', 'novel-standard'],
  resolveWritingSkillSelection: (configured: string, options: Array<{ name: string }>) =>
    options.length === 0 || options.some((option) => option.name === configured)
      ? configured || 'novel-lite'
      : options.find((option) => option.name === 'novel-lite')?.name || options[0].name,
  useWritingSkillOptions: useWritingSkillOptionsMock,
}))

vi.mock('@/features/changes/use-change-review', async (importOriginal) => ({
  ...await importOriginal<typeof import('@/features/changes/use-change-review')>(),
  useProjectChangeGroups: useProjectChangeGroupsMock,
}))

describe('AgentPanel', () => {
  beforeEach(() => {
    setConfiguredLocale('zh-CN')
    vi.mocked(fetchSettings).mockClear()
    vi.mocked(refreshSettings).mockReset().mockImplementation(() => fetchSettings())
    vi.mocked(updateUserSettings).mockClear()
    vi.mocked(updateUserSettings).mockImplementation(async (settings) => ({
      default: {},
      global: {},
      user: settings,
      workspace: {},
      effective: {
        ide_story_teller_id: 'classic',
        ide_image_preset_id: 'game-cg',
        writing_skill_default: 'novel-lite',
        ...settings,
      },
      resolved_agent_tool_manifests: {},
      resolved_agent_contexts: {},
      revisions: { user: 'r2' },
      paths: {
        denova_dir: '',
        nova_dir: '',
        user_config: '',
        workspace_config: '',
      },
    }))
    useWritingSkillOptionsMock.mockReset()
    useProjectChangeGroupsMock.mockReset()
    useProjectChangeGroupsMock.mockReturnValue({ data: [] })
    useWritingSkillOptionsMock.mockReturnValue([
      {
        name: 'novel-lite',
        description: 'Lite',
        scope: 'builtin',
        path: '/skills/novel-lite/SKILL.md',
        active: true,
        agent: 'ide',
      },
      {
        name: 'novel-standard',
        description: 'Standard',
        scope: 'builtin',
        path: '/skills/novel-standard/SKILL.md',
        active: true,
        agent: 'ide',
      },
      {
        name: 'slow-burn',
        description: '慢热写作',
        scope: 'workspace',
        path: '/book/.nova/skills/slow-burn/SKILL.md',
        active: true,
        agent: 'ide',
      },
    ])
  })

  afterEach(() => {
    setConfiguredLocale('zh-CN')
    vi.useRealTimers()
  })

  it('创作 Agent 顶部切换器不再展示 Review tab，并在输入选项中切换写作 Skill', async () => {
    const user = userEvent.setup()
    const { container } = renderAgentPanel()

    expect(useWritingSkillOptionsMock).toHaveBeenCalledWith('project-workspace', true)
    expect(useProjectChangeGroupsMock).toHaveBeenCalledWith('project-workspace', {
      sessionID: 'session-1',
    })
    expect(screen.getByRole('button', { name: '对话' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '会话' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '运行追踪' })).toBeInTheDocument()
    expect(container.querySelector('aside > div')).toHaveClass('h-9')
    expect(screen.queryByRole('button', { name: '关闭创作 Agent' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '输入动作' }))
    expect(screen.getByText('叙事')).toBeInTheDocument()
    expect(screen.getByText('稳健叙事')).toBeInTheDocument()
    expect(screen.getByText('写作 Skill')).toBeInTheDocument()
    expect(screen.getByText(/Lite/)).toBeInTheDocument()
    const imageGenerationOptions = screen.getByRole('menuitem', { name: '图像生成选项' })
    expect(screen.queryByText('Image Agent')).not.toBeInTheDocument()
    await user.hover(imageGenerationOptions)
    expect(await screen.findByRole('menuitem', { name: '语言模型' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: '图像模型' })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: '图像方案' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Review' })).not.toBeInTheDocument()
  })

  it('General Agent 读取自身 Project 的变更审阅范围', () => {
    renderAgentPanel({ agentKind: 'general' })

    expect(useProjectChangeGroupsMock).toHaveBeenCalledWith('project-workspace', {
      sessionID: 'session-1',
    })
  })

  it('后台 Project 读取自身 Skills 和变更审阅', () => {
    renderAgentPanel()

    expect(useWritingSkillOptionsMock).toHaveBeenCalledWith('project-workspace', true)
    expect(useProjectChangeGroupsMock).toHaveBeenCalledWith('project-workspace', {
      sessionID: 'session-1',
    })
  })

  it('隐藏的 Agent 面板不订阅变更审阅数据', () => {
    renderAgentPanel({ active: false })

    expect(useProjectChangeGroupsMock).toHaveBeenCalledWith('', {
      sessionID: 'session-1',
    })
  })

  it('Project 标识水合前不读取 Skills', () => {
    renderAgentPanel({ projectId: '' })

    expect(useWritingSkillOptionsMock).toHaveBeenCalledWith('', false)
  })

  it('写下一章快捷提示要求同轮同步作品状态且不依赖成章确认', async () => {
    const user = userEvent.setup()
    const handleSend = vi.fn()
    renderAgentPanel({ onSend: handleSend })

    await user.click(screen.getByRole('button', { name: '按细纲写下一章' }))

    expect(handleSend).toHaveBeenCalledWith(
      expect.stringContaining('在同一轮同步更新 setting/progress.md 与 setting/character-states.md'),
      expect.objectContaining({
        writingSkill: 'novel-lite',
        tellerId: 'classic',
      }),
    )
    expect(handleSend.mock.calls[0][0]).toContain('章节是否标记成章不影响同步')
    expect(handleSend.mock.calls[0][0]).not.toContain('由我在章节列表确认后再标记为成章')
  })

  it('续写下一段以实际最新非空章节为目标而非当前选中文件', async () => {
    const user = userEvent.setup()
    const handleSend = vi.fn()
    renderAgentPanel({
      currentChapter: {
        path: 'chapters/volume-1/ch00001.md',
        file_name: 'ch00001.md',
        display_title: '第一章 穿越与觉醒',
        index: 1,
        words: 3067,
        status: 'confirmed',
        confirmed: true,
        updated_at: '',
        volume: '第一卷',
        volume_path: 'chapters/volume-1',
      },
      onSend: handleSend,
    })

    await user.click(screen.getByRole('button', { name: '续写下一段' }))

    expect(handleSend).toHaveBeenCalledWith(
      expect.stringContaining('实际最新的非空章节'),
      expect.objectContaining({ writingSkill: 'novel-lite' }),
    )
    expect(handleSend.mock.calls[0][0]).toContain('不要以当前编辑器选中的文件')
    expect(handleSend.mock.calls[0][0]).toContain('不要创建新章节')
    expect(handleSend.mock.calls[0][0]).not.toContain('第一章 穿越与觉醒')
  })

  it('英文界面的快捷创作发送对应英文提示', async () => {
    const user = userEvent.setup()
    const handleSend = vi.fn()
    act(() => setConfiguredLocale('en-US'))
    renderAgentPanel({ onSend: handleSend })

    await user.click(screen.getByRole('button', { name: 'Continue Next Paragraph' }))

    expect(handleSend).toHaveBeenCalledWith(
      expect.stringContaining('Continue the actual latest non-empty chapter'),
      expect.objectContaining({ writingSkill: 'novel-lite' }),
    )
    expect(handleSend.mock.calls[0][0]).toContain('Do not create a new chapter')
    expect(handleSend.mock.calls[0][0]).not.toContain('续写实际最新的非空章节正文')
  })

  it('创作 Agent 将思考和工具调用折叠到同一个执行过程', async () => {
    const user = userEvent.setup()
    renderAgentPanel({
      messages: [
        {
          id: 'assistant-trace',
          role: 'assistant',
          parts: [
            { type: 'reasoning', text: '读取章节上下文' },
            {
              type: 'dynamic-tool',
              toolName: 'read',
              toolCallId: 'tool-1',
              state: 'output-available',
              input: { path: 'chapters/ch01.md' },
              output: 'ok',
            },
            { type: 'text', text: '已完成续写。' },
          ],
        },
      ],
    })

    expect(screen.getByRole('button', { name: /执行过程.*1 次工具调用/ })).toBeInTheDocument()
    expect(screen.queryByText('读取章节上下文')).not.toBeInTheDocument()
    expect(screen.queryByText('读取')).not.toBeInTheDocument()
    expect(screen.getByText('已完成续写。')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /执行过程.*1 次工具调用/ }))
    const thinkingButton = screen.getByRole('button', { name: '展开思考' })
    expect(thinkingButton).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByRole('region', { name: '思考内容' })).not.toBeInTheDocument()
    expect(screen.getByText('读取')).toBeInTheDocument()

    await user.click(thinkingButton)
    expect(screen.getByRole('region', { name: '思考内容' })).toHaveTextContent('读取章节上下文')
  })

  it('创作 Agent 运行中自动展开执行过程', () => {
    renderAgentPanel({
      isStreaming: true,
      isExecutionActive: true,
      messages: [
        {
          id: 'assistant-running-trace',
          role: 'assistant',
          parts: [
            {
              type: 'reasoning',
              text: '正在读取章节上下文',
              state: 'streaming',
            },
          ],
        },
      ],
    })

    expect(screen.getByRole('button', { name: /正在执行/ })).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByRole('region', { name: '思考内容' })).toHaveTextContent('正在读取章节上下文')
  })

  it('真实消息链路优先展示工具的原始输入流', () => {
    const inputText = '{"path":"chapters/ch01.md","edits":[{"old_string":"旧正文仍在流式生成'
    renderAgentPanel({
      isStreaming: true,
      isExecutionActive: true,
      messages: [createAgentToolMessage({
        id: 'tool-streaming-edit',
        name: 'edit',
        state: 'input-streaming',
        inputText,
        input: {
          path: 'chapters/ch01.md',
          edits: [{ old_string: '旧正文仍在流式生成' }],
        },
        metadata: { run_id: 'run-streaming-edit' },
      })],
    })

    expect(document.querySelector('[data-nova-scroll-lock="tool-input-stream"]')?.textContent).toBe(inputText)
  })

  it('变更审阅卡只在所属 Run 终止后挂载', () => {
    useProjectChangeGroupsMock.mockReturnValue({
      data: [{
        id: 'run-live-review',
        review_thread_id: 'run-live-review',
        run_id: 'run-live-review',
        session_id: 'session-1',
        created_at: '2026-08-13T00:00:00Z',
        review_status: 'pending',
        apply_state: 'applied',
        can_undo: true,
        change_set_count: 1,
        paths: ['draft.md'],
      }],
    })

    const activeView = renderAgentPanel({
      isStreaming: true,
      isExecutionActive: true,
      runtimeProjection: {
        active: true,
        phase: 'running',
        active_operation_id: 'run-live-review',
      },
      messages: [{
        id: 'assistant-live-review',
        role: 'assistant',
        parts: [{ type: 'text', text: '正在修改', state: 'streaming' }],
      }],
    })

    expect(screen.queryByText('draft.md')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '审阅' })).not.toBeInTheDocument()

    activeView.unmount()
    renderAgentPanel({
      isStreaming: false,
      isExecutionActive: false,
      runtimeProjection: {
        active: false,
        phase: 'idle',
        last_operation: {
          operation_id: 'operation-live-review',
          command_id: 'command-live-review',
          status: 'aborted',
        },
      },
      messages: [{
        id: 'assistant-live-review',
        role: 'assistant',
        metadata: { run_id: 'run-live-review' },
        parts: [{ type: 'text', text: '已中止修改' }],
      }],
    })

    expect(screen.getByText('draft.md')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '审阅' })).toBeEnabled()
  })

  it('空闲恢复探测不会展开已完成的执行过程', () => {
    renderAgentPanel({
      isStreaming: true,
      isExecutionActive: false,
      activityContent: '正在恢复...',
      messages: [
        {
          id: 'assistant-completed-trace',
          role: 'assistant',
          metadata: { run_id: 'run-completed' },
          parts: [
            { type: 'reasoning', text: '已完成的历史思考' },
            { type: 'text', text: '历史回复' },
          ],
        },
      ],
    })

    expect(screen.getByRole('button', { name: /执行过程/ })).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('已完成的历史思考')).not.toBeInTheDocument()
    expect(screen.getByText('历史回复')).toBeInTheDocument()
  })

  it('打开 SubAgent 详情时通知外层扩展右栏', async () => {
    const user = userEvent.setup()
    const handleDetailsChange = vi.fn()

    renderAgentPanel({
      messages: [
        {
          id: 'subagent-output-1',
          role: 'assistant',
          metadata: {
            agent_name: 'researcher',
            subagent: true,
            subagent_session_id: 'run-1-subagent-01-researcher',
          },
          parts: [{ type: 'text', text: '调研摘要' }],
        },
      ],
      onSubAgentDetailsChange: handleDetailsChange,
    })

    expect(handleDetailsChange).toHaveBeenLastCalledWith(false)
    await user.click(screen.getByRole('button', { name: /researcher 输出/ }))
    expect(handleDetailsChange).toHaveBeenLastCalledWith(true)
    expect(screen.getAllByText('researcher 子会话').length).toBeGreaterThan(0)
    expect(screen.getByRole('separator', { name: '调整 SubAgent 详情宽度' })).toBeInTheDocument()
    expect(screen.getAllByRole('button', { name: '输入动作' })).toHaveLength(1)

    await user.click(screen.getAllByRole('button', { name: '关闭 SubAgent 详情' })[0])
    expect(handleDetailsChange).toHaveBeenLastCalledWith(false)
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

    window.dispatchEvent(
      new CustomEvent('nova:writing-agent-init', {
        detail: {
          autoSend: true,
          prompt: '/chapter-illustration\n目标章节 / Target chapter: chapters/ch01.md',
        },
      }),
    )

    await waitFor(() => {
      expect(handleSend).toHaveBeenCalledWith(
        expect.stringContaining('/chapter-illustration'),
        expect.objectContaining({
          writingSkill: 'novel-lite',
          tellerId: 'classic',
        }),
      )
    })
  })

  it('在输入选项中切换叙事风格后用于下一轮创作 Agent 请求', async () => {
    const user = userEvent.setup()
    const handleSend = vi.fn()
    renderAgentPanel({
      tellers: [{ id: 'classic', name: '默认叙事', style_rules: [] } as any, { id: 'slow-burn', name: '慢热叙事', style_rules: [] } as any],
      onSend: handleSend,
    })

    await user.click(screen.getByRole('button', { name: '输入动作' }))
    await user.hover(screen.getByText('叙事'))
    const slowBurnItem = await screen.findByText('慢热叙事')
    fireEvent.click(slowBurnItem.closest('[role="menuitem"]') || slowBurnItem)

    window.dispatchEvent(
      new CustomEvent('nova:writing-agent-init', {
        detail: { autoSend: true, prompt: '继续写下一段' },
      }),
    )

    await waitFor(() => {
      expect(handleSend).toHaveBeenCalledWith(
        '继续写下一段',
        expect.objectContaining({
          tellerId: 'slow-burn',
          writingSkill: 'novel-lite',
        }),
      )
    })
  })

  it('关闭面板后由稳定 owner 完成仍在 afterDelay 中的偏好保存', async () => {
    const overrides: AgentPanelOverrides = {
      tellers: [{ id: 'classic', name: '默认叙事', style_rules: [] } as any, { id: 'slow-burn', name: '慢热叙事', style_rules: [] } as any],
    }
    function Owner({ open }: { open: boolean }) {
      const composerSettings = usePersistedUserSettings({
        workspace: '/workspace',
        defaults: WRITING_COMPOSER_SETTING_DEFAULTS,
      })
      return open ? <AgentPanel {...defaultAgentPanelProps(overrides, composerSettings)} /> : null
    }

    const view = render(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 52 }}>
        <Owner open />
      </VirtuosoMockContext.Provider>,
    )
    await waitFor(() => expect(screen.getByRole('button', { name: '输入动作' })).toBeEnabled())

    const user = userEvent.setup()
    await user.click(screen.getByRole('button', { name: '输入动作' }))
    await user.hover(screen.getByText('叙事'))
    const slowBurnItem = await screen.findByText('慢热叙事')
    vi.useFakeTimers()
    fireEvent.click(slowBurnItem.closest('[role="menuitem"]') || slowBurnItem)
    expect(updateUserSettings).not.toHaveBeenCalled()

    view.rerender(
      <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 52 }}>
        <Owner open={false} />
      </VirtuosoMockContext.Provider>,
    )
    await vi.advanceTimersByTimeAsync(1000)

    expect(updateUserSettings).toHaveBeenCalledWith(expect.objectContaining({ ide_story_teller_id: 'slow-burn' }))
  })

  it('发送开始时移走审阅意见，并在请求失败时恢复', async () => {
    const user = userEvent.setup()
    const handleSend = vi
      .fn()
      .mockImplementationOnce(async (_message, options) => {
        options?.onSubmissionStart?.()
        options?.onSubmissionError?.()
        return false
      })
      .mockImplementationOnce(async (_message, options) => {
        options?.onSubmissionStart?.()
        return true
      })
    const handleSubmitted = vi.fn()
    const handleSubmissionFailed = vi.fn()
    renderAgentPanel({
      onSend: handleSend,
      onReviewFeedbackSubmitted: handleSubmitted,
      onReviewFeedbackSubmissionFailed: handleSubmissionFailed,
      reviewFeedback: [
        {
          reviewThreadId: 'thread-1',
          comments: [{ id: 'comment-1', group_id: 'group-1', body: '把这里写得更克制' }],
        },
      ],
    })

    await user.click(screen.getByRole('button', { name: '发送' }))
    await waitFor(() => expect(handleSend).toHaveBeenCalledTimes(1))
    expect(handleSubmitted).toHaveBeenCalledTimes(1)
    expect(handleSubmissionFailed).toHaveBeenCalledTimes(1)

    await user.click(screen.getByRole('button', { name: '发送' }))
    await waitFor(() => expect(handleSubmitted).toHaveBeenCalledTimes(2))
    expect(handleSubmissionFailed).toHaveBeenCalledTimes(1)
  })

  it('同时提交批注与 Diff 审阅意见并保留各自来源', async () => {
    const user = userEvent.setup()
    const handleSend = vi.fn().mockResolvedValue(true)
    renderAgentPanel({
      onSend: handleSend,
      onReviewFeedbackRemove: vi.fn(),
      reviewFeedback: [
        {
          source: 'workspace_change',
          reviewThreadId: 'diff-thread',
          comments: [
            {
              id: 'diff-comment',
              body: '调整 Diff 里的转场',
              review_path: 'chapters/ch01.md',
            },
          ],
        },
        {
          source: 'document',
          reviewThreadId: 'document-thread',
          comments: [
            {
              id: 'document-comment',
              body: '正文这里需要更克制',
              path: 'chapters/ch02.md',
            },
          ],
        },
      ],
    })

    expect(screen.getByText('Diff · chapters/ch01.md — 调整 Diff 里的转场')).toBeInTheDocument()
    expect(screen.getByText('批注 · chapters/ch02.md — 正文这里需要更克制')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '发送' }))

    await waitFor(() =>
      expect(handleSend).toHaveBeenCalledWith(
        '请处理这 2 条审阅意见。',
        expect.objectContaining({
          reviewFeedback: [
            {
              source: 'workspace_change',
              reviewThreadId: 'diff-thread',
              commentIds: ['diff-comment'],
            },
            {
              source: 'document',
              reviewThreadId: 'document-thread',
              commentIds: ['document-comment'],
            },
          ],
        }),
      ),
    )
  })

  it('将批注引用点击交给工作台导航', async () => {
    const user = userEvent.setup()
    const handleOpen = vi.fn()
    const selection = {
      source: 'document' as const,
      reviewThreadId: 'document-thread',
      comments: [
        {
          id: 'document-comment',
          body: '正文这里需要更克制',
          path: 'chapters/ch02.md',
          review_line: 111,
        },
      ],
    }
    renderAgentPanel({
      onReviewFeedbackOpen: handleOpen,
      onReviewFeedbackRemove: vi.fn(),
      reviewFeedback: [selection],
    })

    await user.click(
      screen.getByRole('button', {
        name: /批注 · chapters\/ch02\.md · 第 111 行 — 正文这里需要更克制/,
      }),
    )

    expect(handleOpen).toHaveBeenCalledWith(selection, selection.comments[0])
  })

  it('在超过单次评论上限时保留反馈并阻止发送', async () => {
    const user = userEvent.setup()
    const handleSend = vi.fn().mockResolvedValue(true)
    renderAgentPanel({
      onSend: handleSend,
      onReviewFeedbackRemove: vi.fn(),
      reviewFeedback: [
        {
          reviewThreadId: 'thread-1',
          comments: Array.from({ length: 257 }, (_, index) => ({
            id: `comment-${index}`,
            group_id: 'group-1',
            body: `意见 ${index}`,
          })),
        },
      ],
    })

    expect(screen.getByRole('alert')).toHaveTextContent('一次最多提交 256 条审阅意见')
    await user.click(screen.getByRole('button', { name: '发送' }))
    expect(handleSend).not.toHaveBeenCalled()
  })

  it('冷恢复时只放开服务器投影的 Stop，仍禁止发送新指令', async () => {
    const user = userEvent.setup()
    const handleStop = vi.fn()
    renderAgentPanel({
      isStreaming: true,
      onStop: handleStop,
      runtimeProjection: {
        active: false,
        phase: 'running',
        recovery_paused: true,
        runtime_recoverable: true,
        stream_attached: false,
        active_operation_id: 'operation-recovery',
        recovery_actions: [
          {
            kind: 'abort',
            command_id: 'recovery-abort-1',
            operation_id: 'operation-recovery',
          },
        ],
      },
    })

    expect(screen.getByText('正在从持久化状态恢复已接受的 Agent 运行…')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /发送方式/ })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '发送' })).not.toBeInTheDocument()
    const stopButton = screen.getByRole('button', { name: '中断 AI 执行' })
    expect(stopButton).toBeEnabled()

    await user.click(stopButton)
    expect(handleStop).toHaveBeenCalledTimes(1)
  })

  it('冷恢复展示流接回后保持输入区可编辑，发送时默认进入队列', () => {
    renderAgentPanel({
      isStreaming: true,
      runtimeProjection: {
        active: true,
        phase: 'running',
        task_id: 'attach-task-1',
        recovery_paused: true,
        runtime_recoverable: true,
        stream_attached: true,
        active_operation_id: 'operation-recovery',
        recovery_actions: [
          {
            kind: 'abort',
            command_id: 'recovery-abort-1',
            operation_id: 'operation-recovery',
          },
        ],
      },
    })

    expect(screen.queryByRole('button', { name: /发送方式/ })).not.toBeInTheDocument()
    expect(screen.getByRole('textbox')).toHaveAttribute('contenteditable', 'true')
    expect(screen.getByRole('button', { name: '中断 AI 执行' })).toBeEnabled()
  })

  it('持久展示已接收请求的运行失败原因', () => {
    renderAgentPanel({
      messages: [
        {
          id: 'failed-user-message',
          role: 'user',
          parts: [{ type: 'text', text: '续写下一段' }],
        },
      ],
      runtimeProjection: {
        active: false,
        phase: 'idle',
        last_operation: {
          operation_id: 'failed-operation',
          command_id: 'failed-command',
          status: 'failed',
          reason: 'provider rejected the tool schema',
        },
      },
    })

    expect(screen.getByRole('alert')).toHaveTextContent('请求失败: provider rejected the tool schema')
  })

  it('已有多条指令排队时仍可继续发送下一条指令', async () => {
    const user = userEvent.setup()
    const handleSend = vi.fn().mockResolvedValue(true)
    renderAgentPanel({
      isStreaming: true,
      onSend: handleSend,
      runtimeProjection: {
        active: true,
        phase: 'running',
        stream_attached: true,
        active_operation_id: 'operation-queue',
        queue: [
          {
            command_id: 'queued-1',
            operation_id: 'operation-queue',
            delivery: 'follow_up',
            message: '第一条排队指令',
          },
          {
            command_id: 'queued-2',
            operation_id: 'operation-queue',
            delivery: 'follow_up',
            message: '第二条排队指令',
          },
        ],
      },
    })

    expect(screen.getByText('第一条排队指令')).toBeInTheDocument()
    expect(screen.getByText('第二条排队指令')).toBeInTheDocument()
    act(() => {
      window.dispatchEvent(
        new CustomEvent('nova:writing-agent-init', {
          detail: { prompt: 'Third queued instruction' },
        }),
      )
    })
    await waitFor(() => expect(screen.getByRole('textbox')).toHaveTextContent('Third queued instruction'))
    expect(screen.getByRole('button', { name: '发送' })).toBeEnabled()

    await user.click(screen.getByRole('button', { name: '发送' }))
    expect(handleSend).toHaveBeenCalledWith(
      'Third queued instruction',
      expect.objectContaining({
        writingSkill: 'novel-lite',
        tellerId: 'classic',
      }),
    )
  })
})

type AgentPanelOverrides = Partial<Omit<ComponentProps<typeof AgentPanel>, 'composerSettings'>>

function renderAgentPanel(overrides: AgentPanelOverrides = {}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  function Owner() {
    const composerSettings = usePersistedUserSettings({
      workspace: overrides.workspace || '/workspace',
      defaults: WRITING_COMPOSER_SETTING_DEFAULTS,
    })
    return <AgentPanel {...defaultAgentPanelProps(overrides, composerSettings)} />
  }
  return render(
    <QueryClientProvider client={queryClient}>
      <VirtuosoMockContext.Provider value={{ viewportHeight: 1200, itemHeight: 52 }}>
        <Owner />
      </VirtuosoMockContext.Provider>
    </QueryClientProvider>,
  )
}

function defaultAgentPanelProps(overrides: AgentPanelOverrides, composerSettings: WritingComposerSettingsController): ComponentProps<typeof AgentPanel> {
  return {
    workspace: '/workspace',
    composerSettings,
    selectedFile: null,
    tellers: [{ id: 'classic', name: '默认叙事', style_rules: [] } as any],
    messages: [],
    sessions: [
      {
        id: 'session-1',
        title: '当前会话',
        active: true,
        message_count: 0,
        created_at: '',
        updated_at: '',
      },
    ],
    activeSessionId: 'session-1',
    isStreaming: false,
    isExecutionActive: false,
    activityContent: '',
    hasEarlierMessages: false,
    isLoadingEarlierHistory: false,
    references: [],
    loreReferences: [],
    loreReferenceLabels: {},
    loreSuggestions: [],
    styleScenes: [],
    textSelections: [],
    planMode: false,
    fileSuggestions: [],
    onCreateSession: vi.fn(),
    onSwitchSession: vi.fn(),
    onRenameSession: vi.fn(),
    onDeleteSession: vi.fn(),
    onLoadEarlierHistory: vi.fn(),
    onRefreshHistory: vi.fn(),
    onSend: vi.fn(),
    onAnalyzeContext: vi.fn().mockResolvedValue({} as any),
    onStop: vi.fn(),
    onReferenceRemove: vi.fn(),
    onLoreReferenceAdd: vi.fn(),
    onLoreReferenceRemove: vi.fn(),
    onStyleSceneAdd: vi.fn(),
    onStyleSceneRemove: vi.fn(),
    onTextSelectionRemove: vi.fn(),
    onPlanModeChange: vi.fn(),
    onPlanModeToggle: vi.fn(),
    onApproveProposedPlan: vi.fn(),
    onExitPlanMode: vi.fn(),
    ...overrides,
    projectId: overrides.projectId ?? 'project-workspace',
  }
}
