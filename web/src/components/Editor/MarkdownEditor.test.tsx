import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { WorkspaceFileRevisionConflictError } from '@/lib/autosave/workspace-file-revision-conflict'
import { WritingDocumentEditor as ProjectMarkdownEditor } from './WritingDocumentEditor'

function MarkdownEditor({ workspace, ...props }: Omit<ComponentProps<typeof ProjectMarkdownEditor>, 'projectId'> & { projectId?: string; workspace?: string }) {
  return <ProjectMarkdownEditor {...props} projectId={props.projectId || workspace || 'project-editor-test'} />
}

const toastMock = vi.hoisted(() => ({
  error: vi.fn(),
  success: vi.fn(),
}))

const editorStateMock = vi.hoisted(() => ({ create: vi.fn((config: unknown) => config) }))
const workspaceApiMock = vi.hoisted(() => ({ readFile: vi.fn() }))
const documentReviewAnnotationsMock = vi.hoisted(() => ({
  prepareSnapshot: null as null | (() => Promise<{ content: string; revision: string }>),
  revealComment: vi.fn(),
}))
const conflictArchiveMock = vi.hoisted(() => ({
  preserve: vi.fn().mockResolvedValue({ id: 'conflict-1', path: '/conflicts/conflict-1.json', storage: 'server' }),
}))

const tiptapMock = vi.hoisted(() => {
  const handlers = new Map<string, Set<(...args: unknown[]) => void>>()
  const chainApi = {
    focus: vi.fn(() => chainApi),
    setMeta: vi.fn(() => chainApi),
    setContent: vi.fn(() => chainApi),
    insertContentAt: vi.fn(() => chainApi),
    run: vi.fn(() => true),
  }
  const schema = {
    nodeFromJSON: vi.fn((json: unknown) => ({
      json,
      content: { size: 100 },
      textContent: '',
      forEach: vi.fn(),
    })),
  }
  const state = {
    schema,
    plugins: [],
    doc: {
      content: { size: 100 },
      textContent: '',
      forEach: vi.fn(),
      descendants: vi.fn(),
    },
    selection: { from: 0, to: 0, head: 0, empty: true },
    tr: {} as {
      setMeta: ReturnType<typeof vi.fn>
      replaceWith: ReturnType<typeof vi.fn>
    },
  }
  state.tr = {
    setMeta: vi.fn(),
    replaceWith: vi.fn((_from: number, _to: number, doc: unknown) => ({
      doc,
      selection: state.selection,
    })),
  }
  const editor = {
    commands: {
      setContent: vi.fn(),
      focus: vi.fn(),
      setTextSelection: vi.fn(),
    },
    chain: vi.fn(() => chainApi),
    storage: {
      characterCount: {
        characters: vi.fn(() => 0),
      },
    },
    schema,
    markdown: {
      parse: vi.fn((source: string) => ({
        type: 'doc',
        content: [{
          type: 'paragraph',
          content: source ? [{ type: 'text', text: source }] : undefined,
        }],
      })),
    },
    state,
    view: {
      dispatch: vi.fn(),
      updateState: vi.fn(),
      dom: document.createElement('div'),
      hasFocus: vi.fn(() => false),
    },
    isDestroyed: false,
    setEditable: vi.fn(),
    getText: () => tiptapMock.text,
    getMarkdown: () => tiptapMock.markdown,
    getJSON: () => tiptapMock.json,
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
    created: false,
    json: { type: 'doc', content: [{ type: 'paragraph' }] },
    markdown: '',
    text: '',
    emit(event: string) {
      handlers.get(event)?.forEach((handler) => handler())
    },
    reset() {
      handlers.clear()
      this.useEditorOptions = null
      this.created = false
      this.json = { type: 'doc', content: [{ type: 'paragraph' }] }
      this.markdown = ''
      this.text = ''
      editor.state.selection = { from: 0, to: 0, head: 0, empty: true }
      editor.state.doc.forEach.mockReset()
      editor.state.doc.descendants.mockReset()
      vi.clearAllMocks()
      editor.view.hasFocus.mockReturnValue(false)
      editor.storage.characterCount.characters.mockReturnValue(0)
    },
  }
})

