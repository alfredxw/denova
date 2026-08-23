import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ChapterSummary } from '@/lib/api'
import { ChapterOutlineItem } from './ChapterOutlineItem'

describe('ChapterOutlineItem', () => {
  it('does not render an unchanged chapter row again when another chapter becomes active', () => {
    let titleReads = 0
    const chapter = {
      path: 'chapters/ch01.md',
      file_name: 'ch01.md',
      index: 1,
      words: 1200,
      status: 'draft',
      confirmed: false,
      updated_at: '2026-07-31T00:00:00Z',
      volume: '',
      volume_path: '',
    } as ChapterSummary
    Object.defineProperty(chapter, 'display_title', {
      configurable: true,
      get: () => {
        titleReads += 1
        return '第一章'
      },
    })
    const onSelectFile = vi.fn()
    const onSetChapterConfirmed = vi.fn()
    const onReferenceFile = vi.fn()
    const renderItem = (active: boolean) => (
      <ChapterOutlineItem
        chapter={chapter}
        active={active}
        onSelectFile={onSelectFile}
        onSetChapterConfirmed={onSetChapterConfirmed}
        onReferenceFile={onReferenceFile}
      />
    )

    const { rerender } = render(renderItem(false))
    const readsAfterFirstRender = titleReads

    rerender(renderItem(false))
    expect(titleReads).toBe(readsAfterFirstRender)

    rerender(renderItem(true))
    expect(titleReads).toBeGreaterThan(readsAfterFirstRender)

    fireEvent.contextMenu(screen.getByText('第一章'))
    expect(screen.getByRole('menu')).toHaveClass('nova-file-tree-menu-surface', 'nova-file-tree-menu-radix')
    expect(screen.getByRole('menuitem', { name: '引用到 Chat' })).toHaveClass('nova-file-tree-menu-item')
  })
})
