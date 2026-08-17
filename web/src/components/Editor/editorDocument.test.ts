import { Editor } from '@tiptap/core'
import StarterKit from '@tiptap/starter-kit'
import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  ParsedMarkdownDocumentCache,
  placeEditorCaretAtClick,
  replaceEditorDocument,
  replaceEditorDocumentWithFreshState,
  resetEditorStateHistory,
} from './editorDocument'

describe('ParsedMarkdownDocumentCache', () => {
  it('reuses only an exact source match and evicts the least recently used entry', () => {
    const cache = new ParsedMarkdownDocumentCache(2)
    const first = { type: 'doc', content: [{ type: 'paragraph' }] }
    const second = { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'two' }] }] }
    const third = { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'three' }] }] }

    cache.set('/books/demo\u0000chapters/one.md', 'one', first)
    cache.set('/books/demo\u0000chapters/two.md', 'two', second)

    expect(cache.get('/books/demo\u0000chapters/one.md', 'one')).toBe(first)
    expect(cache.get('/books/demo\u0000chapters/one.md', 'changed')).toBeUndefined()

    cache.set('/books/demo\u0000chapters/three.md', 'three', third)

    expect(cache.get('/books/demo\u0000chapters/two.md', 'two')).toBeUndefined()
    expect(cache.get('/books/demo\u0000chapters/one.md', 'one')).toBe(first)
    expect(cache.get('/books/demo\u0000chapters/three.md', 'three')).toBe(third)
  })
})

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

  it('isolates undo history after replacing one file with another', () => {
    editor = new Editor({
      extensions: [StarterKit],
      content: '<p>first file</p>',
    })
    editor.commands.setTextSelection(11)
    editor.commands.insertContent(' draft')

    replaceEditorDocument(editor, '<p>second file</p>', {
      contentType: 'html',
      preserveSelection: false,
    })
    resetEditorStateHistory(editor)
    editor.commands.setTextSelection(12)
    editor.commands.insertContent(' draft')

    expect(editor.getText()).toBe('second file draft')
    expect(editor.commands.undo()).toBe(true)
    expect(editor.getText()).toBe('second file')
    expect(editor.commands.undo()).toBe(false)
    expect(editor.getText()).toBe('second file')
  })

  it('replaces a file and resets its history with one view state update', () => {
    editor = new Editor({
      extensions: [StarterKit],
      content: '<p>first file</p>',
    })
    editor.commands.setTextSelection(11)
    editor.commands.insertContent(' draft')
    const nextDocument = editor.schema.nodeFromJSON({
      type: 'doc',
      content: [{
        type: 'paragraph',
        content: [{ type: 'text', text: 'second file' }],
      }],
    })
    const updateState = vi.spyOn(editor.view, 'updateState')

    replaceEditorDocumentWithFreshState(editor, nextDocument)

    expect(updateState).toHaveBeenCalledTimes(1)
    expect(editor.getText()).toBe('second file')

    editor.commands.setTextSelection(12)
    editor.commands.insertContent(' draft')
    expect(editor.commands.undo()).toBe(true)
    expect(editor.getText()).toBe('second file')
    expect(editor.commands.undo()).toBe(false)
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
