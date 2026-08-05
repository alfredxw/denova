import { Schema } from '@tiptap/pm/model'
import { EditorState } from '@tiptap/pm/state'
import { EditorView } from '@tiptap/pm/view'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { createDialogueHighlightPlugin, dialogueHighlightPluginKey } from './editorDialogueDecorations'

const schema = new Schema({
  nodes: {
    doc: { content: 'paragraph+' },
    paragraph: {
      content: 'text*',
      toDOM: () => ['p', 0],
    },
    text: {},
  },
})

describe('editor dialogue decorations', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('builds a new document off the navigation task and updates later edits incrementally', () => {
    const queuedFrames = new Map<number, FrameRequestCallback>()
    let nextFrameID = 1
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      const frameID = nextFrameID
      nextFrameID += 1
      queuedFrames.set(frameID, callback)
      return frameID
    })
    vi.spyOn(window, 'cancelAnimationFrame').mockImplementation((frameID) => {
      queuedFrames.delete(frameID)
    })

    const plugin = createDialogueHighlightPlugin()
    const document = schema.node('doc', null, [
      schema.node('paragraph', null, schema.text('旁白 “第一句对白” 结束。')),
      schema.node('paragraph', null, schema.text('没有对白。')),
    ])
    const host = documentElement('div')
    const view = new EditorView(host, {
      state: EditorState.create({ schema, doc: document, plugins: [plugin] }),
    })

    expect(dialogueHighlightPluginKey.getState(view.state)?.decorations.find()).toHaveLength(0)
    flushAnimationFrames(queuedFrames)
    expect(host.querySelectorAll('.nova-editor-dialogue-highlight')).toHaveLength(1)

    view.dispatch(view.state.tr.insertText('“新增对白” ', 1))
    expect(host.querySelectorAll('.nova-editor-dialogue-highlight')).toHaveLength(2)

    view.destroy()
    host.remove()
  })
})

function documentElement<K extends keyof HTMLElementTagNameMap>(tagName: K) {
  const element = document.createElement(tagName)
  document.body.appendChild(element)
  return element
}

function flushAnimationFrames(frames: Map<number, FrameRequestCallback>) {
  while (frames.size > 0) {
    const pending = Array.from(frames.values())
    frames.clear()
    for (const callback of pending) callback(performance.now())
  }
}
