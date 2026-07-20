import { render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { MarkdownRichEditor } from './MarkdownRichEditor'

const tiptapMock = vi.hoisted(() => {
  const chainApi = {
    setMeta: vi.fn(() => chainApi),
    setContent: vi.fn(() => chainApi),
    run: vi.fn(() => true),
  }
  const editor = {
    commands: {
      focus: vi.fn(),
      setTextSelection: vi.fn(),
    },
    chain: vi.fn(() => chainApi),
    isDestroyed: false,
    getMarkdown: vi.fn(() => tiptapMock.markdown),
    state: {
      doc: { content: { size: 100 } },
      selection: { from: 1, to: 1 },
      tr: { setMeta: vi.fn(() => 'tr-with-search-meta') },
    },
    view: {
      dispatch: vi.fn(),
      dom: document.createElement('div'),
      hasFocus: vi.fn(() => false),
    },
  }
  interface CapturedOptions {
    content?: string
    contentType?: string
    editorProps?: {
      attributes?: Record<string, string>
      handleKeyDown?: (
        view: unknown,
        event: { key: string; metaKey: boolean; ctrlKey: boolean; altKey: boolean; preventDefault?: () => void; stopPropagation?: () => void },
      ) => boolean
      handleClick?: unknown
    }
    onUpdate?: (args: { editor: unknown }) => void
  }
  return {
    editor,
    chainApi,
    markdown: '',
    useEditorOptions: null as CapturedOptions | null,
    reset() {
      this.markdown = ''
      this.useEditorOptions = null
      editor.state.selection = { from: 1, to: 1 }
      vi.clearAllMocks()
      editor.view.hasFocus.mockReturnValue(false)
    },
  }
})

const decorationsMock = vi.hoisted(() => ({
  searchStateRef: null as { current: { query: string; index: number; useRegex: boolean } } | null,
  matches: [] as Array<{ from: number; to: number }>,
  findSearchMatches: vi.fn(),
  selectSearchMatch: vi.fn(),
}))

vi.mock('@tiptap/react', () => ({
  EditorContent: () => <div data-testid="rich-editor-content" />,
  useEditor: (options: unknown) => {
    tiptapMock.useEditorOptions = options as typeof tiptapMock.useEditorOptions
    return tiptapMock.editor
  },
}))

vi.mock('@tiptap/starter-kit', () => ({ default: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-table', () => ({ TableKit: { configure: () => ({}) } }))
vi.mock('@tiptap/extension-image', () => ({ default: { extend: () => ({ configure: () => ({}) }) } }))
vi.mock('@tiptap/markdown', () => ({ Markdown: { configure: () => ({}) } }))

vi.mock('./editorDecorations', () => ({
  createSearchHighlightExtension: (ref: { current: { query: string; index: number; useRegex: boolean } }) => {
    decorationsMock.searchStateRef = ref
    return { name: 'novaSearchHighlight' }
  },
  findSearchMatches: (...args: unknown[]) => decorationsMock.findSearchMatches(...args),
  searchPluginKey: 'search-plugin-key',
  selectSearchMatch: (...args: unknown[]) => decorationsMock.selectSearchMatch(...args),
}))

vi.mock('./editorDocument', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./editorDocument')>()
  return {
    ...actual,
    createIndentedHardBreakExtension: () => ({ name: 'hardBreak' }),
    createWorkspaceImageExtension: () => ({ name: 'workspaceImage' }),
  }
})

describe('MarkdownRichEditor', () => {
  beforeEach(() => {
    tiptapMock.reset()
    decorationsMock.searchStateRef = null
    decorationsMock.matches = []
    decorationsMock.findSearchMatches.mockReset()
    decorationsMock.findSearchMatches.mockImplementation(() => decorationsMock.matches)
    decorationsMock.selectSearchMatch.mockClear()
  })

  it('以 markdown 形式加载初始内容并暴露可访问名称', () => {
    render(<MarkdownRichEditor value="# 世界观" onChange={vi.fn()} aria-label="正文" />)

    const options = tiptapMock.useEditorOptions
    expect(options?.content).toBe('# 世界观')
    expect(options?.contentType).toBe('markdown')
    expect(options?.editorProps?.attributes?.['aria-label']).toBe('正文')
    expect(options?.editorProps?.attributes?.role).toBe('textbox')
    expect(options?.editorProps?.handleClick).toBeTypeOf('function')
  })

  it('文档更新时把规范化后的 markdown 传给 onChange', () => {
    const onChange = vi.fn()
    render(<MarkdownRichEditor value="" onChange={onChange} />)

    tiptapMock.markdown = '标题  \n\n\n\n\n下一段'
    tiptapMock.useEditorOptions?.onUpdate?.({ editor: tiptapMock.editor })

    expect(onChange).toHaveBeenCalledWith('标题\n\n\n下一段\n')
  })

  it('搜索词非空时刷新高亮并定位首个匹配', () => {
    decorationsMock.matches = [{ from: 2, to: 5 }]
    render(<MarkdownRichEditor value="林川的设定" onChange={vi.fn()} highlightQuery="林川" />)

    expect(decorationsMock.searchStateRef?.current).toEqual({ query: '林川', index: 0, useRegex: false })
    expect(tiptapMock.editor.view.dispatch).toHaveBeenCalled()
    expect(decorationsMock.selectSearchMatch).toHaveBeenCalledWith(tiptapMock.editor, { from: 2, to: 5 })
  })

  it('搜索词为空时清除高亮且不定位匹配', () => {
    render(<MarkdownRichEditor value="内容" onChange={vi.fn()} highlightQuery="  " />)

    expect(decorationsMock.searchStateRef?.current).toEqual({ query: '', index: 0, useRegex: false })
    expect(tiptapMock.editor.view.dispatch).toHaveBeenCalled()
    expect(decorationsMock.selectSearchMatch).not.toHaveBeenCalled()
  })

  it('Cmd/Ctrl+S 触发保存回调并阻止默认行为', () => {
    const onSaveShortcut = vi.fn()
    render(<MarkdownRichEditor value="" onChange={vi.fn()} onSaveShortcut={onSaveShortcut} />)

    const handleKeyDown = tiptapMock.useEditorOptions?.editorProps?.handleKeyDown
    const saveEvent = { key: 's', metaKey: true, ctrlKey: false, altKey: false, preventDefault: vi.fn(), stopPropagation: vi.fn() }
    expect(handleKeyDown?.(null, saveEvent)).toBe(true)
    expect(onSaveShortcut).toHaveBeenCalledTimes(1)
    expect(saveEvent.preventDefault).toHaveBeenCalled()
    expect(saveEvent.stopPropagation).toHaveBeenCalled()

    expect(handleKeyDown?.(null, { key: 'a', metaKey: true, ctrlKey: false, altKey: false })).toBe(false)
    expect(onSaveShortcut).toHaveBeenCalledTimes(1)
  })

  it('外部 value 变更时回灌文档且不进撤销历史', () => {
    tiptapMock.markdown = '旧内容'
    const { rerender } = render(<MarkdownRichEditor value="旧内容" onChange={vi.fn()} />)
    tiptapMock.chainApi.setContent.mockClear()

    rerender(<MarkdownRichEditor value="新内容" onChange={vi.fn()} />)

    expect(tiptapMock.chainApi.setMeta).toHaveBeenCalledWith('addToHistory', false)
    expect(tiptapMock.chainApi.setContent).toHaveBeenCalledWith('新内容', { emitUpdate: false, contentType: 'markdown' })
  })

  it('外部内容回灌时保留已聚焦的光标位置', () => {
    tiptapMock.markdown = '旧内容'
    tiptapMock.editor.state.selection = { from: 4, to: 4 }
    tiptapMock.editor.view.hasFocus.mockReturnValue(true)
    const { rerender } = render(<MarkdownRichEditor value="旧内容" onChange={vi.fn()} />)

    rerender(<MarkdownRichEditor value="前缀旧内容" onChange={vi.fn()} />)

    expect(tiptapMock.editor.commands.setTextSelection).toHaveBeenCalledWith({ from: 4, to: 4 })
    expect(tiptapMock.editor.commands.focus).toHaveBeenCalled()
  })

  it('自己输入产生的 value 回灌不会重写文档', () => {
    const onChange = vi.fn()
    const { rerender } = render(<MarkdownRichEditor value="旧内容" onChange={onChange} />)

    tiptapMock.markdown = '新内容'
    tiptapMock.useEditorOptions?.onUpdate?.({ editor: tiptapMock.editor })
    expect(onChange).toHaveBeenCalledWith('新内容\n')
    tiptapMock.chainApi.setContent.mockClear()

    rerender(<MarkdownRichEditor value={'新内容\n'} onChange={onChange} />)

    expect(tiptapMock.chainApi.setContent).not.toHaveBeenCalled()
  })
})
