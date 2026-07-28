import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import type { DocumentReviewController } from '@/features/document-review/controller'
import type { LoreItem } from '@/lib/api'
import { LoreWorkspaceEditor } from './LoreWorkspaceEditor'

vi.mock('@/components/Editor/MarkdownRichEditor', () => ({
  MarkdownRichEditor: (props: { value: string; 'aria-label'?: string }) => (
    <div role="textbox" aria-label={props['aria-label']}>
      {props.value}
    </div>
  ),
}))

describe('LoreWorkspaceEditor', () => {
  it('confirms before deleting the current lore item', async () => {
    const user = userEvent.setup()
    const onDelete = vi.fn().mockResolvedValue(true)

    render(
      <LoreWorkspaceEditor
        draft={loreItem()}
        tagDraft=""
        autosaveStatus="saved"
        autosaveError={null}
        documentReview={{} as DocumentReviewController}
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
