import { Editor } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { afterEach, describe, expect, it } from 'vitest'

import { placeEditorCaretAtClick, replaceEditorDocument } from './editorDocument'

describe('replaceEditorDocument', () => {
  let editor: Editor | null = null

  afterEach(() => {
    editor?.destroy()
    editor = null
  })

  it('preserves the caret while replacing the current document', () => {
    editor = new Editor({
      extensions: [StarterKit],
      content: '<p>abcdef</p>',
    })
    editor.commands.setTextSelection(4)

    replaceEditorDocument(editor, '<p>abcXYZdef</p>', {
      contentType: 'html',
      preserveSelection: true,
    })

    expect(editor.state.selection.from).toBe(4)
    expect(editor.state.selection.to).toBe(4)
  })

  it('clamps a restored caret when the replacement is shorter', () => {
    editor = new Editor({
      extensions: [StarterKit],
      content: '<p>abcdef</p>',
    })
    editor.commands.setTextSelection(7)

    replaceEditorDocument(editor, '<p>x</p>', {
      contentType: 'html',
      preserveSelection: true,
    })

    expect(editor.state.selection.from).toBe(2)
    expect(editor.state.selection.to).toBe(2)
  })

  it('moves a stale end-of-document caret to the clicked document position', () => {
    editor = new Editor({
      extensions: [StarterKit],
      content: '<p>abcdef</p>',
    })
    editor.commands.setTextSelection(7)

    const handled = placeEditorCaretAtClick(editor.view, 3)

    expect(handled).toBe(true)
    expect(editor.state.selection.from).toBe(3)
    expect(editor.state.selection.to).toBe(3)
  })

  it('does not collapse a text range created by pointer selection', () => {
    editor = new Editor({
      extensions: [StarterKit],
      content: '<p>abcdef</p>',
    })
    editor.commands.setTextSelection({ from: 2, to: 5 })

    const handled = placeEditorCaretAtClick(editor.view, 3)

    expect(handled).toBe(false)
    expect(editor.state.selection.from).toBe(2)
    expect(editor.state.selection.to).toBe(5)
  })
})
