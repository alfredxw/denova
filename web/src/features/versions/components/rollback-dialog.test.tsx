import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { RollbackDialog } from './rollback-dialog'
import type { VersionItem } from './version-timeline'
import type { VersionRestorePlan } from '@/lib/api'

const version: VersionItem = {
  id: 'abcdef123456',
  title: '完善第一章',
  description: 'abcdef1',
  createdAt: '2026-05-17',
  author: 'tester',
  changedPaths: ['chapters/ch01.md'],
}

const plan: VersionRestorePlan = {
  target: {
    id: 'abcdef123456',
    message: '完善第一章',
    created_at: '2026-05-17T12:00:00Z',
    source: 'manual',
    file_count: 2,
    total_bytes: 120,
    changed_paths: ['chapters/ch01.md'],
  },
  scope: 'paths',
  paths: ['chapters/ch01.md'],
  changes: [
    {
      path: 'chapters/ch01.md',
      status: 'modified',
      text: true,
      binary: false,
    },
  ],
  will_create_backup: false,
  current_dirty: true,
}

describe('RollbackDialog', () => {
  it('calls onRollback after confirm', async () => {
    const user = userEvent.setup()
    const handleRollback = vi.fn()

    render(
      <RollbackDialog
        open
        version={version}
        onOpenChange={vi.fn()}
        onRollback={handleRollback}
      />,
    )

    await user.click(screen.getByRole('button', { name: '确认回滚' }))

    expect(handleRollback).toHaveBeenCalledWith(version)
  })

  it('shows restore plan details for path restore', () => {
    render(
      <RollbackDialog
        open
        version={version}
        plan={plan}
        onOpenChange={vi.fn()}
        onRollback={vi.fn()}
      />,
    )

    expect(screen.getByText('确认恢复文件？')).toBeInTheDocument()
    expect(screen.getByText('chapters/ch01.md')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '恢复文件' })).toBeInTheDocument()
  })
})
