import { Editor, Extension } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { Plugin } from '@tiptap/pm/state'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createDocumentReviewExtension, documentReviewPluginKey, type DocumentReviewDecorationState } from './documentReviewDecorations'

describe('document review decorations', () => {
  let editor: Editor | null = null

  afterEach(() => {
    editor?.destroy()
    editor = null
  })

  it('keeps document comments active without blocking manuscript edits', () => {
    const reviewState = { current: { enabled: true, decorations: [] } as DocumentReviewDecorationState }
    editor = new Editor({
      extensions: [StarterKit, createDocumentReviewExtension(reviewState, () => undefined)],
      content: '<p>原始正文</p>',
    })

    editor.commands.setTextSelection({ from: 1, to: 3 })
    expect(editor.state.selection.from).toBe(1)
    expect(editor.state.selection.to).toBe(3)

    editor.commands.insertContent('可以写入')
    expect(editor.getText()).toContain('可以写入')
  })

  it('renders only an underline until the anchored text is clicked', () => {
    const onHighlightClick = vi.fn()
    const reviewState = { current: {
      enabled: true,
      decorations: [{ key: 'comment:1', from: 1, to: 3, widgetPos: 5, showWidget: false }],
      onHighlightClick,
    } as DocumentReviewDecorationState }
    editor = new Editor({
      extensions: [StarterKit, createDocumentReviewExtension(reviewState, () => undefined)],
      content: '<p>原始正文</p>',
    })

    const highlight = editor.view.dom.querySelector<HTMLElement>('[data-document-review-key="comment:1"]')
    expect(highlight).not.toBeNull()
    expect(highlight).toHaveTextContent('原始')
    expect(editor.view.dom.querySelector('[data-document-review-target="comment:1"]')).toBeNull()

    editor.commands.insertContentAt(1, '前置')
    const mappedHighlight = editor.view.dom.querySelector<HTMLElement>('[data-document-review-key="comment:1"]')
    expect(mappedHighlight).toHaveTextContent('原始')

    editor.view.someProp('handleClick', (handleClick) => handleClick(editor!.view, 1, {
      button: 0,
      detail: 1,
      target: mappedHighlight,
    } as unknown as MouseEvent))
    expect(onHighlightClick).toHaveBeenCalledWith('comment:1')

    reviewState.current = {
      ...reviewState.current,
      decorations: [{ key: 'comment:1', from: 1, to: 3, widgetPos: 5, showWidget: true }],
    }
    editor.view.dispatch(editor.state.tr.setMeta(documentReviewPluginKey, true))
    expect(editor.view.dom.querySelector('[data-document-review-target="comment:1"]')).not.toBeNull()
  })

  it('keeps textarea deletion events inside the comment widget', () => {
    const handleKeyDown = vi.fn(() => false)
    const keyDownProbe = Extension.create({
      name: 'documentReviewKeyDownProbe',
      addProseMirrorPlugins: () => [new Plugin({ props: { handleKeyDown } })],
    })
    const reviewState = { current: {
      enabled: true,
      decorations: [{ key: 'comment:1', from: 1, to: 3, widgetPos: 5, showWidget: true }],
    } as DocumentReviewDecorationState }
    editor = new Editor({
      extensions: [StarterKit, createDocumentReviewExtension(reviewState, () => undefined), keyDownProbe],
      content: '<p>原始正文</p>',
    })

    editor.view.dom.querySelector('p')?.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Backspace' }))
    expect(handleKeyDown).toHaveBeenCalledTimes(1)
    handleKeyDown.mockClear()

    const target = editor.view.dom.querySelector<HTMLElement>('[data-document-review-target="comment:1"]')
    const textarea = document.createElement('textarea')
    target?.append(textarea)
    textarea.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, key: 'Backspace' }))
    expect(handleKeyDown).not.toHaveBeenCalled()
  })
})
