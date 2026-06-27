import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { TellerEditor } from './SettingPanelTellerEditor'
import type { Teller } from '../types'

describe('TellerEditor style contents', () => {
  it('edits image prompt and caps it at 4000 chars', async () => {
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

    const editor = screen.getByPlaceholderText(/描述互动图像/)
    fireEvent.change(editor, { target: { value: '图'.repeat(4050) } })

    await waitFor(() => {
      expect(currentDraft.image_prompt).toHaveLength(4000)
      expect(screen.getByText('4000/4000')).toBeInTheDocument()
    })
  })

  it('uploads style content and truncates it to 8000 chars', async () => {
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

    const content = '风'.repeat(8050)
    const file = new File([content], 'style.md', { type: 'text/markdown' })
    Object.defineProperty(file, 'text', { value: () => Promise.resolve(content) })
    const input = document.querySelector('input[type="file"]') as HTMLInputElement
    fireEvent.change(input, { target: { files: [file] } })

    await waitFor(() => expect(screen.getByText('风格内容')).toBeInTheDocument())
    fireEvent.click(screen.getByRole('button', { name: '保存' }))

    await waitFor(() => {
      const saved = currentDraft.style_rules?.[0]?.style_contents?.[0] || ''
      expect(saved).toHaveLength(8000)
    })
  })

  it('keeps long style content scrollable inside the dialog and uses the Nova save style', async () => {
    render(<Harness initial={teller()} onChange={() => {}} onSave={() => {}} />)

    fireEvent.click(screen.getByRole('button', { name: '自定义' }))

    const dialog = await screen.findByRole('dialog')
    const editor = within(dialog).getByRole('textbox')
    expect(editor.className).toContain('overflow-y-auto')
    expect(editor.className).toContain('[field-sizing:fixed]')

    const save = within(dialog).getByRole('button', { name: '保存' })
    expect(save).toHaveClass('bg-[var(--nova-active)]')
    expect(save).toHaveClass('text-[var(--nova-text)]')

    const footer = save.closest('[data-slot="dialog-footer"]')
    expect(footer).toHaveClass('!mx-0')
    expect(footer).toHaveClass('!mb-0')
    expect(footer).toHaveClass('bg-[var(--nova-surface)]/95')
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

function teller(): Teller {
  return {
    version: 4,
    id: 'custom',
    name: '自定义',
    description: '',
    random_event_rate: 0,
    style_rules: [{ scene: '激烈打斗', style_contents: [] }],
    tags: [],
    context_policy: { creator: 'always', lore: 'relevant', runtime_state: 'always' },
    slots: [{ id: 'identity', name: '系统提示', target: 'system', enabled: true, content: '规则' }],
    custom: true,
  }
}
