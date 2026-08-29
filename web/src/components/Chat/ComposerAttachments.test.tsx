import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  AttachmentPreviewScopeProvider,
  ComposerAttachmentTray,
  SentMessageAttachments,
} from './ComposerAttachments'

describe('chat attachment previews', () => {
  it('opens an image selected in the composer without removing it', () => {
    const onRemove = vi.fn()
    const file = new File(['image'], 'draft.png', { type: 'image/png' })
    render(
      <ComposerAttachmentTray
        items={[{ id: 'draft-1', file, previewURL: 'blob:draft-preview' }]}
        onRemove={onRemove}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: '预览 draft.png' }))

    expect(screen.getByRole('dialog', { name: 'draft.png' })).toBeInTheDocument()
    expect(screen.getAllByAltText('draft.png').some((image) => image.getAttribute('src') === 'blob:draft-preview')).toBe(true)
    expect(onRemove).not.toHaveBeenCalled()
  })

  it('renders a persisted image thumbnail and opens the scoped server preview', () => {
    const attachmentID = 'att_0123456789abcdef0123456789abcdef'
    const expectedURL = `/api/projects/project%20one/attachments/${attachmentID}?scope=story&scope_id=story%2Fone`
    render(
      <AttachmentPreviewScopeProvider projectId="project one" scope={{ kind: 'story', id: 'story/one' }}>
        <SentMessageAttachments attachments={[{
          id: attachmentID,
          name: 'scene.png',
          media_type: 'image/png',
          size: 8,
        }]} />
      </AttachmentPreviewScopeProvider>,
    )

    expect(screen.getByAltText('scene.png')).toHaveAttribute('src', expectedURL)
    fireEvent.click(screen.getByRole('button', { name: '预览 scene.png' }))

    expect(screen.getByRole('dialog', { name: 'scene.png' })).toBeInTheDocument()
    expect(screen.getAllByAltText('scene.png').some((image) => image.getAttribute('src') === expectedURL)).toBe(true)
  })
})
