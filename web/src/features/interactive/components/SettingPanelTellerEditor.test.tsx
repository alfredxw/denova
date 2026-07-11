import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { runConfigManagerStream } from '@/lib/api'
import { ImagePresetEditor } from './SettingPanelSections'
import { TellerEditor } from './SettingPanelTellerEditor'
import { getStyleReferences, readStyleReferenceFile, saveStyleReference, updateStyleReferenceFile } from '../api'
import type { ImagePreset, Teller } from '../types'

vi.mock('@/lib/api', () => ({
  runConfigManagerStream: vi.fn(),
}))

vi.mock('../api', () => ({
  getStyleReferences: vi.fn(),
  readStyleReferenceFile: vi.fn(),
  saveStyleReference: vi.fn(),
  updateStyleReferenceFile: vi.fn(),
}))

describe('TellerEditor style contents', () => {
  beforeEach(() => {
    vi.mocked(getStyleReferences).mockReset()
    vi.mocked(readStyleReferenceFile).mockReset()
    vi.mocked(saveStyleReference).mockReset()
    vi.mocked(updateStyleReferenceFile).mockReset()
    vi.mocked(runConfigManagerStream).mockReset()
    vi.mocked(getStyleReferences).mockResolvedValue([])
    vi.mocked(readStyleReferenceFile).mockResolvedValue({
      reference: styleReference(),
      content: '# 克制细腻\n\n动作、对白和停顿承载情绪。\n',
      revision: 'r1',
    })
    vi.mocked(saveStyleReference).mockImplementation(async (input) => ({
      name: input.name,
      description: input.description || '',
      path: `/tmp/.denova/styles/${input.filename || 'style.md'}`,
      display_path: `.denova/styles/${input.filename || 'style.md'}`,
    }))
    vi.mocked(updateStyleReferenceFile).mockImplementation(async (input) => ({
      reference: styleReference(),
      content: `${input.content.replace(/\s+$/, '')}\n`,
      revision: 'r2',
    }))
  })

  it('edits image preset tool request slot and caps it at 4000 chars', async () => {
    let currentDraft = imagePreset()
    render(
      <ImagePresetHarness
        initial={currentDraft}
        onChange={(draft) => {
          currentDraft = draft
        }}
        onSave={() => {}}
      />,
    )

    const editorShell = screen.getByTestId('image-preset-editor')
    expect(editorShell).toHaveClass('image-preset-editor')
    expect(within(editorShell).getByTestId('preset-metadata')).toBeInTheDocument()
    expect(editorShell.querySelector('.image-preset-layout')).toBeInTheDocument()
    expect(editorShell.querySelector('.image-preset-rule-grid')).toBeInTheDocument()

    const editor = screen.getByPlaceholderText(/高质量游戏 CG/)
    fireEvent.change(editor, { target: { value: '图'.repeat(4050) } })

    await waitFor(() => {
      expect(currentDraft.slots?.[0]?.content).toHaveLength(4000)
      expect(screen.getByText('4000/4000')).toBeInTheDocument()
    })
  })

  it('shows legacy image preset prompt as a tool request rule', () => {
    render(<ImagePresetHarness initial={{ ...imagePreset(), slots: undefined, prompt: '旧图像风格' }} onChange={() => {}} onSave={() => {}} />)

    expect(screen.getAllByText('图像请求 Prompt').length).toBeGreaterThan(0)
    expect(screen.getByDisplayValue('旧图像风格')).toBeInTheDocument()
  })

  it('adds toggles and deletes image preset rules', () => {
    let currentDraft = imagePreset()
    render(<ImagePresetHarness initial={currentDraft} onChange={(draft) => { currentDraft = draft }} onSave={() => {}} />)

    fireEvent.click(screen.getByRole('button', { name: '新增注入规则' }))
    expect(currentDraft.slots).toHaveLength(2)
    expect(screen.getByText('新图像规则')).toBeInTheDocument()

    fireEvent.click(screen.getAllByLabelText('停用规则')[1])
    expect(currentDraft.slots?.[1]?.enabled).toBe(false)

    fireEvent.click(screen.getByRole('button', { name: '删除注入规则' }))
    expect(currentDraft.slots).toHaveLength(1)
  })

  it('uploads a direct shared style reference and attaches its path', async () => {
    let currentDraft = teller()
    const onSave = vi.fn()
    render(
      <Harness
        initial={currentDraft}
        onChange={(draft) => {
          currentDraft = draft
        }}
        onSave={onSave}
      />,
    )

    const content = '风'.repeat(40500)
    const file = new File([content], 'style.md', { type: 'text/markdown' })
    Object.defineProperty(file, 'text', { value: () => Promise.resolve(content) })
    fireEvent.click(screen.getAllByRole('button', { name: '上传/粘贴' })[1])
    const input = document.querySelectorAll('input[type="file"]')[1] as HTMLInputElement
    fireEvent.change(input, { target: { files: [file] } })

    await waitFor(() => expect(screen.getByText('导入文风参考')).toBeInTheDocument())
    expect(screen.getByText('40000/40000')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: '直接保存' }))

    await waitFor(() => {
      const saved = currentDraft.style_rules?.[0]?.style_refs?.[0] || ''
      expect(saved).toBe('.denova/styles/style.md')
    })
  })

  it('keeps uploaded source scrollable inside the dialog and uses the Nova primary style', async () => {
    render(<Harness initial={teller()} onChange={() => {}} onSave={() => {}} />)

    const content = '风'.repeat(500)
    const file = new File([content], 'style.md', { type: 'text/markdown' })
    Object.defineProperty(file, 'text', { value: () => Promise.resolve(content) })
    fireEvent.click(screen.getAllByRole('button', { name: '上传/粘贴' })[0])
    const input = document.querySelectorAll('input[type="file"]')[0] as HTMLInputElement
    fireEvent.change(input, { target: { files: [file] } })

    const dialog = await screen.findByRole('dialog')
    const editor = within(dialog).getAllByRole('textbox').find((node) => node.tagName.toLowerCase() === 'textarea')
    if (!editor) throw new Error('textarea editor missing')
    expect(editor.className).toContain('overflow-y-auto')
    expect(editor.className).toContain('[field-sizing:fixed]')

    const save = within(dialog).getByRole('button', { name: 'AI提炼文风' })
    expect(save).toHaveClass('bg-[var(--nova-active)]')
    expect(save).toHaveClass('text-[var(--nova-text)]')

    const footer = save.closest('[data-slot="dialog-footer"]')
    expect(footer).toHaveClass('!mx-0')
    expect(footer).toHaveClass('!mb-0')
    expect(footer).toHaveClass('bg-[var(--nova-surface)]/95')

    const progressEmpty = within(dialog).getByText('开始提炼后这里会显示配置 Agent 的实时进展。')
    expect(progressEmpty.parentElement).toHaveClass('flex')
    expect(progressEmpty.parentElement).toHaveClass('flex-col')
  })

  it('saves pasted text as a global style reference', async () => {
    let currentDraft = teller()
    render(
      <Harness
        initial={currentDraft}
        onChange={(draft) => {
          currentDraft = draft
        }}
        onSave={() => {}}
      />,
    )

    fireEvent.click(screen.getAllByRole('button', { name: '上传/粘贴' })[0])

    const dialog = await screen.findByRole('dialog')
    const editor = within(dialog).getAllByRole('textbox').find((node) => node.tagName.toLowerCase() === 'textarea')
    if (!editor) throw new Error('textarea editor missing')
    fireEvent.change(editor, { target: { value: '克制短句，动作承载情绪。' } })
    fireEvent.click(within(dialog).getByRole('button', { name: '直接保存' }))

    await waitFor(() => {
      expect(currentDraft.style_refs?.[0]).toMatch(/^\.denova\/styles\/style-\d+\.md$/)
    })
    const calls = vi.mocked(saveStyleReference).mock.calls
    expect(calls[calls.length - 1]?.[0].content).toBe('克制短句，动作承载情绪。')
  })

  it('streams AI style extraction in the Chat panel, reads the generated file, and saves edited Markdown with revision', async () => {
    let currentDraft = teller()
    let extractedPath = ''
    vi.mocked(runConfigManagerStream).mockResolvedValue(streamText('<style_reference_markdown>\n# 克制雨夜\n\n## 总体原则\n\n短句推进，动作承载情绪。\n</style_reference_markdown>'))
    vi.mocked(readStyleReferenceFile).mockImplementation(async (path) => {
      if (path.startsWith('.denova/styles/style-')) {
        extractedPath = path
        return {
          reference: {
            name: '克制雨夜',
            description: '短句推进，动作承载情绪。',
            path: `/tmp/${path}`,
            display_path: path,
          },
          content: '# 克制雨夜\n\n## 总体原则\n\n短句推进，动作承载情绪。',
          revision: 'r-extracted',
        }
      }
      return {
        reference: styleReference(),
        content: '# 克制细腻\n\n动作、对白和停顿承载情绪。\n',
        revision: 'r1',
      }
    })
    render(
      <Harness
        initial={currentDraft}
        onChange={(draft) => {
          currentDraft = draft
        }}
        onSave={() => {}}
      />,
    )

    fireEvent.click(screen.getAllByRole('button', { name: '上传/粘贴' })[0])
    const dialog = await screen.findByRole('dialog')
    const editor = within(dialog).getAllByRole('textbox').find((node) => node.tagName.toLowerCase() === 'textarea') as HTMLTextAreaElement | undefined
    if (!editor) throw new Error('textarea editor missing')
    fireEvent.change(editor, { target: { value: '雨落得很轻。他没有立刻回答。' } })
    fireEvent.click(within(dialog).getByRole('button', { name: 'AI提炼文风' }))

    expect(within(dialog).getByText('提炼进展')).toBeInTheDocument()
    await waitFor(() => expect(readStyleReferenceFile).toHaveBeenCalled())
    expect(within(dialog).getByText(/已写入并选择/)).toBeInTheDocument()
    expect(editor).toHaveValue('# 克制雨夜\n\n## 总体原则\n\n短句推进，动作承载情绪。')
    expect(currentDraft.style_refs?.[0]).toMatch(/^\.denova\/styles\/style-\d+\.md$/)
    expect(readStyleReferenceFile).toHaveBeenCalledWith(extractedPath)
    expect(saveStyleReference).not.toHaveBeenCalled()

    fireEvent.change(editor, { target: { value: '# 克制雨夜\n\n编辑后的文风规则。' } })
    fireEvent.click(within(dialog).getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(updateStyleReferenceFile).toHaveBeenCalledWith({
        path: extractedPath,
        content: '# 克制雨夜\n\n编辑后的文风规则。',
        base_revision: 'r-extracted',
      })
    })
  })

  it('opens a selected shared style reference for editing and saves with revision', async () => {
    const existing = styleReference()
    vi.mocked(getStyleReferences).mockResolvedValue([existing])
    let currentDraft: Teller = { ...teller(), style_refs: [existing.display_path] }
    render(
      <Harness
        initial={currentDraft}
        onChange={(draft) => {
          currentDraft = draft
        }}
        onSave={() => {}}
      />,
    )

    const editButton = await screen.findByRole('button', { name: '编辑 克制细腻' })
    fireEvent.click(editButton)

    const dialog = await screen.findByRole('dialog')
    const editor = await within(dialog).findByPlaceholderText('编辑 Markdown 文风参考内容。')
    expect(readStyleReferenceFile).toHaveBeenCalledWith(existing.display_path)
    fireEvent.change(editor, { target: { value: '# 新文风\n\n对白更锋利。' } })
    fireEvent.click(within(dialog).getByRole('button', { name: '保存' }))

    await waitFor(() => {
      expect(updateStyleReferenceFile).toHaveBeenCalledWith({
        path: existing.display_path,
        content: '# 新文风\n\n对白更锋利。',
        base_revision: 'r1',
      })
      expect(screen.queryByText('编辑文风参考')).not.toBeInTheDocument()
    })
    expect(currentDraft.style_refs).toEqual([existing.display_path])
  })

  it('opens picker style reference editing without toggling selection', async () => {
    const existing = styleReference()
    vi.mocked(getStyleReferences).mockResolvedValue([existing])
    let currentDraft = teller()
    render(
      <Harness
        initial={currentDraft}
        onChange={(draft) => {
          currentDraft = draft
        }}
        onSave={() => {}}
      />,
    )

    fireEvent.click(screen.getAllByRole('button', { name: '选择参考' })[0])
    const pickerEdit = await screen.findByRole('button', { name: '编辑 克制细腻' })
    fireEvent.click(pickerEdit)

    await screen.findByText('编辑文风参考')
    expect(readStyleReferenceFile).toHaveBeenCalledWith(existing.display_path)
    expect(currentDraft.style_refs).toEqual([])
  })

  it('keeps the teller editor scrollable when style rules grow', () => {
    const { container } = render(<Harness initial={teller()} onChange={() => {}} onSave={() => {}} />)

    expect(container.firstElementChild).toHaveClass('overflow-hidden')

    const contentScroll = screen.getByTestId('teller-content-scroll')
    expect(contentScroll).toHaveClass('min-h-0', 'flex-1', 'overflow-y-auto')
    expect(within(contentScroll).getByText('文风参考')).toBeInTheDocument()

    const injectGrid = container.querySelector('.teller-injection-layout')
    expect(injectGrid).toHaveClass('flex-1')
    expect(injectGrid).toHaveClass('min-w-0')
    expect(injectGrid).not.toHaveClass('overflow-y-auto')
    expect(injectGrid).not.toHaveClass('lg:grid-cols-[280px_minmax(0,1fr)]')

    const sceneInput = screen.getByPlaceholderText('场景描述，如：激烈打斗 / 日常对话 / 压抑悬疑')
    expect(sceneInput).toHaveClass('md:flex-1')
    expect(sceneInput.parentElement).toHaveClass('md:flex-wrap')
  })

  it('allows decimal random event rates without collapsing intermediate input', async () => {
    let currentDraft = teller()
    render(
      <Harness
        initial={currentDraft}
        onChange={(draft) => {
          currentDraft = draft
        }}
        onSave={() => {}}
      />,
    )

    const rateInput = screen.getByRole('textbox', { name: '随机事件率' })
    fireEvent.change(rateInput, { target: { value: '0.' } })
    expect(rateInput).toHaveValue('0.')
    expect(currentDraft.random_event_rate).toBe(0)

    fireEvent.change(rateInput, { target: { value: '0.15' } })
    expect(rateInput).toHaveValue('0.15')
    expect(currentDraft.random_event_rate).toBe(0.15)
  })

  it('keeps orchestration editing out of narrative styles', () => {
    render(<Harness initial={teller()} onChange={() => {}} onSave={() => {}} />)

    const editorShell = screen.getByTestId('teller-editor')
    expect(editorShell).toHaveClass('teller-editor')
    expect(within(editorShell).getByTestId('preset-metadata')).toBeInTheDocument()
    expect(editorShell.querySelector('.teller-injection-layout')).toBeInTheDocument()
    expect(editorShell.querySelector('.teller-rule-grid')).toBeInTheDocument()
    expect(screen.queryByText('叙事编排')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '新增事件包' })).not.toBeInTheDocument()
    expect(screen.getByText('注入规则')).toBeInTheDocument()
  })
})

