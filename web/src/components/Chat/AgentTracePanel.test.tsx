import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentTracePanel } from './AgentTracePanel'
import { downloadAgentRunTrace, exportAgentRunTrace, getAgentRunTrace, getAgentRunTraces } from '@/lib/api'
import { toast } from 'sonner'

vi.mock('@/lib/api', () => ({
  downloadAgentRunTrace: vi.fn(),
  exportAgentRunTrace: vi.fn(),
  getAgentRunTrace: vi.fn(),
  getAgentRunTraces: vi.fn(),
}))

vi.mock('sonner', () => ({
  toast: {
    error: vi.fn(),
    success: vi.fn(),
  },
}))

const runID = 'run-support-export'

describe('AgentTracePanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(getAgentRunTraces).mockResolvedValue([summaryFixture()])
    vi.mocked(getAgentRunTrace).mockResolvedValue(traceFixture())
    vi.mocked(exportAgentRunTrace).mockResolvedValue({
      filename: `${runID}.jsonl`,
      blob: new Blob(['trace export']),
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('copies the selected run ID', async () => {
    const user = userEvent.setup()
    render(<AgentTracePanel />)

    expect(await screen.findByTitle(runID)).toHaveTextContent(runID)
    await user.click(await screen.findByRole('button', { name: '复制运行 ID' }))

    expect(await screen.findByRole('button', { name: '已复制运行 ID' })).toBeInTheDocument()
  })

  it('exports the complete selected trace file', async () => {
    const user = userEvent.setup()
    render(<AgentTracePanel />)

    await user.click(await screen.findByRole('button', { name: '导出 Trace 文件' }))

    await waitFor(() => {
      expect(exportAgentRunTrace).toHaveBeenCalledWith(runID)
      expect(downloadAgentRunTrace).toHaveBeenCalledWith({
        filename: `${runID}.jsonl`,
        blob: expect.any(Blob),
      })
      expect(toast.success).toHaveBeenCalledWith(`已开始下载 ${runID}.jsonl`)
    })
  })
})

function summaryFixture() {
  return {
    id: runID,
    created_at: '2026-07-22T00:00:00Z',
    path: `.denova/runs/${runID}.jsonl`,
    status: 'success',
    events: 1,
    context_parts: 0,
  }
}

function traceFixture() {
  return {
    summary: summaryFixture(),
    records: [{
      type: 'run_finished',
      run_id: runID,
      created_at: '2026-07-22T00:00:01Z',
      data: { status: 'success' },
    }],
  }
}
