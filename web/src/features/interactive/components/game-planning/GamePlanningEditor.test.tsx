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
  it('edits a localized built-in template in place without exposing Markdown syntax', async () => {
    const user = userEvent.setup()

    renderEditor(builtInTemplate)

    const nameInput = screen.getByLabelText('名称')
    expect(nameInput).toHaveValue('经典冒险')
    expect(screen.getByLabelText('小节标题')).toHaveValue('长期方向')
    expect(screen.getByLabelText<HTMLTextAreaElement>('小节说明').value).toContain('不可逆转的变化')
    expect(screen.queryByText('##')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: '复制并修改' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '添加小节' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '删除小节' })).toBeDisabled()

    await user.type(nameInput, '·调整')
    expect(nameInput).toHaveValue('经典冒险·调整')
    expect(screen.getByText('内置覆盖')).toBeInTheDocument()
    expect(screen.getByLabelText('小节标题')).toHaveValue('长期方向')
    expect(screen.getByLabelText<HTMLTextAreaElement>('小节说明').value).toContain('不可逆转的变化')
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

    renderEditor(customTemplate, onValidityChange)
    await waitFor(() => expect(onValidityChange).toHaveBeenLastCalledWith(true))

    const nameInput = screen.getByLabelText('名称')
    await user.clear(nameInput)
    expect(screen.getByText('请填写规划模板名称。')).toBeInTheDocument()
    await waitFor(() => expect(onValidityChange).toHaveBeenLastCalledWith(false))
    await user.type(nameInput, '自定义规划')

    await user.click(screen.getByRole('button', { name: '添加小节' }))
    expect(screen.getByText('每个规划小节都需要标题。')).toBeInTheDocument()
    await waitFor(() => expect(onValidityChange).toHaveBeenLastCalledWith(false))

    const titleInputs = screen.getAllByLabelText('小节标题')
    await user.type(titleInputs[1], '方向')
    expect(screen.getByText('同一模板中的小节标题不能重复。')).toBeInTheDocument()

    await user.clear(titleInputs[1])
    await user.type(titleInputs[1], '近期节拍')
    await waitFor(() => expect(onValidityChange).toHaveBeenLastCalledWith(true))
  })
})

function renderEditor(
  initial: GamePlanningTemplate,
  onValidityChange = vi.fn(),
): void {
  function Harness() {
    const [draft, setDraft] = useState<GamePlanningTemplate | null>(initial)
    return (
      <GamePlanningEditor
        draft={draft}
        setDraft={setDraft}
        onValidityChange={onValidityChange}
      />
    )
  }

  render(<Harness />)
}
