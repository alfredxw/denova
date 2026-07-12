import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { StateSchemaOverview } from './StateSchemaOverview'

const retryMock = vi.fn()
const skipMock = vi.fn()

vi.mock('../../api', () => ({
  retryInteractiveStateSchema: (...args: unknown[]) => retryMock(...args),
  skipInteractiveStateSchema: (...args: unknown[]) => skipMock(...args),
}))

describe('StateSchemaOverview', () => {
  beforeEach(() => {
    retryMock.mockReset().mockResolvedValue({ status: 'running' })
    skipMock.mockReset().mockResolvedValue({ status: 'skipped' })
  })

  it('shows the current revision, visible schema, adaptation changes, and warnings', () => {
    render(<StateSchemaOverview
      storyId="story-1"
      schema={{
        version: 3,
        revision: 2,
        system: {
          templates: [{ id: 'protagonist', name: '主角', fields: [
            { name: '危机压力', type: 'number', default: 1, visibility: 'visible' },
            { name: '幕后真相', type: 'string', visibility: 'hidden' },
          ] }],
          initial_actors: [{ id: 'protagonist', name: '林川', template_id: 'protagonist' }],
        },
      }}
      initialization={{
        mode: 'after_opening', status: 'ready', target_revision: 2,
        changes: [{ kind: 'field', op: 'add', template_id: 'protagonist', target_id: '危机压力', reason: '首轮出现追捕' }],
        warnings: ['旧压力值无法转换，已使用默认值'],
      }}
    />)

    expect(screen.getByText('rev 2')).toBeInTheDocument()
    expect(screen.getByText('危机压力')).toBeInTheDocument()
    expect(screen.queryByText('幕后真相')).not.toBeInTheDocument()
    expect(screen.getByText(/首轮出现追捕/)).toBeInTheDocument()
    expect(screen.getByText('旧压力值无法转换，已使用默认值')).toBeInTheDocument()
  })

  it('retries or locks the preset after a failed adaptation', async () => {
    const onRefresh = vi.fn()
    render(<StateSchemaOverview storyId="story-1" initialization={{ mode: 'after_opening', status: 'failed', error: '模型不可用' }} onRefresh={onRefresh} />)

    fireEvent.click(screen.getByRole('button', { name: '重试适配' }))
    await waitFor(() => expect(retryMock).toHaveBeenCalledWith('story-1'))
    expect(onRefresh).toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '固定使用当前预设' }))
    await waitFor(() => expect(skipMock).toHaveBeenCalledWith('story-1'))
  })
})
