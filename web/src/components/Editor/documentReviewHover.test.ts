import { Editor } from '@tiptap/core'
import { Markdown } from '@tiptap/markdown'
import StarterKit from '@tiptap/starter-kit'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { documentReviewRangeAtCoordinates } from './documentReviewHover'

describe('document review hover targeting', () => {
  let editor: Editor | null = null

  afterEach(() => {
    editor?.destroy()
    editor = null
  })

  it('treats a wrapped paragraph as one source line across the full editor width', () => {
    editor = new Editor({
      extensions: [StarterKit, Markdown],
      content: '这是一个会换成多行显示、但底层仍然只有一个文本块的段落。\n\n第二段。\n',
      contentType: 'markdown',
    })
    let firstPosition = 0
    let secondPosition = 0
    editor.state.doc.descendants((node, position) => {
      if (!node.isTextblock) return true
      if (!firstPosition) firstPosition = position + 1
      else secondPosition = position + 1
      return false
    })
    vi.spyOn(editor.view.dom, 'getBoundingClientRect').mockReturnValue(rect(100, 20, 400, 108))
    const positionAtCoordinates = vi.spyOn(editor.view, 'posAtCoords')
      .mockImplementation(({ left, top }) => ({ pos: top < 90 ? firstPosition : secondPosition, inside: left }))

    expect(documentReviewRangeAtCoordinates(editor, 760, 68)?.displayQuote).toContain('底层仍然只有一个文本块')
    expect(documentReviewRangeAtCoordinates(editor, 760, 112)?.displayQuote).toBe('第二段。')
    expect(positionAtCoordinates).toHaveBeenNthCalledWith(1, { left: 499, top: 68 })
    expect(positionAtCoordinates).toHaveBeenNthCalledWith(2, { left: 499, top: 112 })
  })
})

function rect(left: number, top: number, width: number, height: number): DOMRect {
  return {
    x: left,
    y: top,
    left,
    top,
    right: left + width,
    bottom: top + height,
    width,
    height,
    toJSON: () => ({}),
  }
}