vi.mock('@tiptap/react', () => ({
  EditorContent: ({ className }: { className?: string }) => <div data-testid="editor-content" className={className} />,
  useEditor: (options: { onCreate?: (payload: { editor: typeof tiptapMock.editor }) => void }) => {
    tiptapMock.useEditorOptions = options
    if (!tiptapMock.created) {
      tiptapMock.created = true
      options.onCreate?.({ editor: tiptapMock.editor })
    }
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
vi.mock('@/lib/api-client/workspace', () => ({
  MISSING_WORKSPACE_REVISION: 'missing',
}))
vi.mock('@/lib/api-client/project-files', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api-client/project-files')>()
  return {
    ...actual,
    readProjectFile: async (projectId: string, path: string) => ({
      ...await workspaceApiMock.readFile(path),
      project_id: projectId,
    }),
  }
})
vi.mock('@/lib/api-client/autosave-conflicts', () => ({ preserveAutosaveConflict: conflictArchiveMock.preserve }))
vi.mock('@/features/files/ProjectSourceEditor', async () => {
  const { forwardRef, useImperativeHandle, useRef, useState } = await import('react')
  return {
    ProjectTextEditor: forwardRef<unknown, {
      document: { path: string }
      value: string
      wordWrap: boolean
      onChange: (value: string) => void
    }>((props, ref) => {
      const [value, setValue] = useState(props.value)
      const valueRef = useRef(value)
      valueRef.current = value
      useImperativeHandle(ref, () => ({
        getValue: () => valueRef.current,
        replaceValue: (nextValue: string) => {
          valueRef.current = nextValue
          setValue(nextValue)
        },
        revealLine: vi.fn(),
        focus: vi.fn(),
      }))
      return (
        <textarea
          aria-label={`source:${props.document.path}`}
          data-word-wrap={String(props.wordWrap)}
          value={value}
          onChange={(event) => {
            valueRef.current = event.target.value
            setValue(event.target.value)
            props.onChange(event.target.value)
          }}
        />
      )
    }),
  }
})
vi.mock('./DocumentReviewAnnotations', async () => {
  const { forwardRef, useImperativeHandle } = await import('react')
  return {
    DocumentReviewAnnotations: forwardRef<unknown, { onPrepareSnapshot: () => Promise<{ content: string; revision: string }> }>((props, ref) => {
      documentReviewAnnotationsMock.prepareSnapshot = props.onPrepareSnapshot
      useImperativeHandle(ref, () => ({
        revealComment: documentReviewAnnotationsMock.revealComment,
        startSelectionComment: vi.fn(),
      }))
      return null
    }),
  }
})

describe('MarkdownEditor', () => {
  beforeEach(() => {
    vi.useRealTimers()
    window.localStorage.clear()
    tiptapMock.reset()
    workspaceApiMock.readFile.mockReset()
    conflictArchiveMock.preserve.mockReset().mockResolvedValue({ id: 'conflict-1', path: '/conflicts/conflict-1.json', storage: 'server' })
    documentReviewAnnotationsMock.prepareSnapshot = null
    documentReviewAnnotationsMock.revealComment.mockReset().mockReturnValue(true)
  })

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it('在编辑器设置中保留字体和字号配置', async () => {
    const user = userEvent.setup()
    const onFontFamilyChange = vi.fn()
    const onFontSizeChange = vi.fn()

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="第一章"
        onSave={vi.fn()}
        readingTypography={{
          fontFamily: 'source-han-serif',
          fontSize: 18,
          loading: false,
          status: 'saved',
          onFontFamilyChange,
          onFontSizeChange,
          onRetry: vi.fn(),
        }}
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
    expect(screen.getByText('阅读字体')).toBeInTheDocument()
    expect(screen.getByText('阅读字号 (px)')).toBeInTheDocument()
    expect(screen.queryByText('字体与字号')).not.toBeInTheDocument()
    expect(screen.queryByText('与全局外观设置共享，修改后自动保存')).not.toBeInTheDocument()
    expect(screen.getByText('思源宋体阅读')).toBeInTheDocument()
    const readingFontSize = screen.getByRole('slider', { name: '阅读字号 (px)' })
    expect(readingFontSize).toHaveValue('18')
    fireEvent.change(readingFontSize, { target: { value: '22' } })
    expect(onFontSizeChange).toHaveBeenCalledWith(22)
    expect(screen.queryByText('行号')).not.toBeInTheDocument()
  })

  it('在章节标题栏实时显示当前章节字数和光标行号，但不显示更新时间', () => {
    tiptapMock.editor.storage.characterCount.characters.mockReturnValue(10)

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="第一行\n\n第二行\n\n第三行"
        onSave={vi.fn()}
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

    expect(screen.getByText('10 字')).toBeInTheDocument()
    expect(screen.queryByText(/更新/)).not.toBeInTheDocument()
    expect(screen.queryByText('行 1')).not.toBeInTheDocument()

    act(() => {
      tiptapMock.emit('focus')
    })
    expect(screen.getByText('行 1')).toBeInTheDocument()

    tiptapMock.editor.storage.characterCount.characters.mockReturnValue(11)
    tiptapMock.editor.state.doc.forEach.mockImplementation((callback) => {
      callback({ nodeSize: 3 }, 0)
      callback({ nodeSize: 3 }, 3)
      callback({ nodeSize: 3 }, 6)
    })
    act(() => {
      tiptapMock.editor.state.selection = { from: 7, to: 7, head: 7, empty: true }
      tiptapMock.emit('update')
    })

    expect(screen.getByText('11 字')).toBeInTheDocument()
    expect(screen.getByText('行 3')).toBeInTheDocument()

    act(() => {
      tiptapMock.emit('blur')
    })
    expect(screen.queryByText('行 3')).not.toBeInTheDocument()
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

  it('注册点击光标定位处理器', () => {
    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content="第一章"
        onSave={vi.fn()}
      />,
    )

    const options = tiptapMock.useEditorOptions as { editorProps?: { handleClick?: unknown } }
    expect(options.editorProps?.handleClick).toBeTypeOf('function')
  })

  it('在文档与 Monaco 源码间切换时共享未保存草稿且不触发保存', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn()
    tiptapMock.markdown = '## 标题\n\n正文'

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content={'## 标题\n\n正文\n'}
        onSave={onSave}
        autoSaveEnabled={false}
      />,
    )

    act(() => tiptapMock.emit('update'))
    await user.click(screen.getByRole('radio', { name: '源码' }))
    const source = screen.getByRole('textbox', { name: 'source:chapters/ch01.md' })
    expect(source).toHaveValue('## 标题\n\n正文\n')

    const exactSource = '---\ntitle: 示例\n---\n\n# 标题  \n'
    fireEvent.change(source, { target: { value: exactSource } })
    await user.click(screen.getByRole('radio', { name: '文档' }))
    expect(tiptapMock.editor.markdown.parse).toHaveBeenCalledWith(exactSource)

    tiptapMock.markdown = exactSource
    await user.click(screen.getByRole('radio', { name: '源码' }))
    expect(screen.getByRole('textbox', { name: 'source:chapters/ch01.md' })).toHaveValue(exactSource)
    expect(onSave).not.toHaveBeenCalled()
  })

  it('从 Monaco 源码模式自动保存完全一致的 Markdown', async () => {
    const user = userEvent.setup()
    const onSave = vi.fn().mockResolvedValue({ revision: 'r2' })
    tiptapMock.markdown = '# 初始'

    render(
      <MarkdownEditor
        fileName="chapters/ch01.md"
        content={'# 初始\n'}
        revision="r1"
        onSave={onSave}
        autoSaveDelayMs={1}
      />,
    )

    await user.click(screen.getByRole('radio', { name: '源码' }))
    const exactSource = '# 标题  \n\n```ts\nconst value = 1\n```\n'
    fireEvent.change(screen.getByRole('textbox', { name: 'source:chapters/ch01.md' }), {
      target: { value: exactSource },
    })

    await waitFor(() => expect(onSave).toHaveBeenCalledWith('chapters/ch01.md', exactSource, 'r1'))
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
    expect(onSave).toHaveBeenLastCalledWith('chapters/ch01.md', '第一版\n', '')

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
    expect(onSave).toHaveBeenLastCalledWith('chapters/ch01.md', '第二版\n', '')
  })

  it('旧快照保存完成时新草稿仍等待自动保存，状态继续显示未保存', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<{ revision: string }>()
    const onSave = vi.fn(() => firstSave.promise)

    render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content="初始"
        revision="r1"
        onSave={onSave}
        autoSaveDelayMs={100}
      />,
    )

    act(() => {
      tiptapMock.markdown = '第一版'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
      tiptapMock.markdown = '第二版'
      tiptapMock.emit('update')
    })

    await act(async () => {
      firstSave.resolve({ revision: 'r2' })
      await firstSave.promise
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(screen.getByLabelText('内容有未保存改动')).toBeInTheDocument()
  })

  it('文件从磁盘删除后保留内容并暂停自动保存，手动保存使用 missing revision 重建', async () => {
    vi.useFakeTimers()
    const onSave = vi.fn().mockResolvedValue({ revision: 'r2' })
    const { rerender } = render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content="保留内容"
        revision="r1"
        onSave={onSave}
        autoSaveDelayMs={100}
      />,
    )

    rerender(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content="保留内容"
        revision="missing"
        onSave={onSave}
        autoSaveDelayMs={100}
      />,
    )
    expect(screen.getByRole('alert')).toHaveTextContent('文件已从磁盘删除')

    act(() => {
      tiptapMock.markdown = '删除后继续编辑'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
    })
    expect(onSave).not.toHaveBeenCalled()

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: '保存' }))
      await Promise.resolve()
    })
    expect(onSave).toHaveBeenCalledWith('chapters/ch01.md', '删除后继续编辑\n', 'missing')
  })

  it('前一次保存返回新 revision 后，排队的编辑在真正发送时沿用该 revision', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<{ revision: string }>()
    const onSave = vi.fn((_path: string, content: string) => (
      content === '第一版\n' ? firstSave.promise : Promise.resolve({ revision: 'r3' })
    ))

    render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content="初始"
        revision="r1"
        onSave={onSave}
        autoSaveDelayMs={100}
      />,
    )
    act(() => {
      tiptapMock.markdown = '第一版'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
      tiptapMock.markdown = '第二版'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
    })
    expect(onSave).toHaveBeenCalledTimes(1)

    await act(async () => {
      firstSave.resolve({ revision: 'r2' })
      await firstSave.promise
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledTimes(2)
    expect(onSave).toHaveBeenLastCalledWith('chapters/ch01.md', '第二版\n', 'r2')
    expect(conflictArchiveMock.preserve).not.toHaveBeenCalled()
  })

  it('保存中的旧快照先回灌时仍保留手动保存的新草稿，且不误报并发修改', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<{ revision: string }>()
    const secondSave = deferred<{ revision: string }>()
    const onSave = vi.fn((_path: string, content: string) => (
      content === '第一版\n' ? firstSave.promise : secondSave.promise
    ))
    const { rerender } = render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content="初始"
        revision="r1"
        onSave={onSave}
        autoSaveDelayMs={100}
      />,
    )

    act(() => {
      tiptapMock.markdown = '第一版'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
      tiptapMock.markdown = '第二版'
      tiptapMock.emit('update')
      fireEvent.click(screen.getByRole('button', { name: '保存' }))
    })
    expect(onSave).toHaveBeenCalledTimes(1)

    // useWorkspace publishes the acknowledged file snapshot before its async
    // save callback finishes (it still refreshes summary/version state).
    rerender(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content={'第一版\n'}
        revision="r2"
        onSave={onSave}
        autoSaveDelayMs={100}
      />,
    )

    await act(async () => {
      firstSave.resolve({ revision: 'r2' })
      await firstSave.promise
      await Promise.resolve()
    })

    expect(conflictArchiveMock.preserve).not.toHaveBeenCalled()
    expect(onSave).toHaveBeenCalledTimes(2)
    expect(onSave).toHaveBeenLastCalledWith('chapters/ch01.md', '第二版\n', 'r2')
    expect(screen.getByLabelText('正在保存')).toBeInTheDocument()

    await act(async () => {
      secondSave.resolve({ revision: 'r3' })
      await secondSave.promise
    })
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

    expect(saveOutline).toHaveBeenCalledWith('setting/outline.md', '大纲修改后\n', '')

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
    expect(saveProgress).toHaveBeenCalledWith('setting/progress.md', '进度修改后\n', '')
  })

  it('保存进行中连续切换多个文件时不会让后一个草稿覆盖中间文件', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<boolean>()
    const onSave = vi.fn((path: string) => (
      path === 'setting/a.md' ? firstSave.promise : Promise.resolve(true)
    ))
    const { rerender } = render(
      <MarkdownEditor
        fileName="setting/a.md"
        content="A 初始"
        onSave={onSave}
        autoSaveDelayMs={100}
      />,
    )

    act(() => {
      tiptapMock.markdown = 'A 修改后'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
    })
    expect(onSave).toHaveBeenCalledWith('setting/a.md', 'A 修改后\n', '')

    rerender(
      <MarkdownEditor fileName="setting/b.md" content="B 初始" onSave={onSave} autoSaveDelayMs={100} />,
    )
    act(() => {
      tiptapMock.markdown = 'B 修改后'
      tiptapMock.emit('update')
    })
    rerender(
      <MarkdownEditor fileName="setting/c.md" content="C 初始" onSave={onSave} autoSaveDelayMs={100} />,
    )
    act(() => {
      tiptapMock.markdown = 'C 修改后'
      tiptapMock.emit('update')
    })

    await act(async () => {
      firstSave.resolve(true)
      await firstSave.promise
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledTimes(3)
    expect(onSave).toHaveBeenCalledWith('setting/b.md', 'B 修改后\n', '')
    expect(onSave).toHaveBeenCalledWith('setting/c.md', 'C 修改后\n', '')
  })

  it('跨文档批次中一个后台草稿失败时仍保存后续文档并保留失败项重试', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<boolean>()
    let bAttempts = 0
    let flush: (() => Promise<boolean>) | null = null
    const onSave = vi.fn((path: string) => {
      if (path === 'setting/a.md') return firstSave.promise
      if (path === 'setting/b.md' && ++bAttempts === 1) return Promise.reject(new Error('B save failed'))
      return Promise.resolve(true)
    })
    const { rerender } = render(
      <MarkdownEditor
        fileName="setting/a.md"
        content="A 初始"
        onSave={onSave}
        autoSaveDelayMs={100}
        onFlushHandlerChange={(handler) => { flush = handler }}
      />,
    )

    act(() => {
      tiptapMock.markdown = 'A 修改后'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
    })
    rerender(
      <MarkdownEditor fileName="setting/b.md" content="B 初始" onSave={onSave} autoSaveDelayMs={100} onFlushHandlerChange={(handler) => { flush = handler }} />,
    )
    act(() => {
      tiptapMock.markdown = 'B 修改后'
      tiptapMock.emit('update')
    })
    rerender(
      <MarkdownEditor fileName="setting/c.md" content="C 初始" onSave={onSave} autoSaveDelayMs={100} onFlushHandlerChange={(handler) => { flush = handler }} />,
    )
    act(() => {
      tiptapMock.markdown = 'C 修改后'
      tiptapMock.emit('update')
    })

    await act(async () => {
      firstSave.resolve(true)
      await firstSave.promise
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledWith('setting/c.md', 'C 修改后\n', '')
    expect(toastMock.error).toHaveBeenCalled()

    let retried = false
    await act(async () => {
      retried = await flush!()
    })
    expect(retried).toBe(true)
    expect(bAttempts).toBe(2)
  })

  it('后台文档 revision 冲突时自行合并外部修改并继续保存后续文档', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<{ revision: string }>()
    const onSave = vi.fn((path: string, _content: string, baseRevision: string) => {
      if (path === 'setting/a.md') return firstSave.promise
      if (path === 'setting/b.md' && baseRevision === 'b1') {
        return Promise.reject(new WorkspaceFileRevisionConflictError(
          new Error('revision conflict'),
          {
            workspace: '/books/demo',
            content: 'B 基线第一行\nB 不变中间行\nB 外部第三行\n',
            revision: 'b2',
          },
        ))
      }
      return Promise.resolve({ revision: `${baseRevision}-saved` })
    })
    const { rerender } = render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="setting/a.md"
        content="A 初始"
        revision="a1"
        onSave={onSave}
        autoSaveDelayMs={100}
      />,
    )

    act(() => {
      tiptapMock.markdown = 'A 修改后'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
    })
    rerender(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="setting/b.md"
        content={'B 基线第一行\nB 不变中间行\nB 基线第三行\n'}
        revision="b1"
        onSave={onSave}
        autoSaveDelayMs={100}
      />,
    )
    act(() => {
      tiptapMock.markdown = 'B 本地第一行\nB 不变中间行\nB 基线第三行'
      tiptapMock.emit('update')
    })
    rerender(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="setting/c.md"
        content="C 初始"
        revision="c1"
        onSave={onSave}
        autoSaveDelayMs={100}
      />,
    )
    act(() => {
      tiptapMock.markdown = 'C 修改后'
      tiptapMock.emit('update')
    })

    await act(async () => {
      firstSave.resolve({ revision: 'a2' })
      await firstSave.promise
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledWith(
      'setting/b.md',
      'B 本地第一行\nB 不变中间行\nB 外部第三行\n',
      'b2',
    )
    expect(onSave).toHaveBeenCalledWith('setting/c.md', 'C 修改后\n', 'c1')
    expect(conflictArchiveMock.preserve).not.toHaveBeenCalled()
  })

  it('关闭当前文档自动保存不会取消其他文档已排队的兜底保存', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<boolean>()
    const onSave = vi.fn((path: string) => (
      path === 'setting/a.md' ? firstSave.promise : Promise.resolve(true)
    ))
    const { rerender } = render(
      <MarkdownEditor fileName="setting/a.md" content="A 初始" onSave={onSave} autoSaveDelayMs={100} />,
    )

    act(() => {
      tiptapMock.markdown = 'A 修改后'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
    })
    rerender(
      <MarkdownEditor fileName="setting/b.md" content="B 初始" onSave={onSave} autoSaveDelayMs={100} />,
    )
    act(() => {
      tiptapMock.markdown = 'B 修改后'
      tiptapMock.emit('update')
    })
    rerender(
      <MarkdownEditor fileName="setting/c.md" content="C 初始" onSave={onSave} autoSaveEnabled={false} autoSaveDelayMs={100} />,
    )

    await act(async () => {
      firstSave.resolve(true)
      await firstSave.promise
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledWith('setting/b.md', 'B 修改后\n', '')
  })

  it('关闭自动保存不会取消当前文档已排队的手动保存', async () => {
    vi.useFakeTimers()
    const firstSave = deferred<boolean>()
    const onSave = vi.fn((path: string) => (
      path === 'setting/a.md' ? firstSave.promise : Promise.resolve(true)
    ))
    const { rerender } = render(
      <MarkdownEditor fileName="setting/a.md" content="A 初始" onSave={onSave} autoSaveDelayMs={100} />,
    )

    act(() => {
      tiptapMock.markdown = 'A 修改后'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(100)
    })
    rerender(
      <MarkdownEditor fileName="setting/b.md" content="B 初始" onSave={onSave} autoSaveDelayMs={100} />,
    )
    act(() => {
      tiptapMock.markdown = 'B 修改后'
      tiptapMock.emit('update')
    })
    rerender(
      <MarkdownEditor fileName="setting/c.md" content="C 初始" onSave={onSave} autoSaveDelayMs={100} />,
    )
    rerender(
      <MarkdownEditor fileName="setting/b.md" content="B 初始" onSave={onSave} autoSaveEnabled={false} autoSaveDelayMs={100} />,
    )

    await act(async () => {
      firstSave.resolve(true)
      await firstSave.promise
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledWith('setting/b.md', 'B 修改后\n', '')
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
    expect(onSave).toHaveBeenCalledWith('setting/outline.md', '大纲尚未到保存时间\n', '')
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
    expect(onSave).toHaveBeenLastCalledWith('chapters/ch01.md', '修改后\n', '')

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

    expect(onSave).toHaveBeenCalledWith('chapters/ch01.md', '修改后\n', '')
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
    expect(onSave).toHaveBeenCalledWith('chapters/ch01.md', '导航前草稿\n', '')
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

    expect(onSave).toHaveBeenCalledWith('chapters/ch01.md', '第一章未保存草稿\n', '')
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

    expect(onSave).toHaveBeenCalledWith('chapters/ch01.md', '关闭 Tab 前草稿\n', '')
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

  it('同一章节的外部内容回灌保留已聚焦的光标位置', () => {
    tiptapMock.editor.state.selection = { from: 5, to: 5, head: 5, empty: true }
    tiptapMock.editor.state.doc.content = { size: 100 }
    tiptapMock.editor.view.hasFocus.mockReturnValue(true)
    const { rerender } = render(
      <MarkdownEditor fileName="chapters/ch01.md" content="第一章" onSave={vi.fn()} />,
    )

    rerender(
      <MarkdownEditor fileName="chapters/ch01.md" content="Agent 更新后的第一章" onSave={vi.fn()} />,
    )

    expect(tiptapMock.editor.commands.setTextSelection).toHaveBeenCalledWith({ from: 5, to: 5 })
    expect(tiptapMock.editor.commands.focus).toHaveBeenCalled()
  })

  it('切换章节时不继承上一个文档的光标位置', () => {
    const { rerender } = render(
      <MarkdownEditor fileName="chapters/ch01.md" content="第一章" onSave={vi.fn()} />,
    )
    tiptapMock.editor.state.selection = { from: 5, to: 5, head: 5, empty: true }
    tiptapMock.editor.view.hasFocus.mockReturnValue(true)
    tiptapMock.editor.commands.setTextSelection.mockClear()

    rerender(
      <MarkdownEditor fileName="chapters/ch02.md" content="第二章" onSave={vi.fn()} />,
    )

    expect(tiptapMock.editor.commands.setTextSelection).not.toHaveBeenCalled()
  })

  it('切回已解析章节时复用文档，原始内容变化后重新解析', () => {
    const { rerender } = render(
      <MarkdownEditor workspace="/books/demo" fileName="chapters/ch01.md" content="第一章" onSave={vi.fn()} />,
    )
    tiptapMock.editor.markdown.parse.mockClear()

    rerender(
      <MarkdownEditor workspace="/books/demo" fileName="chapters/ch02.md" content="第二章" onSave={vi.fn()} />,
    )
    rerender(
      <MarkdownEditor workspace="/books/demo" fileName="chapters/ch01.md" content="第一章" onSave={vi.fn()} />,
    )

    expect(tiptapMock.editor.markdown.parse).toHaveBeenCalledTimes(1)
    expect(tiptapMock.editor.markdown.parse).toHaveBeenLastCalledWith('第二章')

    rerender(
      <MarkdownEditor workspace="/books/demo" fileName="chapters/ch02.md" content="第二章（外部更新）" onSave={vi.fn()} />,
    )

    expect(tiptapMock.editor.markdown.parse).toHaveBeenCalledTimes(2)
    expect(tiptapMock.editor.markdown.parse).toHaveBeenLastCalledWith('第二章（外部更新）')
  })

  it('切换文件后恢复各自的滚动位置', () => {
    const animationFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      callback(0)
      return 1
    })
    const { rerender } = render(
      <MarkdownEditor fileName="chapters/ch01.md" content="第一章" onSave={vi.fn()} />,
    )
    const scrollContainer = screen.getByTestId('editor-content').parentElement as HTMLDivElement
    scrollContainer.scrollTop = 360

    rerender(
      <MarkdownEditor fileName="chapters/ch02.md" content="第二章" onSave={vi.fn()} />,
    )
    expect(scrollContainer.scrollTop).toBe(0)
    scrollContainer.scrollTop = 180

    rerender(
      <MarkdownEditor fileName="chapters/ch01.md" content="第一章" onSave={vi.fn()} />,
    )
    expect(scrollContainer.scrollTop).toBe(360)

    animationFrame.mockRestore()
  })

  it('跨文件搜索定位等待目标文件滚动位置恢复完成', () => {
    const animationFrames: FrameRequestCallback[] = []
    const animationFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      animationFrames.push(callback)
      return animationFrames.length
    })
    const { rerender } = render(
      <MarkdownEditor fileName="chapters/ch01.md" content="第一章" onSave={vi.fn()} />,
    )

    act(() => {
      animationFrames.splice(0).forEach((callback) => callback(0))
    })
    tiptapMock.editor.view.dispatch.mockClear()

    rerender(
      <MarkdownEditor
        fileName="chapters/ch02.md"
        content="目标章节"
        onSave={vi.fn()}
        searchIntent={{ query: '目标', line: 1, nonce: 1 }}
      />,
    )

    // Applying the new document updates decorations once. The explicit reveal
    // must remain queued behind the file-position restoration frame.
    expect(tiptapMock.editor.view.dispatch).toHaveBeenCalledTimes(1)
    expect(animationFrames).toHaveLength(2)

    act(() => {
      animationFrames.shift()?.(0)
    })
    expect(tiptapMock.editor.view.dispatch).toHaveBeenCalledTimes(1)

    act(() => {
      animationFrames.shift()?.(0)
    })
    expect(tiptapMock.editor.view.dispatch).toHaveBeenCalledTimes(2)

    animationFrame.mockRestore()
  })

  it('dirty 草稿收到 Agent 非重叠更新时自然合并，并沿用原用户编辑的 afterDelay', async () => {
    vi.useFakeTimers()
    const onSave = vi.fn().mockResolvedValue({ revision: 'rev-3' })
    const { rerender } = render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content={'第一行\n中间行\n第二行\n'}
        revision="rev-1"
        onSave={onSave}
        autoSaveDelayMs={1000}
      />,
    )

    act(() => {
      tiptapMock.markdown = '本地第一行\n中间行\n第二行'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(600)
    })
    act(() => {
      rerender(
        <MarkdownEditor
          workspace="/books/demo"
          fileName="chapters/ch01.md"
          content={'第一行\n中间行\nAgent 第二行\n'}
          revision="rev-2"
          onSave={onSave}
          autoSaveDelayMs={1000}
        />,
      )
    })

    expect(tiptapMock.chainApi.setContent).toHaveBeenLastCalledWith(
      '本地第一行\n中间行\nAgent 第二行\n',
      { emitUpdate: false, contentType: 'markdown' },
    )
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(conflictArchiveMock.preserve).not.toHaveBeenCalled()

    await act(async () => {
      await vi.advanceTimersByTimeAsync(399)
    })
    expect(onSave).not.toHaveBeenCalled()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1)
    })

    expect(onSave).toHaveBeenCalledWith(
      'chapters/ch01.md',
      '本地第一行\n中间行\nAgent 第二行\n',
      'rev-2',
    )
  })

  it('本地草稿与外部更新重叠时保留双方版本但不阻塞编辑器', async () => {
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

    expect(screen.getByRole('alert')).toHaveTextContent('检测到并发编辑')
    expect(screen.getByRole('alert')).toHaveTextContent('自动保存已暂停')
    expect(conflictArchiveMock.preserve).toHaveBeenCalledOnce()
    expect(tiptapMock.chainApi.setContent).not.toHaveBeenCalledWith('Agent 新版本', expect.anything())

    await user.click(screen.getByRole('button', { name: '载入工作区版本' }))

    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(tiptapMock.chainApi.setMeta).toHaveBeenLastCalledWith('addToHistory', false)
    expect(tiptapMock.chainApi.setContent).toHaveBeenLastCalledWith('Agent 新版本', { emitUpdate: false, contentType: 'markdown' })
  })

  it('重叠修改归档后暂停自动保存，明确保留合并结果后才写入', async () => {
    vi.useFakeTimers()
    const onSave = vi.fn().mockResolvedValue({ revision: 'rev-3' })
    const { rerender } = render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content="初始"
        revision="rev-1"
        onSave={onSave}
        autoSaveDelayMs={1000}
      />,
    )

    act(() => {
      tiptapMock.markdown = '本地草稿'
      tiptapMock.emit('update')
      vi.advanceTimersByTime(600)
    })
    rerender(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content="Agent 新版本"
        revision="rev-2"
        onSave={onSave}
        autoSaveDelayMs={1000}
      />,
    )

    expect(screen.getByRole('alert')).toHaveTextContent('检测到并发编辑')
    await act(async () => { await vi.advanceTimersByTimeAsync(5000) })
    expect(onSave).not.toHaveBeenCalled()

    act(() => {
      tiptapMock.markdown = '冲突后继续编辑'
      tiptapMock.emit('update')
    })
    await act(async () => { await vi.advanceTimersByTimeAsync(1000) })
    expect(onSave).not.toHaveBeenCalled()

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: '保留合并结果' }))
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledWith('chapters/ch01.md', '冲突后继续编辑\n', 'rev-2')
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('自动保存收到重叠 revision 冲突时暂停，不在后台自动重试覆盖', async () => {
    vi.useFakeTimers()
    const onSave = vi.fn((_path: string, _content: string, baseRevision: string) => {
      if (baseRevision === 'rev-1') {
        return Promise.reject(new WorkspaceFileRevisionConflictError(
          new Error('revision conflict'),
          {
            workspace: '/books/demo',
            content: 'Agent 新版本',
            revision: 'rev-2',
          },
        ))
      }
      return Promise.resolve({ revision: 'rev-3' })
    })
    render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content="初始"
        revision="rev-1"
        onSave={onSave}
        autoSaveDelayMs={100}
      />,
    )

    act(() => {
      tiptapMock.markdown = '本地草稿'
      tiptapMock.emit('update')
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(100)
      await Promise.resolve()
      await Promise.resolve()
    })

    expect(onSave).toHaveBeenCalledTimes(1)
    expect(conflictArchiveMock.preserve).toHaveBeenCalledOnce()
    expect(screen.getByRole('alert')).toHaveTextContent('自动保存已暂停')

    act(() => {
      tiptapMock.markdown = '冲突后继续编辑'
      tiptapMock.emit('update')
    })
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(onSave).toHaveBeenCalledTimes(1)

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: '保留合并结果' }))
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(onSave).toHaveBeenLastCalledWith('chapters/ch01.md', '冲突后继续编辑\n', 'rev-2')
  })

  it('保留合并结果会显式保存，失败时保留冲突提示以便重试', async () => {
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

    await user.click(screen.getByRole('button', { name: '保留合并结果' }))
    expect(onSave).toHaveBeenLastCalledWith('chapters/ch01.md', '本地草稿\n', '')
    expect(screen.getByRole('alert')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '保留合并结果' }))
    expect(onSave).toHaveBeenCalledTimes(2)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('冲突归档失败后可重试归档，成功前不会覆盖工作区版本', async () => {
    const user = userEvent.setup()
    const log = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    conflictArchiveMock.preserve
      .mockRejectedValueOnce(new Error('archive offline'))
      .mockResolvedValueOnce({ id: 'conflict-retry', path: '/conflicts/conflict-retry.json', storage: 'server' })
    const onSave = vi.fn().mockResolvedValue(true)
    const { rerender } = render(
      <MarkdownEditor workspace="/books/demo" fileName="chapters/ch01.md" content="初始" onSave={onSave} autoSaveEnabled={false} />,
    )

    act(() => {
      tiptapMock.markdown = '本地草稿'
      tiptapMock.emit('update')
    })
    rerender(
      <MarkdownEditor workspace="/books/demo" fileName="chapters/ch01.md" content="Agent 新版本" onSave={onSave} autoSaveEnabled={false} />,
    )
    await act(async () => { await Promise.resolve(); await Promise.resolve() })
    expect(onSave).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: '保留合并结果' }))

    expect(conflictArchiveMock.preserve).toHaveBeenCalledTimes(2)
    expect(onSave).toHaveBeenCalledOnce()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    log.mockRestore()
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

  it('正文评论常驻编辑器且不再要求切换只读审阅模式', () => {
    const documentReview = {
      comments: [],
      onCreate: vi.fn(),
      onUpdate: vi.fn(),
      onDelete: vi.fn(),
    }

    render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content={'正文\n'}
        onSave={vi.fn().mockResolvedValue(true)}
        documentReview={documentReview}
      />,
    )

    expect(screen.queryByRole('group', { name: '编辑器模式' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '审阅' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '保存' })).toBeInTheDocument()
    expect(tiptapMock.editor.view.dom).not.toHaveAttribute('aria-readonly')
    expect(tiptapMock.editor.setEditable).not.toHaveBeenCalled()
  })

  it('将正文评论导航意图交给当前文件的评论注释层', async () => {
    const comment = {
      id: 'comment-1',
      thread_id: 'review-1',
      target: { kind: 'workspace_file' as const, id: 'chapters/ch01.md' },
      body: '调整这里',
      created_at: '',
      updated_at: '',
      anchor: {
        kind: 'text-range' as const,
        encoding: 'utf8-bytes-v1' as const,
        revision: 'sha256:chapter',
        start: 0,
        end: 2,
        quote: '正文',
        display_quote: '正文',
      },
    }

    render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="chapters/ch01.md"
        content="正文"
        onSave={vi.fn().mockResolvedValue(true)}
        documentReviewNavigationIntent={{ commentID: comment.id, nonce: 1 }}
        documentReview={{
          comments: [comment],
          onCreate: vi.fn(),
          onUpdate: vi.fn(),
          onDelete: vi.fn(),
        }}
      />,
    )

    await waitFor(() => expect(documentReviewAnnotationsMock.revealComment).toHaveBeenCalledWith(comment.id))
  })

  it('原始 Markdown 与 TipTap 仅格式化不同也能准备正文评论快照', async () => {
    const content = '# 创作者指令\n## 创作约束\n- 第一项\n'
    tiptapMock.markdown = '# 创作者指令\n\n## 创作约束\n\n- 第一项\n'
    workspaceApiMock.readFile.mockResolvedValue({
      workspace: '/books/demo',
      path: 'CREATOR.md',
      content,
      revision: 'sha256:canonical',
    })

    render(
      <MarkdownEditor
        workspace="/books/demo"
        fileName="CREATOR.md"
        content={content}
        onSave={vi.fn().mockResolvedValue(true)}
        documentReview={{
          comments: [],
          onCreate: vi.fn(),
          onUpdate: vi.fn(),
          onDelete: vi.fn(),
        }}
      />,
    )

    expect(documentReviewAnnotationsMock.prepareSnapshot).not.toBeNull()
    await expect(documentReviewAnnotationsMock.prepareSnapshot!()).resolves.toEqual({
      content,
      revision: 'sha256:canonical',
    })
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