function Harness({ initial, onChange, onSave }: { initial: Teller; onChange: (draft: Teller) => void; onSave: () => void }) {
  const [draft, setDraftState] = useState<Teller | null>(initial)
  const setDraft = (next: Teller | null) => {
    setDraftState(next)
    if (next) onChange(next)
  }
  return (
    <TellerEditor
      workspace="/tmp/book"
      draft={draft}
      setDraft={setDraft}
      activeSlotId="identity"
      setActiveSlotId={() => {}}
      onSave={onSave}
    />
  )
}

function ImagePresetHarness({ initial, onChange, onSave }: { initial: ImagePreset; onChange: (draft: ImagePreset) => void; onSave: () => void }) {
  const [draft, setDraftState] = useState<ImagePreset | null>(initial)
  const setDraft = (next: ImagePreset | null) => {
    setDraftState(next)
    if (next) onChange(next)
  }
  return (
    <ImagePresetEditor
      draft={draft}
      setDraft={setDraft}
      onSave={onSave}
    />
  )
}

function imagePreset(): ImagePreset {
  return {
    version: 2,
    id: 'custom-image',
    name: '自定义图像方案',
    description: '',
    prompt: '## 图像请求 Prompt（tool_request）\n\n',
    slots: [{ id: 'tool_request', name: '图像请求 Prompt', target: 'tool_request', enabled: true, content: '' }],
    custom: true,
  }
}

function teller(): Teller {
  return {
    version: 6,
    id: 'custom',
    name: '自定义',
    description: '',
    random_event_rate: 0,
    style_refs: [],
    style_rules: [{ scene: '激烈打斗', style_refs: [] }],
    context_policy: { creator: 'always', lore: 'relevant', runtime_state: 'always' },
    slots: [{ id: 'identity', name: '系统提示', target: 'system', enabled: true, content: '规则' }],
    custom: true,
  }
}

function styleReference() {
  return {
    name: '克制细腻',
    description: '动作、对白和停顿承载情绪',
    path: '/tmp/.denova/styles/restraint.md',
    display_path: '.denova/styles/restraint.md',
  }
}

function streamText(content: string) {
  return new ReadableStream({
    start(controller) {
      controller.enqueue({ type: 'start', messageId: 'assistant-style' })
      controller.enqueue({ type: 'text-start', id: 'text-style' })
      controller.enqueue({ type: 'text-delta', id: 'text-style', delta: content })
      controller.enqueue({ type: 'text-end', id: 'text-style' })
      controller.enqueue({ type: 'finish', finishReason: 'stop' })
      controller.close()
    },
  })
}
