import { act, fireEvent, render, screen } from '@testing-library/react'
import type { Editor } from '@tiptap/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SelectionToolbar } from './SelectionToolbar'

type EditorEvent = 'focus' | 'blur' | 'selectionUpdate' | 'update'

function createEditor() {
  const listeners = new Map<EditorEvent, Set<() => void>>()
  const container = document.createElement('div')
  container.className = 'relative'
  const dom = document.createElement('div')
  container.append(dom)
  document.body.append(container)
  const editor = {
    isFocused: false,
    state: { selection: { from: 2, to: 8, head: 8 } },
    view: {
      dom,
      coordsAtPos: vi.fn(() => ({ top: 12, bottom: 28, left: 40, right: 40 })),
    },
    on: vi.fn((event: EditorEvent, listener: () => void) => {
      const handlers = listeners.get(event) ?? new Set()
      handlers.add(listener)
      listeners.set(event, handlers)
    }),
    off: vi.fn((event: EditorEvent, listener: () => void) => {
      listeners.get(event)?.delete(listener)
    }),
  }
  return {
    editor: editor as unknown as Editor,
    setFocused(focused: boolean) {
      editor.isFocused = focused
      for (const listener of listeners.get(focused ? 'focus' : 'blur') ?? []) listener()
    },
    cleanup: () => container.remove(),
  }
}

describe('SelectionToolbar', () => {
  beforeEach(() => vi.clearAllMocks())

  it('hides a retained ProseMirror selection after the editor loses focus', () => {
    const fixture = createEditor()
    const { unmount } = render(
      <SelectionToolbar editor={fixture.editor} mode="comment" onAction={vi.fn()} />,
    )

    expect(screen.queryByRole('button', { name: '添加评论' })).not.toBeInTheDocument()
    act(() => fixture.setFocused(true))
    expect(screen.getByRole('button', { name: '添加评论' })).toBeInTheDocument()
    act(() => fixture.setFocused(false))
    expect(screen.queryByRole('button', { name: '添加评论' })).not.toBeInTheDocument()

    unmount()
    fixture.cleanup()
  })

  it('keeps the editor selection active while invoking the toolbar action', () => {
    const fixture = createEditor()
    const onAction = vi.fn()
    fixture.setFocused(true)
    const { unmount } = render(
      <SelectionToolbar editor={fixture.editor} mode="comment" onAction={onAction} />,
    )
    const button = screen.getByRole('button', { name: '添加评论' })

    expect(fireEvent.pointerDown(button)).toBe(false)
    fireEvent.click(button)
    expect(onAction).toHaveBeenCalledOnce()

    unmount()
    fixture.cleanup()
  })
})
