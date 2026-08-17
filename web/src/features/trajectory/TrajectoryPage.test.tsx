import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { getAgentRunTrace, getGlobalAgentRunTraces } from '@/lib/api'
import { setConfiguredLocale } from '@/i18n'
import { TrajectoryPage } from './TrajectoryPage'

const responsiveState = vi.hoisted(() => ({ compact: false }))
const toast = vi.hoisted(() => ({ warning: vi.fn(), success: vi.fn(), error: vi.fn() }))

vi.mock('sonner', () => ({ toast }))

vi.mock('@/lib/api', () => ({
  downloadAgentRunTrace: vi.fn(),
  exportAgentRunTrace: vi.fn(),
  getAgentRunTrace: vi.fn(),
  getGlobalAgentRunTraces: vi.fn(),
}))

vi.mock('./HarnessWorkspace', () => ({
  HarnessWorkspace: ({ runs, onToggleEvidence, onViewRun }: {
    runs: Array<{ trajectory_uri: string }>
    onToggleEvidence: (uri: string) => void
    onViewRun: (uri: string) => void
  }) => (
    <div>
      <span>Harness workspace</span>
      <button type="button" disabled={!runs[0]} onClick={() => onToggleEvidence(runs[0].trajectory_uri)}>Select evidence</button>
      <button type="button" onClick={() => onViewRun('trajectory://projects/project-1/runs/run-page-test')}>View selected trajectory</button>
    </div>
  ),
}))

vi.mock('@/hooks/useIsMobile', () => ({
  useIsMobile: () => responsiveState.compact,
}))

