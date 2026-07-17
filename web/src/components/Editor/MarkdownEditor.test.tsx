import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MarkdownEditor } from './MarkdownEditor'
import { server } from '@/test/msw/server'

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
}))

const editorStateMock = vi.hoisted(() => ({ create: vi.fn((config: unknown) => config) }))

const tiptapMock = vi.hoisted(() => {
  const handlers = new Map<string, Set<(...args: unknown[]) => void>>()
  const chainApi = {
    focus: vi.fn(() => chainApi),
    setMeta: vi.fn(() => chainApi),
    setContent: vi.fn(() => chainApi),
    insertContentAt: vi.fn(() => chainApi),
    run: vi.fn(() => true),
  }
  const editor = {
    commands: {
      setContent: vi.fn(),
      focus: vi.fn(),
    },
    chain: vi.fn(() => chainApi),
    storage: {
      characterCount: {
        characters: () => 0,
      },
    },
    state: {
      doc: {
        textContent: '',
        forEach: vi.fn(),
      },
      selection: { from: 0, to: 0, head: 0, empty: true },
      tr: { setMeta: vi.fn() },
    },
    view: {
      dispatch: vi.fn(),
      updateState: vi.fn(),
      dom: document.createElement('div'),
    },
    isDestroyed: false,
    setEditable: vi.fn(),
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
    chainApi,
    handlers,
    useEditorOptions: null as unknown,
    markdown: '',
    text: '',
    emit(event: string) {
      handlers.get(event)?.forEach((handler) => handler())
    },
    reset() {
      handlers.clear()
      this.useEditorOptions = null
      this.markdown = ''
      this.text = ''
      editor.state.selection = { from: 0, to: 0, head: 0, empty: true }
      editor.state.doc.forEach.mockReset()
      vi.clearAllMocks()
    },
  }
})

vi.mock('@tiptap/react', () => ({
  EditorContent: () => <div data-testid="editor-content" />,
  useEditor: (options: unknown) => {
    tiptapMock.useEditorOptions = options
    return tiptapMock.editor
  },
}))

vi.mock('@tiptap/pm/state', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tiptap/pm/state')>()
  return { ...actual, EditorState: editorStateMock }
})

