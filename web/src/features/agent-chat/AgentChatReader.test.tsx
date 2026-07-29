import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useEffect, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentChatReader } from './AgentChatReader'

const mocks = vi.hoisted(() => ({
  flush: vi.fn(async () => true),
  readFile: vi.fn(),
  editorMounts: 0,
  editorUnmounts: 0,
}))

vi.mock('@/lib/api', () => ({ readFile: mocks.readFile }))

vi.mock('@/components/layout/adaptive-surface', () => ({
  AdaptiveSurface: ({ left, children }: {
    left: { content: ReactNode }
    children: ReactNode | ((controls: { isMobile: boolean; openLeft: () => void }) => ReactNode)
  }) => (
    <div>
      <aside>{left.content}</aside>
      <main>{typeof children === 'function' ? children({ isMobile: false, openLeft: vi.fn() }) : children}</main>
    </div>
  ),
}))

vi.mock('@/components/workbench/outline/ChapterOutline', () => ({
  ChapterOutline: ({ selectedFile, onSelectFile, onOpenLoreTab }: {
    selectedFile: string | null
    onSelectFile: (path: string) => void
    onOpenLoreTab?: () => void
  }) => (
    <nav aria-label="shared writing outline">
      <span data-testid="outline-selection">{selectedFile}</span>
      <button type="button" onClick={() => onSelectFile('chapters/ch02.md')}>open chapter two</button>
      <button type="button" onClick={onOpenLoreTab}>open shared lore</button>
    </nav>
  ),
}))

vi.mock('@/components/Editor/MarkdownEditor', () => ({
  MarkdownEditor: ({
    fileName,
    documentReview,
    documentReviewNavigationIntent,
    onFlushHandlerChange,
  }: {
    fileName: string
    documentReview: { comments: Array<{ id: string }> }
    documentReviewNavigationIntent?: { commentID: string } | null
    onFlushHandlerChange?: (handler: (() => Promise<boolean>) | null) => void
  }) => {
    useEffect(() => {
      mocks.editorMounts += 1
      onFlushHandlerChange?.(mocks.flush)
      return () => {
        mocks.editorUnmounts += 1
        onFlushHandlerChange?.(null)
      }
    }, [onFlushHandlerChange])
    return (
      <div data-testid="shared-markdown-editor">
        {fileName}|{documentReview.comments.length}|{documentReviewNavigationIntent?.commentID || 'none'}
      </div>
    )
  },
}))

const chapters = [
  {
    path: 'chapters/ch01.md', file_name: 'ch01.md', display_title: '第一章', index: 1,
    words: 100, status: 'draft', confirmed: false, updated_at: '', volume: '第一卷', volume_path: 'chapters/volume-01',
  },
  {
    path: 'chapters/ch02.md', file_name: 'ch02.md', display_title: '第二章', index: 2,
    words: 120, status: 'draft', confirmed: false, updated_at: '', volume: '第一卷', volume_path: 'chapters/volume-01',
  },
]

describe('AgentChatReader shared writing surface', () => {
  beforeEach(() => {
    mocks.flush.mockReset().mockResolvedValue(true)
    mocks.readFile.mockReset().mockImplementation(async (path: string) => ({ content: `content:${path}`, revision: `revision:${path}` }))
    mocks.editorMounts = 0
    mocks.editorUnmounts = 0
  })

  it('uses the writing outline and editor, flushing before local file navigation', async () => {
    const user = userEvent.setup()
    const onOpenLoreTab = vi.fn()
    render(
      <AgentChatReader
        workspace="/books/a"
        tree={[]}
        summary={{ title: 'A', author: '', chapter_count: 2, total_words: 220, chapters, chapter_plans: [] }}
        initialPath="chapters/ch01.md"
        onSaveFile={vi.fn(async () => ({ revision: 'saved' }))}
        documentReview={{ comments: [{ id: 'comment-1' }] } as never}
        onOpenLoreTab={onOpenLoreTab}
        onSetChapterConfirmed={vi.fn()}
        onFlushHandlerChange={vi.fn()}
      />,
    )

    await waitFor(() => expect(screen.getByTestId('shared-markdown-editor')).toHaveTextContent('chapters/ch01.md|1|none'))
    expect(screen.getByRole('navigation', { name: 'shared writing outline' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'open chapter two' }))
    await waitFor(() => expect(mocks.flush).toHaveBeenCalledTimes(1))
    await waitFor(() => expect(screen.getByTestId('shared-markdown-editor')).toHaveTextContent('chapters/ch02.md|1|none'))
    expect(mocks.editorMounts).toBe(1)
    expect(mocks.editorUnmounts).toBe(0)

    await user.click(screen.getByRole('button', { name: 'open shared lore' }))
    expect(onOpenLoreTab).toHaveBeenCalledTimes(1)
  })

  it('opens and reveals a workspace-file comment inside the shared editor', async () => {
    const { rerender } = render(
      <AgentChatReader
        workspace="/books/a"
        tree={[]}
        summary={{ title: 'A', author: '', chapter_count: 2, total_words: 220, chapters, chapter_plans: [] }}
        initialPath="chapters/ch01.md"
        onSaveFile={vi.fn(async () => ({ revision: 'saved' }))}
        documentReview={{ comments: [] } as never}
        navigationIntent={null}
        onSetChapterConfirmed={vi.fn()}
        onFlushHandlerChange={vi.fn()}
      />,
    )

    await screen.findByTestId('shared-markdown-editor')
    rerender(
      <AgentChatReader
        workspace="/books/a"
        tree={[]}
        summary={{ title: 'A', author: '', chapter_count: 2, total_words: 220, chapters, chapter_plans: [] }}
        initialPath="chapters/ch01.md"
        onSaveFile={vi.fn(async () => ({ revision: 'saved' }))}
        documentReview={{ comments: [] } as never}
        navigationIntent={{
          workspace: '/books/a',
          target: { kind: 'workspace_file', id: 'chapters/ch02.md' },
          commentID: 'comment-2',
          nonce: 1,
        }}
        onSetChapterConfirmed={vi.fn()}
        onFlushHandlerChange={vi.fn()}
      />,
    )

    await waitFor(() => expect(screen.getByTestId('shared-markdown-editor')).toHaveTextContent('chapters/ch02.md|0|comment-2'))
  })
})
