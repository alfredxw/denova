import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import { getAgentRunTrace, getAgentRunTraces } from '@/lib/api'
import { setConfiguredLocale } from '@/i18n'
import { TrajectoryPage } from './TrajectoryPage'

const responsiveState = vi.hoisted(() => ({ compact: false }))

vi.mock('@/lib/api', () => ({
  downloadAgentRunTrace: vi.fn(),
  exportAgentRunTrace: vi.fn(),
  getAgentRunTrace: vi.fn(),
  getAgentRunTraces: vi.fn(),
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

  it('shows the project-scoped empty state', async () => {
    vi.mocked(getAgentRunTraces).mockResolvedValue([])

    renderPage()

    expect(await screen.findByText('还没有运行轨迹')).toBeInTheDocument()
    expect(getAgentRunTraces).toHaveBeenCalledWith('project-test', 100)
    expect(getAgentRunTrace).not.toHaveBeenCalled()
  })

  it('renders model-visible records, content inspectors, and the secondary call analysis', async () => {
    vi.mocked(getAgentRunTraces).mockResolvedValue([summaryFixture()])
    vi.mocked(getAgentRunTrace).mockResolvedValue(traceFixture())

    renderPage()

    expect(await screen.findByRole('region', { name: '模型可见记录' })).toBeInTheDocument()
    expect(screen.getAllByText('初始 System Prompt').length).toBeGreaterThan(0)
    expect(screen.getByRole('tab', { name: 'System Prompt' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: '工具' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '调用与事件' }))
    expect(await screen.findByRole('tree', { name: '调用树' })).toBeInTheDocument()
    expect(screen.getAllByText('writing').length).toBeGreaterThan(0)
    expect(screen.getAllByText('model-a').length).toBeGreaterThan(0)
    expect(screen.getAllByText('read_file').length).toBeGreaterThan(0)
    expect(screen.getByRole('button', { name: 'model-a, 3.00 s' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '压缩空闲' })).toBeInTheDocument()
    expect(screen.getAllByText(/TTFT 500 ms/).length).toBeGreaterThan(0)
    await waitFor(() => expect(getAgentRunTrace).toHaveBeenCalledWith('project-test', 'run-page-test'))
  })

  it('opens the inspector on demand instead of covering the compact ledger initially', async () => {
    responsiveState.compact = true
    vi.mocked(getAgentRunTraces).mockResolvedValue([summaryFixture()])
    vi.mocked(getAgentRunTrace).mockResolvedValue(traceFixture())

    renderPage()

    expect(await screen.findByRole('region', { name: '模型可见记录' })).toBeInTheDocument()
    expect(screen.queryByRole('complementary', { name: '记录检查器' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '助手 · 请求 #1' }))
    expect(screen.getByRole('complementary', { name: '记录检查器' })).toBeInTheDocument()
  })
})

function renderPage() {
  return render(
    <TooltipProvider delayDuration={0}>
      <TrajectoryPage target={{ kind: 'project', projectId: 'project-test' }} />
    </TooltipProvider>,
  )
}

function summaryFixture() {
  return {
    id: 'run-page-test',
    created_at: '2026-08-13T10:00:00.000Z',
    path: '.denova/runs/run-page-test.jsonl',
    status: 'success',
    events: 3,
    context_parts: 1,
    llm_calls: 1,
    tool_calls: 1,
    duration_ms: 4_000,
    agent_kind: 'writing',
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
          content: { status: 'success', message: { role: 'assistant', content: '## Summary\n\nDone.', reasoning_content: 'I should be concise.' } },
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
          attrs: { tool_name: 'read_file' },
        },
      },
    ],
  }
}
