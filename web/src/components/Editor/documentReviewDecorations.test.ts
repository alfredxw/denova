import { Editor } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { afterEach, describe, expect, it } from 'vitest'
import { createDocumentReviewExtension, type DocumentReviewDecorationState } from './documentReviewDecorations'

describe('document review decorations', () => {
  let editor: Editor | null = null

  afterEach(() => {
    editor?.destroy()
    editor = null
  })

  it('keeps selections active while rejecting manuscript mutations in Review mode', () => {
    const reviewState = { current: { enabled: false, decorations: [] } as DocumentReviewDecorationState }
    editor = new Editor({
      extensions: [StarterKit, createDocumentReviewExtension(reviewState, () => undefined)],
      content: '<p>原始正文</p>',
    })

    reviewState.current = { enabled: true, decorations: [] }
    editor.commands.setTextSelection({ from: 1, to: 3 })
    expect(editor.state.selection.from).toBe(1)
    expect(editor.state.selection.to).toBe(3)

    editor.commands.insertContent('不应写入')
    expect(editor.getText()).toBe('原始正文')

    reviewState.current = { enabled: false, decorations: [] }
    editor.commands.insertContent('可以写入')
    expect(editor.getText()).toContain('可以写入')
  })
})
