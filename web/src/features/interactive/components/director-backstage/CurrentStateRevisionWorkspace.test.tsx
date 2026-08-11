import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { setConfiguredLocale } from '@/i18n'
import type { Snapshot } from '../../types'
import { CurrentStateRevisionWorkspace } from './CurrentStateRevisionWorkspace'

vi.mock('../../api', () => ({
  createInteractiveStateRevision: vi.fn(),
  undoInteractiveStateRevision: vi.fn(),
  restoreInteractiveStateRevision: vi.fn(),
}))

function snapshotFixture(overrides: Partial<Snapshot> = {}): Snapshot {
  return {
    story_id: 'story-1',
    branch_id: 'main',
    head_id: 'head-1',
    turns: [],
    current_turn: { id: 'turn-1' } as Snapshot['current_turn'],
    state: {
      metadata: {
        tags: ['opening'],
      },
      published: true,
    },
    state_revisions: [{
      id: 'rev-1',
      type: 'state_revision',
      parent_id: 'turn-1',
      branch_id: 'main',
      ts: '2026-08-03T08:00:00Z',
      base_turn_id: 'turn-1',
      source: 'manual_state_editor',
      action: 'apply',
      ops: [{ op: 'set', path: 'count', value: 4 }],
    }],
    ...overrides,
  }
}

describe('CurrentStateRevisionWorkspace', () => {
  beforeEach(() => {
    setConfiguredLocale('zh-CN')
  })

  it('localizes invalid values and edits booleans with a select control', async () => {
    const user = userEvent.setup()
    render(<CurrentStateRevisionWorkspace storyId="story-1" branchId="main" snapshot={snapshotFixture()} onBack={vi.fn()} />)

    const tagsInput = screen.getByRole('textbox', { name: 'Metadata / Tags' })
    fireEvent.change(tagsInput, { target: { value: 'not json' } })
    await user.click(screen.getByRole('button', { name: '检查变更' }))

    expect(screen.getByRole('alert')).toHaveTextContent('无效值：Metadata / Tags')

    expect(screen.queryByRole('textbox', { name: 'Published' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('combobox', { name: 'Published' }))
    await user.click(screen.getByRole('option', { name: '否' }))
    expect(screen.getByRole('combobox', { name: 'Published' })).toHaveTextContent('否')
  })

  it('formats revision history dates with the active locale', async () => {
    setConfiguredLocale('en-US')
    const user = userEvent.setup()
    render(<CurrentStateRevisionWorkspace storyId="story-1" branchId="main" snapshot={snapshotFixture()} onBack={vi.fn()} />)

    await user.click(screen.getByRole('tab', { name: 'Revision history' }))

    expect(screen.getByText(/Aug 3, 2026/)).toBeInTheDocument()
  })

  it('treats Escape like the back control and keeps unsaved-change protection', async () => {
    const user = userEvent.setup()
    const onBack = vi.fn()
    render(<CurrentStateRevisionWorkspace storyId="story-1" branchId="main" snapshot={snapshotFixture()} onBack={onBack} />)

    const tagsInput = screen.getByRole('textbox', { name: 'Metadata / Tags' })
    fireEvent.change(tagsInput, { target: { value: '{"tags":["changed"]}' } })
    await user.keyboard('{Escape}')

    expect(screen.getByRole('alertdialog', { name: '放弃未保存的变更？' })).toBeInTheDocument()
    expect(onBack).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: '继续编辑' }))
    expect(screen.getByRole('textbox', { name: 'Metadata / Tags' })).toHaveValue('{"tags":["changed"]}')

    await user.keyboard('{Escape}')
    expect(screen.getByRole('alertdialog', { name: '放弃未保存的变更？' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '放弃变更' }))
    expect(onBack).toHaveBeenCalledTimes(1)
  })

  it('lets Escape close the discard dialog before triggering the workspace back', async () => {
    const user = userEvent.setup()
    const onBack = vi.fn()
    render(<CurrentStateRevisionWorkspace storyId="story-1" branchId="main" snapshot={snapshotFixture()} onBack={onBack} />)

    const tagsInput = screen.getByRole('textbox', { name: 'Metadata / Tags' })
    fireEvent.change(tagsInput, { target: { value: '{"tags":["changed"]}' } })
    await user.keyboard('{Escape}')
    expect(screen.getByRole('alertdialog', { name: '放弃未保存的变更？' })).toBeInTheDocument()

    await user.keyboard('{Escape}')

    expect(screen.queryByRole('alertdialog', { name: '放弃未保存的变更？' })).not.toBeInTheDocument()
    expect(onBack).not.toHaveBeenCalled()
    expect(screen.getByRole('heading', { name: '修订当前状态' })).toBeInTheDocument()
  })
})
