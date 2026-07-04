import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { ImagePresetEditor } from './SettingPanelSections'
import { TellerEditor } from './SettingPanelTellerEditor'
import type { ImagePreset, Teller } from '../types'

describe('TellerEditor style contents', () => {
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
    const input = document.querySelector('input[type="file"]') as HTMLInputElement
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
    const input = document.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(input, { target: { files: [file] } })

    const dialog = await screen.findByRole('dialog')
    const editor = within(dialog).getAllByRole('textbox').find((node) => node.tagName.toLowerCase() === 'textarea')
    if (!editor) throw new Error('textarea editor missing')
    expect(editor.className).toContain('overflow-y-auto')
    expect(editor.className).toContain('[field-sizing:fixed]')

    const save = within(dialog).getByRole('button', { name: '提炼为 md' })
    expect(save).toHaveClass('bg-[var(--nova-active)]')
    expect(save).toHaveClass('text-[var(--nova-text)]')

    const footer = save.closest('[data-slot="dialog-footer"]')
    expect(footer).toHaveClass('!mx-0')
    expect(footer).toHaveClass('!mb-0')
    expect(footer).toHaveClass('bg-[var(--nova-surface)]/95')
  })

  it('keeps the teller editor scrollable when style rules grow', () => {
    const { container } = render(<Harness initial={teller()} onChange={() => {}} onSave={() => {}} />)

    expect(container.firstElementChild).toHaveClass('overflow-y-auto')
    expect(container.firstElementChild).not.toHaveClass('md:overflow-hidden')

    const injectGrid = container.querySelector('.min-h-\\[520px\\]')
    expect(injectGrid).toHaveClass('flex-1')
    expect(injectGrid).toHaveClass('lg:grid-cols-[280px_minmax(0,1fr)]')

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
      tagDraft=""
      setTagDraft={() => {}}
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
      tagDraft=""
      setTagDraft={() => {}}
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
    tags: [],
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
    style_rules: [{ scene: '激烈打斗', style_refs: [] }],
    tags: [],
    context_policy: { creator: 'always', lore: 'relevant', runtime_state: 'always' },
    slots: [{ id: 'identity', name: '系统提示', target: 'system', enabled: true, content: '规则' }],
    custom: true,
  }
}
