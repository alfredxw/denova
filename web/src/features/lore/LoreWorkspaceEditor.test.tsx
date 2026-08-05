import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { DocumentReviewController } from '@/features/document-review/controller'
import type { CreateDocumentCommentRequest, DocumentReviewComment } from '@/features/document-review/types'
import type { LoreItem } from '@/lib/api'
import { LoreWorkspaceEditor } from './LoreWorkspaceEditor'

vi.mock('@/components/Editor/MarkdownRichEditor', () => ({
  MarkdownEditor: (props: {
    value: string
    mode: 'rich' | 'source'
    highlightQuery?: string
    onChange: (value: string) => void
    review?: { target: { id: string } }
    'aria-label'?: string
  }) => (
    <div>
      <textarea
        aria-label={props['aria-label']}
        data-content-mode={props.mode}
        data-highlight-query={props.highlightQuery ?? ''}
        data-review-target={props.review?.target.id ?? ''}
        value={props.value}
        onChange={(event) => props.onChange(event.target.value)}
      />
      {props.highlightQuery && props.value.includes(props.highlightQuery)
        ? <mark>{props.highlightQuery}</mark>
        : null}
    </div>
  ),
}))

describe('LoreWorkspaceEditor', () => {
  it('switches between rich text and editable raw lore', async () => {
    const user = userEvent.setup()
    const onDraftChange = vi.fn()

    render(
      <LoreWorkspaceEditor
        projectId="project-lore-test"
        draft={loreItem()}
        tagDraft=""
        autosaveStatus="saved"
        autosaveError={null}
        documentReview={reviewController()}
        highlightQuery="body"
        onDraftChange={onDraftChange}
        onTagDraftChange={vi.fn()}
        onPrepareSnapshot={vi.fn().mockResolvedValue({
          content: 'Original body',
          revision: 'r1',
        })}
        onFlush={vi.fn().mockResolvedValue(true)}
        onDelete={vi.fn().mockResolvedValue(true)}
      />,
    )

    expect(screen.getByRole('textbox', { name: '编辑设定：Original name' }))
      .toHaveValue('Original body')
    expect(screen.getByRole('textbox', { name: '编辑设定：Original name' }))
      .toHaveAttribute('data-highlight-query', 'body')

    await user.click(screen.getByRole('button', { name: 'Raw' }))

    const rawEditor = screen.getByRole('textbox', {
      name: '编辑 Raw 设定：Original name',
    })
    expect(rawEditor).toHaveValue('Original body')
    expect(rawEditor.parentElement?.querySelector('mark')).toHaveTextContent('body')
    expect(screen.getByRole('button', { name: 'Raw' })).toHaveAttribute(
      'aria-pressed',
      'true',
    )

    fireEvent.change(rawEditor, { target: { value: '# Raw body' } })
    expect(onDraftChange).toHaveBeenLastCalledWith({
      ...loreItem(),
      content: '# Raw body',
    })

    await user.click(screen.getByRole('button', { name: '富文本' }))
    expect(screen.getByRole('textbox', { name: '编辑设定：Original name' }))
      .toBeInTheDocument()
  })

  it('keeps the document review controller attached in raw mode', async () => {
    const user = userEvent.setup()
    const documentReview = reviewController()

    render(
      <LoreWorkspaceEditor
        projectId="project-lore-test"
        draft={loreItem()}
        tagDraft=""
        autosaveStatus="saved"
        autosaveError={null}
        documentReview={documentReview}
        onDraftChange={vi.fn()}
        onTagDraftChange={vi.fn()}
        onPrepareSnapshot={vi.fn().mockResolvedValue({
          content: 'Original body',
          revision: 'sha256:raw',
        })}
        onFlush={vi.fn().mockResolvedValue(true)}
        onDelete={vi.fn().mockResolvedValue(true)}
      />,
    )

    await user.click(screen.getByRole('button', { name: 'Raw' }))
    const rawEditor = screen.getByRole('textbox', {
      name: '编辑 Raw 设定：Original name',
    })
    expect(rawEditor).toHaveAttribute('data-content-mode', 'source')
    expect(rawEditor).toHaveAttribute('data-review-target', 'lore-1')
  })

  it('confirms before deleting the current lore item', async () => {
    const user = userEvent.setup()
    const onDelete = vi.fn().mockResolvedValue(true)

    render(
      <LoreWorkspaceEditor
        projectId="project-lore-test"
        draft={loreItem()}
        tagDraft=""
        autosaveStatus="saved"
        autosaveError={null}
        documentReview={reviewController()}
        onDraftChange={vi.fn()}
        onTagDraftChange={vi.fn()}
        onPrepareSnapshot={vi.fn().mockResolvedValue({
          content: 'Original body',
          revision: 'r1',
        })}
        onFlush={vi.fn().mockResolvedValue(true)}
        onDelete={onDelete}
      />,
    )

    await user.click(screen.getByRole('button', { name: '删除资料' }))

    const dialog = await screen.findByRole('alertdialog', {
      name: '删除资料',
    })
    expect(within(dialog).getByText('删除资料「Original name」？'))
      .toBeInTheDocument()
    expect(onDelete).not.toHaveBeenCalled()

    await user.click(within(dialog).getByRole('button', { name: '删除' }))

    await waitFor(() => {
      expect(onDelete).toHaveBeenCalledWith('lore-1')
    })
  })
})

function loreItem(): LoreItem {
  return {
    id: 'lore-1',
    enabled: true,
    type: 'character',
    type_source: 'manual',
    name: 'Original name',
    importance: 'important',
    load_mode: 'auto',
    tags: [],
    brief_description: '',
    keywords: [],
    content: 'Original body',
    created_at: 'r0',
    updated_at: 'r1',
  }
}

function reviewController(comments: DocumentReviewComment[] = []): DocumentReviewController {
  return {
    comments,
    onCreate: vi.fn(async (request: CreateDocumentCommentRequest) => ({
      id: 'comment-created',
      thread_id: 'thread-1',
      target: request.target,
      body: request.body,
      anchor: request.anchor,
      created_at: 'r1',
      updated_at: 'r1',
    })),
    onUpdate: vi.fn(async (comment: DocumentReviewComment, body: string) => ({ ...comment, body })),
    onDelete: vi.fn(async (comment: DocumentReviewComment) => ({ ...comment, deleted: true })),
  }
}
