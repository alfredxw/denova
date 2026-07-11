import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { PresetConfigSectionEditor } from './PresetConfigSectionEditor'

vi.mock('@monaco-editor/react', () => ({
  Editor: ({ value, onChange, options }: {
    value?: string
    onChange?: (value?: string) => void
    options?: { ariaLabel?: string }
  }) => (
    <textarea
      aria-label={options?.ariaLabel}
      data-testid="preset-config-json-input"
      value={value}
      onChange={(event) => onChange?.(event.target.value)}
    />
  ),
}))

vi.mock('next-themes', () => ({
  useTheme: () => ({ resolvedTheme: 'dark' }),
}))

describe('PresetConfigSectionEditor', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('accepts array JSON for array-backed sections and blocks the wrong shape', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const onValidityChange = vi.fn()
    const { container } = render(<ArraySectionHarness onChange={onChange} onValidityChange={onValidityChange} />)

    expect(container.querySelector('section')).toHaveClass('self-start')

    await user.click(screen.getByRole('button', { name: 'JSON' }))
    const editor = screen.getByTestId('preset-config-json-input')

    fireEvent.change(editor, { target: { value: '[{"id":"next"}]' } })
    expect(onChange).toHaveBeenLastCalledWith([{ id: 'next' }])
    expect(onValidityChange).toHaveBeenLastCalledWith(true)

    fireEvent.change(editor, { target: { value: '{"id":"wrong-shape"}' } })
    expect(screen.getByText('必须输入 JSON 数组')).toBeInTheDocument()
    expect(onChange).toHaveBeenCalledTimes(1)
    expect(onValidityChange).toHaveBeenLastCalledWith(false)

    await user.click(screen.getByRole('button', { name: '可视化' }))
    expect(screen.getByRole('button', { name: 'JSON' })).toHaveAttribute('aria-pressed', 'true')

    fireEvent.change(editor, { target: { value: '[]' } })
    await user.click(screen.getByRole('button', { name: '可视化' }))
    expect(screen.getByRole('button', { name: '可视化' })).toHaveAttribute('aria-pressed', 'true')
  })
})

function ArraySectionHarness({ onChange, onValidityChange }: {
  onChange: (value: Array<{ id: string }>) => void
  onValidityChange: (valid: boolean) => void
}) {
  const [value, setValue] = useState<Array<{ id: string }>>([{ id: 'initial' }])
  return (
    <PresetConfigSectionEditor
      sectionId="memory-structure-test"
      resetKey="memory-structure-test"
      title="记忆结构"
      description="编辑记忆结构"
      summary={`${value.length}`}
      value={value}
      onChange={(next) => {
        setValue(next)
        onChange(next)
      }}
      onSave={vi.fn()}
      onValidityChange={onValidityChange}
    >
      {() => <div data-testid="array-visual-editor" />}
    </PresetConfigSectionEditor>
  )
}
