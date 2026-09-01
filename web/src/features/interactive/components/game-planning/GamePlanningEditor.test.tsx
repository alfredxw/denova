import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import type { GamePlanningTemplate } from '../../types'
import { GamePlanningEditor } from './GamePlanningEditor'

const builtInTemplate: GamePlanningTemplate = {
  version: 1,
  id: 'default',
  name: 'Classic adventure',
  description: 'Built-in description',
  sections: [
    {
      id: 'long-term-direction',
      title: 'Long-term direction',
      description: 'Built-in section guidance',
    },
  ],
  custom: false,
}

describe('GamePlanningEditor', () => {
  it('shows built-in templates as localized, read-only forms that can be copied', async () => {
    const user = userEvent.setup()
    const onCopy = vi.fn()

    renderEditor(builtInTemplate, onCopy)

    expect(screen.getByLabelText('名称')).toHaveValue('经典冒险')
    expect(screen.getByLabelText('名称')).toBeDisabled()
    expect(screen.getByLabelText('小标题')).toHaveValue('长期方向')
    expect(screen.getByLabelText('小标题')).toBeDisabled()
    expect(screen.queryByRole('button', { name: '添加章节' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '复制并修改' }))
    expect(onCopy).toHaveBeenCalledWith(builtInTemplate)
  })

  it('adds structured sections and rejects missing or duplicate titles', async () => {
    const user = userEvent.setup()
    const onValidityChange = vi.fn()
    const customTemplate: GamePlanningTemplate = {
      ...builtInTemplate,
      id: 'custom-plan',
      name: '自定义规划',
      sections: [{ id: 'direction', title: '方向', description: '规划方向。' }],
      custom: true,
    }

    renderEditor(customTemplate, vi.fn(), onValidityChange)
    await waitFor(() => expect(onValidityChange).toHaveBeenLastCalledWith(true))

    const nameInput = screen.getByLabelText('名称')
    await user.clear(nameInput)
    expect(screen.getByText('请填写规划模板名称。')).toBeInTheDocument()
    await waitFor(() => expect(onValidityChange).toHaveBeenLastCalledWith(false))
    await user.type(nameInput, '自定义规划')

    await user.click(screen.getByRole('button', { name: '添加章节' }))
    expect(screen.getByRole('alert')).toHaveTextContent('每个规划章节都需要小标题。')
    await waitFor(() => expect(onValidityChange).toHaveBeenLastCalledWith(false))

    const titleInputs = screen.getAllByLabelText('小标题')
    await user.type(titleInputs[1], '方向')
    expect(screen.getByRole('alert')).toHaveTextContent('同一模板中的小标题不能重复。')

    await user.clear(titleInputs[1])
    await user.type(titleInputs[1], '近期节拍')
    await waitFor(() => expect(onValidityChange).toHaveBeenLastCalledWith(true))
  })
})

function renderEditor(
  initial: GamePlanningTemplate,
  onCopy = vi.fn(),
  onValidityChange = vi.fn(),
): void {
  function Harness() {
    const [draft, setDraft] = useState<GamePlanningTemplate | null>(initial)
    return (
      <GamePlanningEditor
        draft={draft}
        setDraft={setDraft}
        onCopy={onCopy}
        onValidityChange={onValidityChange}
      />
    )
  }

  render(<Harness />)
}
