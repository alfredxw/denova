import { Editor } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { Markdown } from '@tiptap/markdown'
import { afterEach, describe, expect, it } from 'vitest'
import {
  captureDocumentReviewSelection,
  commentWidgetPosition,
  createDocumentReviewAnchor,
  textBlockRangeAtPosition,
  type EditorReviewRange,
} from './documentReviewAnchors'

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
    const anchor = createAnchor(editor, { content, revision: 'sha256:test' }, {
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

  it('maps a multi-block selection when canonical Markdown uses equivalent source formatting', () => {
    const content = '# 标题\n\n* 第一项\n* 第二项\n'
    editor = new Editor({ extensions: [StarterKit, Markdown], content, contentType: 'markdown' })
    const range = textRange(editor, '标题', '第二项')

    const anchor = createAnchor(editor, { content, revision: 'sha256:multi-block' }, {
      ...range,
      widgetPos: commentWidgetPosition(editor.state.doc, range.to),
      kind: 'text-range',
      displayQuote: editor.state.doc.textBetween(range.from, range.to, '\n').trim(),
    })

    expect(anchor).toMatchObject({
      revision: 'sha256:multi-block',
      start: new TextEncoder().encode(content.slice(0, content.indexOf('标题'))).length,
      end: new TextEncoder().encode(content.slice(0, content.indexOf('第二项') + '第二项'.length)).length,
      quote: '标题\n\n* 第一项\n* 第二项',
      display_quote: '标题\n第一项\n第二项',
    })
  })

  it('keeps anchor bytes tied to the raw canonical line endings', () => {
    const content = '第一段  \r\n\r\n第二段\r\n'
    editor = new Editor({ extensions: [StarterKit, Markdown], content, contentType: 'markdown' })
    const range = textRange(editor, '第二段')

    const anchor = createAnchor(editor, { content, revision: 'sha256:crlf' }, {
      ...range,
      widgetPos: commentWidgetPosition(editor.state.doc, range.to),
      kind: 'text-range',
      displayQuote: '第二段',
    })

    const start = content.indexOf('第二段')
    expect(anchor).toMatchObject({
      start: new TextEncoder().encode(content.slice(0, start)).length,
      end: new TextEncoder().encode(content.slice(0, start + '第二段'.length)).length,
      quote: '第二段',
    })
  })

  it.each([
    {
      name: 'ordered-list markers',
      content: '1) 第一项\n2) 第二项\n',
      startText: '第一项',
      endText: '第二项',
    },
    {
      name: 'blockquote continuation markers',
      content: '> 第一段\n>\n> 第二段\n',
      startText: '第一段',
      endText: '第二段',
    },
    {
      name: 'equivalent emphasis markers',
      content: '_第一段_\n\n__第二段__\n',
      startText: '第一段',
      endText: '第二段',
    },
  ])('maps selections across $name', ({ content, startText, endText }) => {
    editor = new Editor({ extensions: [StarterKit, Markdown], content, contentType: 'markdown' })
    const range = textRange(editor, startText, endText)
    const anchor = createAnchor(editor, { content, revision: 'sha256:syntax' }, {
      ...range,
      widgetPos: commentWidgetPosition(editor.state.doc, range.to),
      kind: 'text-range',
      displayQuote: editor.state.doc.textBetween(range.from, range.to, '\n').trim(),
    })

    const start = content.indexOf(startText)
    const end = content.indexOf(endText, start) + endText.length
    expect(anchor.start).toBe(new TextEncoder().encode(content.slice(0, start)).length)
    expect(anchor.end).toBe(new TextEncoder().encode(content.slice(0, end)).length)
    expect(anchor.quote).toBe(content.slice(start, end))
  })

  it('rejects a canonical snapshot whose parsed document no longer matches the captured selection', () => {
    const content = '第一段\n\n第二段\n'
    editor = new Editor({ extensions: [StarterKit, Markdown], content, contentType: 'markdown' })
    const range = textRange(editor, '第二段')
    const reviewRange: EditorReviewRange = {
      ...range,
      widgetPos: commentWidgetPosition(editor.state.doc, range.to),
      kind: 'text-range',
      displayQuote: '第二段',
    }
    const selection = captureDocumentReviewSelection(editor, reviewRange)

    expect(() => createDocumentReviewAnchor(editor!, {
      content: '第一段\n\n已被外部替换\n',
      revision: 'sha256:external',
    }, selection)).toThrow('The editor and workspace snapshots differ')
  })

  it('keeps the selected document stable while the canonical snapshot is loading', () => {
    const content = '第一段\n\n第二段\n'
    editor = new Editor({ extensions: [StarterKit, Markdown], content, contentType: 'markdown' })
    const range = textRange(editor, '第二段')
    const reviewRange: EditorReviewRange = {
      ...range,
      widgetPos: commentWidgetPosition(editor.state.doc, range.to),
      kind: 'text-range',
      displayQuote: '第二段',
    }
    const selection = captureDocumentReviewSelection(editor, reviewRange)
    editor.commands.insertContentAt(1, '加载期间新增')

    const anchor = createDocumentReviewAnchor(editor, { content, revision: 'sha256:frozen' }, selection)
    expect(anchor.quote).toBe('第二段')
    expect(anchor.editor_from).toBe(range.from)
    expect(anchor.editor_to).toBe(range.to)
  })

  it('ignores an editor-only empty paragraph appended after a terminal Markdown block', () => {
    const content = '## 规则\n- 第一项\n- 第二项\n'
    editor = new Editor({ extensions: [StarterKit, Markdown], content, contentType: 'markdown' })
    const range = textRange(editor, '第一项')
    const topLevelNodes = Array.from(
      { length: editor.state.doc.childCount },
      (_, index) => editor!.state.doc.child(index),
    )
    topLevelNodes.push(editor.schema.nodes.paragraph.create())
    const documentWithTrailingParagraph = editor.state.doc.type.create(editor.state.doc.attrs, topLevelNodes)
    const selection = {
      document: documentWithTrailingParagraph,
      range: {
        ...range,
        widgetPos: commentWidgetPosition(documentWithTrailingParagraph, range.to),
        kind: 'text-range' as const,
        displayQuote: '第一项',
      },
    }

    const anchor = createDocumentReviewAnchor(editor, { content, revision: 'sha256:trailing-node' }, selection)
    expect(anchor.quote).toBe('第一项')
  })

  it('maps the selected occurrence when equivalent multi-line source appears more than once', () => {
    const content = '* 重复一\n* 重复二\n\n分隔\n\n* 重复一\n* 重复二\n'
    editor = new Editor({ extensions: [StarterKit, Markdown], content, contentType: 'markdown' })
    const range = textRange(editor, '重复一', '重复二', 1)

    const anchor = createAnchor(editor, { content, revision: 'sha256:repeated-blocks' }, {
      ...range,
      widgetPos: commentWidgetPosition(editor.state.doc, range.to),
      kind: 'text-range',
      displayQuote: editor.state.doc.textBetween(range.from, range.to, '\n').trim(),
    })

    const start = content.lastIndexOf('重复一')
    const end = content.lastIndexOf('重复二') + '重复二'.length
    expect(anchor.start).toBe(new TextEncoder().encode(content.slice(0, start)).length)
    expect(anchor.end).toBe(new TextEncoder().encode(content.slice(0, end)).length)
    expect(anchor.quote).toBe(content.slice(start, end))
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

function textRange(editor: Editor, startText: string, endText = startText, occurrence = 0): { from: number; to: number } {
  const starts: number[] = []
  const ends: number[] = []
  editor.state.doc.descendants((node, position) => {
    if (!node.isText || !node.text) return
    let start = node.text.indexOf(startText)
    while (start >= 0) {
      starts.push(position + start)
      start = node.text.indexOf(startText, start + 1)
    }
    let end = node.text.indexOf(endText)
    while (end >= 0) {
      ends.push(position + end + endText.length)
      end = node.text.indexOf(endText, end + 1)
    }
  })
  const from = starts[occurrence] ?? -1
  const to = ends[occurrence] ?? -1
  if (from < 0 || to < from) throw new Error(`Could not find editor range: ${startText} -> ${endText}`)
  return { from, to }
}

function createAnchor(
  editor: Editor,
  snapshot: { content: string; revision: string },
  range: EditorReviewRange,
) {
  return createDocumentReviewAnchor(editor, snapshot, captureDocumentReviewSelection(editor, range))
}
