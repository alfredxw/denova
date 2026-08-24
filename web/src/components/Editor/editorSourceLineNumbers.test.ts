import { Editor } from '@tiptap/core'
import { Markdown } from '@tiptap/markdown'
import StarterKit from '@tiptap/starter-kit'
import { afterEach, describe, expect, it } from 'vitest'

import { createSourceLineNumberExtension, sourceLineNumbersPluginKey } from './editorSourceLineNumbers'

describe('source line number decorations', () => {
  let editor: Editor | null = null

  afterEach(() => {
    editor?.destroy()
    editor = null
  })

  it('uses canonical Markdown source lines instead of ProseMirror block indexes', () => {
    editor = new Editor({
      extensions: [StarterKit, Markdown, createSourceLineNumberExtension()],
      content: '# Heading\n\nFirst line  \nSecond line\n\n- One\n- Two\n',
      contentType: 'markdown',
    })

    const decorations = sourceLineNumbersPluginKey.getState(editor.state)

    expectSourceLines(decorations, ['1', '3', '6'])
  })

  it('updates following source lines when a block gains a line break', () => {
    editor = new Editor({
      extensions: [StarterKit, Markdown, createSourceLineNumberExtension()],
      content: 'Alpha\n\nOmega\n',
      contentType: 'markdown',
    })

    editor.commands.setContent('Alpha  \nBeta\n\nOmega\n', { contentType: 'markdown' })
    const decorations = sourceLineNumbersPluginKey.getState(editor.state)

    expectSourceLines(decorations, ['1', '4'])
  })
})

function expectSourceLines(decorations: ReturnType<typeof sourceLineNumbersPluginKey.getState>, lines: string[]) {
  expect(decorations?.find()).toHaveLength(lines.length)
  for (const line of lines) {
    expect(decorations?.find(undefined, undefined, (spec) => spec.sourceLine === Number(line))).toHaveLength(1)
  }
}
