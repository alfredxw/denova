import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MarkdownEditor } from './MarkdownEditor'

const tiptapMock = vi.hoisted(() => {
  const handlers = new Map<string, Set<(...args: unknown[]) => void>>()
  const editor = {
    commands: {
      setContent: vi.fn(),
      focus: vi.fn(),
    },
    storage: {
      characterCount: {
        characters: () => 0,
      },
    },
    state: {
      doc: { textContent: '' },
      selection: { from: 0, to: 0, empty: true },
      tr: { setMeta: vi.fn() },
    },
    view: {
      dispatch: vi.fn(),
      dom: document.createElement('div'),
    },
    isDestroyed: false,
    getText: () => tiptapMock.text,
    getMarkdown: () => tiptapMock.markdown,
    getHTML: () => '',
    on: vi.fn((event: string, handler: (...args: unknown[]) => void) => {
      const set = handlers.get(event) ?? new Set()
      set.add(handler)
      handlers.set(event, set)
    }),
    off: vi.fn((event: string, handler: (...args: unknown[]) => void) => {
      handlers.get(event)?.delete(handler)
    }),
  }
  return {
    editor,
    handlers,
    markdown: '',
    text: '',
    emit(event: string) {
      handlers.get(event)?.forEach((handler) => handler())
    },
    reset() {
      handlers.clear()
      this.markdown = ''
      this.text = ''
      vi.clearAllMocks()
    },
  }
})

vi.mock('@tiptap/react', () => ({
  EditorContent: () => <div data-testid="editor-content" />,
  useEditor: () => tiptapMock.editor,
}))

vi.mock('@tiptap/starter-kit', () => ({ default: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-character-count', () => ({ CharacterCount: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-placeholder', () => ({ default: { configure: () => ({}) } }))
vi.mock('@tiptap/markdown', () => ({ Markdown: { configure: () => ({}) } }))

describe('MarkdownEditor', () => {
  beforeEach(() => {
    vi.useRealTimers()
    tiptapMock.reset()
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it('打开编辑器设置 Popover 后展示行间距和背景主题', async () => {
    const user = userEvent.setup()

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="第一章"
        onSave={vi.fn()}
      />,
    )

    await user.click(screen.getByRole('button', { name: '编辑器设置' }))

    expect(screen.getByText('编辑器设置')).toBeInTheDocument()
    expect(screen.getByText('行间距')).toBeInTheDocument()
    expect(screen.getByText('背景主题')).toBeInTheDocument()
  })

  it('自动保存进行中继续编辑时串行保存最新内容，避免旧请求晚返回覆盖新内容', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<boolean>()
    const onSave = vi.fn((content: string) => content === '第一版' ? firstSave.promise : Promise.resolve(true))

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="初始"
        onSave={onSave}
      />,
    )

    act(() => {
      tiptapMock.markdown = '第一版'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(1200)
    })

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenLastCalledWith('第一版\n')

    act(() => {
      tiptapMock.markdown = '第二版'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(1200)
    })

    expect(onSave).toHaveBeenCalledTimes(1)

    await act(async () => {
      firstSave.resolve(true)
      await firstSave.promise
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledTimes(2)
    expect(onSave).toHaveBeenLastCalledWith('第二版\n')
  })
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}
