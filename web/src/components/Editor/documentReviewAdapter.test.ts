import { Editor } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { Markdown } from '@tiptap/markdown'
import { afterEach, describe, expect, it } from 'vitest'
import { richMarkdownReviewAdapter, sourceMarkdownReviewAdapter } from './documentReviewAdapter'
import { commentWidgetPosition, type EditorReviewRange } from './documentReviewAnchors'
import {
  createMarkdownSourceDocument,
  RawMarkdown,
  readMarkdownSourceDocument,
} from './markdownSourceDocument'

describe('source Markdown review adapter', () => {
  let editor: Editor | null = null

  afterEach(() => {
    editor?.destroy()
    editor = null
  })

  it('keeps Markdown syntax literal and creates canonical UTF-8 review anchors', () => {
    const content = '# 世界观\n\n**角色😀**设定\n'
    const canonical = content.trimEnd()
    editor = new Editor({
      extensions: [StarterKit, RawMarkdown, Markdown],
      content: createMarkdownSourceDocument(content),
    })
    const source = readMarkdownSourceDocument(editor)
    expect(source?.content).toBe(content)
    expect(editor.state.doc.textContent).toContain('**角色😀**')

    const sourceStart = content.indexOf('角色')
    const sourceEnd = sourceStart + '角色😀'.length
    const range: EditorReviewRange = {
      from: source!.contentStart + sourceStart,
      to: source!.contentStart + sourceEnd,
      widgetPos: commentWidgetPosition(editor.state.doc, source!.contentStart + sourceEnd),
      kind: 'text-range',
      displayQuote: '角色😀',
    }
    const frozen = sourceMarkdownReviewAdapter.captureSelection(editor, range)
    const anchor = sourceMarkdownReviewAdapter.createAnchor(
      editor,
      { content: canonical, revision: 'sha256:source-review' },
      frozen,
    )

    expect(anchor).toMatchObject({
      encoding: 'utf8-bytes-v1',
      revision: 'sha256:source-review',
      start: new TextEncoder().encode(canonical.slice(0, sourceStart)).length,
      end: new TextEncoder().encode(canonical.slice(0, sourceEnd)).length,
      quote: '角色😀',
      display_quote: '角色😀',
      editor_from: range.from,
      editor_to: range.to,
    })
    expect(sourceMarkdownReviewAdapter.resolveRange(editor, anchor)).toMatchObject({
      from: range.from,
      to: range.to,
      displayQuote: '角色😀',
    })

    editor.destroy()
    editor = new Editor({
      extensions: [StarterKit, RawMarkdown, Markdown],
      content: canonical,
      contentType: 'markdown',
    })
    const richRange = richMarkdownReviewAdapter.resolveRange(editor, anchor)
    expect(richRange).not.toBeNull()
    expect(editor.state.doc.textBetween(richRange!.from, richRange!.to)).toBe('角色😀')
  })

  it('edits the literal Markdown source inside TipTap without parsing its syntax', () => {
    const content = '# 标题\n\n正文\n'
    editor = new Editor({
      extensions: [StarterKit, RawMarkdown, Markdown],
      content: createMarkdownSourceDocument(content),
    })
    const source = readMarkdownSourceDocument(editor)!
    const insertion = source.contentStart + content.indexOf('正文')

    editor.commands.insertContentAt(insertion, '**补充**')

    expect(readMarkdownSourceDocument(editor)?.content).toBe('# 标题\n\n**补充**正文\n')
  })
})
