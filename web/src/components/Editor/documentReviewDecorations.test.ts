import { Editor, Extension } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { Plugin } from '@tiptap/pm/state'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { createDocumentReviewExtension, documentReviewKeysFromElement, documentReviewPluginKey, type DocumentReviewDecorationState } from './documentReviewDecorations'

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

  it('renders a keyboard-operable disclosure until the anchored text is expanded', () => {
    const onHighlightClick = vi.fn()
    const reviewState = { current: {
      enabled: true,
      decorations: [{ key: 'comment:1', from: 1, to: 3, widgetPos: 5, showWidget: false }],
      onHighlightClick,
      expandLabel: 'Expand comment',
      collapseLabel: 'Collapse comment',
    } as DocumentReviewDecorationState }
    editor = new Editor({
      extensions: [StarterKit, createDocumentReviewExtension(reviewState, () => undefined)],
      content: '<p>原始正文</p>',
    })

    const highlight = reviewHighlights(editor, 'comment:1')[0] ?? null
    expect(highlight).not.toBeNull()
    expect(highlight).toHaveTextContent('原始')
    expect(highlight).toHaveAttribute('role', 'button')
    expect(highlight).toHaveAttribute('tabindex', '0')
    expect(highlight).toHaveAttribute('aria-expanded', 'false')
    expect(highlight).toHaveAttribute('aria-label', 'Expand comment')
    expect(editor.view.dom.querySelector('[data-document-review-target="comment:1"]')).toBeNull()

    editor.commands.insertContentAt(1, '前置')
    const mappedHighlight = reviewHighlights(editor, 'comment:1')[0] ?? null
    expect(mappedHighlight).toHaveTextContent('原始')

    const preventDefault = vi.fn()
    editor.view.someProp('handleKeyDown', (handleKeyDown) => handleKeyDown(editor!.view, {
      key: ' ',
      target: mappedHighlight,
      preventDefault,
    } as unknown as KeyboardEvent))
    expect(preventDefault).toHaveBeenCalledTimes(1)
    expect(onHighlightClick).toHaveBeenCalledWith(['comment:1'])
    onHighlightClick.mockClear()

    editor.view.someProp('handleClick', (handleClick) => handleClick(editor!.view, 1, {
      button: 0,
      detail: 1,
      target: mappedHighlight,
    } as unknown as MouseEvent))
    expect(onHighlightClick).toHaveBeenCalledWith(['comment:1'])

    reviewState.current = {
      ...reviewState.current,
      decorations: [{ key: 'comment:1', from: 1, to: 3, widgetPos: 5, showWidget: true }],
    }
    editor.view.dispatch(editor.state.tr.setMeta(documentReviewPluginKey, true))
    const expandedHighlight = reviewHighlights(editor, 'comment:1')[0] ?? null
    const target = editor.view.dom.querySelector<HTMLElement>('[data-document-review-target="comment:1"]')
    expect(target).not.toBeNull()
    expect(expandedHighlight).toHaveAttribute('aria-expanded', 'true')
    expect(expandedHighlight).toHaveAttribute('aria-label', 'Collapse comment')
    expect(expandedHighlight?.getAttribute('aria-controls')).toBe(target?.id)
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

  it('exposes one keyboard disclosure when ProseMirror splits a comment across blocks', () => {
    const reviewState = { current: {
      enabled: true,
      decorations: [],
      onHighlightClick: vi.fn(),
      expandLabel: 'Expand comment',
      collapseLabel: 'Collapse comment',
    } as DocumentReviewDecorationState }
    editor = new Editor({
      extensions: [StarterKit, createDocumentReviewExtension(reviewState, () => undefined)],
      content: '<p>Alpha</p><p>Beta</p>',
    })
    reviewState.current.decorations = [{
      key: 'comment:multi-block',
      from: 1,
      to: editor.state.doc.content.size - 1,
      widgetPos: editor.state.doc.content.size,
      showWidget: false,
    }]
    editor.view.dispatch(editor.state.tr.setMeta(documentReviewPluginKey, true))

    const fragments = reviewHighlights(editor, 'comment:multi-block')
    expect(fragments.length).toBeGreaterThan(1)
    expect(fragments.filter((fragment) => fragment.tabIndex === 0)).toHaveLength(1)
    expect(fragments.filter((fragment) => fragment.getAttribute('role') === 'button')).toHaveLength(1)
  })

  it('keeps nested comment highlights disjoint with one disclosure per comment', () => {
    const onHighlightClick = vi.fn()
    const reviewState = { current: {
      enabled: true,
      decorations: [],
      onHighlightClick,
      expandLabel: 'Expand comment',
      collapseLabel: 'Collapse comment',
    } as DocumentReviewDecorationState }
    editor = new Editor({
      extensions: [StarterKit, createDocumentReviewExtension(reviewState, () => undefined)],
      content: '<p>Intro Alpha text tail</p><p>Middle text</p><p>Prefix Gamma text outro</p>',
    })
    const findRange = (value: string) => {
      let result = { from: -1, to: -1 }
      editor!.state.doc.descendants((node, position) => {
        const offset = node.isText && node.text ? node.text.indexOf(value) : -1
        if (offset >= 0) result = { from: position + offset, to: position + offset + value.length }
      })
      return result
    }
    const alpha = findRange('Alpha text')
    const gamma = findRange('Gamma text')
    const whole = { from: findRange('Intro').from, to: findRange('outro').to }
    reviewState.current.decorations = [
      { key: 'comment:alpha', ...alpha, widgetPos: alpha.to },
      { key: 'comment:gamma', ...gamma, widgetPos: gamma.to },
      { key: 'comment:whole', ...whole, widgetPos: whole.to },
    ]
    editor.view.dispatch(editor.state.tr.setMeta(documentReviewPluginKey, true))

    const highlights = Array.from(editor.view.dom.querySelectorAll<HTMLElement>('.nova-document-review-highlight'))
    const disclosureKeys = highlights
      .filter((highlight) => highlight.tabIndex === 0)
      .flatMap(documentReviewKeysFromElement)
      .sort()
    expect(disclosureKeys).toEqual(['comment:alpha', 'comment:gamma', 'comment:whole'])
    expect(editor.view.dom.querySelector('.nova-document-review-highlight .nova-document-review-highlight')).toBeNull()

    const alphaHighlight = highlights.find((highlight) => documentReviewKeysFromElement(highlight).includes('comment:alpha'))
    editor.view.someProp('handleClick', (handleClick) => handleClick(editor!.view, alpha.from, {
      button: 0,
      detail: 1,
      target: alphaHighlight,
    } as unknown as MouseEvent))
    expect(onHighlightClick).toHaveBeenCalledWith(['comment:alpha'])
  })
})

function reviewHighlights(instance: Editor, key: string): HTMLElement[] {
  return Array.from(instance.view.dom.querySelectorAll<HTMLElement>('[data-document-review-keys]'))
    .filter((highlight) => documentReviewKeysFromElement(highlight).includes(key))
}