describe('TrajectoryPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    setConfiguredLocale('zh-CN')
    responsiveState.compact = false
  })

  it('shows the global empty state', async () => {
    vi.mocked(getGlobalAgentRunTraces).mockResolvedValue({ runs: [], issues: [] })

    renderPage()

    expect(await screen.findByText('还没有运行轨迹')).toBeInTheDocument()
    expect(getGlobalAgentRunTraces).toHaveBeenCalledWith(100)
    expect(getAgentRunTrace).not.toHaveBeenCalled()
  })

  it('uses compact inline details by default and can switch to the resizable side inspector', async () => {
    const user = userEvent.setup()
    vi.mocked(getGlobalAgentRunTraces).mockResolvedValue({ runs: [summaryFixture()], issues: [] })
    vi.mocked(getAgentRunTrace).mockResolvedValue(traceFixture())

    renderPage()

    expect(await screen.findByRole('region', { name: '模型可见记录' })).toBeInTheDocument()
    expect(screen.getAllByText('初始 System Prompt').length).toBeGreaterThan(0)
    expect(screen.getByText('1 次调用 · 1 个结果')).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: '时间总览' })).not.toBeInTheDocument()
    const systemRow = screen.getByRole('button', { name: /系统.*初始 System Prompt/ })
    fireEvent.click(systemRow)
    expect(systemRow).toHaveAttribute('aria-current', 'true')
    expect(systemRow.className).not.toContain('ring-')
    expect(systemRow.className).not.toContain('border-strong')
    expect(screen.getByRole('region', { name: '记录检查器' })).toBeInTheDocument()
    expect(screen.queryByRole('complementary', { name: '记录检查器' })).not.toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'System Prompt' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '工具' })).toBeInTheDocument()
    expect(screen.getAllByText('字符数').every((label) => label.closest('dl')?.dataset.layout === 'wrap')).toBe(true)
    fireEvent.click(screen.getByRole('button', { name: '调试' }))
    fireEvent.click(screen.getByRole('button', { name: '#1 · system' }))
    expect(screen.getByRole('region', { name: '记录检查器' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'System Prompt' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '易读' }))
    fireEvent.click(screen.getByRole('button', { name: /工具调用.*read_file/ }))
    expect(screen.getByRole('tab', { name: '摘要' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '参数' })).toBeInTheDocument()
    await user.click(screen.getByRole('tab', { name: '响应' }))
    expect((await screen.findAllByText('Chapter contents')).length).toBeGreaterThan(0)
    fireEvent.click(screen.getByRole('button', { name: /助手.*请求 #1/ }))
    expect(screen.getByRole('tab', { name: '预览' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '关闭记录检查器' }))
    expect(screen.queryByRole('region', { name: '记录检查器' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: '侧栏' }))
    fireEvent.click(screen.getByRole('button', { name: /系统.*初始 System Prompt/ }))
    expect(screen.getByRole('complementary', { name: '记录检查器' })).toBeInTheDocument()
    expect(screen.getByRole('separator', { name: '调整记录检查器宽度' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '关闭记录检查器' }))
    expect(screen.queryByRole('complementary', { name: '记录检查器' })).not.toBeInTheDocument()

    const requestRow = screen.getByRole('button', { name: '请求 #1' })
    fireEvent.click(requestRow)
    expect(requestRow).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(requestRow)
    expect(requestRow).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(screen.getByRole('tab', { name: '时间线' }))
    expect(screen.getByRole('region', { name: '时间总览' })).toBeInTheDocument()
    expect(await screen.findByRole('tree', { name: '调用树' })).toBeInTheDocument()
    expect(screen.getAllByText('writing').length).toBeGreaterThan(0)
    expect(screen.getAllByText('model-a').length).toBeGreaterThan(0)
    expect(screen.getAllByText('read_file').length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: 'model-a, 3.00 s' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '压缩空闲' })).toBeInTheDocument()
    expect(screen.getAllByText(/TTFT 500 ms/).length).toBeGreaterThan(0)
    await waitFor(() => expect(getAgentRunTrace).toHaveBeenCalledWith('project-1', 'run-page-test'))
  })

  it('keeps inline details on compact screens and opens a drawer only in side mode', async () => {
    responsiveState.compact = true
    vi.mocked(getGlobalAgentRunTraces).mockResolvedValue({ runs: [summaryFixture()], issues: [] })
    vi.mocked(getAgentRunTrace).mockResolvedValue(traceFixture())

    renderPage()

    expect(await screen.findByRole('region', { name: '模型可见记录' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /助手.*请求 #1/ }))
    expect(screen.getByRole('region', { name: '记录检查器' })).toBeInTheDocument()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /助手.*请求 #1/ }))
    expect(screen.queryByRole('region', { name: '记录检查器' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '侧栏' }))
    fireEvent.click(screen.getByRole('button', { name: /助手.*请求 #1/ }))
    expect(screen.getByRole('dialog')).toBeInTheDocument()
    expect(screen.getByRole('complementary', { name: '记录检查器' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '预览' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '关闭' }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
  })

  it('keeps Harness beside Trajectory and returns to the exact global Run', async () => {
    const user = userEvent.setup()
    vi.mocked(getGlobalAgentRunTraces).mockResolvedValue({ runs: [summaryFixture()], issues: [] })
    vi.mocked(getAgentRunTrace).mockResolvedValue(traceFixture())

    renderPage()

    const trajectoryTab = await screen.findByRole('tab', { name: '轨迹' })
    const workspaceTabs = trajectoryTab.closest('[role="tablist"]')
    const pageTitle = screen.getByRole('heading', { name: '轨迹' })
    expect(workspaceTabs).not.toBeNull()
    expect(workspaceTabs!.compareDocumentPosition(pageTitle) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()

    await user.click(await screen.findByRole('tab', { name: 'Harness' }))
    expect(screen.getByText('Harness workspace')).toBeVisible()
    await user.click(screen.getByRole('button', { name: 'View selected trajectory' }))
    expect(screen.getByRole('tab', { name: '轨迹' })).toHaveAttribute('data-state', 'active')
    await waitFor(() => expect(getAgentRunTrace).toHaveBeenCalledWith('project-1', 'run-page-test'))
  })

  it('groups Runs by Project and Session while retaining the flat Run view', async () => {
    const user = userEvent.setup()
    const sharedSession = 's-session-shared'
    const sessionTitle = 'A deliberately long Session title that must stay on one line'
    const firstRun = { ...summaryFixture(), session_id: sharedSession, session_title: sessionTitle }
    const olderRun = {
      ...summaryFixture(),
      id: 'run-older',
      trajectory_uri: 'trajectory://projects/project-1/runs/run-older',
      session_id: sharedSession,
      session_title: sessionTitle,
      created_at: '2026-08-13T09:00:00.000Z',
    }
    const otherProjectRun = {
      ...summaryFixture(),
      id: 'run-other-project',
      project_id: 'project-2',
      project_name: 'Second Book',
      trajectory_uri: 'trajectory://projects/project-2/runs/run-other-project',
      session_id: sharedSession,
      session_title: 'Other Project Session',
      created_at: '2026-08-13T08:00:00.000Z',
    }
    vi.mocked(getGlobalAgentRunTraces).mockResolvedValue({ runs: [firstRun, olderRun, otherProjectRun], issues: [] })
    vi.mocked(getAgentRunTrace).mockResolvedValue(traceFixture())

    renderPage()

    const firstSession = await screen.findByRole('button', { name: `First Book · ${sessionTitle} · s-session-shared · 2 个 Run` })
    const otherProjectSession = screen.getByRole('button', { name: 'Second Book · Other Project Session · s-session-shared · 1 个 Run' })
    await waitFor(() => expect(firstSession).toHaveAttribute('aria-expanded', 'true'))
    expect(otherProjectSession).toHaveAttribute('aria-expanded', 'false')
    const visibleSessionTitle = within(firstSession).getByText(sessionTitle)
    expect(visibleSessionTitle).toHaveClass('truncate')
    expect(visibleSessionTitle).toHaveAttribute('title', sessionTitle)
    expect(within(firstSession.parentElement!).getByRole('button', { name: /run-older/ })).toBeInTheDocument()

    await user.click(otherProjectSession)
    expect(within(otherProjectSession.parentElement!).getByRole('button', { name: /run-other-project/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /^Run$/ }))
    expect(screen.queryByRole('button', { name: `First Book · ${sessionTitle} · s-session-shared · 2 个 Run` })).not.toBeInTheDocument()
    expect(screen.getAllByText('First Book')).toHaveLength(2)
    expect(screen.getByRole('button', { name: '会话' })).toHaveAttribute('aria-pressed', 'false')
  })

  it('shows partial Project failures without blocking healthy Runs', async () => {
    vi.mocked(getGlobalAgentRunTraces).mockResolvedValue({
      runs: [summaryFixture()],
      issues: [{ project_id: 'broken', project_name: 'Broken Project', message: 'unavailable' }],
    })
    vi.mocked(getAgentRunTrace).mockResolvedValue(traceFixture())

    renderPage()

    expect(await screen.findByText('1 个项目的 Run 暂时无法读取，其余结果仍可使用。')).toBeInTheDocument()
    expect(screen.getByText('First Book')).toBeInTheDocument()
  })

  it('warns when refresh removes disappeared evidence', async () => {
    const user = userEvent.setup()
    vi.mocked(getGlobalAgentRunTraces)
      .mockResolvedValueOnce({ runs: [summaryFixture()], issues: [] })
      .mockResolvedValueOnce({ runs: [], issues: [] })
    vi.mocked(getAgentRunTrace).mockResolvedValue(traceFixture())

    renderPage()

    await user.click(await screen.findByRole('tab', { name: 'Harness' }))
    await user.click(screen.getByRole('button', { name: 'Select evidence' }))
    await user.click(screen.getByRole('button', { name: '刷新轨迹' }))

    await waitFor(() => expect(toast.warning).toHaveBeenCalledWith('刷新后有 1 条已选 Run 消失，已从证据范围中移除。'))
  })
})

function renderPage() {
  return render(
    <TooltipProvider delayDuration={0}>
      <TrajectoryPage />
    </TooltipProvider>,
  )
}

function summaryFixture() {
  return {
    id: 'run-page-test',
    project_id: 'project-1',
    project_name: 'First Book',
    trajectory_uri: 'trajectory://projects/project-1/runs/run-page-test',
    created_at: '2026-08-13T10:00:00.000Z',
    path: '.denova/runs/run-page-test.jsonl',
    status: 'success',
    events: 3,
    context_parts: 1,
    llm_calls: 1,
    tool_calls: 1,
    duration_ms: 4_000,
    agent_kind: 'writing',
    session_id: 's-page-test',
    content_captured: true,
  }
}

function traceFixture() {
  return {
    summary: summaryFixture(),
    records: [
      {
        type: 'llm_input',
        run_id: 'run-page-test',
        created_at: '2026-08-13T10:00:00.000Z',
        data: {
          span_id: 'model', call_id: 'llm-1',
          content: {
            source: 'agent model boundary', purpose: 'developer trajectory inspection',
            messages: [
              { role: 'system', content: 'You are a writing assistant.' },
              { role: 'user', content: 'Summarize this chapter.' },
            ],
            tools: [{ name: 'read_file', description: 'Read a project file', parameters: { type: 'object' } }],
          },
        },
      },
      {
        type: 'llm_output',
        run_id: 'run-page-test',
        created_at: '2026-08-13T10:00:03.000Z',
        data: {
          span_id: 'model', call_id: 'llm-1',
          content: {
            status: 'success',
            message: {
              role: 'assistant', content: '## Summary\n\nDone.', reasoning_content: 'I should be concise.',
              tool_calls: [{ id: 'call-read', type: 'function', function: { name: 'read_file', arguments: '{"path":"chapter.md"}' } }],
            },
          },
        },
      },
      {
        type: 'tool_output',
        run_id: 'run-page-test',
        created_at: '2026-08-13T10:00:02.100Z',
        data: {
          span_id: 'tool', call_id: 'call-read',
          content: {
            source: 'agent tool boundary', purpose: 'developer trajectory inspection', tool_name: 'read_file',
            provider_call_id: 'call-read', execution_id: 'tool-execution', status: 'success', result: 'Chapter contents',
            original_bytes: 16, returned_bytes: 16, truncated: false,
          },
        },
      },
      {
        type: 'agent_run',
        run_id: 'run-page-test',
        created_at: '2026-08-13T10:00:04.000Z',
        data: {
          span_id: 'run', parent_span_id: '', status: 'success', duration_ms: 4_000,
          started_at: '2026-08-13T10:00:00.000Z', ended_at: '2026-08-13T10:00:04.000Z',
          attrs: { agent_kind: 'writing' },
        },
      },
      {
        type: 'llm_call',
        run_id: 'run-page-test',
        created_at: '2026-08-13T10:00:03.000Z',
        data: {
          span_id: 'model', parent_span_id: 'run', status: 'success', duration_ms: 3_000,
          started_at: '2026-08-13T10:00:00.000Z', ended_at: '2026-08-13T10:00:03.000Z',
          attrs: { model: 'model-a', ttft_ms: 500, prompt_tokens: 100, cached_prompt_tokens: 60, completion_tokens: 20 },
        },
      },
      {
        type: 'tool_call',
        run_id: 'run-page-test',
        created_at: '2026-08-13T10:00:02.000Z',
        data: {
          span_id: 'tool', parent_span_id: 'model', status: 'success', duration_ms: 1_000,
          started_at: '2026-08-13T10:00:01.000Z', ended_at: '2026-08-13T10:00:02.000Z',
          attrs: { tool_name: 'read_file', provider_call_id: 'call-read', execution_id: 'tool-execution' },
        },
      },
    ],
  }
}
