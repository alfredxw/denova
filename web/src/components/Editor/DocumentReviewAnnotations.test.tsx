import { useLayoutEffect, useRef, useState } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Editor } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { Markdown } from '@tiptap/markdown'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { CreateDocumentCommentRequest, DocumentReviewComment } from '@/features/document-review/types'
import { DocumentReviewAnnotations, type DocumentReviewAnnotationsHandle } from './DocumentReviewAnnotations'
import { createDocumentReviewExtension, documentReviewKeysFromElement, type DocumentReviewDecorationState, type DocumentReviewPortalTarget } from './documentReviewDecorations'

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
    await waitFor(() => expect(findHighlight(editor!, alphaKey)).toBeDefined())
    const alphaHighlight = findHighlight(editor, alphaKey)
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

  it('opens one draft when a multi-block selection contains multiple existing comments', async () => {
    const markdown = 'Intro Alpha text tail\n\nMiddle text\n\nPrefix Gamma text outro\n'
    const revision = 'sha256:overlapping-review-state'
    const decorationStateRef = { current: { enabled: false, decorations: [] } as DocumentReviewDecorationState }
    let updatePortalTargets: (targets: DocumentReviewPortalTarget[]) => void = () => undefined
    editor = new Editor({
      extensions: [StarterKit, Markdown, createDocumentReviewExtension(decorationStateRef, (targets) => updatePortalTargets(targets))],
      content: markdown,
      contentType: 'markdown',
    })
    const alpha = textRange(editor, 'Alpha text')
    const gamma = textRange(editor, 'Gamma text')
    const selection = { from: textRange(editor, 'Intro').from, to: textRange(editor, 'outro').to }
    const annotationRef = { current: null as DocumentReviewAnnotationsHandle | null }
    const comments = [
      reviewComment('comment-alpha', 'Alpha comment', markdown, revision, alpha, 'Alpha text'),
      reviewComment('comment-gamma', 'Gamma comment', markdown, revision, gamma, 'Gamma text'),
    ]

    function Harness() {
      const [portalTargets, setPortalTargets] = useState<DocumentReviewPortalTarget[]>([])
      const containerRef = useRef<HTMLDivElement | null>(null)
      useLayoutEffect(() => {
        updatePortalTargets = (targets) => act(() => setPortalTargets(targets))
        return () => { updatePortalTargets = () => undefined }
      }, [])
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
            onCreate={vi.fn()}
            onUpdate={vi.fn()}
            onDelete={vi.fn()}
          />
        </div>
      )
    }

    render(<Harness />)
    await waitFor(() => expect(editor!.view.dom.querySelectorAll('[data-document-review-keys]')).toHaveLength(2))
    const alphaKey = `comment:${revision}:${markdown.indexOf('Alpha text')}:${markdown.indexOf('Alpha text') + 'Alpha text'.length}`
    const gammaKey = `comment:${revision}:${markdown.indexOf('Gamma text')}:${markdown.indexOf('Gamma text') + 'Gamma text'.length}`
    act(() => {
      clickHighlight(editor!, alphaKey, alpha.from)
      clickHighlight(editor!, gammaKey, gamma.from)
    })
    await screen.findByText('Alpha comment')
    await screen.findByText('Gamma comment')

    act(() => {
      editor!.commands.setTextSelection(selection)
      annotationRef.current?.startSelectionComment()
    })

    const draft = await screen.findByPlaceholderText('补充审阅背景，或说明希望如何调整…')
    expect(draft).toBeInTheDocument()
    expect(editor.state.doc.textBetween(selection.from, selection.to, '\n')).toContain('Middle text')
    await waitFor(() => expect(decorationStateRef.current.decorations).toHaveLength(3))
  })
})

function reviewComment(
  id: string,
  body: string,
  markdown: string,
  revision: string,
  range: { from: number; to: number },
  quote: string,
): DocumentReviewComment {
  const start = markdown.indexOf(quote)
  return {
    id,
    thread_id: 'document-thread',
    path: 'chapters/ch01.md',
    body,
    created_at: '2026-07-18T00:00:00Z',
    updated_at: '2026-07-18T00:00:00Z',
    anchor: {
      kind: 'text-range',
      encoding: 'utf8-bytes-v1',
      revision,
      start,
      end: start + quote.length,
      quote,
      display_quote: quote,
      editor_from: range.from,
      editor_to: range.to,
    },
  }
}

function clickHighlight(instance: Editor, key: string, position: number): void {
  const highlight = findHighlight(instance, key)
  if (!highlight) throw new Error(`missing highlight: ${key}`)
  instance.view.someProp('handleClick', (handleClick) => handleClick(instance.view, position, {
    button: 0,
    detail: 1,
    target: highlight,
  } as unknown as MouseEvent))
}

function findHighlight(instance: Editor, key: string): HTMLElement | undefined {
  return Array.from(instance.view.dom.querySelectorAll<HTMLElement>('[data-document-review-keys]'))
    .find((element) => documentReviewKeysFromElement(element).includes(key))
}

function textRange(instance: Editor, value: string): { from: number; to: number } {
  let found: { from: number; to: number } | null = null
  instance.state.doc.descendants((node, position) => {
    const index = node.isText && node.text ? node.text.indexOf(value) : -1
    if (index >= 0) found = { from: position + index, to: position + index + value.length }
  })
  if (!found) throw new Error(`missing text: ${value}`)
  return found
}
