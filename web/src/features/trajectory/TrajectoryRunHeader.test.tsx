import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { toast } from 'sonner'
import i18next from '@/i18n'
import type { AgentRunTrace } from '@/lib/api'
import { TrajectoryRunHeader } from './TrajectoryRunHeader'

vi.mock('sonner', () => ({ toast: { success: vi.fn(), error: vi.fn() } }))

const trace: AgentRunTrace = {
  summary: { id: 'run-long-complete-identity', created_at: '', path: '', status: 'success', events: 0, context_parts: 0, parent_run_id: 'parent' },
  records: [],
  children: [
    { id: 'child-1', agent_name: 'Writer', created_at: '', path: '', status: 'success', events: 0, context_parts: 0, llm_calls: 3, tool_calls: 2 },
    { id: 'child-old', agent_name: 'Reader', created_at: '', path: '', status: 'unavailable', events: 0, context_parts: 0 },
  ],
}

describe('TrajectoryRunHeader', () => {
  const writeText = vi.fn()
  beforeEach(async () => {
    vi.clearAllMocks()
    writeText.mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } })
    await i18next.changeLanguage('en-US')
  })

  it('copies the exact ID and resource URI and reports clipboard failure', async () => {
    render(<TrajectoryRunHeader trace={trace} trajectoryURI="trajectory://projects/book/runs/run-long-complete-identity" onOpenRun={vi.fn()} />)
    fireEvent.click(screen.getByRole('button', { name: /^Copy Run ID$/ }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(trace.summary.id))
    fireEvent.click(screen.getByRole('button', { name: 'Copy trajectory reference' }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('trajectory://projects/book/runs/run-long-complete-identity'))
    writeText.mockRejectedValueOnce(new Error('clipboard denied'))
    fireEvent.click(screen.getByRole('button', { name: /^Copy Run ID$/ }))
    await waitFor(() => expect(toast.error).toHaveBeenCalled())
  })

  it('opens children and parent by exact Run identity and explains unavailable history', async () => {
    const open = vi.fn()
    render(<TrajectoryRunHeader trace={trace} trajectoryURI="trajectory:run" onOpenRun={open} />)
    expect(screen.queryByRole('button', { name: 'Open subagent: Writer' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Subagents (2)' }))
    fireEvent.click(screen.getByRole('button', { name: 'Open subagent: Writer' }))
    expect(open).toHaveBeenCalledWith('child-1')
    expect(screen.getByRole('button', { name: 'Open subagent: Reader' })).toBeDisabled()
    expect(screen.getByText(/Trace not captured/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Copy Run ID for Reader' }))
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('child-old'))
    fireEvent.click(screen.getByRole('button', { name: 'Back to parent Run' }))
    expect(open).toHaveBeenCalledWith('parent')
  })
})
