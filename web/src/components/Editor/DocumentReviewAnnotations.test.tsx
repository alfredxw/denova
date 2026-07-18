import { useLayoutEffect, useRef, useState } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Editor } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { Markdown } from '@tiptap/markdown'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { CreateDocumentCommentRequest, DocumentReviewComment } from '@/features/document-review/types'
import { DocumentReviewAnnotations, type DocumentReviewAnnotationsHandle } from './DocumentReviewAnnotations'
import { createDocumentReviewExtension, type DocumentReviewDecorationState, type DocumentReviewPortalTarget } from './documentReviewDecorations'

describe('DocumentReviewAnnotations', () => {
  let editor: Editor | null = null

  afterEach(() => {
    editor?.destroy()
    editor = null
  })

  it('keeps an expanded comment open after a different comment is submitted', async () => {
    const user = userEvent.setup()
    const markdown = 'Alpha text\n\nBeta text\n'
    const revision = 'sha256:review-state'
    const decorationStateRef = { current: { enabled: false, decorations: [] } as DocumentReviewDecorationState }
    let updatePortalTargets: (targets: DocumentReviewPortalTarget[]) => void = () => undefined
    editor = new Editor({
      extensions: [StarterKit, Markdown, createDocumentReviewExtension(decorationStateRef, (targets) => updatePortalTargets(targets))],
      content: markdown,
      contentType: 'markdown',
    })
    const alpha = textRange(editor, 'Alpha text')
    const beta = textRange(editor, 'Beta text')
    const annotationRef = { current: null as DocumentReviewAnnotationsHandle | null }
    const initialComment: DocumentReviewComment = {
      id: 'comment-alpha',
      thread_id: 'document-thread',
      path: 'chapters/ch01.md',
      body: 'Alpha comment',
      created_at: '2026-07-18T00:00:00Z',
      updated_at: '2026-07-18T00:00:00Z',
      anchor: {
        kind: 'text-range',
        encoding: 'utf8-bytes-v1',
        revision,
        start: markdown.indexOf('Alpha text'),
        end: markdown.indexOf('Alpha text') + 'Alpha text'.length,
        quote: 'Alpha text',
        display_quote: 'Alpha text',
        editor_from: alpha.from,
        editor_to: alpha.to,
      },
    }

    function Harness() {
      const [comments, setComments] = useState([initialComment])
      const [portalTargets, setPortalTargets] = useState<DocumentReviewPortalTarget[]>([])
      const containerRef = useRef<HTMLDivElement | null>(null)
      useLayoutEffect(() => {
        updatePortalTargets = (targets) => act(() => setPortalTargets(targets))
        return () => { updatePortalTargets = () => undefined }
      }, [])
      const createComment = async (request: CreateDocumentCommentRequest) => {
        const created: DocumentReviewComment = {
          id: 'comment-beta',
          thread_id: 'document-thread',
          path: request.path,
          body: request.body,
          anchor: request.anchor,
          created_at: '2026-07-18T00:01:00Z',
          updated_at: '2026-07-18T00:01:00Z',
        }
        setComments((current) => [...current, created])
        return created
      }
      return (
        <div ref={containerRef}>
          <div ref={(node) => {
            if (node && editor && editor.view.dom.parentNode !== node) node.append(editor.view.dom)
          }} />
          <DocumentReviewAnnotations
            ref={(handle) => { annotationRef.current = handle }}
            editor={editor!}
            fileName="chapters/ch01.md"
            containerRef={containerRef}
            comments={comments}
            decorationStateRef={decorationStateRef}
            portalTargets={portalTargets}
            onPrepareSnapshot={async () => ({ content: markdown, revision })}
            onCreate={createComment}
            onUpdate={vi.fn()}
            onDelete={vi.fn()}
          />
        </div>
      )
    }

    render(<Harness />)
    const alphaKey = `comment:${revision}:${markdown.indexOf('Alpha text')}:${markdown.indexOf('Alpha text') + 'Alpha text'.length}`
    await waitFor(() => expect(editor!.view.dom.querySelector(`[data-document-review-key="${alphaKey}"]`)).not.toBeNull())
    const alphaHighlight = editor.view.dom.querySelector<HTMLElement>(`[data-document-review-key="${alphaKey}"]`)
    act(() => {
      editor!.view.someProp('handleClick', (handleClick) => handleClick(editor!.view, alpha.from, {
        button: 0,
        detail: 1,
        target: alphaHighlight,
      } as unknown as MouseEvent))
    })
    await screen.findByText('Alpha comment')

    act(() => {
      editor!.commands.setTextSelection(beta)
      annotationRef.current?.startSelectionComment()
    })
    const draft = await screen.findByPlaceholderText('补充审阅背景，或说明希望如何调整…')
    await user.type(draft, 'Beta comment')
    await user.click(screen.getByRole('button', { name: '添加评论' }))

    await waitFor(() => {
      expect(screen.getByText('Alpha comment')).toBeInTheDocument()
      expect(screen.getByText('Beta comment')).toBeInTheDocument()
      expect(screen.getAllByRole('button', { name: '折叠' })).toHaveLength(2)
    })
  })
})

function textRange(instance: Editor, value: string): { from: number; to: number } {
  let found: { from: number; to: number } | null = null
  instance.state.doc.descendants((node, position) => {
    const index = node.isText && node.text ? node.text.indexOf(value) : -1
    if (index >= 0) found = { from: position + index, to: position + index + value.length }
  })
  if (!found) throw new Error(`missing text: ${value}`)
  return found
}
