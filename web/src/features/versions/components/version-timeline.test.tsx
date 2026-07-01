import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { VersionTimeline, type VersionItem } from './version-timeline'

const version: VersionItem = {
  id: 'abcdef123456',
  title: '完善第一章',
  description: '手动',
  createdAt: '2026-05-17',
  author: '2 files',
  changedPaths: ['chapters/ch01.md'],
}

describe('VersionTimeline', () => {
  it('emits path restore actions from changed files', async () => {
    const user = userEvent.setup()
    const handleRestorePath = vi.fn()

    render(
      <VersionTimeline
        versions={[version]}
        onRestorePath={handleRestorePath}
      />,
    )

    await user.click(screen.getByRole('button', { name: '恢复此文件' }))

    expect(handleRestorePath).toHaveBeenCalledWith(version, 'chapters/ch01.md')
  })
})
