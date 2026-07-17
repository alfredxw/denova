import { Editor } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { Markdown } from '@tiptap/markdown'
import { afterEach, describe, expect, it } from 'vitest'
import { commentWidgetPosition, createDocumentReviewAnchor, textBlockRangeAtPosition } from './documentReviewAnchors'

describe('document review anchors', () => {
  let editor: Editor | null = null

  afterEach(() => {
    editor?.destroy()
    editor = null
  })

  it('maps the exact repeated TipTap selection to canonical Markdown UTF-8 bytes', () => {
    const content = '开头 **目标😀** 与目标😀结尾\n'
    editor = new Editor({ extensions: [StarterKit, Markdown], content, contentType: 'markdown' })
    const ranges: Array<{ from: number; to: number }> = []
    editor.state.doc.descendants((node, position) => {
      if (!node.isText || !node.text) return
      let offset = node.text.indexOf('目标😀')
      while (offset >= 0) {
        ranges.push({ from: position + offset, to: position + offset + '目标😀'.length })
        offset = node.text.indexOf('目标😀', offset + 1)
      }
    })
    expect(ranges).toHaveLength(2)

    const selected = ranges[1]
    const anchor = createDocumentReviewAnchor(editor, { content, revision: 'sha256:test' }, {
      ...selected,
      widgetPos: commentWidgetPosition(editor.state.doc, selected.to),
      kind: 'text-range',
      displayQuote: '目标😀',
    })
    const expectedStart = new TextEncoder().encode(content.slice(0, content.lastIndexOf('目标😀'))).length
    expect(anchor).toMatchObject({
      revision: 'sha256:test',
      encoding: 'utf8-bytes-v1',
      start: expectedStart,
      end: expectedStart + new TextEncoder().encode('目标😀').length,
      quote: '目标😀',
      display_quote: '目标😀',
      editor_from: selected.from,
      editor_to: selected.to,
    })
  })

  it('anchors the hovered source line to its enclosing text block', () => {
    editor = new Editor({ extensions: [StarterKit, Markdown], content: '第一段\n\n第二段\n', contentType: 'markdown' })
    const secondParagraphPosition = editor.state.doc.textContent.indexOf('第二段') + 3
    const range = textBlockRangeAtPosition(editor.state.doc, secondParagraphPosition)
    expect(range).toMatchObject({ kind: 'text-block', displayQuote: '第二段' })
    expect(range && editor.state.doc.textBetween(range.from, range.to, '\n')).toBe('第二段')
  })

  it('places list-item comments directly after the anchored text block', () => {
    editor = new Editor({
      extensions: [StarterKit, Markdown],
      content: '- **成长性**：逐步解锁\n- **代价**：消耗神识\n',
      contentType: 'markdown',
    })
    let position = 0
    editor.state.doc.descendants((node, nodePosition) => {
      if (position || !node.isText || !node.text?.includes('成长性')) return
      position = nodePosition + node.text.indexOf('成长性') + 1
    })
    const range = textBlockRangeAtPosition(editor.state.doc, position)

    expect(range).not.toBeNull()
    expect(editor.state.doc.resolve(range!.widgetPos).parent.type.name).toBe('listItem')
    expect(range!.widgetPos).toBeLessThan(editor.state.doc.child(0).nodeSize)
  })
})
