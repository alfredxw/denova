import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { ImagePreset } from '../../types'
import { ImagePresetEditor, imagePresetDraftIsValid } from './ImagePresetEditor'

const toastMock = vi.hoisted(() => ({ error: vi.fn() }))

vi.mock('@/lib/toast', () => ({ toast: toastMock }))

const singleRulePreset: ImagePreset = {
  version: 2,
  id: 'custom-image-1',
  name: '厚涂风',
  description: '',
  prompt: '厚涂风格',
  slots: [{ id: 'tool_request', name: '图像请求 Prompt', target: 'tool_request', enabled: true, content: '厚涂风格' }],
  custom: true,
}

describe('ImagePresetEditor', () => {
  it('keeps the last enabled rule from being disabled', async () => {
    const user = userEvent.setup()
    const onDraft = vi.fn()
    toastMock.error.mockReset()

    renderEditor(singleRulePreset, onDraft)

    const toggle = screen.getByRole('switch', { name: '停用规则' })
    await user.click(toggle)

    expect(toastMock.error).toHaveBeenCalledWith('图像方案至少需要一条启用且有内容的规则，最后一条无法关闭。')
    expect(onDraft).not.toHaveBeenCalled()
    expect(screen.getByRole('switch', { name: '停用规则' })).toBeChecked()
  })

  it('disables a rule when another enabled rule keeps the preset usable', async () => {
    const user = userEvent.setup()
    const onDraft = vi.fn()
    toastMock.error.mockReset()
    const twoRulePreset: ImagePreset = {
      ...singleRulePreset,
      slots: [
        ...singleRulePreset.slots!,
        { id: 'agent_system', name: '风格说明', target: 'agent_system', enabled: true, content: '保持写实' },
      ],
    }

    renderEditor(twoRulePreset, onDraft)

    await user.click(screen.getAllByRole('switch', { name: '停用规则' })[1])

    expect(toastMock.error).not.toHaveBeenCalled()
    expect(onDraft).toHaveBeenCalledWith(expect.objectContaining({
      slots: [
        expect.objectContaining({ id: 'tool_request', enabled: true }),
        expect.objectContaining({ id: 'agent_system', enabled: false }),
      ],
    }))
  })
})

describe('imagePresetDraftIsValid', () => {
  it('mirrors the server rule for names and usable rules', () => {
    expect(imagePresetDraftIsValid(null)).toBe(true)
    expect(imagePresetDraftIsValid(singleRulePreset)).toBe(true)
    expect(imagePresetDraftIsValid({ ...singleRulePreset, name: '   ' })).toBe(false)
    expect(imagePresetDraftIsValid({
      ...singleRulePreset,
      slots: [{ id: 'tool_request', name: '图像请求 Prompt', target: 'tool_request', enabled: false, content: '厚涂风格' }],
    })).toBe(false)
    expect(imagePresetDraftIsValid({
      ...singleRulePreset,
      slots: [{ id: 'tool_request', name: '图像请求 Prompt', target: 'tool_request', enabled: true, content: '  ' }],
    })).toBe(false)
  })
})

function renderEditor(initial: ImagePreset, onDraft: (draft: ImagePreset) => void): void {
  function Harness() {
    const [draft, setDraft] = useState<ImagePreset | null>(initial)
    return (
      <ImagePresetEditor
        draft={draft}
        setDraft={(next) => {
          setDraft(next)
          if (next) onDraft(next)
        }}
        onSave={() => {}}
      />
    )
  }

  render(<Harness />)
}
