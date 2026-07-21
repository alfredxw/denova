import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { WorkbenchNoticePill } from './WorkbenchNoticePill'

describe('WorkbenchNoticePill', () => {
  it('opens update settings and keeps the existing update copy', () => {
    const onOpenSettings = vi.fn()
    const onDismiss = vi.fn()

    render(
      <WorkbenchNoticePill
        notice={{ kind: 'update', latestVersion: 'v0.4.0' }}
        expanded
        onOpenSettings={onOpenSettings}
        onDismiss={onDismiss}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '发现新版本 v0.4.0' }))
    expect(onOpenSettings).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: '关闭更新提示' }))
    expect(onDismiss).toHaveBeenCalledTimes(1)
  })

  it('shows the bilingual Star prompt surface with a safe external link', () => {
    const onDismiss = vi.fn()

    render(
      <WorkbenchNoticePill
        notice={{ kind: 'star' }}
        expanded
        starSecondaryText="description"
        onOpenSettings={vi.fn()}
        onDismiss={onDismiss}
      />,
    )

    expect(screen.getByText('喜欢 Denova？')).toBeInTheDocument()
    expect(screen.getByText('如果 Denova 已经帮到你，在 GitHub 点个 Star 可以帮助更多创作者发现它。')).toBeInTheDocument()
    const link = screen.getByRole('link', { name: '点个 Star' })
    expect(link).toHaveAttribute('href', 'https://github.com/alfredxw/denova')
    expect(link).toHaveAttribute('target', '_blank')

    fireEvent.click(link)
    expect(onDismiss).toHaveBeenCalledTimes(1)
  })
})