vi.mock('@tiptap/starter-kit', () => ({ default: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-character-count', () => ({ CharacterCount: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-placeholder', () => ({ default: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-image', () => ({ default: { extend: () => ({ configure: () => ({}) }) } }))
vi.mock('@tiptap/extension-table', () => ({ TableKit: { configure: vi.fn((options) => ({ name: 'tableKit', options })) } }))
vi.mock('@tiptap/markdown', () => ({ Markdown: { configure: () => ({}) } }))
vi.mock('sonner', () => ({ toast: toastMock }))

describe('MarkdownEditor', () => {
  beforeEach(() => {
    vi.useRealTimers()
    window.localStorage.clear()
    tiptapMock.reset()
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it('打开编辑器设置 Popover 后展示行间距、对白高亮和背景主题', async () => {
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
    expect(screen.getByText('对白高亮')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '选择对白高亮颜色' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '选择色相' })).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: '十六进制颜色' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '恢复默认' })).toBeInTheDocument()
    expect(screen.getByText('背景主题')).toBeInTheDocument()
  })

  it('在更新时间右侧实时显示光标所在行号', () => {
    const onLineChange = vi.fn()
    tiptapMock.editor.state.doc.forEach.mockImplementation((callback) => {
      callback({ nodeSize: 3 }, 0)
      callback({ nodeSize: 3 }, 3)
      callback({ nodeSize: 3 }, 6)
    })

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="第一行\n\n第二行\n\n第三行"
        onSave={vi.fn()}
        onLineChange={onLineChange}
        chapterSummary={{
          path: 'chapters/ch01.md',
          file_name: 'ch01.md',
          display_title: '第一章',
          index: 1,
          words: 10,
          status: 'draft',
          confirmed: false,
          updated_at: '2026-07-11 22:00',
          volume: '',
          volume_path: '',
        }}
      />,
    )

    expect(onLineChange).toHaveBeenLastCalledWith(1)

    act(() => {
      tiptapMock.editor.state.selection = { from: 7, to: 7, head: 7, empty: true }
      tiptapMock.emit('selectionUpdate')
    })

    expect(onLineChange).toHaveBeenLastCalledWith(3)
    expect(document.querySelector('.nova-editor-statusbar')).not.toBeInTheDocument()
  })

  it('注册 TipTap table 扩展以展示 GFM Markdown 表格', () => {
    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content={'| 角色 | 状态 |\n| --- | --- |\n| 阿宁 | 待命 |'}
        onSave={vi.fn()}
      />,
    )

    const options = tiptapMock.useEditorOptions as { extensions?: Array<{ name?: string; options?: unknown }> }
    expect(options.extensions).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          name: 'tableKit',
          options: { table: { resizable: false } },
        }),
      ]),
    )
  })

  it('默认对白高亮跟随编辑器背景主题变化，手动颜色优先', async () => {
    const user = userEvent.setup()

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="“第一句对白。”"
        onSave={vi.fn()}
      />,
    )

    const editorContainer = screen.getByTestId('editor-content').parentElement
    expect(editorContainer).toHaveStyle('--nova-editor-dialogue-highlight: var(--nova-dialogue-highlight)')

    await user.click(screen.getByRole('button', { name: '编辑器设置' }))
    await user.click(screen.getByRole('button', { name: /纸张/ }))

    expect(editorContainer).toHaveStyle('--nova-editor-dialogue-highlight: #8a3f13')
    expect(screen.getByRole('textbox', { name: '十六进制颜色' })).toHaveValue('#8a3f13')

    fireEvent.change(screen.getByRole('textbox', { name: '十六进制颜色' }), { target: { value: '#336699' } })

    expect(editorContainer).toHaveStyle('--nova-editor-dialogue-highlight: #336699')
    expect(screen.getByRole('textbox', { name: '十六进制颜色' })).toHaveValue('#336699')

    await user.click(screen.getByRole('button', { name: '恢复默认' }))

    expect(editorContainer).toHaveStyle('--nova-editor-dialogue-highlight: #8a3f13')
    expect(screen.getByRole('textbox', { name: '十六进制颜色' })).toHaveValue('#8a3f13')
  })

  it('自动保存进行中继续编辑时串行保存最新内容，避免旧请求晚返回覆盖新内容', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<boolean>()
    const onSave = vi.fn((_path: string, content: string) => content === '第一版\n' ? firstSave.promise : Promise.resolve(true))

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="初始"
        onSave={onSave}
        autoSaveDelayMs={1200}
      />,
    )

    act(() => {
      tiptapMock.markdown = '第一版'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(1200)
    })

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenLastCalledWith('chapters/ch01.md', '第一版\n')

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
    expect(onSave).toHaveBeenLastCalledWith('chapters/ch01.md', '第二版\n')
  })

  it('切换 workspace 后丢弃旧工作区中尚未执行的保存', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<boolean>()
    const saveWorkspaceA = vi.fn(() => firstSave.promise)
    const saveWorkspaceB = vi.fn(() => Promise.resolve(true))
    const { rerender } = render(
      <MarkdownEditor workspace="/books/a" fileName="chapters/ch01.md" content="初始" onSave={saveWorkspaceA} autoSaveDelayMs={100} />,
    )

    act(() => {
      tiptapMock.markdown = '第一版'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
    })
    act(() => {
      tiptapMock.markdown = '第二版'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
    })
    expect(saveWorkspaceA).toHaveBeenCalledTimes(1)

    rerender(
      <MarkdownEditor workspace="/books/b" fileName="chapters/ch01.md" content="B 工作区" onSave={saveWorkspaceB} autoSaveDelayMs={100} />,
    )

    await act(async () => {
      firstSave.resolve(true)
      await firstSave.promise
      await Promise.resolve()
    })

    expect(saveWorkspaceA).toHaveBeenCalledTimes(1)
    expect(saveWorkspaceB).not.toHaveBeenCalled()
  })

  it('切换文件时为排队中的自动保存保留各自的目标文件', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<boolean>()
    const saveOutline = vi.fn(() => firstSave.promise)
    const saveProgress = vi.fn(() => Promise.resolve(true))
    const { rerender } = render(
      <MarkdownEditor
        fileName="setting/outline.md"
        content="大纲初始内容"
        onSave={saveOutline}
        autoSaveDelayMs={1200}
      />,
    )

    act(() => {
      tiptapMock.markdown = '大纲修改后'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(1200)
    })

    expect(saveOutline).toHaveBeenCalledWith('setting/outline.md', '大纲修改后\n')

    rerender(
      <MarkdownEditor
        fileName="setting/progress.md"
        content="进度初始内容"
        onSave={saveProgress}
        autoSaveDelayMs={1200}
      />,
    )

    act(() => {
      tiptapMock.markdown = '进度修改后'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(1200)
    })

    expect(saveProgress).not.toHaveBeenCalled()

    await act(async () => {
      firstSave.resolve(true)
      await firstSave.promise
      await Promise.resolve()
    })

    expect(saveOutline).toHaveBeenCalledTimes(1)
    expect(saveProgress).toHaveBeenCalledTimes(1)
    expect(saveProgress).toHaveBeenCalledWith('setting/progress.md', '进度修改后\n')
  })

  it('自动保存延迟期间切换文件会立即保存旧文件草稿', async () => {
    vi.useFakeTimers()
    const onSave = vi.fn(() => Promise.resolve(true))
    const { rerender } = render(
      <MarkdownEditor
        fileName="setting/outline.md"
        content="大纲初始内容"
        onSave={onSave}
        autoSaveDelayMs={1200}
      />,
    )

    act(() => {
      tiptapMock.markdown = '大纲尚未到保存时间'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(600)
    })

    rerender(
      <MarkdownEditor
        fileName="setting/progress.md"
        content="进度初始内容"
        onSave={onSave}
        autoSaveDelayMs={1200}
      />,
    )

    await act(async () => {
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenCalledWith('setting/outline.md', '大纲尚未到保存时间\n')
  })

  it('用户修改后按配置延迟自动保存，不按周期重复保存', async () => {
    vi.useFakeTimers()
    const onSave = vi.fn(() => Promise.resolve(true))

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="初始"
        onSave={onSave}
        autoSaveDelayMs={900}
      />,
    )

    act(() => {
      tiptapMock.markdown = '修改后'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(899)
    })

    expect(onSave).not.toHaveBeenCalled()

    await act(async () => {
      vi.advanceTimersByTime(1)
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(onSave).toHaveBeenLastCalledWith('chapters/ch01.md', '修改后\n')

    await act(async () => {
      vi.advanceTimersByTime(5000)
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledTimes(1)
  })

  it('手动保存成功只更新编辑器保存状态，不弹出成功 toast', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn(() => Promise.resolve(true))

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="初始"
        onSave={onSave}
      />,
    )

    act(() => {
      tiptapMock.markdown = '修改后'
      tiptapMock.emit('update')
    })

    await user.click(screen.getByRole('button', { name: '保存' }))

    expect(onSave).toHaveBeenCalledWith('chapters/ch01.md', '修改后\n')
    expect(toastMock.success).not.toHaveBeenCalled()
  })

  it('关闭自动保存后用户修改不会自动写入文件', () => {
    vi.useFakeTimers()
    const onSave = vi.fn(() => Promise.resolve(true))

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="初始"
        onSave={onSave}
        autoSaveEnabled={false}
      />,
    )

    act(() => {
      tiptapMock.markdown = '未自动保存'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(10000)
    })

    expect(onSave).not.toHaveBeenCalled()
  })

  it('关闭自动保存后导航 flush 仍会等待草稿保存', async () => {
    const onSave = vi.fn(() => Promise.resolve(true))
    let flush: (() => Promise<boolean>) | null = null
    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="初始"
        onSave={onSave}
        autoSaveEnabled={false}
        onFlushHandlerChange={(handler) => { flush = handler }}
      />,
    )

    act(() => {
      tiptapMock.markdown = '导航前草稿'
      tiptapMock.emit('update')
    })
    let saved = false
    await act(async () => {
      saved = await flush!()
    })

    expect(saved).toBe(true)
    expect(onSave).toHaveBeenCalledWith('chapters/ch01.md', '导航前草稿\n')
  })

  it('关闭自动保存后直接切换文件仍会保存旧文件草稿', async () => {
    const onSave = vi.fn(() => Promise.resolve(true))
    const { rerender } = render(
      <MarkdownEditor fileName="chapters/ch01.md" content="第一章" onSave={onSave} autoSaveEnabled={false} />,
    )

    act(() => {
      tiptapMock.markdown = '第一章未保存草稿'
      tiptapMock.emit('update')
    })
    rerender(<MarkdownEditor fileName="data/state.json" content="{}" onSave={onSave} autoSaveEnabled={false} />)
    await act(async () => {
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledWith('chapters/ch01.md', '第一章未保存草稿\n')
  })

  it('编辑器卸载时兜底保存尚未 flush 的草稿', async () => {
    const onSave = vi.fn(() => Promise.resolve(true))
    const { unmount } = render(
      <MarkdownEditor fileName="chapters/ch01.md" content="第一章" onSave={onSave} autoSaveEnabled={false} />,
    )

    act(() => {
      tiptapMock.markdown = '关闭 Tab 前草稿'
      tiptapMock.emit('update')
    })
    unmount()
    await act(async () => {
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledWith('chapters/ch01.md', '关闭 Tab 前草稿\n')
  })

  it('外部内容同步不会触发自动保存', () => {
    vi.useFakeTimers()
    const onSave = vi.fn(() => Promise.resolve(true))
    const { rerender } = render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="初始"
        onSave={onSave}
        autoSaveDelayMs={900}
      />,
    )

    rerender(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="Agent 写入的新内容"
        onSave={onSave}
        autoSaveDelayMs={900}
      />,
    )

    act(() => {
      vi.advanceTimersByTime(5000)
    })

    expect(onSave).not.toHaveBeenCalled()
    expect(tiptapMock.chainApi.setMeta).toHaveBeenLastCalledWith('addToHistory', false)
    expect(tiptapMock.chainApi.setContent).toHaveBeenLastCalledWith(
      'Agent 写入的新内容',
      { emitUpdate: false, contentType: 'markdown' },
    )
  })

  it('本地草稿未保存时保留内容并提示外部更新冲突', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn(() => Promise.resolve(true))
    const { rerender } = render(
      <MarkdownEditor fileName="chapters/ch01.md" content="初始" onSave={onSave} autoSaveEnabled={false} />,
    )
    tiptapMock.chainApi.setContent.mockClear()

    act(() => {
      tiptapMock.markdown = '本地草稿'
      tiptapMock.emit('update')
    })
    rerender(<MarkdownEditor fileName="chapters/ch01.md" content="Agent 新版本" onSave={onSave} autoSaveEnabled={false} />)

    expect(screen.getByRole('alert')).toHaveTextContent('工作区版本已发生变化')
    expect(tiptapMock.chainApi.setContent).not.toHaveBeenCalledWith('Agent 新版本', expect.anything())

    await user.click(screen.getByRole('button', { name: '载入工作区版本' }))

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(tiptapMock.chainApi.setMeta).toHaveBeenLastCalledWith('addToHistory', false)
    expect(tiptapMock.chainApi.setContent).toHaveBeenLastCalledWith('Agent 新版本', { emitUpdate: false, contentType: 'markdown' })
  })

  it('保留本地版本会显式保存，失败时保留冲突提示以便重试', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn()
      .mockResolvedValueOnce(false)
      .mockResolvedValueOnce(true)
    const { rerender } = render(
      <MarkdownEditor workspace="/books/demo" fileName="chapters/ch01.md" content="初始" onSave={onSave} autoSaveEnabled={false} />,
    )

    act(() => {
      tiptapMock.markdown = '本地草稿'
      tiptapMock.emit('update')
    })
    rerender(<MarkdownEditor workspace="/books/demo" fileName="chapters/ch01.md" content="Agent 新版本" onSave={onSave} autoSaveEnabled={false} />)

    await user.click(screen.getByRole('button', { name: '保留草稿并覆盖' }))
    expect(onSave).toHaveBeenLastCalledWith('chapters/ch01.md', '本地草稿\n')
    expect(screen.getByRole('alert')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '保留草稿并覆盖' }))
    expect(onSave).toHaveBeenCalledTimes(2)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('切换文件时清空本地历史，外部同步事务不进入 undo 栈', () => {
    const { rerender } = render(
      <MarkdownEditor fileName="chapters/ch01.md" content="第一章" onSave={vi.fn()} />,
    )
    tiptapMock.chainApi.setMeta.mockClear()
    editorStateMock.create.mockClear()

    rerender(<MarkdownEditor fileName="chapters/ch01.md" content="Agent 修改第一章" onSave={vi.fn()} />)
    expect(tiptapMock.chainApi.setMeta).toHaveBeenLastCalledWith('addToHistory', false)
    expect(editorStateMock.create).toHaveBeenCalledTimes(1)

    rerender(<MarkdownEditor fileName="chapters/ch02.md" content="第二章" onSave={vi.fn()} />)
    expect(editorStateMock.create).toHaveBeenCalledTimes(2)
    expect(tiptapMock.editor.view.updateState).toHaveBeenCalled()
  })

  it('点击生成本章插画按钮时提交当前章节路径', async () => {
    const user = userEvent.setup()
    const onGenerateIllustration = vi.fn()

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="第一章"
        onSave={vi.fn()}
        chapterSummary={{
          path: 'chapters/ch01.md',
          file_name: 'ch01.md',
          display_title: '第一章',
          index: 1,
          words: 100,
          status: 'draft',
          confirmed: false,
          updated_at: '',
          volume: '',
          volume_path: '',
        }}
        onGenerateIllustration={onGenerateIllustration}
      />,
    )

    await user.click(screen.getByRole('button', { name: '生成本章插画' }))

    expect(onGenerateIllustration).toHaveBeenCalledWith('chapters/ch01.md')
  })

  it('插入插画 signal 时向 Markdown 文档插入 image node', async () => {
    tiptapMock.editor.state.selection = { from: 5, to: 5, head: 5, empty: true }

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="第一章"
        onSave={vi.fn()}
        illustrationInsertSignal={{
          nonce: 1,
          illustration: {
            schema: 'chapter_illustration.v1',
            chapter_path: 'chapters/ch01.md',
            image_path: 'assets/illustrations/ch01/run/image.png',
            meta_path: 'assets/illustrations/ch01/run/meta.json',
            markdown: '![雨夜](assets/illustrations/ch01/run/image.png)',
            alt_text: '雨夜',
            profile_id: 'default',
            provider: 'openai',
            model: 'gpt-image-1',
          },
        }}
      />,
    )

    expect(tiptapMock.chainApi.insertContentAt).toHaveBeenCalledWith(5, {
      type: 'image',
      attrs: {
        src: 'assets/illustrations/ch01/run/image.png',
        alt: '雨夜',
        title: '雨夜',
      },
    })
    expect(tiptapMock.chainApi.run).toHaveBeenCalled()
    expect(toastMock.success).not.toHaveBeenCalled()
  })

  it('保存并校验规范正文后进入只读审阅模式，再显式返回编辑模式', async () => {
    const user = userEvent.setup()
    tiptapMock.markdown = '正文\n'
    const documentReview = {
      comments: [],
      onCreate: vi.fn(),
      onUpdate: vi.fn(),
      onDelete: vi.fn(),
    }
    server.use(http.get('/api/workspace/file', ({ request }) => {
      expect(new URL(request.url).searchParams.get('path')).toBe('chapters/ch01.md')
      return HttpResponse.json({ workspace: '/books/demo', path: 'chapters/ch01.md', content: '正文\n', revision: 'sha256:body' })
    }))

    const { rerender } = render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content={'正文\n'}
        onSave={vi.fn().mockResolvedValue(true)}
        documentReview={documentReview}
      />,
    )

    await user.click(screen.getByRole('button', { name: '审阅' }))
    await waitFor(() => expect(screen.getByRole('button', { name: '审阅' })).toHaveAttribute('aria-pressed', 'true'))
    expect(tiptapMock.editor.view.dom).toHaveAttribute('aria-readonly', 'true')
    expect(tiptapMock.editor.setEditable).toHaveBeenLastCalledWith(true)

    await user.click(screen.getByRole('button', { name: '编辑' }))
    expect(tiptapMock.editor.view.dom).not.toHaveAttribute('aria-readonly')
    expect(tiptapMock.editor.setEditable).toHaveBeenLastCalledWith(true)

    await user.click(screen.getByRole('button', { name: '审阅' }))
    await waitFor(() => expect(screen.getByRole('button', { name: '审阅' })).toHaveAttribute('aria-pressed', 'true'))
    rerender(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch02.md"
        content={'第二章\n'}
        onSave={vi.fn().mockResolvedValue(true)}
        documentReview={documentReview}
      />,
    )

    await waitFor(() => expect(screen.getByRole('button', { name: '编辑' })).toHaveAttribute('aria-pressed', 'true'))
    expect(tiptapMock.editor.view.dom).not.toHaveAttribute('aria-readonly')
    expect(tiptapMock.chainApi.setContent).toHaveBeenLastCalledWith('第二章\n', { emitUpdate: false, contentType: 'markdown' })
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
